package vibegrid

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	outboxBatchSize   = 20
	outboxMaxAttempts = 10
)

type OutboxEvent struct {
	ID            int64
	Topic         string
	AggregateType string
	AggregateID   string
	Payload       json.RawMessage
	AttemptCount  int
}

type NotificationOutbox interface {
	Claim(ctx context.Context, limit int) ([]OutboxEvent, error)
	MarkDelivered(ctx context.Context, id int64) error
	MarkFailed(ctx context.Context, id int64, message string, retryAt time.Time, dead bool) error
}

type NotificationDeliverer interface {
	Deliver(ctx context.Context, event OutboxEvent) error
}

type PostgresNotificationOutbox struct {
	db *sql.DB
}

type NotificationOutboxStats struct {
	Pending              int64
	Retrying             int64
	Dead                 int64
	OldestPendingSeconds float64
}

func NewPostgresNotificationOutbox(database *sql.DB) *PostgresNotificationOutbox {
	return &PostgresNotificationOutbox{db: database}
}

func (store *PostgresNotificationOutbox) Stats(ctx context.Context) NotificationOutboxStats {
	var stats NotificationOutboxStats
	err := store.db.QueryRowContext(ctx, `
		select
			count(*) filter (where status = 'PENDING' and attempt_count = 0),
			count(*) filter (where status in ('PENDING', 'PROCESSING') and attempt_count > 0),
			count(*) filter (where status = 'DEAD'),
			coalesce(extract(epoch from (now() - min(created_at) filter (where status in ('PENDING', 'PROCESSING')))), 0)
		from notification_outbox`).Scan(&stats.Pending, &stats.Retrying, &stats.Dead, &stats.OldestPendingSeconds)
	if err != nil {
		return NotificationOutboxStats{}
	}
	return stats
}

// Claim uses SKIP LOCKED so multiple workers can safely share the queue. Events
// abandoned by a crashed worker are reclaimable after five minutes.
func (store *PostgresNotificationOutbox) Claim(ctx context.Context, limit int) ([]OutboxEvent, error) {
	if limit <= 0 || limit > outboxBatchSize {
		limit = outboxBatchSize
	}
	rows, err := store.db.QueryContext(ctx, `
		with claimable as (
			select id
			from notification_outbox
			where available_at <= now()
			  and (status = 'PENDING' or (status = 'PROCESSING' and locked_at < now() - interval '5 minutes'))
			order by available_at, id
			limit $1
			for update skip locked
		)
		update notification_outbox o
		set status = 'PROCESSING', locked_at = now(), attempt_count = attempt_count + 1
		from claimable c
		where o.id = c.id
		returning o.id, o.topic, o.aggregate_type, o.aggregate_id, o.payload, o.attempt_count`, limit)
	if err != nil {
		return nil, fmt.Errorf("claim notification outbox: %w", err)
	}
	defer rows.Close()
	events := make([]OutboxEvent, 0, limit)
	for rows.Next() {
		var event OutboxEvent
		if err := rows.Scan(&event.ID, &event.Topic, &event.AggregateType, &event.AggregateID, &event.Payload, &event.AttemptCount); err != nil {
			return nil, fmt.Errorf("scan notification outbox: %w", err)
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read notification outbox: %w", err)
	}
	return events, nil
}

func (store *PostgresNotificationOutbox) MarkDelivered(ctx context.Context, id int64) error {
	_, err := store.db.ExecContext(ctx, `
		update notification_outbox
		set status = 'DELIVERED', delivered_at = now(), locked_at = null, last_error = ''
		where id = $1 and status = 'PROCESSING'`, id)
	if err != nil {
		return fmt.Errorf("mark notification delivered: %w", err)
	}
	return nil
}

func (store *PostgresNotificationOutbox) MarkFailed(ctx context.Context, id int64, message string, retryAt time.Time, dead bool) error {
	status := "PENDING"
	if dead {
		status = "DEAD"
	}
	message = truncateRunes(strings.TrimSpace(message), 1000)
	_, err := store.db.ExecContext(ctx, `
		update notification_outbox
		set status = $2, available_at = $3, locked_at = null, last_error = $4
		where id = $1 and status = 'PROCESSING'`, id, status, retryAt, message)
	if err != nil {
		return fmt.Errorf("mark notification failed: %w", err)
	}
	return nil
}

type WebhookNotificationDeliverer struct {
	url    string
	client *http.Client
}

func NewWebhookNotificationDeliverer(webhookURL string) *WebhookNotificationDeliverer {
	return &WebhookNotificationDeliverer{
		url:    strings.TrimSpace(webhookURL),
		client: &http.Client{Timeout: 5 * time.Second},
	}
}

func (deliverer *WebhookNotificationDeliverer) Deliver(ctx context.Context, event OutboxEvent) error {
	body, err := json.Marshal(map[string]any{
		"text": "VibeGrid " + event.Topic + ": " + event.AggregateType + " " + event.AggregateID,
		"event": map[string]any{
			"id":      event.ID,
			"topic":   event.Topic,
			"payload": json.RawMessage(event.Payload),
		},
	})
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, deliverer.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create notification request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := deliverer.client.Do(req)
	if err != nil {
		return fmt.Errorf("deliver notification: %w", err)
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("notification provider returned %s", response.Status)
	}
	return nil
}

func RunNotificationOutbox(ctx context.Context, logger *slog.Logger, store NotificationOutbox, deliverer NotificationDeliverer) {
	if store == nil || deliverer == nil {
		return
	}
	go func() {
		runNotificationBatch(ctx, logger, store, deliverer)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				runNotificationBatch(ctx, logger, store, deliverer)
			}
		}
	}()
}

func runNotificationBatch(ctx context.Context, logger *slog.Logger, store NotificationOutbox, deliverer NotificationDeliverer) {
	batchCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	events, err := store.Claim(batchCtx, outboxBatchSize)
	if err != nil {
		logger.Warn("notification outbox claim failed", "error", err)
		return
	}
	for _, event := range events {
		deliveryCtx, deliveryCancel := context.WithTimeout(ctx, 6*time.Second)
		err := deliverer.Deliver(deliveryCtx, event)
		deliveryCancel()
		if err == nil {
			if markErr := store.MarkDelivered(ctx, event.ID); markErr != nil {
				logger.Warn("notification outbox completion failed", "event_id", event.ID, "error", markErr)
			}
			continue
		}
		dead := event.AttemptCount >= outboxMaxAttempts
		retryAt := time.Now().Add(outboxRetryDelay(event.AttemptCount))
		if markErr := store.MarkFailed(ctx, event.ID, err.Error(), retryAt, dead); markErr != nil {
			logger.Warn("notification outbox retry update failed", "event_id", event.ID, "error", markErr)
			continue
		}
		logger.Warn("notification delivery failed", "event_id", event.ID, "topic", event.Topic, "attempt", event.AttemptCount, "dead", dead, "error", err)
	}
}

func outboxRetryDelay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	delay := time.Minute << min(attempt-1, 6)
	if delay > time.Hour {
		return time.Hour
	}
	return delay
}
