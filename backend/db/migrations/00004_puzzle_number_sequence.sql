-- +goose Up
-- Allocate puzzle numbers in the database so concurrent public creates cannot
-- race on max(puzzle_number) + 1.
create sequence if not exists puzzle_number_seq;

select setval(
  'puzzle_number_seq',
  greatest(coalesce((select max(puzzle_number) from puzzles), 0) + 1, 1),
  false
);

alter table puzzles
  alter column puzzle_number set default nextval('puzzle_number_seq');

-- +goose Down
alter table puzzles
  alter column puzzle_number drop default;

drop sequence if exists puzzle_number_seq;
