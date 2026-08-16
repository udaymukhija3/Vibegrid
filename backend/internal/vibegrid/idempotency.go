package vibegrid

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	idempotencyHeader          = "Idempotency-Key"
	idempotencyReplayHeader    = "Idempotency-Replayed"
	maxIdempotencyKeyLength    = 128
	minIdempotencyKeyLength    = 8
	maxIdempotencyRequestBytes = maxAdminBodyBytes
)

type idempotencyResponse struct {
	status int
	header http.Header
	body   []byte
}

type IdempotencyStore interface {
	Execute(
		ctx context.Context,
		scope, keyHash, requestHash string,
		action func(context.Context) idempotencyResponse,
	) (response idempotencyResponse, replayed, conflict bool, err error)
	PruneExpired(ctx context.Context, before time.Time, limit int) (int64, error)
}

type PostgresIdempotencyStore struct {
	db *sql.DB
}

func NewPostgresIdempotencyStore(database *sql.DB) *PostgresIdempotencyStore {
	return &PostgresIdempotencyStore{db: database}
}

type transactionContextKey struct{}

func contextWithTransaction(ctx context.Context, tx *sql.Tx) context.Context {
	return context.WithValue(ctx, transactionContextKey{}, tx)
}

func transactionFromContext(ctx context.Context) *sql.Tx {
	tx, _ := ctx.Value(transactionContextKey{}).(*sql.Tx)
	return tx
}

// errIdempotencyKeyRaced reports that another writer committed this key first.
// It never leaves this file: Execute turns it into the winner's response.
var errIdempotencyKeyRaced = errors.New("idempotency key committed by a concurrent writer")

// Execute serializes matching keys with a transaction-scoped advisory lock and
// commits the domain write and replay record in the same transaction. Store
// methods participating in idempotent creation use transactionFromContext, so a
// crash cannot leave a committed mutation without its replayable response.
func (store *PostgresIdempotencyStore) Execute(
	ctx context.Context,
	scope, keyHash, requestHash string,
	action func(context.Context) idempotencyResponse,
) (idempotencyResponse, bool, bool, error) {
	response, replayed, conflict, err := store.execute(ctx, scope, keyHash, requestHash, action)
	if !errors.Is(err, errIdempotencyKeyRaced) {
		return response, replayed, conflict, err
	}
	// The primary key, not the advisory lock, is what actually guarantees one
	// record per key — the lock is only an optimization that spares us running
	// the domain callback twice, and it cannot serialize writers whose lock and
	// insert reach different backends. So when the constraint does fire, this
	// caller simply lost the race: its own mutation rolled back with the failed
	// transaction, exactly one resource exists, and the honest answer is the
	// winner's stored response rather than an error for a request that worked.
	return store.replayWinner(ctx, scope, keyHash, requestHash)
}

func (store *PostgresIdempotencyStore) replayWinner(
	ctx context.Context,
	scope, keyHash, requestHash string,
) (idempotencyResponse, bool, bool, error) {
	stored, found, err := loadIdempotencyResponse(ctx, store.db, scope, keyHash, requestHash)
	if err != nil {
		return idempotencyResponse{}, false, false, err
	}
	if !found {
		// The row that just rejected our insert is gone. Pruning only removes
		// records older than the retention window, so this means the table was
		// truncated mid-request and we have nothing truthful to replay.
		return idempotencyResponse{}, false, false, errors.New("idempotency winner disappeared before it could be replayed")
	}
	if stored.status == http.StatusConflict {
		return idempotencyResponse{}, false, true, nil
	}
	return stored, true, false, nil
}

func (store *PostgresIdempotencyStore) execute(
	ctx context.Context,
	scope, keyHash, requestHash string,
	action func(context.Context) idempotencyResponse,
) (idempotencyResponse, bool, bool, error) {
	tx, err := store.db.BeginTx(ctx, nil)
	if err != nil {
		return idempotencyResponse{}, false, false, fmt.Errorf("begin idempotency transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The primary key prevents duplicates at commit; the advisory lock avoids
	// running the expensive domain callback twice while concurrent requests race.
	lockID := advisoryLockID(scope + "\x00" + keyHash)
	if _, err := tx.ExecContext(ctx, `select pg_advisory_xact_lock($1)`, lockID); err != nil {
		return idempotencyResponse{}, false, false, fmt.Errorf("lock idempotency key: %w", err)
	}

	stored, found, err := loadIdempotencyResponse(ctx, tx, scope, keyHash, requestHash)
	if err != nil {
		return idempotencyResponse{}, false, false, err
	}
	if found {
		if stored.status == http.StatusConflict {
			return idempotencyResponse{}, false, true, nil
		}
		if err := tx.Commit(); err != nil {
			return idempotencyResponse{}, false, false, fmt.Errorf("commit idempotency replay: %w", err)
		}
		return stored, true, false, nil
	}

	response := action(contextWithTransaction(ctx, tx))
	if response.status == 0 {
		response.status = http.StatusOK
	}
	if response.status < http.StatusOK || response.status >= http.StatusMultipleChoices {
		return response, false, false, nil
	}

	storedHeaders, err := json.Marshal(durableResponseHeaders(response.header))
	if err != nil {
		return idempotencyResponse{}, false, false, fmt.Errorf("encode idempotency response headers: %w", err)
	}
	// response_headers is jsonb, but the pool talks simple protocol so it stays
	// compatible with transaction-mode poolers. In that mode pgx has no parameter
	// OIDs to encode against and renders a []byte as a bytea literal, which
	// Postgres rejects for a json column (SQLSTATE 22P02) — that turned every
	// idempotent mutation into a 503. Bind the document as text and let the
	// explicit cast, not the driver's guess, choose the type.
	if _, err := tx.ExecContext(ctx,
		`insert into idempotency_keys
		 (scope, key_hash, request_hash, status_code, response_headers, response_body)
		 values ($1, $2, $3, $4, $5::jsonb, $6)`,
		scope, keyHash, requestHash, response.status, string(storedHeaders), response.body,
	); err != nil {
		if isUniqueViolation(err) {
			return idempotencyResponse{}, false, false, errIdempotencyKeyRaced
		}
		return idempotencyResponse{}, false, false, fmt.Errorf("store idempotency response: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return idempotencyResponse{}, false, false, fmt.Errorf("commit idempotent mutation: %w", err)
	}
	return response, false, false, nil
}

// loadIdempotencyResponse takes a rowQuerier rather than the transaction so the
// lost-race path can read the winner's row on the pool, after its own aborted
// transaction has been rolled back.
func loadIdempotencyResponse(
	ctx context.Context,
	querier rowQuerier,
	scope, keyHash, requestHash string,
) (idempotencyResponse, bool, error) {
	var storedRequestHash string
	var response idempotencyResponse
	var headersJSON []byte
	err := querier.QueryRowContext(ctx,
		`select request_hash, status_code, response_headers, response_body
		 from idempotency_keys
		 where scope = $1 and key_hash = $2`,
		scope, keyHash,
	).Scan(&storedRequestHash, &response.status, &headersJSON, &response.body)
	if errors.Is(err, sql.ErrNoRows) {
		return idempotencyResponse{}, false, nil
	}
	if err != nil {
		return idempotencyResponse{}, false, fmt.Errorf("load idempotency response: %w", err)
	}
	if storedRequestHash != requestHash {
		return idempotencyResponse{status: http.StatusConflict}, true, nil
	}
	if err := json.Unmarshal(headersJSON, &response.header); err != nil {
		return idempotencyResponse{}, false, fmt.Errorf("decode idempotency response headers: %w", err)
	}
	return response, true, nil
}

func (store *PostgresIdempotencyStore) PruneExpired(ctx context.Context, before time.Time, limit int) (int64, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()
	if limit <= 0 {
		return 0, nil
	}
	result, err := store.db.ExecContext(ctx,
		`delete from idempotency_keys
		 where ctid in (
		   select ctid from idempotency_keys
		   where created_at < $1
		   order by created_at
		   limit $2
		 )`,
		before.UTC(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("prune idempotency keys: %w", err)
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("count pruned idempotency keys: %w", err)
	}
	return deleted, nil
}

func (server *Server) withIdempotency(
	operation string,
	caller func(http.ResponseWriter, *http.Request) string,
	next http.HandlerFunc,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimSpace(r.Header.Get(idempotencyHeader))
		if key == "" || server.idempotency == nil {
			next(w, r)
			return
		}
		if !validIdempotencyKey(key) {
			writeError(w, http.StatusBadRequest, "Idempotency-Key must be 8-128 characters using letters, numbers, '.', '_', ':', or '-'.")
			return
		}

		body, err := readIdempotencyBody(r)
		if err != nil {
			if isRequestBodyTooLarge(err) {
				writeError(w, http.StatusRequestEntityTooLarge, "That request is too large.")
			} else {
				writeError(w, http.StatusBadRequest, "Could not read that request.")
			}
			return
		}
		callerScope := caller(w, r)
		scope := operation + ":" + digestString(callerScope)
		requestHash := digestBytes(body)

		response, replayed, conflict, err := server.idempotency.Execute(
			r.Context(), scope, digestString(key), requestHash,
			func(actionCtx context.Context) idempotencyResponse {
				recorder := newBufferedResponse()
				next(recorder, r.WithContext(actionCtx))
				return idempotencyResponse{
					status: recorder.status,
					header: recorder.header.Clone(),
					body:   append([]byte(nil), recorder.body.Bytes()...),
				}
			},
		)
		if err != nil {
			slog.Error("idempotent mutation failed", "operation", operation, "error", err, "request_id", requestIDFromContext(r.Context()))
			writeError(w, http.StatusServiceUnavailable, "Could not safely process that request.")
			return
		}
		if conflict {
			writeError(w, http.StatusConflict, "That Idempotency-Key was already used with a different request.")
			return
		}
		if replayed {
			response.header.Set(idempotencyReplayHeader, "true")
		}
		writeBufferedHTTPResponse(w, response)
	}
}

func (server *Server) guestIdempotencyCaller(w http.ResponseWriter, r *http.Request) string {
	sessionID := EnsureSessionID(w, r, server.secureCookies)
	if _, err := r.Cookie(sessionCookieName); err != nil {
		r.AddCookie(&http.Cookie{Name: sessionCookieName, Value: sessionID})
	}
	return "guest:" + sessionID
}

func (server *Server) adminIdempotencyCaller(_ http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(adminSessionCookieName); err == nil && cookie.Value != "" {
		return "admin-session:" + cookie.Value
	}
	return "admin-token:" + bearerToken(r)
}

func (server *Server) creatorIdempotencyCaller(_ http.ResponseWriter, r *http.Request) string {
	return "creator:" + r.PathValue("id") + ":" + r.Header.Get(creatorClaimHeader)
}

func validIdempotencyKey(key string) bool {
	if len(key) < minIdempotencyKeyLength || len(key) > maxIdempotencyKeyLength {
		return false
	}
	for index := 0; index < len(key); index++ {
		character := key[index]
		if (character >= 'a' && character <= 'z') ||
			(character >= 'A' && character <= 'Z') ||
			(character >= '0' && character <= '9') ||
			character == '.' || character == '_' || character == ':' || character == '-' {
			continue
		}
		return false
	}
	return true
}

func readIdempotencyBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, int64(maxIdempotencyRequestBytes)+1))
	_ = r.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(body) > maxIdempotencyRequestBytes {
		return nil, &http.MaxBytesError{Limit: int64(maxIdempotencyRequestBytes)}
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func durableResponseHeaders(headers http.Header) http.Header {
	durable := headers.Clone()
	for _, key := range []string{"Connection", "Content-Length", "Date", "Set-Cookie", "Transfer-Encoding"} {
		durable.Del(key)
	}
	return durable
}

func writeBufferedHTTPResponse(w http.ResponseWriter, response idempotencyResponse) {
	for key, values := range response.header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(response.status)
	_, _ = w.Write(response.body)
}

func advisoryLockID(value string) int64 {
	digest := sha256.Sum256([]byte(value))
	var lockID uint64
	for index := 0; index < 8; index++ {
		lockID = lockID<<8 | uint64(digest[index])
	}
	return int64(lockID)
}

func digestString(value string) string {
	return digestBytes([]byte(value))
}

func digestBytes(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}
