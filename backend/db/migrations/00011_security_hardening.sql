-- +goose Up
-- Revocable, opaque admin browser sessions. Only a hash of the random cookie
-- token is stored, so a database read cannot be replayed as an admin cookie.
create table admin_sessions (
  token_hash text primary key,
  expires_at timestamptz not null,
  created_at timestamptz not null default now(),
  revoked_at timestamptz
);

create index admin_sessions_expiry_idx
  on admin_sessions (expires_at);

-- Anonymous attempt retention is enforced by the application cleanup worker;
-- this index keeps its bounded oldest-first deletes inexpensive.
create index attempts_started_at_idx
  on attempts (started_at);

-- +goose Down
drop index if exists attempts_started_at_idx;
drop index if exists admin_sessions_expiry_idx;
drop table admin_sessions;
