-- +goose Up
-- The browser cookie used to be both bearer token and durable identity. Anyone
-- who copied it kept crew access until the cookie naturally expired, and there
-- was no server-side revocation or safe rotation path. Store only a SHA-256
-- token hash and map it to the stable anonymous identity used by crew rows.
create table guest_sessions (
  token_hash   text primary key,
  identity_id  text not null,
  created_at   timestamptz not null default now(),
  last_seen_at timestamptz not null default now(),
  expires_at   timestamptz not null,
  revoked_at   timestamptz,
  constraint guest_sessions_token_hash_check check (token_hash ~ '^[0-9a-f]{64}$'),
  constraint guest_sessions_identity_check check (identity_id ~ '^[0-9a-f]{32}$')
);

-- Rotation revokes the old token before installing its replacement. At most
-- one bearer token for an anonymous identity can therefore be live at a time.
create unique index guest_sessions_one_active_identity
  on guest_sessions (identity_id)
  where revoked_at is null;
create index guest_sessions_expiry_idx on guest_sessions (expires_at);

-- +goose Down
drop table guest_sessions;
