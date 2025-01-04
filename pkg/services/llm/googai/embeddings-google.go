package googai

import (
	"context"
	"fmt"

	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/vertexai"
)

func GenerateEmbedding(ctx context.Context, text string, model llm.ServiceName) (llm.Embedding, error) {
	mod := model.String()
	if !vertexai.IsDefinedEmbedder(mod) {
		return nil, fmt.Errorf("embedFlow: model not found: %s", mod)
	}

	embedder := vertexai.Embedder(mod)
	embeddings, err := embedder.Embed(ctx, &ai.EmbedRequest{
		Documents: []*ai.Document{
			{
				Content: []*ai.Part{
					ai.NewTextPart(text),
				},
			},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate embedding: %w", err)
	}

	if len(embeddings.Embeddings) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	return embeddings.Embeddings[0].Embedding, nil
}
