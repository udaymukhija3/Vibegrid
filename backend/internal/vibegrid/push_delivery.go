package vibegrid

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	webpush "github.com/SherClockHolmes/webpush-go"
)

// CrewDigestTopic is the one reminder this product sends. It is a digest, not a
// feed: a crew round has at most one thing worth interrupting someone for on a
// given day, and sending per-event pushes ("Bea made a card") would train people
// to turn them off long before the loop paid out.
const CrewDigestTopic = "crew.round_digest"

const (
	pushTimeToLive = 12 * 60 * 60 // seconds; a reminder for today is worthless tomorrow
	pushURGENCY    = "normal"
)

// crewDigestPayload is what the sweeper enqueues. It carries the crew and the
// date only — never what any member has or has not done. Per-member state is
// resolved at delivery, because the outbox retries and a reminder that says
// "nobody has played yet" must not still say that an hour later when they have.
type crewDigestPayload struct {
	CrewID string `json:"crewId"`
	Date   string `json:"date"`
}

// pushNotification is the JSON the service worker receives.
type pushNotification struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url"`
	Tag   string `json:"tag"`
}

// VAPIDKeys identifies this server to the push services. Subject must be a
// mailto: or https: URL the push operator can use to reach whoever runs it.
type VAPIDKeys struct {
	Public  string
	Private string
	Subject string
}

func (keys VAPIDKeys) configured() bool {
	return keys.Public != "" && keys.Private != "" && keys.Subject != ""
}

// CrewDigestSource is the read side the deliverer needs to decide what, if
// anything, each member should be told.
type CrewDigestSource interface {
	CrewByID(ctx context.Context, crewID string) (Crew, error)
	CrewSnapshot(ctx context.Context, crewID string, boardIDs []string) (VibeCrewSnapshot, error)
}

// WebPushDeliverer turns one enqueued crew digest into at most one notification
// per subscribed member.
type WebPushDeliverer struct {
	keys          VAPIDKeys
	subscriptions PushSubscriptionStore
	source        CrewDigestSource
	boardForDate  func(ctx context.Context, date string) (VibeBoard, error)
	publicBaseURL string
	clock         func() time.Time
	client        *http.Client
	logger        *slog.Logger
}

func NewWebPushDeliverer(
	keys VAPIDKeys,
	subscriptions PushSubscriptionStore,
	source CrewDigestSource,
	boardForDate func(ctx context.Context, date string) (VibeBoard, error),
	publicBaseURL string,
	clock func() time.Time,
	logger *slog.Logger,
) *WebPushDeliverer {
	return &WebPushDeliverer{
		keys:          keys,
		subscriptions: subscriptions,
		source:        source,
		boardForDate:  boardForDate,
		publicBaseURL: strings.TrimRight(publicBaseURL, "/"),
		clock:         clock,
		client:        &http.Client{Timeout: 10 * time.Second},
		logger:        logger,
	}
}

func (deliverer *WebPushDeliverer) Deliver(ctx context.Context, event OutboxEvent) error {
	var payload crewDigestPayload
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		// A payload that cannot be parsed will never parse. Reporting success
		// retires it instead of burning ten retries on a permanent defect.
		deliverer.logger.Error("discarding unreadable crew digest", "event", event.ID, "error", err)
		return nil
	}

	crew, err := deliverer.source.CrewByID(ctx, payload.CrewID)
	if errors.Is(err, ErrCrewNotFound) {
		return nil
	} else if err != nil {
		return fmt.Errorf("load crew for digest: %w", err)
	}

	today, err := deliverer.boardForDate(ctx, payload.Date)
	if err != nil {
		return fmt.Errorf("load digest board: %w", err)
	}
	judgeDate := shiftVibeDate(payload.Date, -1)
	judge, err := deliverer.boardForDate(ctx, judgeDate)
	if err != nil {
		return fmt.Errorf("load digest ballot board: %w", err)
	}
	resultDate := shiftVibeDate(payload.Date, -2)
	result, err := deliverer.boardForDate(ctx, resultDate)
	if err != nil {
		return fmt.Errorf("load digest result board: %w", err)
	}

	snapshot, err := deliverer.source.CrewSnapshot(ctx, crew.ID, []string{today.ID, judge.ID, result.ID})
	if err != nil {
		return fmt.Errorf("load crew snapshot for digest: %w", err)
	}

	sessionIDs := make([]string, 0, len(snapshot.Members))
	for _, member := range snapshot.Members {
		sessionIDs = append(sessionIDs, member.SessionID)
	}
	subscriptions, err := deliverer.subscriptions.SubscriptionsForSessions(ctx, sessionIDs)
	if err != nil {
		return fmt.Errorf("load digest subscriptions: %w", err)
	}

	var failures []string
	for _, member := range snapshot.Members {
		notification, send := crewDigestFor(crew, member, today, judge, result, snapshot, deliverer.publicBaseURL, payload.Date)
		if !send {
			continue
		}
		for _, subscription := range subscriptions[member.SessionID] {
			if err := deliverer.send(ctx, subscription, notification); err != nil {
				failures = append(failures, err.Error())
			}
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("crew digest delivery: %s", strings.Join(failures, "; "))
	}
	return nil
}

// send pushes one notification. A push service reporting the subscription gone
// is the one signal that justifies forgetting it; everything else is treated as
// transient so the outbox retries rather than silently dropping a person.
func (deliverer *WebPushDeliverer) send(ctx context.Context, subscription PushSubscription, notification pushNotification) error {
	body, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("encode notification: %w", err)
	}
	response, err := webpush.SendNotificationWithContext(ctx, body, &webpush.Subscription{
		Endpoint: subscription.Endpoint,
		Keys: webpush.Keys{
			P256dh: subscription.P256dh,
			Auth:   subscription.Auth,
		},
	}, &webpush.Options{
		HTTPClient:      deliverer.client,
		Subscriber:      deliverer.keys.Subject,
		VAPIDPublicKey:  deliverer.keys.Public,
		VAPIDPrivateKey: deliverer.keys.Private,
		TTL:             pushTimeToLive,
		Urgency:         pushURGENCY,
	})
	if err != nil {
		return fmt.Errorf("push send: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4<<10))

	switch {
	case response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusGone:
		if err := deliverer.subscriptions.DeleteSubscription(ctx, subscription.Endpoint); err != nil {
			deliverer.logger.Error("could not forget a gone push subscription", "error", err)
		}
		return nil
	case response.StatusCode >= 200 && response.StatusCode < 300:
		if err := deliverer.subscriptions.MarkSubscriptionDelivered(ctx, subscription.Endpoint, deliverer.clock()); err != nil {
			deliverer.logger.Error("could not record push delivery", "error", err)
		}
		return nil
	default:
		return fmt.Errorf("push service returned %d", response.StatusCode)
	}
}

// crewDigestFor decides what one member is told, and whether they are told
// anything at all. Silence is the default: a member with nothing outstanding
// gets no notification, because the fastest way to lose the permission is to
// spend it on "nothing has happened".
//
// The order is deliberate. A revealed result is the payoff the whole two-day
// loop exists for and outranks everything; an open ballot is the step that
// makes tomorrow's result possible; today's card is the routine ask.
func crewDigestFor(
	crew Crew,
	member VibeRoundMember,
	today, judge, result VibeBoard,
	snapshot VibeCrewSnapshot,
	baseURL string,
	date string,
) (pushNotification, bool) {
	crewURL := baseURL + "/crew/" + crew.InviteCode
	resultSubmissions := submissionsForBoard(snapshot.Submissions, result.ID)
	judgeSubmissions := submissionsForBoard(snapshot.Submissions, judge.ID)
	todaySubmissions := submissionsForBoard(snapshot.Submissions, today.ID)

	memberSubmitted := func(submissions []VibeSubmission) bool {
		for _, submission := range submissions {
			if submission.MemberID == member.MemberID {
				return true
			}
		}
		return false
	}

	// The result only lands for people who were actually in that round.
	if len(resultSubmissions) >= minOfficialVibeCards && memberSubmitted(resultSubmissions) {
		votes := votesForBoard(snapshot.Votes, result.ID)
		if len(votes) >= minOfficialVibeBallots {
			return pushNotification{
				Title: crew.Name,
				Body:  "The result is in. See which card the crew picked.",
				URL:   crewURL,
				Tag:   "vibegrid-result-" + result.PublishDate,
			}, true
		}
	}

	// An open ballot they are eligible for and have not cast.
	if memberSubmitted(judgeSubmissions) && len(judgeSubmissions) >= minOfficialVibeCards {
		voted := false
		for _, vote := range votesForBoard(snapshot.Votes, judge.ID) {
			if vote.VoterMemberID == member.MemberID {
				voted = true
				break
			}
		}
		if !voted {
			return pushNotification{
				Title: crew.Name,
				Body:  fmt.Sprintf("%d cards are waiting on your vote.", len(judgeSubmissions)),
				URL:   crewURL,
				Tag:   "vibegrid-judge-" + judge.PublishDate,
			}, true
		}
	}

	// Today's card, only while there is still a round to join.
	if !memberSubmitted(todaySubmissions) {
		if len(todaySubmissions) > 0 {
			return pushNotification{
				Title: crew.Name,
				Body:  fmt.Sprintf("%d of %d have made today's card.", len(todaySubmissions), len(snapshot.Members)),
				URL:   crewURL,
				Tag:   "vibegrid-today-" + date,
			}, true
		}
		return pushNotification{
			Title: crew.Name,
			Body:  today.Prompt,
			URL:   crewURL,
			Tag:   "vibegrid-today-" + date,
		}, true
	}
	return pushNotification{}, false
}

func shiftVibeDate(date string, days int) string {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return date
	}
	return parsed.AddDate(0, 0, days).Format("2006-01-02")
}

// routedDeliverer sends each event to the deliverer that owns its topic. The
// outbox worker takes a single deliverer, and operator webhooks and member
// reminders are different destinations with different failure modes.
type routedDeliverer struct {
	routes   map[string]NotificationDeliverer
	fallback NotificationDeliverer
}

func NewRoutedDeliverer(fallback NotificationDeliverer, routes map[string]NotificationDeliverer) NotificationDeliverer {
	return &routedDeliverer{routes: routes, fallback: fallback}
}

func (deliverer *routedDeliverer) Deliver(ctx context.Context, event OutboxEvent) error {
	if route, ok := deliverer.routes[event.Topic]; ok {
		return route.Deliver(ctx, event)
	}
	if deliverer.fallback != nil {
		return deliverer.fallback.Deliver(ctx, event)
	}
	// Nothing owns this topic. Retrying cannot change that, so retire the event
	// rather than let it age into the dead-letter count and mask a real backlog.
	return nil
}
