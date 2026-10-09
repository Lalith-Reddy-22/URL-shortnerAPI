-- gen_random_uuid() lives in pgcrypto on Postgres 12; Postgres 13+ has it in core.
-- CREATE EXTENSION is harmless if the function already exists.
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE users (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email         TEXT        NOT NULL,
    password_hash TEXT        NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT users_email_key UNIQUE (email)
);

CREATE TABLE links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id         UUID        NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- Public identifier: either a 7-char base62 code or a custom alias.
    code            TEXT        NOT NULL,
    original_url    TEXT        NOT NULL,
    expires_at      TIMESTAMPTZ,
    -- Click counters live on the same row so stats and redirect lookup are one read.
    -- Increments are additive (click_count = click_count + n), so lost-update is avoided.
    click_count     BIGINT      NOT NULL DEFAULT 0,
    last_clicked_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT links_code_key UNIQUE (code),
    CONSTRAINT links_code_len CHECK (char_length(code) BETWEEN 1 AND 64),
    CONSTRAINT links_url_len  CHECK (char_length(original_url) <= 2048)
);

-- Owner listing is "my links, newest first".
CREATE INDEX links_user_id_created_at_idx ON links (user_id, created_at DESC);
