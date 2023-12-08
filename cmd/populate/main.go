package main

import (
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
		embeddings, err := llm.CreateEmbedding(ctx, []string{string(content)})
		if err != nil {
			return fmt.Errorf("failed to create embeddings: %w", err)
		}
		contentHash := md5.Sum([]byte(content))
		if err := db.CreateDocs(ctx, postgres.CreateDocsParams{
			Content:    content,
			ContentMd5: contentHash[:],
			Embedding:  pgvector.NewVector(embeddings[0]),
		}); err != nil {
			return fmt.Errorf("failed to create doc: %w", err)
		}
		slog.Info("Created doc", "path", path)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to walk docs: %w", err)
	}
	slog.Info("Population complete!")
	return nil
}
