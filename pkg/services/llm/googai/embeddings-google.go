package googai

import (
	"context"
	"errors"
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
		return nil, fmt.Errorf("aiplatform.NewPredictionClient: %w", err)
	}
	return &Client{
		client:   client,
		location: location,
		project:  project,
	}, nil
}

// EmbedSingle is a convenience method for single document embedding
func (cl *Client) EmbedSingle(ctx context.Context, text string, model llm.ServiceName) (llm.Embedding, int64, error) {
	var rsp EmbedResponse
	err := cl.Embed(ctx, &EmbedRequest{
		Documents: []string{text},
		Model:     model,
	}, &rsp)
	if err != nil {
		return nil, 0, err
	}
	if len(rsp.Embeddings) == 0 {
		return nil, 0, fmt.Errorf("no embeddings returned")
	}
	return rsp.Embeddings[0], rsp.BillableCharacterCount, nil
}

type EmbedResponse struct {
	Embeddings             []llm.Embedding
	BillableCharacterCount int64
}

// Embed generates embeddings for one or more documents in a single request
func (cl *Client) Embed(ctx context.Context, req *EmbedRequest, rsp *EmbedResponse) error {
	if err := req.Validate(); err != nil {
		return err
	}
	endpoint := fmt.Sprintf("projects/%s/locations/%s/publishers/google/models/%s", cl.project, cl.location, req.Model.String())
	instances := make([]*structpb.Value, len(req.Documents))
	for i, doc := range req.Documents {
		if doc == "" {
			return fmt.Errorf("document %d is empty", i)
		}
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
	resp, err := cl.client.Predict(ctx, &aiplatformpb.PredictRequest{
		Endpoint:  endpoint,
		Instances: instances,
	})
	if err != nil {
		return fmt.Errorf("failed to generate embeddings: %w", err)
	}
	if len(resp.Predictions) == 0 {
		return fmt.Errorf("no embeddings returned")
	}
	rsp.BillableCharacterCount = int64(resp.Metadata.GetStructValue().Fields["billableCharacterCount"].GetNumberValue())
	embeddings := make([]llm.Embedding, len(resp.Predictions))
	for i, prediction := range resp.Predictions {
		values := prediction.GetStructValue().Fields["embeddings"].GetStructValue().Fields["values"].GetListValue().Values
		embedding := make(llm.Embedding, len(values))
		for j, value := range values {
			embedding[j] = float32(value.GetNumberValue())
		}
		embeddings[i] = embedding
	}
	rsp.Embeddings = embeddings
	return nil
}

func (req *EmbedRequest) Validate() error {
	if len(req.Documents) == 0 {
		return fmt.Errorf("no documents provided")
	}
	switch req.Model {
	case llm.ModelTextEmbedding004, llm.ModelTextEmbedding005:
		return nil
	case "":
		return errors.New("model is required")
	default:
		return fmt.Errorf("unsupported model: %s", req.Model)
	}
}
