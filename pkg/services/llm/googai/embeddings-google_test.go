package googai_test

import (
	"context"
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

	tests := []struct {
		name    string
		text    string
		model   llm.ServiceName
		wantErr bool
	}{
		{
			name:    "Basic embedding",
			text:    "Hello, world!",
			model:   llm.ModelTextEmbedding004,
			wantErr: false,
		},
		{
			name:    "Empty text",
			text:    "",
			model:   llm.ModelTextEmbedding004,
			wantErr: true,
		},
		{
			name:    "Long text",
			text:    "This is a longer piece of text that we want to generate embeddings for. It contains multiple sentences and should still work fine with the embedding model.",
			model:   llm.ModelTextEmbedding004,
			wantErr: false,
		},
		{
			name:    "Invalid model",
			text:    "Hello, world!",
			model:   "invalid-model",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			embedding, err := client.GenerateEmbedding(ctx, tt.text, tt.model)
			if (err != nil) != tt.wantErr {
				t.Errorf("GenerateEmbedding() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(embedding) == 0 {
					t.Error("Expected non-empty embedding, got empty embedding")
				}
				// Log the dimensionality of the embedding
				t.Logf("Embedding dimensionality: %d", len(embedding))
			}
		})
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
			embedding, err := client.GenerateEmbedding(ctx, "Hello, world!", llm.ModelTextEmbedding004)
			if err != nil {
				errCh <- err
				return
			}
			if len(embedding) == 0 {
				errCh <- err
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
