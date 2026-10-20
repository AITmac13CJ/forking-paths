CREATE TABLE IF NOT EXISTS paths (
    id SERIAL PRIMARY KEY,
    slug TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    medium_label TEXT NOT NULL,
    via_label TEXT NOT NULL,
    release_label TEXT NOT NULL,
    spend_label TEXT NOT NULL,
    finish_time_label TEXT NOT NULL,
    finish_date_label TEXT NOT NULL,
    done_word TEXT NOT NULL,
    doing_title TEXT NOT NULL,
    status_want TEXT NOT NULL,
    status_doing TEXT NOT NULL,
    status_done TEXT NOT NULL,
    status_hold TEXT NOT NULL,
    badges TEXT[] NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS media (
    id SERIAL PRIMARY KEY,
    path_id INT NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT NOT NULL,
    sort_order INT NOT NULL DEFAULT 0,
    UNIQUE (path_id, name)
);

CREATE TABLE IF NOT EXISTS vias (
    id SERIAL PRIMARY KEY,
    path_id INT NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    UNIQUE (path_id, name)
);

CREATE TABLE IF NOT EXISTS tags (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL UNIQUE
);

CREATE TABLE IF NOT EXISTS entries (
    id SERIAL PRIMARY KEY,
    path_id INT NOT NULL REFERENCES paths(id) ON DELETE CASCADE,
    title TEXT NOT NULL,
    medium_id INT REFERENCES media(id) ON DELETE SET NULL,
    via_id INT REFERENCES vias(id) ON DELETE SET NULL,
    release_date DATE,
    status_kind TEXT NOT NULL DEFAULT 'want' CHECK (status_kind IN ('want', 'doing', 'done', 'hold')),
    spend_minutes INT NOT NULL DEFAULT 0 CHECK (spend_minutes >= 0),
    finish_minutes INT CHECK (finish_minutes IS NULL OR finish_minutes >= 0),
    finish_date DATE,
    rating DOUBLE PRECISION CHECK (rating IS NULL OR (rating >= 0 AND rating <= 5)),
    badges TEXT[] NOT NULL DEFAULT '{}',
    cover JSONB NOT NULL DEFAULT '{}'::jsonb,
    cover_url TEXT NOT NULL DEFAULT '',
    cover_landscape_url TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS entry_tags (
    entry_id INT NOT NULL REFERENCES entries(id) ON DELETE CASCADE,
    tag_id INT NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    PRIMARY KEY (entry_id, tag_id)
);

CREATE INDEX IF NOT EXISTS entries_finish_date_idx ON entries (finish_date DESC);
CREATE INDEX IF NOT EXISTS entries_status_idx ON entries (status_kind);
CREATE INDEX IF NOT EXISTS entries_path_idx ON entries (path_id);
CREATE INDEX IF NOT EXISTS entries_medium_idx ON entries (medium_id);

ALTER TABLE entries ADD COLUMN IF NOT EXISTS progress_earned INT;
ALTER TABLE entries ADD COLUMN IF NOT EXISTS progress_total INT;

ALTER TABLE entries ADD COLUMN IF NOT EXISTS cover_landscape_url TEXT NOT NULL DEFAULT '';
ALTER TABLE entries ADD COLUMN IF NOT EXISTS platform_ref TEXT NOT NULL DEFAULT '';
CREATE INDEX IF NOT EXISTS entries_platform_ref_idx ON entries (platform_ref);

CREATE TABLE IF NOT EXISTS platform_accounts (
    provider TEXT PRIMARY KEY,
    account TEXT NOT NULL DEFAULT '',
    secret TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS users (
    id SERIAL PRIMARY KEY,
    username TEXT NOT NULL,
    username_key TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    recovery_hash TEXT NOT NULL,
    public_garden BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

ALTER TABLE users ADD COLUMN IF NOT EXISTS public_garden BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS sessions (
    token_hash TEXT PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS sessions_user_idx ON sessions (user_id);

ALTER TABLE entries ADD COLUMN IF NOT EXISTS user_id INT REFERENCES users(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS entries_user_idx ON entries (user_id);

ALTER TABLE platform_accounts ADD COLUMN IF NOT EXISTS user_id INT REFERENCES users(id) ON DELETE CASCADE;

CREATE TABLE IF NOT EXISTS sync_jobs (
    id SERIAL PRIMARY KEY,
    user_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind TEXT NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('running', 'done', 'error')),
    message TEXT NOT NULL DEFAULT '',
    done_count INT NOT NULL DEFAULT 0,
    total_count INT NOT NULL DEFAULT 0,
    updated_count INT NOT NULL DEFAULT 0,
    created_count INT NOT NULL DEFAULT 0,
    acked BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    finished_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS sync_jobs_user_idx ON sync_jobs (user_id, acked, id DESC);
CREATE UNIQUE INDEX IF NOT EXISTS sync_jobs_running_uidx ON sync_jobs (user_id, kind) WHERE status = 'running';
