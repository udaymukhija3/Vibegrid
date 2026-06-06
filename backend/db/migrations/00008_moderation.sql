-- +goose Up
-- Reports, appeals, and audit events are the moderation control plane for
-- community puzzles. Reports are public writes; audit rows are admin-only.
create table moderation_reports (
  id              text primary key,
  puzzle_id       text not null references puzzles(id) on delete cascade,
  reporter_session_id text,
  reason          text not null,
  details         text not null default '',
  contact         text not null default '',
  status          text not null default 'OPEN',
  resolution_note text not null default '',
  created_at      timestamptz not null default now(),
  resolved_at     timestamptz
);

create index if not exists moderation_reports_status_created_idx
  on moderation_reports (status, created_at desc);

create index if not exists moderation_reports_puzzle_idx
  on moderation_reports (puzzle_id, created_at desc);

create table moderation_appeals (
  id              text primary key,
  puzzle_id       text not null references puzzles(id) on delete cascade,
  contact         text not null default '',
  message         text not null,
  status          text not null default 'OPEN',
  resolution_note text not null default '',
  created_at      timestamptz not null default now(),
  resolved_at     timestamptz
);

create index if not exists moderation_appeals_status_created_idx
  on moderation_appeals (status, created_at desc);

create table moderation_actions (
  id         uuid primary key default gen_random_uuid(),
  report_id text references moderation_reports(id) on delete set null,
  appeal_id text references moderation_appeals(id) on delete set null,
  puzzle_id text references puzzles(id) on delete set null,
  actor      text not null,
  action     text not null,
  reason     text not null default '',
  note       text not null default '',
  created_at timestamptz not null default now()
);

create index if not exists moderation_actions_created_idx
  on moderation_actions (created_at desc);

create index if not exists moderation_actions_puzzle_idx
  on moderation_actions (puzzle_id, created_at desc);

-- +goose Down
drop index if exists moderation_actions_puzzle_idx;
drop index if exists moderation_actions_created_idx;
drop table moderation_actions;
drop index if exists moderation_appeals_status_created_idx;
drop table moderation_appeals;
drop index if exists moderation_reports_puzzle_idx;
drop index if exists moderation_reports_status_created_idx;
drop table moderation_reports;
