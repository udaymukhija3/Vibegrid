-- +goose Up
-- Mode is part of the attempt contract, not a mutable browser preference. Old
-- attempts predate this field, so they are conservatively classified as Medium.
alter table attempts
  add column mode text not null default 'medium';

alter table attempts
  add constraint attempts_mode_check
  check (mode in ('easy', 'medium', 'hard'));

-- +goose Down
alter table attempts drop constraint if exists attempts_mode_check;
alter table attempts drop column if exists mode;
