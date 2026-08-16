-- +goose Up
create table idempotency_keys (
    scope text not null,
    key_hash char(64) not null,
    request_hash char(64) not null,
    created_at timestamptz not null default now(),
    status_code integer not null check (status_code between 200 and 299),
    response_headers jsonb not null default '{}'::jsonb,
    response_body bytea not null check (octet_length(response_body) <= 131072),
    primary key (scope, key_hash)
);

create index idempotency_keys_created_at_idx on idempotency_keys(created_at);

-- +goose Down
drop table idempotency_keys;
