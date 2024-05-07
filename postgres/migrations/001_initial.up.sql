BEGIN;

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE docs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    path TEXT NOT NULL UNIQUE,
    content BYTEA NOT NULL,
    embedding vector(768) NOT NULL
);

COMMIT;
