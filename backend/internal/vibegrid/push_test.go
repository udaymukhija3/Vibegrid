package vibegrid

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func testLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestPushSubscriptionValidationRejectsHostileEndpoints(t *testing.T) {
	valid := PushSubscription{
		SessionID: "session-a",
		Endpoint:  "https://fcm.googleapis.com/fcm/send/abc123",
		P256dh:    "BK1t2_9vQq7Yc4Xn",
		Auth:      "c2VjcmV0",
	}
	if err := validatePushSubscription(valid); err != nil {
		t.Fatalf("a real subscription was rejected: %v", err)
	}

	// The endpoint is whatever the page passed to subscribe, so it is
	// attacker-influenced. Anything but a plain https URL would turn the
	// delivery worker into a request forwarder.
	for name, endpoint := range map[string]string{
		"plaintext http":   "http://push.example.com/send/abc",
		"loopback file":    "file:///etc/passwd",
		"internal scheme":  "gopher://localhost:70/1",
		"credentials":      "https://user:pass@push.example.com/send",
		"missing host":     "https:///send/abc",
		"embedded newline": "https://push.example.com/send\nHost: evil",
		"empty":            "",
		"absurdly long":    "https://push.example.com/" + strings.Repeat("a", maxPushEndpointLength),
	} {
		t.Run(name, func(t *testing.T) {
			hostile := valid
			hostile.Endpoint = endpoint
			if err := validatePushSubscription(hostile); !errors.Is(err, ErrPushSubscriptionInvalid) {
				t.Fatalf("endpoint %q was accepted", endpoint)
			}
		})
	}

	for name, key := range map[string]string{
		"empty key":     "",
		"non base64url": "not a key!",
		"oversized":     strings.Repeat("A", maxPushKeyLength+1),
	} {
		t.Run("p256dh "+name, func(t *testing.T) {
			hostile := valid
			hostile.P256dh = key
			if err := validatePushSubscription(hostile); !errors.Is(err, ErrPushSubscriptionInvalid) {
				t.Fatalf("key %q was accepted", key)
			}
		})
	}

	anonymous := valid
	anonymous.SessionID = ""
	if err := validatePushSubscription(anonymous); !errors.Is(err, ErrPushSubscriptionInvalid) {
		t.Fatal("a subscription with no session was accepted")
	}
}

// TestCrewDigestSaysNothingWhenThereIsNothingToSay is the guard on the only
// permission this product will ever get from a browser. A reminder that says
// "nothing has happened" is how that permission gets revoked, so a member who
// is fully up to date must be left alone.
func TestCrewDigestSaysNothingWhenThereIsNothingToSay(t *testing.T) {
	today, _ := VibeBoardForDate("2026-08-21")
	judge, _ := VibeBoardForDate("2026-08-20")
	result, _ := VibeBoardForDate("2026-08-19")
	crew := Crew{ID: "crew-internal", InviteCode: "crew_public", Name: "Night Shift"}
	me := VibeRoundMember{MemberID: "member-a", SessionID: "session-a", DisplayName: "Ari"}
	other := VibeRoundMember{MemberID: "member-b", SessionID: "session-b", DisplayName: "Bea"}

	card := func(id, boardID, memberID string, board VibeBoard) VibeSubmission {
		return VibeSubmission{ID: id, BoardID: boardID, MemberID: memberID, Title: "A card",
			SelectedTileIDs: []string{board.Tiles[0].ID, board.Tiles[1].ID, board.Tiles[2].ID, board.Tiles[3].ID}}
	}

	// Made today's card, judged yesterday's ballot, and the older round never
	// reached a result. There is nothing to interrupt anyone for.
	quiet := VibeCrewSnapshot{
		Members: []VibeRoundMember{me, other},
		Submissions: []VibeSubmission{
			card("today-a", today.ID, "member-a", today),
			card("judge-a", judge.ID, "member-a", judge),
			card("judge-b", judge.ID, "member-b", judge),
		},
		Votes: []VibeVote{
			{BoardID: judge.ID, VoterMemberID: "member-a", SubmissionID: "judge-b"},
		},
	}
	if _, send := crewDigestFor(crew, me, today, judge, result, quiet, "https://vibegrid.test", "2026-08-21"); send {
		t.Fatal("a member who is fully caught up was sent a reminder")
	}

	// A member who has not made today's card is worth one nudge.
	behind := quiet
	behind.Submissions = []VibeSubmission{card("judge-a", judge.ID, "member-a", judge)}
	behind.Votes = nil
	notification, send := crewDigestFor(crew, me, today, judge, result, behind, "https://vibegrid.test", "2026-08-21")
	if !send {
		t.Fatal("a member with no card today was not reminded")
	}
	if notification.URL != "https://vibegrid.test/crew/crew_public" {
		t.Fatalf("reminder pointed somewhere unexpected: %q", notification.URL)
	}
	if notification.Title != crew.Name {
		t.Fatalf("reminder should be titled for the crew, got %q", notification.Title)
	}
}

// TestCrewDigestLeadsWithThePayoff pins the priority order. The revealed result
// is what the whole two-day delay exists for; being asked to make another card
// instead of being told yesterday's result landed would bury the one moment
// that earns the wait.
func TestCrewDigestLeadsWithThePayoff(t *testing.T) {
	today, _ := VibeBoardForDate("2026-08-21")
	judge, _ := VibeBoardForDate("2026-08-20")
	result, _ := VibeBoardForDate("2026-08-19")
	crew := Crew{ID: "crew-internal", InviteCode: "crew_public", Name: "Night Shift"}
	me := VibeRoundMember{MemberID: "member-a", SessionID: "session-a"}
	other := VibeRoundMember{MemberID: "member-b", SessionID: "session-b"}
	card := func(id, boardID, memberID string, board VibeBoard) VibeSubmission {
		return VibeSubmission{ID: id, BoardID: boardID, MemberID: memberID, Title: "A card",
			SelectedTileIDs: []string{board.Tiles[0].ID}}
	}

	// Everything is outstanding at once: a finished result, an open ballot, and
	// no card today.
	snapshot := VibeCrewSnapshot{
		Members: []VibeRoundMember{me, other},
		Submissions: []VibeSubmission{
			card("result-a", result.ID, "member-a", result),
			card("result-b", result.ID, "member-b", result),
			card("judge-a", judge.ID, "member-a", judge),
			card("judge-b", judge.ID, "member-b", judge),
		},
		Votes: []VibeVote{
			{BoardID: result.ID, VoterMemberID: "member-a", SubmissionID: "result-b"},
			{BoardID: result.ID, VoterMemberID: "member-b", SubmissionID: "result-a"},
		},
	}
	notification, send := crewDigestFor(crew, me, today, judge, result, snapshot, "https://vibegrid.test", "2026-08-21")
	if !send || !strings.Contains(notification.Body, "result is in") {
		t.Fatalf("the reveal did not outrank the other asks: %#v", notification)
	}

	// A member who sat out that round is not told about its result.
	stranger := VibeRoundMember{MemberID: "member-c", SessionID: "session-c"}
	notification, send = crewDigestFor(crew, stranger, today, judge, result, snapshot, "https://vibegrid.test", "2026-08-21")
	if send && strings.Contains(notification.Body, "result is in") {
		t.Fatal("a member who never played that round was told its result")
	}

	// With the result seen, the open ballot is next.
	voted := snapshot
	voted.Submissions = []VibeSubmission{
		card("judge-a", judge.ID, "member-a", judge),
		card("judge-b", judge.ID, "member-b", judge),
	}
	voted.Votes = nil
	notification, send = crewDigestFor(crew, me, today, judge, result, voted, "https://vibegrid.test", "2026-08-21")
	if !send || !strings.Contains(notification.Body, "waiting on your vote") {
		t.Fatalf("an open ballot did not outrank today's card: %#v", notification)
	}
}

type stubDigestOutbox struct {
	enqueued []string
}

func (outbox *stubDigestOutbox) EnqueueCrewDigest(_ context.Context, crewID, date string) error {
	outbox.enqueued = append(outbox.enqueued, crewID+"@"+date)
	return nil
}

type stubSubscriptionStore struct {
	PushSubscriptionStore
	crews []string
}

func (store *stubSubscriptionStore) CrewsAwaitingDigest(context.Context) ([]string, error) {
	return store.crews, nil
}

// TestPushSweeperHoldsUntilTheDigestHour keeps a restart from firing the
// evening's reminder at breakfast. The sweeper runs every fifteen minutes, so
// without the hour gate every morning boot would notify everybody.
func TestPushSweeperHoldsUntilTheDigestHour(t *testing.T) {
	subscriptions := &stubSubscriptionStore{crews: []string{"crew-1", "crew-2"}}

	morning := &stubDigestOutbox{}
	early := NewPushDigestSweeper(subscriptions, morning,
		func() time.Time { return time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC) },
		18, testLogger())
	early.SweepOnce(context.Background())
	if len(morning.enqueued) != 0 {
		t.Fatalf("swept before the digest hour: %v", morning.enqueued)
	}

	evening := &stubDigestOutbox{}
	late := NewPushDigestSweeper(subscriptions, evening,
		func() time.Time { return time.Date(2026, 8, 21, 18, 5, 0, 0, time.UTC) },
		18, testLogger())
	late.SweepOnce(context.Background())
	// Sweeping twice must not double up; the dedupe key is what makes that safe
	// in the store, and the sweeper is expected to lean on it rather than track
	// state of its own.
	late.SweepOnce(context.Background())
	if len(evening.enqueued) != 4 {
		t.Fatalf("expected both crews enqueued on each sweep, got %v", evening.enqueued)
	}
	for _, entry := range evening.enqueued {
		if !strings.HasSuffix(entry, "@2026-08-21") {
			t.Fatalf("digest was dated wrong: %q", entry)
		}
	}

	// An out-of-range hour falls back rather than never firing.
	if got := NewPushDigestSweeper(subscriptions, evening, time.Now, 99, testLogger()).hourUTC; got != DefaultPushDigestHourUTC {
		t.Fatalf("invalid digest hour was kept: %d", got)
	}
}

// TestRoutedDelivererSendsEachTopicToItsOwner covers the split between operator
// webhooks and member reminders: they are different destinations with different
// failure modes, and the outbox worker takes only one deliverer.
func TestRoutedDelivererSendsEachTopicToItsOwner(t *testing.T) {
	var wentToPush, wentToWebhook int
	push := delivererFunc(func(context.Context, OutboxEvent) error { wentToPush++; return nil })
	webhook := delivererFunc(func(context.Context, OutboxEvent) error { wentToWebhook++; return nil })

	routed := NewRoutedDeliverer(webhook, map[string]NotificationDeliverer{CrewDigestTopic: push})
	ctx := context.Background()
	if err := routed.Deliver(ctx, OutboxEvent{Topic: CrewDigestTopic}); err != nil {
		t.Fatalf("crew digest: %v", err)
	}
	if err := routed.Deliver(ctx, OutboxEvent{Topic: "operator.report_created"}); err != nil {
		t.Fatalf("operator event: %v", err)
	}
	if wentToPush != 1 || wentToWebhook != 1 {
		t.Fatalf("routing sent push=%d webhook=%d", wentToPush, wentToWebhook)
	}

	// With no fallback an unowned topic is retired, not retried: nothing will
	// ever claim it, and leaving it to age would inflate the dead-letter count
	// that operators watch for real backlogs.
	orphan := NewRoutedDeliverer(nil, map[string]NotificationDeliverer{})
	if err := orphan.Deliver(ctx, OutboxEvent{Topic: "operator.report_created"}); err != nil {
		t.Fatalf("unowned topic should retire quietly, got %v", err)
	}
}

type delivererFunc func(context.Context, OutboxEvent) error

func (fn delivererFunc) Deliver(ctx context.Context, event OutboxEvent) error { return fn(ctx, event) }
