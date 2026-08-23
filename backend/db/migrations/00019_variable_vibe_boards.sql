-- +goose Up
-- The original 12-fragment board row remains byte-for-byte compatible with the
-- previous release. New boards add 16 immutable expansion fragments in a
-- separate table, giving the current release a seven-row master palette while
-- keeping the migration additive and the old binary bootable during rollback.
create table vibe_board_expansions (
  board_id    text primary key references vibe_daily_boards(id) on delete restrict,
  tiles       jsonb not null,
  created_at  timestamptz not null default now(),
  constraint vibe_board_expansions_tiles_array_check check (jsonb_typeof(tiles) = 'array'),
  constraint vibe_board_expansions_tiles_count_check check (jsonb_array_length(tiles) = 16)
);

create table vibe_crew_boards (
  crew_id               text not null references crews(id) on delete cascade,
  board_id              text not null references vibe_daily_boards(id) on delete restrict,
  member_count_snapshot integer not null,
  tile_count            integer not null,
  created_at            timestamptz not null default now(),
  primary key (crew_id, board_id),
  constraint vibe_crew_boards_member_count_check check (member_count_snapshot between 1 and 20),
  constraint vibe_crew_boards_tile_count_check check (
    tile_count between 12 and 28 and mod(tile_count, 4) = 0
  )
);

-- Cards created before this migration used the full 12-fragment board. Freeze
-- those rounds at their historical size so later joins cannot rewrite them.
insert into vibe_crew_boards (crew_id, board_id, member_count_snapshot, tile_count)
select distinct
  s.crew_id,
  s.board_id,
  greatest(1, least(20, (select count(*)::integer from crew_members m where m.crew_id = s.crew_id))),
  jsonb_array_length(b.tiles)
from vibe_submissions s
join vibe_daily_boards b on b.id = s.board_id;

-- +goose Down
drop table vibe_crew_boards;
drop table vibe_board_expansions;
