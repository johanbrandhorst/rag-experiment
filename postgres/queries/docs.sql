-- name: CreateDoc :exec
INSERT INTO docs (content, content_md5, embedding) VALUES ($1, $2, $3);

-- name: HasDoc :one
SELECT EXISTS(SELECT 1 FROM docs WHERE content_md5 = $1);

-- name: FindTop3DocsByEmbedding :many
SELECT content FROM docs ORDER BY (1 - (embedding <=> $1::vector)) LIMIT 3;
