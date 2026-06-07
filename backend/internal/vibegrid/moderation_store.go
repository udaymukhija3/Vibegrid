package vibegrid

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	ReportStatusOpen      = "OPEN"
	ReportStatusActioned  = "ACTIONED"
	ReportStatusDismissed = "DISMISSED"
	AppealStatusOpen      = "OPEN"
	AppealStatusResolved  = "RESOLVED"
)

var ErrReportNotFound = errors.New("report not found")
var ErrAppealNotFound = errors.New("appeal not found")

type ModerationStore interface {
	CreateReport(ctx context.Context, input ReportInput, sessionID string) (ModerationReport, error)
	ListReports(ctx context.Context) ([]ModerationReport, error)
	ResolveReport(ctx context.Context, reportID, status, note, actor string) (ModerationReport, error)
	CreateAppeal(ctx context.Context, input AppealInput) (ModerationAppeal, error)
	ListAppeals(ctx context.Context) ([]ModerationAppeal, error)
	ResolveAppeal(ctx context.Context, appealID, note, actor string) (ModerationAppeal, error)
	AddAction(ctx context.Context, action ModerationActionInput) error
	AuditLog(ctx context.Context, limit int) ([]ModerationAction, error)
}

type ReportInput struct {
	PuzzleID string `json:"puzzleId"`
	Reason   string `json:"reason"`
	Details  string `json:"details"`
	Contact  string `json:"contact"`
}

type AppealInput struct {
	PuzzleID string `json:"puzzleId"`
	Contact  string `json:"contact"`
	Message  string `json:"message"`
}

type ModerationReport struct {
	ID             string  `json:"id"`
	PuzzleID       string  `json:"puzzleId"`
	PuzzleNumber   int     `json:"puzzleNumber"`
	PuzzleStatus   string  `json:"puzzleStatus"`
	PuzzleOrigin   string  `json:"puzzleOrigin"`
	Reason         string  `json:"reason"`
	Details        string  `json:"details"`
	Contact        string  `json:"contact"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"createdAt"`
	ResolvedAt     *string `json:"resolvedAt,omitempty"`
	ResolutionNote string  `json:"resolutionNote"`
}

type ModerationAppeal struct {
	ID             string  `json:"id"`
	PuzzleID       string  `json:"puzzleId"`
	PuzzleNumber   int     `json:"puzzleNumber"`
	PuzzleStatus   string  `json:"puzzleStatus"`
	PuzzleOrigin   string  `json:"puzzleOrigin"`
	Contact        string  `json:"contact"`
	Message        string  `json:"message"`
	Status         string  `json:"status"`
	CreatedAt      string  `json:"createdAt"`
	ResolvedAt     *string `json:"resolvedAt,omitempty"`
	ResolutionNote string  `json:"resolutionNote"`
}

type ModerationActionInput struct {
	ReportID string
	AppealID string
	PuzzleID string
	Actor    string
	Action   string
	Reason   string
	Note     string
}

type ModerationAction struct {
	ID           string  `json:"id"`
	ReportID     *string `json:"reportId,omitempty"`
	AppealID     *string `json:"appealId,omitempty"`
	PuzzleID     *string `json:"puzzleId,omitempty"`
	PuzzleNumber *int    `json:"puzzleNumber,omitempty"`
	Actor        string  `json:"actor"`
	Action       string  `json:"action"`
	Reason       string  `json:"reason"`
	Note         string  `json:"note"`
	CreatedAt    string  `json:"createdAt"`
}

type PostgresModerationStore struct {
	db *sql.DB
}

func NewPostgresModerationStore(database *sql.DB) *PostgresModerationStore {
	return &PostgresModerationStore{db: database}
}

func (store *PostgresModerationStore) CreateReport(ctx context.Context, input ReportInput, sessionID string) (ModerationReport, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	id := newID("rpt")
	_, err := store.db.ExecContext(ctx,
		`insert into moderation_reports (id, puzzle_id, reporter_session_id, reason, details, contact)
		 values ($1, $2, $3, $4, $5, $6)`,
		id, input.PuzzleID, nullString(sessionID), input.Reason, input.Details, input.Contact,
	)
	if err != nil {
		return ModerationReport{}, fmt.Errorf("create report: %w", err)
	}
	if err := store.AddAction(ctx, ModerationActionInput{
		ReportID: id,
		PuzzleID: input.PuzzleID,
		Actor:    "public",
		Action:   "REPORT_CREATED",
		Reason:   input.Reason,
		Note:     input.Details,
	}); err != nil {
		return ModerationReport{}, err
	}
	return store.reportByID(ctx, id)
}

func (store *PostgresModerationStore) ListReports(ctx context.Context) ([]ModerationReport, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select r.id, r.puzzle_id, p.puzzle_number, p.status, p.origin,
		        r.reason, r.details, r.contact, r.status, r.created_at,
		        r.resolved_at, r.resolution_note
		 from moderation_reports r
		 join puzzles p on p.id = r.puzzle_id
		 order by case r.status when 'OPEN' then 0 else 1 end, r.created_at desc
		 limit 100`)
	if err != nil {
		return nil, fmt.Errorf("list reports: %w", err)
	}
	defer rows.Close()
	return scanReports(rows)
}

func (store *PostgresModerationStore) ResolveReport(ctx context.Context, reportID, status, note, actor string) (ModerationReport, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	result, err := store.db.ExecContext(ctx,
		`update moderation_reports
		 set status = $1, resolution_note = $2, resolved_at = now()
		 where id = $3`,
		status, note, reportID,
	)
	if err != nil {
		return ModerationReport{}, fmt.Errorf("resolve report: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return ModerationReport{}, fmt.Errorf("resolve report rows: %w", err)
	} else if affected == 0 {
		return ModerationReport{}, ErrReportNotFound
	}
	report, err := store.reportByID(ctx, reportID)
	if err != nil {
		return ModerationReport{}, err
	}
	if err := store.AddAction(ctx, ModerationActionInput{
		ReportID: reportID,
		PuzzleID: report.PuzzleID,
		Actor:    actor,
		Action:   "REPORT_" + status,
		Reason:   report.Reason,
		Note:     note,
	}); err != nil {
		return ModerationReport{}, err
	}
	return report, nil
}

func (store *PostgresModerationStore) CreateAppeal(ctx context.Context, input AppealInput) (ModerationAppeal, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	id := newID("apl")
	_, err := store.db.ExecContext(ctx,
		`insert into moderation_appeals (id, puzzle_id, contact, message)
		 values ($1, $2, $3, $4)`,
		id, input.PuzzleID, input.Contact, input.Message,
	)
	if err != nil {
		return ModerationAppeal{}, fmt.Errorf("create appeal: %w", err)
	}
	if err := store.AddAction(ctx, ModerationActionInput{
		AppealID: id,
		PuzzleID: input.PuzzleID,
		Actor:    "public",
		Action:   "APPEAL_CREATED",
		Note:     input.Message,
	}); err != nil {
		return ModerationAppeal{}, err
	}
	return store.appealByID(ctx, id)
}

func (store *PostgresModerationStore) ListAppeals(ctx context.Context) ([]ModerationAppeal, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	rows, err := store.db.QueryContext(ctx,
		`select a.id, a.puzzle_id, p.puzzle_number, p.status, p.origin,
		        a.contact, a.message, a.status, a.created_at,
		        a.resolved_at, a.resolution_note
		 from moderation_appeals a
		 join puzzles p on p.id = a.puzzle_id
		 order by case a.status when 'OPEN' then 0 else 1 end, a.created_at desc
		 limit 100`)
	if err != nil {
		return nil, fmt.Errorf("list appeals: %w", err)
	}
	defer rows.Close()
	return scanAppeals(rows)
}

func (store *PostgresModerationStore) ResolveAppeal(ctx context.Context, appealID, note, actor string) (ModerationAppeal, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	result, err := store.db.ExecContext(ctx,
		`update moderation_appeals
		 set status = 'RESOLVED', resolution_note = $1, resolved_at = now()
		 where id = $2`,
		note, appealID,
	)
	if err != nil {
		return ModerationAppeal{}, fmt.Errorf("resolve appeal: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return ModerationAppeal{}, fmt.Errorf("resolve appeal rows: %w", err)
	} else if affected == 0 {
		return ModerationAppeal{}, ErrAppealNotFound
	}
	appeal, err := store.appealByID(ctx, appealID)
	if err != nil {
		return ModerationAppeal{}, err
	}
	if err := store.AddAction(ctx, ModerationActionInput{
		AppealID: appealID,
		PuzzleID: appeal.PuzzleID,
		Actor:    actor,
		Action:   "APPEAL_RESOLVED",
		Note:     note,
	}); err != nil {
		return ModerationAppeal{}, err
	}
	return appeal, nil
}

func (store *PostgresModerationStore) AddAction(ctx context.Context, action ModerationActionInput) error {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	_, err := store.db.ExecContext(ctx,
		`insert into moderation_actions (report_id, appeal_id, puzzle_id, actor, action, reason, note)
		 values ($1, $2, $3, $4, $5, $6, $7)`,
		nullString(action.ReportID), nullString(action.AppealID), nullString(action.PuzzleID),
		action.Actor, action.Action, action.Reason, action.Note,
	)
	if err != nil {
		return fmt.Errorf("add moderation action: %w", err)
	}
	return nil
}

func (store *PostgresModerationStore) AuditLog(ctx context.Context, limit int) ([]ModerationAction, error) {
	ctx, cancel := withDatabaseTimeout(ctx)
	defer cancel()

	if limit <= 0 || limit > 200 {
		limit = 100
	}

	rows, err := store.db.QueryContext(ctx,
		`select a.id::text, a.report_id, a.appeal_id, a.puzzle_id, p.puzzle_number,
		        a.actor, a.action, a.reason, a.note, a.created_at
		 from moderation_actions a
		 left join puzzles p on p.id = a.puzzle_id
		 order by a.created_at desc
		 limit $1`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("audit log: %w", err)
	}
	defer rows.Close()

	actions := []ModerationAction{}
	for rows.Next() {
		var action ModerationAction
		var reportID, appealID, puzzleID sql.NullString
		var puzzleNumber sql.NullInt64
		var createdAt time.Time
		if err := rows.Scan(
			&action.ID, &reportID, &appealID, &puzzleID, &puzzleNumber,
			&action.Actor, &action.Action, &action.Reason, &action.Note, &createdAt,
		); err != nil {
			return nil, fmt.Errorf("scan audit log: %w", err)
		}
		action.ReportID = stringPtr(reportID)
		action.AppealID = stringPtr(appealID)
		action.PuzzleID = stringPtr(puzzleID)
		if puzzleNumber.Valid {
			number := int(puzzleNumber.Int64)
			action.PuzzleNumber = &number
		}
		action.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		actions = append(actions, action)
	}
	return actions, rows.Err()
}

func (store *PostgresModerationStore) reportByID(ctx context.Context, reportID string) (ModerationReport, error) {
	rows, err := store.db.QueryContext(ctx,
		`select r.id, r.puzzle_id, p.puzzle_number, p.status, p.origin,
		        r.reason, r.details, r.contact, r.status, r.created_at,
		        r.resolved_at, r.resolution_note
		 from moderation_reports r
		 join puzzles p on p.id = r.puzzle_id
		 where r.id = $1`,
		reportID,
	)
	if err != nil {
		return ModerationReport{}, fmt.Errorf("load report: %w", err)
	}
	defer rows.Close()
	reports, err := scanReports(rows)
	if err != nil {
		return ModerationReport{}, err
	}
	if len(reports) == 0 {
		return ModerationReport{}, ErrReportNotFound
	}
	return reports[0], nil
}

func (store *PostgresModerationStore) appealByID(ctx context.Context, appealID string) (ModerationAppeal, error) {
	rows, err := store.db.QueryContext(ctx,
		`select a.id, a.puzzle_id, p.puzzle_number, p.status, p.origin,
		        a.contact, a.message, a.status, a.created_at,
		        a.resolved_at, a.resolution_note
		 from moderation_appeals a
		 join puzzles p on p.id = a.puzzle_id
		 where a.id = $1`,
		appealID,
	)
	if err != nil {
		return ModerationAppeal{}, fmt.Errorf("load appeal: %w", err)
	}
	defer rows.Close()
	appeals, err := scanAppeals(rows)
	if err != nil {
		return ModerationAppeal{}, err
	}
	if len(appeals) == 0 {
		return ModerationAppeal{}, ErrAppealNotFound
	}
	return appeals[0], nil
}

func scanReports(rows *sql.Rows) ([]ModerationReport, error) {
	reports := []ModerationReport{}
	for rows.Next() {
		var report ModerationReport
		var createdAt time.Time
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&report.ID, &report.PuzzleID, &report.PuzzleNumber, &report.PuzzleStatus, &report.PuzzleOrigin,
			&report.Reason, &report.Details, &report.Contact, &report.Status, &createdAt,
			&resolvedAt, &report.ResolutionNote,
		); err != nil {
			return nil, fmt.Errorf("scan report: %w", err)
		}
		report.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		report.ResolvedAt = timePtr(resolvedAt)
		reports = append(reports, report)
	}
	return reports, rows.Err()
}

func scanAppeals(rows *sql.Rows) ([]ModerationAppeal, error) {
	appeals := []ModerationAppeal{}
	for rows.Next() {
		var appeal ModerationAppeal
		var createdAt time.Time
		var resolvedAt sql.NullTime
		if err := rows.Scan(
			&appeal.ID, &appeal.PuzzleID, &appeal.PuzzleNumber, &appeal.PuzzleStatus, &appeal.PuzzleOrigin,
			&appeal.Contact, &appeal.Message, &appeal.Status, &createdAt,
			&resolvedAt, &appeal.ResolutionNote,
		); err != nil {
			return nil, fmt.Errorf("scan appeal: %w", err)
		}
		appeal.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		appeal.ResolvedAt = timePtr(resolvedAt)
		appeals = append(appeals, appeal)
	}
	return appeals, rows.Err()
}

func nullString(value string) sql.NullString {
	return sql.NullString{String: value, Valid: value != ""}
}

func stringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func timePtr(value sql.NullTime) *string {
	if !value.Valid {
		return nil
	}
	formatted := value.Time.UTC().Format(time.RFC3339)
	return &formatted
}
