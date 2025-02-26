package googai_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/googai"
)

func TestMain(m *testing.M) {
	envs.Load()
	os.Exit(m.Run())
}

func TestGenerateEmbedding(t *testing.T) {
	ctx := context.Background()

	// Create a new client
	client, err := googai.NewClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	models := []llm.ServiceName{
		llm.ModelTextEmbedding004,
		llm.ModelTextEmbedding005,
	}
	tests := []struct {
		name    string
		text    string
		models  []llm.ServiceName
		wantErr bool
	}{
		{
			name:    "Basic embedding",
			text:    "Hello, world!",
			models:  models,
			wantErr: false,
		},
		{
			name:    "Empty text",
			text:    "",
			models:  models,
			wantErr: true,
		},
		{
			name:    "Long text",
			text:    "This is a longer piece of text that we want to generate embeddings for. It contains multiple sentences and should still work fine with the embedding model.",
			models:  models,
			wantErr: false,
		},
		{
			name:    "Invalid model",
			text:    "Hello, world!",
			models:  []llm.ServiceName{"invalid-model"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		for _, model := range tt.models {
			t.Run(fmt.Sprintf("%s/%s", tt.name, model), func(t *testing.T) {
				embedding, count, err := client.EmbedSingle(ctx, tt.text, model)
				if err != nil {
					if tt.wantErr {
						t.Logf("GenerateEmbedding() expected error: %v", err)
						return
					}
					t.Errorf("GenerateEmbedding() error = %v, wantErr %v", err, tt.wantErr)
					return
				}
				if len(embedding) == 0 {
					t.Error("Expected non-empty embedding, got empty embedding")
					return
				}
				if count == 0 {
					t.Error("Expected non-zero billable characters, got zero")
					return
				}
				t.Logf("Embedding dimensionality: %d", len(embedding))
			})
		}
	}
}

func TestClientConcurrency(t *testing.T) {
	ctx := context.Background()

	// Create a new client
	client, err := googai.NewClient(ctx)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test concurrent embedding generation
	const numConcurrent = 5
	errCh := make(chan error, numConcurrent)

	for i := 0; i < numConcurrent; i++ {
		go func() {
			embedding, count, err := client.EmbedSingle(ctx, "Hello, world!", llm.ModelTextEmbedding004)
			if err != nil {
				errCh <- err
				return
			}
			if len(embedding) == 0 {
				errCh <- errors.New("empty embedding")
				return
			}
			if count == 0 {
				errCh <- errors.New("zero billable characters")
				return
			}
			errCh <- nil
		}()
	}

	// Collect results
	for i := 0; i < numConcurrent; i++ {
		if err := <-errCh; err != nil {
			t.Errorf("Concurrent embedding generation failed: %v", err)
		}
	}
}
