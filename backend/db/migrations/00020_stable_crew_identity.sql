-- +goose Up
-- A crew membership row describes who is present now; it must not also be the
-- durable identity used by one-card, one-ballot, and no-self-vote rules. Leaving
-- deletes the active row, so the former default on crew_members.member_id let a
-- browser rejoin with a new UUID and a clean round history.
--
-- This tombstone-style mapping survives leave and removal. Rejoining from the
-- same guest identity restores the same member_id and therefore the same card
-- and ballot history. It disappears only when the crew itself is deleted.
create table crew_member_identities (
  crew_id        text not null references crews(id) on delete cascade,
  session_id     text not null,
  member_id      uuid not null default gen_random_uuid(),
  first_joined_at timestamptz not null default now(),
  primary key (crew_id, session_id),
  constraint crew_member_identities_member_id_key unique (member_id),
  constraint crew_member_identities_mapping_key unique (crew_id, session_id, member_id)
);

-- Preserve every currently active member handle so the migration does not
-- reset card or ballot ownership for existing crews.
insert into crew_member_identities (crew_id, session_id, member_id, first_joined_at)
select crew_id, session_id, member_id, joined_at
from crew_members;

-- New active rows must explicitly reuse the durable mapping. The composite
-- foreign key makes that rule true even for a future backfill or alternate
-- writer, not just the current Go store.
alter table crew_members alter column member_id drop default;
alter table crew_members
  add constraint crew_members_stable_identity_fkey
  foreign key (crew_id, session_id, member_id)
  references crew_member_identities (crew_id, session_id, member_id);

-- A vote used to reference only submission_id. The application transaction
-- checked crew, board, and self-vote rules, but the schema admitted impossible
-- cross-round and self-vote rows from any other writer. Materialize the target
-- member and bind the whole vote target to one submission tuple.
alter table vibe_votes add column target_member_id uuid;
update vibe_votes v
set target_member_id = s.submitted_by_member
from vibe_submissions s
where s.id = v.submission_id;

-- Remove any impossible historical row before making the new invariant
-- enforceable. The shipped application could not create cross-round rows, but
-- this also repairs manual writes and any rows produced through the identity
-- rotation exploit.
delete from vibe_votes v
using vibe_submissions s
where s.id = v.submission_id
  and (
    v.crew_id <> s.crew_id
    or v.board_id <> s.board_id
    or v.voter_member_id = s.submitted_by_member
  );

alter table vibe_votes alter column target_member_id set not null;
alter table vibe_submissions
  add constraint vibe_submissions_vote_target_key
  unique (id, crew_id, board_id, submitted_by_member);
alter table vibe_votes
  add constraint vibe_votes_target_round_fkey
  foreign key (submission_id, crew_id, board_id, target_member_id)
  references vibe_submissions (id, crew_id, board_id, submitted_by_member)
  on delete cascade;
alter table vibe_votes
  add constraint vibe_votes_no_self_vote_check
  check (voter_member_id <> target_member_id);

-- +goose Down
alter table vibe_votes drop constraint if exists vibe_votes_no_self_vote_check;
alter table vibe_votes drop constraint if exists vibe_votes_target_round_fkey;
alter table vibe_submissions drop constraint if exists vibe_submissions_vote_target_key;
alter table vibe_votes drop column if exists target_member_id;
alter table crew_members drop constraint if exists crew_members_stable_identity_fkey;
alter table crew_members alter column member_id set default gen_random_uuid();
drop table crew_member_identities;
