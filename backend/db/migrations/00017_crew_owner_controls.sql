-- +goose Up
-- Splits "which crew is this" from "who is allowed in".
--
-- The crew id used to be the invite secret, which made a leaked link permanent:
-- there was no way to revoke it without deleting the crew. invite_code is now
-- the thing that appears in the URL and the thing that can be rotated, while id
-- stays the stable internal key that foreign keys and member lookups use.
-- Existing crews keep their current links by seeding the code from the id.
alter table crews add column invite_code text;
update crews set invite_code = id where invite_code is null;
alter table crews alter column invite_code set not null;
alter table crews add constraint crews_invite_code_key unique (invite_code);

-- An opaque per-membership handle so an owner can remove someone without the
-- board ever exposing a session id (that cookie *is* the player's identity).
-- Display names would have been the alternative, but a member can change theirs,
-- which turns "remove Ada" into a race.
alter table crew_members add column member_id uuid not null default gen_random_uuid();
alter table crew_members add constraint crew_members_member_id_key unique (member_id);

-- +goose Down
alter table crew_members drop constraint if exists crew_members_member_id_key;
alter table crew_members drop column if exists member_id;
alter table crews drop constraint if exists crews_invite_code_key;
alter table crews drop column if exists invite_code;
