-- +goose Up
-- Repair live databases that applied an earlier attempts/guesses schema before
-- the current idempotent guess write path. All clauses are safe no-ops for
-- fresh databases.
alter table attempts
  add column if not exists completed boolean not null default false,
  add column if not exists failed boolean not null default false,
  add column if not exists solved_group_ids text[] not null default '{}',
  add column if not exists completed_at timestamptz;

-- The current submit path reads and writes every column below. If a database was
-- created from an older local/demo schema, missing any one of them turns every
-- valid guess into a 500 ("Could not record that guess."). Keep the repair
-- additive and preserve existing rows where possible.
alter table attempt_guesses
  add column if not exists client_guess_id text,
  add column if not exists selected_tile_ids text[] not null default '{}',
  add column if not exists is_correct boolean not null default false,
  add column if not exists matched_group_id text,
  add column if not exists revealed boolean not null default false,
  add column if not exists created_at timestamptz not null default now();

update attempt_guesses
   set client_guess_id = gen_random_uuid()::text
 where client_guess_id is null or client_guess_id = '';

alter table attempt_guesses
  alter column client_guess_id set not null;

do $$
begin
  if not exists (
    select 1
      from pg_constraint
     where conname = 'attempt_guesses_attempt_id_client_guess_id_key'
       and conrelid = 'attempt_guesses'::regclass
  ) then
    alter table attempt_guesses
      add constraint attempt_guesses_attempt_id_client_guess_id_key
      unique (attempt_id, client_guess_id);
  end if;
end $$;

-- Bank-generated daily puzzles are virtual content served from code when no
-- scheduled DB puzzle exists. Attempts still need to be writable for them.
alter table attempts
  drop constraint if exists attempts_puzzle_id_fkey;

-- +goose Down
-- This is a production repair migration; keep rollback non-destructive.
select 1;
