-- +goose Up
-- A browser's push endpoint, tied to the same anonymous session that carries
-- crew membership. There are no accounts in this product, so the session is the
-- only identity a reminder can be addressed to; a member who clears cookies
-- loses the subscription along with the crew, which is the existing bargain.
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

-- +goose Down
drop index if exists push_subscriptions_session_idx;
drop table if exists push_subscriptions;
