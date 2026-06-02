-- +goose Up
-- Mutable per-session game state. Puzzle content is currently static and served
-- from the seed package, so puzzle_id is a plain text reference rather than a
-- foreign key; DB-backed puzzles and admin authoring arrive in a later migration.
create table attempts (
  id               uuid primary key default gen_random_uuid(),
  puzzle_id        text not null,
  session_id       text not null,
  mistakes         integer not null default 0,
  guess_count      integer not null default 0,
  completed        boolean not null default false,
  failed           boolean not null default false,
  solved_group_ids text[] not null default '{}',
  started_at       timestamptz not null default now(),
  completed_at     timestamptz,
  unique (puzzle_id, session_id)
);

-- One row per submitted guess. The unique key on (attempt_id, client_guess_id)
-- is what makes guess submission idempotent: a replayed client guess collides
-- and is rejected by the insert inside the transaction.
create table attempt_guesses (
  id                uuid primary key default gen_random_uuid(),
  attempt_id        uuid not null references attempts(id) on delete cascade,
  client_guess_id   text not null,
  selected_tile_ids text[] not null,
  is_correct        boolean not null,
  matched_group_id  text,
  revealed          boolean not null default false,
  created_at        timestamptz not null default now(),
  unique (attempt_id, client_guess_id)
);

-- +goose Down
drop table attempt_guesses;
drop table attempts;
