package firestoremem

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"sync"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/pkg/services/filestorage"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"golang.org/x/sync/errgroup"
)

const (
	ColumnEmbeddingPrompt5   = "embedding_prompt_5"
	ColumnEmbeddingResponse5 = "embedding_response_5"
)

type Client struct {
	firestore *firestore.Client
	fsClient  *filestorage.Client
}

func NewClient(firestore *firestore.Client, fsClient *filestorage.Client) *Client {
	return &Client{firestore: firestore, fsClient: fsClient}
}

type CreatePromptRequest struct {
	DeviceID string `firestore:"device_id"`
	// llm.ModelTextEmbedding005
	EmbeddingPrompt5 firestore.Vector32 `firestore:"embedding_prompt_5"`
	Prompt           string             `firestore:"prompt"`
	Timestamp        time.Time          `firestore:"timestamp,serverTimestamp"`
}

func (cl *Client) conversationsRef(userID string) *firestore.CollectionRef {
	return cl.firestore.Collection("memory").Doc(userID).Collection("conversations")
}

func (cl *Client) CreatePrompt(ctx context.Context, userID string, req *CreatePromptRequest) (string, error) {
	pair := cl.conversationsRef(userID).NewDoc()
	_, err := pair.Create(ctx, req)
	if err != nil {
		return "", err
	}
	return pair.ID, nil
}

type SaveResponseRequest struct {
	UserID, PairID, Response string
	// llm.ModelTextEmbedding005
	EmbeddingResponse5 firestore.Vector32
}

func (cl *Client) SaveResponse(ctx context.Context, req *SaveResponseRequest) error {
	pair := cl.conversationsRef(req.UserID).Doc(req.PairID)
	_, err := pair.Update(ctx, []firestore.Update{
		{Path: "response", Value: req.Response},
		{Path: ColumnEmbeddingResponse5, Value: req.EmbeddingResponse5},
	})
	if err != nil {
		return err
	}
	return nil
}

func (cl *Client) GetEmbeddingPrompt(ctx context.Context, userID, pairID string) (firestore.Vector32, error) {
	pair := cl.conversationsRef(userID).Doc(pairID)
	doc, err := pair.Get(ctx)
	if err != nil {
		return nil, err
	}
	var embedding struct {
		EmbeddingPrompt5 firestore.Vector32 `firestore:"embedding_prompt_5"`
	}
	if err := doc.DataTo(&embedding); err != nil {
		return nil, err
	}
	return embedding.EmbeddingPrompt5, nil
}

func (cl *Client) GetMemoriesV2(ctx context.Context, req *rq.GetMemoriesV2) (rsp rp.MemoriesV2, err error) {
	rsp.ShortTerm, err = cl.getShort(ctx, req)
	if err != nil {
		err = fmt.Errorf("failed to get short term memories: %w", err)
		return
	}
	rsp.LongTerm, err = cl.getLong(ctx, req, rsp.ShortTerm)
	if err != nil {
		err = fmt.Errorf("failed to get long term memories: %w", err)
		return
	}
	return
}

func (cl *Client) getShort(ctx context.Context, req *rq.GetMemoriesV2) ([]rp.Memory, error) {
	if req.ShortTerm == nil || req.ShortTerm.K == 0 {
		return nil, nil
	}
	memoriesRef := cl.conversationsRef(req.UserID)
	query := memoriesRef.OrderBy("timestamp", firestore.Desc).Limit(req.ShortTerm.K)
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to query short term memories: %w", err)
	}
	if len(docs) == 0 {
		return nil, nil
	}
	memories := make([]rp.Memory, 0, len(docs))
	for _, doc := range docs {
		var mem rp.Memory
		if err := doc.DataTo(&mem); err != nil {
			return nil, fmt.Errorf("failed to unmarshal memory: %s, err: %w", doc.Ref.ID, err)
		}
		mem.ID = doc.Ref.ID
		mem.FormatResponse()
		if req.StripImages {
			mem.StripImages()
		} else {
			err := mem.PresignImages(ctx, req.UserID, cl.fsClient)
			if err != nil {
				return nil, fmt.Errorf("failed to presign images: %w", err)
			}
		}
		memories = append(memories, mem)
	}
	return memories, nil
}

func defaultNodeThresholds(nc int) []float64 {
	thresholds := make([]float64, nc)
	for i := range thresholds {
		if i == 0 {
			thresholds[i] = 0.3
		} else {
			thresholds[i] = 0.1
		}
	}
	return thresholds
}

// normalizeVector normalizes a vector to have unit length
func normalizeVector(vector firestore.Vector32) firestore.Vector32 {
	if len(vector) == 0 {
		return vector
	}

	var magnitude float64
	for _, val := range vector {
		magnitude += float64(val * val)
	}
	magnitude = math.Sqrt(magnitude)

	if magnitude == 0 {
		return vector
	}

	normalized := make(firestore.Vector32, len(vector))
	for i, val := range vector {
		normalized[i] = float32(float64(val) / magnitude)
	}

	return normalized
}

// combineVectors adds multiple vectors together and normalizes the result
func combineVectors(vectors []firestore.Vector32) firestore.Vector32 {
	if len(vectors) == 0 {
		return nil
	}

	vectorLen := len(vectors[0])
	if vectorLen == 0 {
		return nil
	}

	combined := make(firestore.Vector32, vectorLen)
	for _, vec := range vectors {
		if len(vec) != vectorLen {
			slog.Warn("vector length mismatch in combineVectors", "expected", vectorLen, "got", len(vec))
			continue
		}
		for i, val := range vec {
			combined[i] += val
		}
	}

	return normalizeVector(combined)
}

func (cl *Client) getLong(ctx context.Context, req *rq.GetMemoriesV2, shortTermMemories []rp.Memory) ([]rp.Memory, error) {
	if req.LongTerm == nil {
		return nil, nil
	}
	if nc, nt := len(req.LongTerm.NodeCounts), len(req.LongTerm.NodeThresholds); nc == 0 {
		return nil, errors.New("no node counts provided")
	} else if nt == 0 {
		req.LongTerm.NodeThresholds = defaultNodeThresholds(nc)
	} else if nc != nt {
		return nil, fmt.Errorf("node thresholds: %v and node counts: %v must be the same length", req.LongTerm.NodeThresholds, req.LongTerm.NodeCounts)
	}

	var err error
	var targetEmbedding firestore.Vector32
	var combinedEmbedding firestore.Vector32
	var useCombinedVector bool

	if req.LongTerm.PairID != "" {
		targetEmbedding, err = cl.GetEmbeddingPrompt(ctx, req.UserID, req.LongTerm.PairID)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding by pairID: %w", err)
		}
		if len(targetEmbedding) == 0 {
			return nil, fmt.Errorf("embedding is empty")
		}
		req.LongTerm.Vector = targetEmbedding
	} else if len(req.LongTerm.Vector) > 0 {
		// Use provided vector directly
		targetEmbedding = req.LongTerm.Vector
	} else {
		return nil, errors.New("neither pairID nor vector provided")
	}

	// Create combined vector from short term memories + target memory
	if len(shortTermMemories) > 0 {
		// Collect vectors from short term memories
		vectors := make([]firestore.Vector32, 0, len(shortTermMemories)+1)
		vectors = append(vectors, targetEmbedding) // Add target vector first

		for _, mem := range shortTermMemories {
			if len(mem.EmbeddingPrompt5) > 0 {
				vectors = append(vectors, mem.EmbeddingPrompt5)
			}
			if len(mem.EmbeddingResponse5) > 0 {
				vectors = append(vectors, mem.EmbeddingResponse5)
			}
		}

		// Combine and normalize
		combinedEmbedding = combineVectors(vectors)
		if len(combinedEmbedding) > 0 {
			useCombinedVector = true
			slog.Debug("created combined vector from memories",
				"vectorCount", len(vectors),
				"shortTermCount", len(shortTermMemories))
		}
	}
	// Keep track of seen memory IDs to avoid duplicates
	memoryCount := 0
	if req.ShortTerm != nil {
		memoryCount = req.ShortTerm.K
	}
	for _, nc := range req.LongTerm.NodeCounts {
		memoryCount += nc
	}
	seenMemories := make(map[string]struct{}, memoryCount)
	var mutex sync.RWMutex
	rootMemories := make([]rp.Memory, 0, memoryCount)
	if len(shortTermMemories) > 0 {
		for _, mem := range shortTermMemories {
			seenMemories[mem.ID] = struct{}{}
		}
	}

	// First search with target vector
	slog.Debug("performing vector search with target vector", "userID", req.UserID)
	memoriesRef := cl.conversationsRef(req.UserID)
	vectorQuery := memoriesRef.FindNearest("embedding_prompt_5",
		targetEmbedding,
		req.LongTerm.NodeCounts[0],
		firestore.DistanceMeasureDotProduct,
		&firestore.FindNearestOptions{
			DistanceResultField: "vector_distance",
			DistanceThreshold:   firestore.Ptr(req.LongTerm.NodeThresholds[0]),
		})
	querySnapshot, err := vectorQuery.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to query long term memories with target vector: %w", err)
	}

	for _, doc := range querySnapshot {
		var mem rp.Memory
		if err := doc.DataTo(&mem); err != nil {
			return nil, fmt.Errorf("failed to unmarshal memory: %s, err: %w", doc.Ref.ID, err)
		}

		mutex.RLock()
		_, seen := seenMemories[doc.Ref.ID]
		mutex.RUnlock()
		if seen {
			slog.Debug("skipping duplicate memory from target vector search", "id", doc.Ref.ID)
			continue
		}

		mem.ID = doc.Ref.ID
		mem.Depth = 0
		mem.FormatResponse()
		slog.Debug("adding root memory from target vector", "id", mem.ID, "distance", mem.VectorDistance)
		if req.StripImages {
			mem.StripImages()
		} else {
			if err := mem.PresignImages(ctx, req.UserID, cl.fsClient); err != nil {
				return nil, fmt.Errorf("failed to presign images: %w", err)
			}
		}
		mutex.Lock()
		seenMemories[mem.ID] = struct{}{}
		mutex.Unlock()
		rootMemories = append(rootMemories, mem)
	}

	// Second search with combined vector if available
	if useCombinedVector {
		slog.Debug("performing vector search with combined vector", "userID", req.UserID)
		combinedQuery := memoriesRef.FindNearest("embedding_prompt_5",
			combinedEmbedding,
			req.LongTerm.NodeCounts[0],
			firestore.DistanceMeasureDotProduct,
			&firestore.FindNearestOptions{
				DistanceResultField: "vector_distance",
				DistanceThreshold:   firestore.Ptr(req.LongTerm.NodeThresholds[0]),
			})
		combQuerySnapshot, err := combinedQuery.Documents(ctx).GetAll()
		if err != nil {
			slog.Warn("failed to query with combined vector, continuing with initial results", "error", err)
		} else {
			// Process results from combined vector
			for _, doc := range combQuerySnapshot {
				mutex.RLock()
				_, seen := seenMemories[doc.Ref.ID]
				mutex.RUnlock()
				if seen {
					slog.Debug("skipping duplicate memory from combined vector search", "id", doc.Ref.ID)
					continue
				}

				var mem rp.Memory
				if err := doc.DataTo(&mem); err != nil {
					return nil, fmt.Errorf("failed to unmarshal memory from combined vector: %s, err: %w", doc.Ref.ID, err)
				}

				mem.ID = doc.Ref.ID
				mem.Depth = 0
				mem.FormatResponse()
				slog.Debug("adding root memory from combined vector", "id", mem.ID, "distance", mem.VectorDistance)
				if req.StripImages {
					mem.StripImages()
				} else {
					if err := mem.PresignImages(ctx, req.UserID, cl.fsClient); err != nil {
						return nil, fmt.Errorf("failed to presign images: %w", err)
					}
				}
				mutex.Lock()
				seenMemories[mem.ID] = struct{}{}
				mutex.Unlock()
				rootMemories = append(rootMemories, mem)
			}
		}
	}

	slog.Debug("found root memories", "count", len(rootMemories))

	// If only one level requested, return early
	if len(req.LongTerm.NodeCounts) == 1 {
		return rootMemories, nil
	}

	// Helper function to recursively find children for a memory at a given depth
	var findChildren func(ctx context.Context, parent *rp.Memory, depth int) error
	findChildren = func(ctx context.Context, parent *rp.Memory, depth int) error {
		if depth >= len(req.LongTerm.NodeCounts) {
			return nil
		}
		var embedding firestore.Vector32
		if len(parent.EmbeddingResponse5) == 0 {
			embedding = parent.EmbeddingPrompt5
		} else {
			embedding = parent.EmbeddingResponse5
		}
		if len(embedding) == 0 {
			slog.Debug("no valid embedding found for parent memory", "id", parent.ID, "depth", depth)
			return nil
		}

		nodeCount := req.LongTerm.NodeCounts[depth]
		// Request more memories than needed since we might skip some duplicates
		adjustedNodeCount := nodeCount * 2
		vectorQuery := memoriesRef.FindNearest("embedding_response_5",
			embedding,
			adjustedNodeCount,
			firestore.DistanceMeasureDotProduct,
			&firestore.FindNearestOptions{
				DistanceResultField: "vector_distance",
				DistanceThreshold:   firestore.Ptr(req.LongTerm.NodeThresholds[depth]),
			})

		querySnapshot, err := vectorQuery.Documents(ctx).GetAll()
		if err != nil {
			return fmt.Errorf("failed to query related memories at depth %d: %w", depth, err)
		}

		children := make([]rp.Memory, 0, nodeCount)
		for _, doc := range querySnapshot {
			mutex.RLock()
			_, seen := seenMemories[doc.Ref.ID]
			mutex.RUnlock()
			if seen {
				slog.Debug("skipping duplicate memory", "id", doc.Ref.ID, "depth", depth)
				continue
			}
			if len(children) >= nodeCount {
				break
			}

			var child rp.Memory
			if err := doc.DataTo(&child); err != nil {
				return fmt.Errorf("failed to unmarshal memory at depth %d: %s, err: %w", depth, doc.Ref.ID, err)
			}
			child.ID = doc.Ref.ID
			child.Depth = depth
			child.FormatResponse()
			slog.Debug("adding child memory", "id", child.ID, "depth", depth, "distance", child.VectorDistance)
			if req.StripImages {
				child.StripImages()
			} else {
				if err := child.PresignImages(ctx, req.UserID, cl.fsClient); err != nil {
					return fmt.Errorf("failed to presign images at depth %d: %w", depth, err)
				}
			}
			mutex.Lock()
			seenMemories[child.ID] = struct{}{}
			mutex.Unlock()
			children = append(children, child)
		}
		parent.Children = children

		// Recursively find children for each child
		g, ctx := errgroup.WithContext(ctx)
		for i := range parent.Children {
			g.Go(func() error {
				if err := findChildren(ctx, &parent.Children[i], depth+1); err != nil {
					return err
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			return err
		}
		return nil
	}

	// Find children for each root memory
	g, ctx := errgroup.WithContext(ctx)
	for i := range rootMemories {
		g.Go(func() error {
			if err := findChildren(ctx, &rootMemories[i], 1); err != nil {
				return err
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}
	slog.Debug("completed memory tree construction", "rootCount", len(rootMemories))
	return rootMemories, nil
}
