-- name: CreateDocs :exec
INSERT INTO docs (path, content, embedding) VALUES ($1, $2, $3);

-- name: HasDoc :one
SELECT EXISTS(SELECT 1 FROM docs WHERE path = $1);

-- name: FindTopDocsByEmbedding :many
SELECT path, content FROM docs ORDER BY embedding <-> $1::vector LIMIT 10;
