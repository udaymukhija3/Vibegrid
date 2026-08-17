-- +goose Up
-- A crew is a private group of friends playing the same daily grid. Membership
-- is keyed by the anonymous session cookie, so joining costs a link click and a
-- display name — no accounts, no email. The crew id IS the invite secret:
-- anyone holding the link can join, which is exactly the sharing model a friend
-- group wants. Ids are generated with crypto/rand and are long enough not to be
-- guessable (see newCrewID).
create table crews (
  id                 text primary key,
  name               text not null,
  created_at         timestamptz not null default now(),
  created_by_session text not null,
  constraint crews_name_check check (char_length(name) between 1 and 40)
);

-- display_name is per crew, not global: the same browser can be "Uday" in one
-- crew and "dad" in another, and there is no global namespace to police.
create table crew_members (
  crew_id      text not null references crews(id) on delete cascade,
  session_id   text not null,
  display_name text not null,
  joined_at    timestamptz not null default now(),
  primary key (crew_id, session_id),
  constraint crew_members_display_name_check check (char_length(display_name) between 1 and 24)
);

-- "Which crews am I in" runs on the home path, so index the session side; the
-- crew side is already served by the primary key's leading column.
create index crew_members_session_idx on crew_members (session_id);

-- +goose Down
drop table crew_members;
drop table crews;
