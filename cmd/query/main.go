package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"time"

	"github.com/google/generative-ai-go/genai"
	"github.com/johanbrandhorst/rag-experiment/postgres"
	"github.com/pgvector/pgvector-go"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const promptTemplate = `
You are a knowledgeable support engineer for the HashiCorp Boundary product. You specialize in answering questions that customers have about using the Boundary product using clear and concise answers. Include code examples where appropriate for the question. Included below are some relevant documents from the Boundary product documentation in markdown format.

%s

Using the information above, answer the following question. Do not format the response as a markdown document, but as if answering a customer over a normal text chat interface:

%s
`

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
	client, err := genai.NewClient(ctx, option.WithAPIKey(os.Getenv("API_KEY")))
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()
	embedModel := client.EmbeddingModel("models/text-embedding-004")
	embedModel.TaskType = genai.TaskTypeRetrievalQuery
	embedding, err := embedModel.EmbedContent(ctx, genai.Text(query))
	if err != nil {
		return fmt.Errorf("failed to create embedding: %w", err)
	}
	docs, err := db.FindTopDocsByEmbedding(ctx, pgvector.NewVector(embedding.Embedding.Values))
	if err != nil {
		return fmt.Errorf("failed to find top docs: %w", err)
	}
	var contextInfo string
	for _, doc := range docs {
		fmt.Println("Retrieved doc:", doc.Path)
		contextInfo += string(doc.Content) + "\n"
	}
	fullPrompt := fmt.Sprintf(promptTemplate, contextInfo, query)
	fmt.Println("Sending prompt")
	before := time.Now()
	defer func() {
		fmt.Println("Response complete, time taken:", time.Since(before))
	}()
	genModel := client.GenerativeModel("gemini-1.5-pro-latest")
	iter := genModel.GenerateContentStream(ctx, genai.Text(fullPrompt))
	for {
		resp, err := iter.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			log.Fatal("failed to iterate: ", err)
		}

		for _, candidate := range resp.Candidates {
			for _, part := range candidate.Content.Parts {
				switch p := part.(type) {
				case genai.Blob:
					fmt.Print("Blob", p.MIMEType, p.Data)
				case genai.FunctionResponse:
					fmt.Print("Function", p.Name, p.Response)
				case genai.Text:
					fmt.Print(p)
				}
			}
		}
	}
	return nil
}
