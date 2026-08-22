# VibeGrid observability runbook

Observability must answer two different questions:

1. Is the service healthy enough to serve and persist crew rounds?
2. Where does the make → judge → reveal loop fail, without logging private
   card content, display names, or invite secrets?

## Availability checks

- `GET /healthz` every minute: process liveness.
- `GET /readyz` every minute: required dependency readiness, including DB.
- `GET /` every five minutes: embedded frontend delivery.
- `GET /api/vibes/today` every five minutes: active board contract.

Alert after two consecutive liveness/readiness failures and after a sustained
frontend/board failure. A production crew deployment should never treat a DB
outage as healthy just because public practice remains available.

## Metrics access

`/metrics` exists only when `VIBEGRID_METRICS_TOKEN` is configured and then
requires:

```text
Authorization: Bearer <token>
```

Mount the token for Prometheus or adapt `monitoring/prometheus.yml` to the target
platform. Never put the token in a public URL or frontend variable.

## Existing technical signals

- HTTP request count, duration, request bytes, and response bytes by bounded
  method/route/status labels.
- Store-operation count/duration by bounded component/operation/status.
- DB open/in-use/idle connections, wait count, and wait duration.
- Legacy puzzle-cache hit/miss/eviction/entry metrics.
- Notification-outbox pending/retrying/dead/oldest-pending gauges.
- Process `vibegrid_up`.

New routes have stable labels:

- `/api/vibes/today`
- `/api/crews/{id}/daily`
- `/api/crews/{id}/submissions`
- `/api/crews/{id}/votes`
- `/api/admin/vibe-boards`

Unknown API paths collapse to `/api/*`. Crew ids, submission ids, board ids,
display names, and titles must never become metric labels.

## Logs

Request logs include request id, method, normalized path, status, latency,
client identity derived under the trusted-proxy policy, and user agent. Ship
stdout/stderr to a durable log store for production.

Do not add:

- invite codes or raw crew URLs;
- card titles or selected fragment text;
- display names or session ids;
- admin/metrics secrets;
- request bodies for crew mutations.

For an incident, correlate request id and bounded route, then query database
state with an explicit crew internal id obtained through authorized operator
work—not by widening routine logs.

## Alert candidates

- readiness down;
- 5xx ratio over 5% for five minutes;
- p95 crew daily latency over one second;
- any sustained submission/vote 5xx increase;
- DB pool wait count increasing continuously;
- DB in-use connections near configured maximum;
- mutation rate-limit backend failure;
- notification outbox dead/pending age if notifications are configured;
- backup freshness heartbeat missing once external backup automation exists.

Product-level quiet rounds or low judging are not paging alerts. They belong in
a privacy-reviewed product dashboard.

## Privacy-safe product events (not yet implemented)

If product analytics is added, use short-lived pseudonymous crew-scoped or
aggregate events and exclude content:

- practice_completed;
- crew_created;
- member_joined with coarse crew-size bucket;
- card_submitted;
- judge_eligible_returned;
- vote_cast;
- result_revealed with official/quiet and tie booleans;
- second_official_reveal.

Never send prompt text, fragment ids/text, card titles, author names, invite
codes, or stable cross-crew identity to analytics by default. Ratify retention
and consent before implementation.

## Incident first look

1. Compare `/healthz` and `/readyz`.
2. Check the board endpoint directly and confirm UTC date.
3. Inspect route status/latency for daily, submissions, and votes.
4. Inspect DB reachability, connections, wait time, and recent migrations.
5. Check deploy SHA and rollback candidate.
6. Preserve evidence before changing state.
7. Use `docs/runbooks/incident-triage.md` and the rollback/restore runbooks.

After recovery, record user-visible impact by stage: unable to load, unable to
make, unable to judge, or unable to reveal. That is more actionable than a
generic “game was down.”
