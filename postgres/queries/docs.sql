-- name: CreateDocs :exec
INSERT INTO docs (content, content_md5, embedding) VALUES ($1, $2, $3);

-- name: HasDoc :one
SELECT EXISTS(SELECT 1 FROM docs WHERE content_md5 = $1);

-- name: FindTop5DocssByEmbedding :many
SELECT content FROM docs where length(content) > 500 ORDER BY (1 - (embedding <=> $1::vector)) LIMIT 5;
