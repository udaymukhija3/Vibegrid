-- +goose Up
-- Creator claims are opaque bearer secrets returned exactly once. Only their
-- SHA-256 hashes are durable, so a database read cannot be replayed as a claim.
alter table puzzles
  add column creator_claim_hash text,
  add column creator_withdrawn_at timestamptz;

alter table puzzles
  add constraint puzzles_creator_claim_hash_check
  check (creator_claim_hash is null or length(creator_claim_hash) = 64);

-- +goose Down
alter table puzzles drop constraint if exists puzzles_creator_claim_hash_check;
alter table puzzles drop column if exists creator_withdrawn_at;
alter table puzzles drop column if exists creator_claim_hash;
