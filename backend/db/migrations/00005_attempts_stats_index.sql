-- +goose Up
-- Keep public puzzle stats refreshes on an index-backed path. The leading
-- puzzle_id predicate matches the stats query; included columns let Postgres
-- satisfy the aggregate from the index when visibility allows.
create index if not exists attempts_puzzle_stats_idx
  on attempts (puzzle_id)
  include (guess_count, completed, failed, mistakes, started_at, completed_at);

-- +goose Down
drop index if exists attempts_puzzle_stats_idx;
