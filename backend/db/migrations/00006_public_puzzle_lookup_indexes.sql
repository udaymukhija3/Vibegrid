-- +goose Up
-- Match the public "today" and archive lookup shape:
-- published editorial puzzles ordered by publish date.
create index if not exists puzzles_public_editorial_idx
  on puzzles (publish_date desc, puzzle_number desc)
  where status = 'PUBLISHED'
    and origin <> 'COMMUNITY'
    and publish_date is not null;

-- Keep targeted puzzle assembly cheap after selecting a public puzzle id.
create index if not exists puzzle_groups_puzzle_sort_idx
  on puzzle_groups (puzzle_id, sort_order);

create index if not exists puzzle_tiles_puzzle_sort_idx
  on puzzle_tiles (puzzle_id, sort_order);

-- +goose Down
drop index if exists puzzle_tiles_puzzle_sort_idx;
drop index if exists puzzle_groups_puzzle_sort_idx;
drop index if exists puzzles_public_editorial_idx;
