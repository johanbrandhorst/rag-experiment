package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/md5"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/johanbrandhorst/rag-experiment/postgres"
	"github.com/pgvector/pgvector-go"
	"github.com/tmc/langchaingo/llms/ollama"
)

var (
	databaseUrl = flag.String(
		"db-url",
		"postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable",
		"URL of the Postgres database to use",
	)
	docsPath = flag.String(
		"docs-path",
		"",
		"Path to the docs directory to populate from",
	)
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, *databaseUrl, *docsPath); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, databaseUrl string, docsPath string) error {
	if docsPath == "" {
		return fmt.Errorf("docs-path must be set")
	}

	db, err := postgres.NewStore(ctx, databaseUrl)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	llm, err := ollama.New(ollama.WithModel("mistral"))
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}
	if err := fs.WalkDir(os.DirFS(docsPath), ".", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		if err != nil {
			return fmt.Errorf("failed to walk dir: %w", err)
		}
		content, err := os.ReadFile(filepath.Join(docsPath, path))
		if err != nil {
			return fmt.Errorf("failed to read file %q: %w", path, err)
		}
		if ok, err := db.HasDoc(ctx, content); err != nil {
			return fmt.Errorf("failed to check if doc exists: %w", err)
		} else if ok {
			// Skip creating embedding if it already exists
			slog.Info("Skipping doc", "path", path)
			return nil
		}
		chunks, err := chunkFile(ctx, content)
		if err != nil {
			return fmt.Errorf("failed to chunk file: %w", err)
		}
		embeddings, err := llm.CreateEmbedding(ctx, chunks)
		if err != nil {
			return fmt.Errorf("failed to create embeddings: %w", err)
		}
		if len(embeddings) != len(chunks) {
			return fmt.Errorf("mismatched number of embeddings and chunks")
		}
		for i, chunk := range chunks {
			chunkHash := md5.Sum([]byte(chunk))
			if err := db.CreateDoc(ctx, postgres.CreateDocParams{
				Content:    chunk,
				ContentMd5: chunkHash[:],
				Embedding:  pgvector.NewVector(embeddings[i]),
			}); err != nil {
				return fmt.Errorf("failed to create doc: %w", err)
			}
		}
		slog.Info("Created doc", "path", path)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to walk docs: %w", err)
	}
	slog.Info("Population complete!")
	return nil
}

func chunkFile(ctx context.Context, content []byte) ([]string, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	scanner.Split(splitByParagraph)

	var chunks []string
	for scanner.Scan() {
		chunks = append(chunks, scanner.Text())
	}

	return chunks, nil
}

// splitByParagraph is a custom split function for bufio.Scanner to split by
// paragraphs (text pieces separated by two newlines).
func splitByParagraph(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if i := bytes.Index(data, []byte("\n\n")); i >= 0 {
		return i + 2, bytes.TrimSpace(data[:i]), nil
	}

	if atEOF && len(data) != 0 {
		return len(data), bytes.TrimSpace(data), nil
	}

	return 0, nil, nil
}
