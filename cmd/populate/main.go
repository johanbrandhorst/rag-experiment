package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/google/generative-ai-go/genai"
	"github.com/johanbrandhorst/rag-experiment/postgres"
	"github.com/pgvector/pgvector-go"
	"google.golang.org/api/option"
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

type doc struct {
	path    string
	content []byte
}

func run(ctx context.Context, databaseUrl string, docsPath string) error {
	if docsPath == "" {
		return fmt.Errorf("docs-path must be set")
	}

	db, err := postgres.NewStore(ctx, databaseUrl)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	model := client.EmbeddingModel("models/text-embedding-004")
	model.TaskType = genai.TaskTypeRetrievalDocument
	var docs []doc
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
		if ok, err := db.HasDoc(ctx, path); err != nil {
			return fmt.Errorf("failed to check if doc exists: %w", err)
		} else if ok {
			// Skip creating embedding if it already exists
			slog.Info("Skipping doc", "path", path)
			return nil
		}
		docs = append(docs, doc{path: path, content: content})
		slog.Info("Read doc", "path", path)
		return nil
	}); err != nil {
		return fmt.Errorf("failed to walk docs: %w", err)
	}
	batch := model.NewBatch()
	for i, doc := range docs {
		batch = batch.AddContent(genai.Text(string(doc.content)))
		if (i > 0 && (i+1)%100 == 0) || i == len(docs)-1 {
			embeddings, err := model.BatchEmbedContents(ctx, batch)
			if err != nil {
				return fmt.Errorf("failed to create embeddings: %w", err)
			}
			for j, embedding := range embeddings.Embeddings {
				var k int
				if i == len(docs)-1 {
					k = len(docs) - 1 - i%100 + j
				} else {
					k = i - 99 + j
				}
				doc := docs[k]
				if err := db.CreateDocs(ctx, postgres.CreateDocsParams{
					Path:      doc.path,
					Content:   doc.content,
					Embedding: pgvector.NewVector(embedding.Values),
				}); err != nil {
					return fmt.Errorf("failed to create doc: %w", err)
				}
				slog.Info("Created doc in DB", "path", doc.path)
			}
			slog.Info("Created docs in DB", "count", i+1)
			batch = model.NewBatch()
		}
	}
	slog.Info("Population complete!")
	return nil
}
