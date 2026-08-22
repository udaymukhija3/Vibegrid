-- +goose Up
-- The reimagined VibeGrid stores the editorial board separately from player
-- interpretations. A board has no answer key: twelve fragments are immutable
-- ingredients for one publish date.
create table vibe_daily_boards (
  id           text primary key,
  publish_date date not null unique,
  board_number integer not null,
  prompt       text not null,
  tiles        jsonb not null,
  created_at   timestamptz not null default now(),
  constraint vibe_daily_boards_prompt_check check (char_length(prompt) between 1 and 140),
  constraint vibe_daily_boards_tiles_array_check check (jsonb_typeof(tiles) = 'array'),
  constraint vibe_daily_boards_tiles_count_check check (jsonb_array_length(tiles) = 12)
);

-- A member authors at most one card per crew and board. display_name is a
-- snapshot: removing or renaming a membership must not rewrite crew history.
-- client_submission_id makes a timed-out browser retry safe.
create table vibe_submissions (
  id                    text primary key,
  crew_id               text not null references crews(id) on delete cascade,
  board_id              text not null references vibe_daily_boards(id) on delete restrict,
  submitted_by_member   uuid not null,
  display_name          text not null,
  title                 text not null,
  selected_tile_ids     text[] not null,
  client_submission_id  text not null,
  created_at            timestamptz not null default now(),
  constraint vibe_submissions_title_check check (char_length(title) between 1 and 40),
  constraint vibe_submissions_tile_count_check check (cardinality(selected_tile_ids) = 4),
  constraint vibe_submissions_tiles_distinct_check check (
    selected_tile_ids[1] <> selected_tile_ids[2]
    and selected_tile_ids[1] <> selected_tile_ids[3]
    and selected_tile_ids[1] <> selected_tile_ids[4]
    and selected_tile_ids[2] <> selected_tile_ids[3]
    and selected_tile_ids[2] <> selected_tile_ids[4]
    and selected_tile_ids[3] <> selected_tile_ids[4]
  ),
  constraint vibe_submissions_one_card unique (crew_id, board_id, submitted_by_member),
  constraint vibe_submissions_replay_key unique (crew_id, submitted_by_member, client_submission_id)
);

create index vibe_submissions_round_idx
  on vibe_submissions (crew_id, board_id, created_at, id);

-- A submitted member gets one ballot. The service transaction verifies that
-- the voter authored a card in this round, the target is in the same round,
-- and the target is not the voter's own card.
create table vibe_votes (
  id              text primary key,
  crew_id         text not null references crews(id) on delete cascade,
  board_id        text not null references vibe_daily_boards(id) on delete restrict,
  voter_member_id uuid not null,
  submission_id   text not null references vibe_submissions(id) on delete cascade,
  client_vote_id  text not null,
  created_at      timestamptz not null default now(),
  constraint vibe_votes_one_ballot unique (crew_id, board_id, voter_member_id),
  constraint vibe_votes_replay_key unique (crew_id, voter_member_id, client_vote_id)
);

create index vibe_votes_result_idx
  on vibe_votes (crew_id, board_id, submission_id);

-- +goose Down
drop table vibe_votes;
drop table vibe_submissions;
drop table vibe_daily_boards;
