package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"

	"github.com/johanbrandhorst/rag-experiment/postgres"
	"github.com/pgvector/pgvector-go"
	"github.com/tmc/langchaingo/llms"
	"github.com/tmc/langchaingo/llms/ollama"
)

var (
	databaseUrl = flag.String(
		"db-url",
		"postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable",
		"URL of the Postgres database to use",
	)
	query = flag.String(
		"query",
		"",
		"What do you want to know about?",
	)
)

func main() {
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := run(ctx, *databaseUrl, *query); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, databaseUrl string, query string) error {
	if query == "" {
		return fmt.Errorf("query must be set")
	}

	db, err := postgres.NewStore(ctx, databaseUrl)
	if err != nil {
		return fmt.Errorf("failed to create store: %w", err)
	}
	llm, err := ollama.New(ollama.WithModel("mistral"))
	if err != nil {
		return fmt.Errorf("failed to create LLM: %w", err)
	}
	embeddings, err := llm.CreateEmbedding(ctx, []string{query})
	if err != nil {
		return fmt.Errorf("failed to create embedding: %w", err)
	}
	docs, err := db.FindTop3DocsByEmbedding(ctx, pgvector.NewVector(embeddings[0]))
	if err != nil {
		return fmt.Errorf("failed to find top 3 docs: %w", err)
	}
	var contextInfo string
	for _, doc := range docs {
		contextInfo += string(doc) + "\n"
	}
	fmt.Println(contextInfo)
	llmQuery := fmt.Sprintf(`Use the below information to answer the subsequent question, as it relates to the HashiCorp Boundary product. Give examples and be as helpful as possible.
Information:
%s

Question: %v`, contextInfo, query)
	_, err = llm.Call(ctx, llmQuery,
		llms.WithStreamingFunc(func(ctx context.Context, chunk []byte) error {
			fmt.Print(string(chunk))
			return nil
		}))
	if err != nil {
		return fmt.Errorf("failed to call LLM: %w", err)
	}
	fmt.Println()
	return nil
}
