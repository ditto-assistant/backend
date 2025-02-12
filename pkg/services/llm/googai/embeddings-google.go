package googai

import (
	"context"
	"fmt"

	aiplatform "cloud.google.com/go/aiplatform/apiv1"
	"cloud.google.com/go/aiplatform/apiv1/aiplatformpb"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/types/known/structpb"
)

type Client struct {
	client   *aiplatform.PredictionClient
	location string
	project  string
}

type EmbedRequest struct {
	Documents []string
	Model     llm.ServiceName
}

func NewClient(ctx context.Context) (*Client, error) {
	location := "us-central1"
	project := envs.PROJECT_ID
	apiEndpoint := fmt.Sprintf("%s-aiplatform.googleapis.com:443", location)
	client, err := aiplatform.NewPredictionClient(ctx, option.WithEndpoint(apiEndpoint))
	if err != nil {
		return nil, fmt.Errorf("failed to create prediction client: %w", err)
	}
	return &Client{
		client:   client,
		location: location,
		project:  project,
	}, nil
}

// GenerateEmbedding is a convenience method for single document embedding
func (cl *Client) GenerateEmbedding(ctx context.Context, text string, model llm.ServiceName) (llm.Embedding, error) {
	embeddings, err := cl.Embed(ctx, EmbedRequest{
		Documents: []string{text},
		Model:     model,
	})
	if err != nil {
		return nil, err
	}
	return embeddings[0], nil
}

// Embed generates embeddings for one or more documents in a single request
func (cl *Client) Embed(ctx context.Context, req EmbedRequest) ([]llm.Embedding, error) {
	if len(req.Documents) == 0 {
		return nil, fmt.Errorf("no documents provided")
	}

	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s", cl.project, cl.location, req.Model.String())

	// Create instances for each document
	instances := make([]*structpb.Value, len(req.Documents))
	for i, doc := range req.Documents {
		instance := &structpb.Value{
			Kind: &structpb.Value_StructValue{
				StructValue: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"content":   structpb.NewStringValue(doc),
						"task_type": structpb.NewStringValue("SEMANTIC_SIMILARITY"),
					},
				},
			},
		}
		instances[i] = instance
	}

	// Make the prediction request
	resp, err := cl.client.Predict(ctx, &aiplatformpb.PredictRequest{
		Endpoint:  endpoint,
		Instances: instances,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to generate embeddings: %w", err)
	}

	if len(resp.Predictions) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}

	// Extract embeddings from response
	embeddings := make([]llm.Embedding, len(resp.Predictions))
	for i, prediction := range resp.Predictions {
		values := prediction.GetStructValue().Fields["embeddings"].GetStructValue().Fields["values"].GetListValue().Values
		embedding := make(llm.Embedding, len(values))
		for j, value := range values {
			embedding[j] = float32(value.GetNumberValue())
		}
		embeddings[i] = embedding
	}

	return embeddings, nil
}
