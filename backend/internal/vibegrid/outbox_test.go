package vibegrid

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"
)

type memoryNotificationOutbox struct {
	events    []OutboxEvent
	delivered []int64
	failed    []int64
	dead      []int64
}

func (store *memoryNotificationOutbox) Claim(context.Context, int) ([]OutboxEvent, error) {
	return store.events, nil
}

func (store *memoryNotificationOutbox) MarkDelivered(_ context.Context, id int64) error {
	store.delivered = append(store.delivered, id)
	return nil
}

func (store *memoryNotificationOutbox) MarkFailed(_ context.Context, id int64, _ string, _ time.Time, dead bool) error {
	store.failed = append(store.failed, id)
	if dead {
		store.dead = append(store.dead, id)
	}
	return nil
}

type selectiveDeliverer struct{}

func (selectiveDeliverer) Deliver(_ context.Context, event OutboxEvent) error {
	if event.Topic == "fail" {
		return errors.New("provider unavailable")
	}
	return nil
}

func TestRunNotificationBatchMarksSuccessRetryAndDead(t *testing.T) {
	store := &memoryNotificationOutbox{events: []OutboxEvent{
		{ID: 1, Topic: "ok", AttemptCount: 1},
		{ID: 2, Topic: "fail", AttemptCount: 2},
		{ID: 3, Topic: "fail", AttemptCount: outboxMaxAttempts},
	}}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runNotificationBatch(context.Background(), logger, store, selectiveDeliverer{})

	if len(store.delivered) != 1 || store.delivered[0] != 1 {
		t.Fatalf("delivered = %v", store.delivered)
	}
	if len(store.failed) != 2 || len(store.dead) != 1 || store.dead[0] != 3 {
		t.Fatalf("failed = %v, dead = %v", store.failed, store.dead)
	}
}

func TestWebhookNotificationDelivererBoundsRequestAndRequiresSuccess(t *testing.T) {
	var body string
	deliverer := NewWebhookNotificationDeliverer("https://hooks.example/notify")
	deliverer.client = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.String() != "https://hooks.example/notify" || request.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %#v", request.URL, request.Header)
		}
		bytes, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		body = string(bytes)
		return &http.Response{StatusCode: http.StatusNoContent, Status: "204 No Content", Header: make(http.Header), Body: io.NopCloser(strings.NewReader(""))}, nil
	})}

	err := deliverer.Deliver(context.Background(), OutboxEvent{
		ID: 7, Topic: "operator.report_created", AggregateType: "report", AggregateID: "rpt_7",
		Payload: []byte(`{"reason":"SPAM"}`),
	})
	if err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	if !strings.Contains(body, "operator.report_created") || !strings.Contains(body, `"reason":"SPAM"`) {
		t.Fatalf("unexpected body: %s", body)
	}
}

func TestOutboxRetryDelayCapsAtOneHour(t *testing.T) {
	if got := outboxRetryDelay(1); got != time.Minute {
		t.Fatalf("first retry = %s", got)
	}
	if got := outboxRetryDelay(99); got != time.Hour {
		t.Fatalf("capped retry = %s", got)
	}
}
