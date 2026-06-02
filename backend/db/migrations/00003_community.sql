-- +goose Up
-- origin distinguishes editorial daily puzzles from user-created community ones.
-- Community puzzles are playable by direct link but never enter the daily
-- rotation or archive, so the shared daily ritual stays intact.
alter table puzzles add column origin text not null default 'EDITORIAL';

-- +goose Down
alter table puzzles drop column origin;
