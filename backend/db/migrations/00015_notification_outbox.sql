-- +goose Up
create table notification_outbox (
  id             bigserial primary key,
  topic          text not null check (char_length(topic) between 1 and 100),
  aggregate_type text not null check (char_length(aggregate_type) between 1 and 50),
  aggregate_id   text not null check (char_length(aggregate_id) between 1 and 200),
  dedupe_key     text not null unique check (char_length(dedupe_key) between 1 and 300),
  payload        jsonb not null default '{}'::jsonb check (jsonb_typeof(payload) = 'object'),
  status         text not null default 'PENDING' check (status in ('PENDING', 'PROCESSING', 'DELIVERED', 'DEAD')),
  attempt_count  integer not null default 0 check (attempt_count >= 0),
  available_at   timestamptz not null default now(),
  locked_at      timestamptz,
  delivered_at   timestamptz,
  last_error     text not null default '' check (char_length(last_error) <= 1000),
  created_at     timestamptz not null default now()
);

create index notification_outbox_claim_idx
  on notification_outbox (available_at, id)
  where status in ('PENDING', 'PROCESSING');

create index notification_outbox_status_created_idx
  on notification_outbox (status, created_at);

-- The function body is dollar-quoted and contains ";" of its own. Without these
-- markers goose cuts the block at the first inner ";" and Postgres sees an
-- unterminated "$$" string (SQLSTATE 42601), which fails the migration and — with
-- VIBEGRID_MIGRATE_ON_BOOT and VIBEGRID_REQUIRE_DATABASE both on — the boot.
-- +goose StatementBegin
create function enqueue_vibegrid_notification() returns trigger language plpgsql as $$
begin
  if tg_table_name = 'moderation_reports' then
    insert into notification_outbox (topic, aggregate_type, aggregate_id, dedupe_key, payload)
    values (
      'operator.report_created', 'report', new.id, 'report-created:' || new.id,
      jsonb_build_object('reportId', new.id, 'puzzleId', new.puzzle_id, 'reason', new.reason)
    ) on conflict (dedupe_key) do nothing;
  elsif tg_table_name = 'moderation_appeals' then
    insert into notification_outbox (topic, aggregate_type, aggregate_id, dedupe_key, payload)
    values (
      'operator.appeal_created', 'appeal', new.id, 'appeal-created:' || new.id,
      jsonb_build_object('appealId', new.id, 'puzzleId', new.puzzle_id)
    ) on conflict (dedupe_key) do nothing;
  elsif tg_table_name = 'puzzles' and tg_op = 'INSERT' and new.origin = 'COMMUNITY' then
    insert into notification_outbox (topic, aggregate_type, aggregate_id, dedupe_key, payload)
    values (
      'operator.community_submitted', 'puzzle', new.id, 'community-submitted:' || new.id,
      jsonb_build_object('puzzleId', new.id, 'puzzleNumber', new.puzzle_number, 'status', new.status)
    ) on conflict (dedupe_key) do nothing;
  elsif tg_table_name = 'puzzles' and tg_op = 'UPDATE' and new.origin = 'COMMUNITY' and new.status is distinct from old.status then
    insert into notification_outbox (topic, aggregate_type, aggregate_id, dedupe_key, payload)
    values (
      'creator.community_status_changed', 'puzzle', new.id,
      'community-status:' || new.id || ':' || new.status,
      jsonb_build_object('puzzleId', new.id, 'puzzleNumber', new.puzzle_number, 'fromStatus', old.status, 'status', new.status)
    ) on conflict (dedupe_key) do nothing;
  end if;
  return new;
end;
$$;
-- +goose StatementEnd

create trigger notification_outbox_community_insert
after insert on puzzles for each row execute function enqueue_vibegrid_notification();

create trigger notification_outbox_community_status
after update of status on puzzles for each row execute function enqueue_vibegrid_notification();

create trigger notification_outbox_report_insert
after insert on moderation_reports for each row execute function enqueue_vibegrid_notification();

create trigger notification_outbox_appeal_insert
after insert on moderation_appeals for each row execute function enqueue_vibegrid_notification();

-- +goose Down
drop trigger if exists notification_outbox_appeal_insert on moderation_appeals;
drop trigger if exists notification_outbox_report_insert on moderation_reports;
drop trigger if exists notification_outbox_community_status on puzzles;
drop trigger if exists notification_outbox_community_insert on puzzles;
drop function if exists enqueue_vibegrid_notification();
drop table if exists notification_outbox;
