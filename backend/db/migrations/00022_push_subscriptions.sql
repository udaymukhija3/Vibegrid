-- +goose Up
-- A browser's push endpoint, keyed by the stable anonymous identity that crew
-- membership uses (the value crew_members.session_id holds since migration
-- 00021), never by the revocable browser bearer. Replacing a leaked browser key
-- therefore keeps reminders attached, and no plaintext bearer is stored here.
-- There are no accounts in this product, so that identity is the only thing a
-- reminder can be addressed to; a member who clears all site data loses the
-- subscription along with the crew, which is the existing bargain.
create table push_subscriptions (
  id              bigserial primary key,
  session_id      text not null check (char_length(session_id) between 1 and 128),
  -- The endpoint is the push service's own URL for this browser. It is unique
  -- per subscription, so re-subscribing the same browser updates in place
  -- instead of accumulating duplicates that would each deliver a copy.
  endpoint        text not null unique check (char_length(endpoint) between 1 and 2000),
  p256dh          text not null check (char_length(p256dh) between 1 and 200),
  auth            text not null check (char_length(auth) between 1 and 100),
  created_at      timestamptz not null default now(),
  last_success_at timestamptz,
  -- Push services fail transiently. Subscriptions are only dropped on an
  -- explicit 404/410 gone, never on a count, so a phone that is merely offline
  -- keeps its reminders.
  failure_count   integer not null default 0 check (failure_count >= 0)
);

create index push_subscriptions_session_idx on push_subscriptions (session_id);

-- The per-event delivery ledger. The outbox retries a crew digest as a whole
-- when any one send fails, so without this a member whose phone already showed
-- the reminder would be shown it again on every retry. A row means "this
-- subscription is settled for this event": the push service accepted it, or
-- rejected it in a way no retry can change. A retry skips settled rows.
--
-- It is keyed by subscription id, not endpoint, so the push service's
-- capability URL is not copied into a second table that would outlive an
-- unsubscribe; forgetting a subscription forgets its ledger rows. Re-subscribing
-- the same browser upserts on endpoint and keeps the id, so it stays settled.
-- push_subscriptions.last_success_at cannot stand in for this: a member of two
-- crews receives two different digests on the same day.
create table push_deliveries (
  event_id        bigint not null references notification_outbox (id) on delete cascade,
  subscription_id bigint not null references push_subscriptions (id) on delete cascade,
  outcome         text not null check (outcome in ('DELIVERED', 'REJECTED')),
  -- What the push service answered, so an operator can tell a VAPID key
  -- mismatch (403) from an oversized payload (413) without the logs.
  status_code     integer not null check (status_code between 200 and 499),
  settled_at      timestamptz not null default now(),
  primary key (event_id, subscription_id),
  check ((outcome = 'DELIVERED') = (status_code between 200 and 299))
);

-- Serves the cascade when a subscription is forgotten; the primary key already
-- serves the per-event lookup.
create index push_deliveries_subscription_idx on push_deliveries (subscription_id);

-- +goose Down
drop index if exists push_deliveries_subscription_idx;
drop table if exists push_deliveries;
drop index if exists push_subscriptions_session_idx;
drop table if exists push_subscriptions;
