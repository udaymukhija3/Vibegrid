-- +goose Up
-- Puzzle content moves into the database so admins can author and publish it.
-- publish_date is nullable (drafts have no date) and unique, which enforces at
-- most one puzzle per calendar date while allowing many undated drafts.
create table puzzles (
  id            text primary key,
  puzzle_number integer not null unique,
  publish_date  date unique,
  status        text not null default 'DRAFT',
  difficulty    text not null default 'MEDIUM',
  created_at    timestamptz not null default now(),
  updated_at    timestamptz not null default now()
);

create table puzzle_groups (
  id          text primary key,
  puzzle_id   text not null references puzzles(id) on delete cascade,
  name        text not null,
  explanation text not null,
  color_index integer not null,
  sort_order  integer not null,
  unique (puzzle_id, color_index),
  unique (puzzle_id, sort_order)
);

create table puzzle_tiles (
  id         text primary key,
  puzzle_id  text not null references puzzles(id) on delete cascade,
  group_id   text not null references puzzle_groups(id) on delete cascade,
  text       text not null,
  sort_order integer not null,
  unique (puzzle_id, text)
);

-- Attempts can now reference real puzzle rows. NOT VALID adds the constraint for
-- all new rows without re-checking pre-existing attempts (which predate the
-- puzzles table), keeping the migration safe to apply on a live database.
alter table attempts
  add constraint attempts_puzzle_id_fkey
  foreign key (puzzle_id) references puzzles(id) on delete cascade not valid;

-- +goose Down
alter table attempts drop constraint attempts_puzzle_id_fkey;
drop table puzzle_tiles;
drop table puzzle_groups;
drop table puzzles;
