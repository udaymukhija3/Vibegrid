-- +goose Up
-- Fixed-window counters for multi-instance rate limits. Postgres upserts make
-- each hit atomic across machines without introducing Redis for launch.
create table rate_limit_hits (
  key          text not null,
  bucket_start timestamptz not null,
  hits         integer not null default 0,
  updated_at   timestamptz not null default now(),
  primary key (key, bucket_start)
);

create index if not exists rate_limit_hits_updated_idx
  on rate_limit_hits (updated_at);

-- +goose Down
drop index if exists rate_limit_hits_updated_idx;
drop table rate_limit_hits;
