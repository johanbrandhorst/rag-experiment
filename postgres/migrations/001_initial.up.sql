BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE docs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content TEXT NOT NULL,
    content_md5 BYTEA NOT NULL UNIQUE,
    embedding vector NOT NULL
);

COMMIT;
