-- +goose Up
-- Admin heatmaps look up wrong guesses by attempt after narrowing attempts by
-- puzzle_id. Keep that join/filter path off the heap as traffic grows.
create index if not exists attempt_guesses_wrong_attempt_idx
  on attempt_guesses (attempt_id)
  include (selected_tile_ids)
  where is_correct = false;

-- Streaks read completed editorial puzzles for one session.
create index if not exists attempts_session_completed_idx
  on attempts (session_id)
  include (puzzle_id)
  where completed = true;

-- +goose Down
drop index if exists attempts_session_completed_idx;
drop index if exists attempt_guesses_wrong_attempt_idx;
