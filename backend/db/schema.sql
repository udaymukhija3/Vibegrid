create table puzzles (
  id text primary key,
  puzzle_number integer not null unique,
  publish_date date not null unique,
  status text not null default 'DRAFT',
  difficulty text not null default 'MEDIUM',
  created_at timestamptz not null default now(),
  updated_at timestamptz not null default now()
);

create table puzzle_groups (
  id text primary key,
  puzzle_id text not null references puzzles(id) on delete cascade,
  name text not null,
  explanation text not null,
  color_index integer not null,
  sort_order integer not null,
  unique (puzzle_id, color_index),
  unique (puzzle_id, sort_order)
);

create table puzzle_tiles (
  id text primary key,
  puzzle_id text not null references puzzles(id) on delete cascade,
  group_id text not null references puzzle_groups(id) on delete cascade,
  text text not null,
  sort_order integer not null,
  unique (puzzle_id, text)
);

create table attempts (
  id text primary key,
  puzzle_id text not null references puzzles(id) on delete cascade,
  user_id text,
  session_id text,
  mistakes integer not null default 0,
  completed boolean not null default false,
  failed boolean not null default false,
  started_at timestamptz not null default now(),
  completed_at timestamptz,
  unique (puzzle_id, user_id),
  unique (puzzle_id, session_id)
);

create table attempt_guesses (
  id text primary key,
  attempt_id text not null references attempts(id) on delete cascade,
  client_guess_id text not null,
  selected_tile_ids text[] not null,
  is_correct boolean not null,
  matched_group_id text,
  created_at timestamptz not null default now(),
  unique (attempt_id, client_guess_id)
);

