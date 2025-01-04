package firestoremem

import (
	"context"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/types/rq"
)

type Client struct {
	firestore *firestore.Client
}

func NewClient(firestore *firestore.Client) *Client {
	return &Client{firestore: firestore}
}

func (cl *Client) GetShort(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func (cl *Client) GetLong(ctx context.Context, req *rq.ParamsLongTermMemoriesV2) ([]string, error) {
	if req == nil {
		return nil, nil
	}
	return nil, nil
}

type Message struct {
	Prompt         string    `firestore:"prompt"`
	Response       string    `firestore:"response"`
	Timestamp      time.Time `firestore:"timestamp"`
	VectorDistance float32   `firestore:"vector_distance"`
	// Used for depth search
	EmbeddingVector []float32 `firestore:"embedding_vector"`
}
