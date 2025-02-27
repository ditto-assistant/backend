package firestoremem

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"golang.org/x/sync/errgroup"
)

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
		if req.LongTerm != nil && req.LongTerm.PairID == doc.Ref.ID {
			continue // skip the base memory
		}
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
	var baseEmbedding, combinedEmbedding firestore.Vector32
	if req.LongTerm.PairID != "" {
		baseEmbedding, err = cl.GetEmbeddingPrompt(ctx, req.UserID, req.LongTerm.PairID)
		if err != nil {
			return nil, fmt.Errorf("failed to get embedding by pairID: %w", err)
		}
		if len(baseEmbedding) == 0 {
			return nil, fmt.Errorf("embedding is empty")
		}
		req.LongTerm.Vector = baseEmbedding
	} else if len(req.LongTerm.Vector) > 0 {
		baseEmbedding = req.LongTerm.Vector
	} else {
		return nil, errors.New("neither pairID nor vector provided")
	}
	shouldSearchShortTermMemories := !req.LongTerm.SkipShortTermContext && len(shortTermMemories) > 0
	if shouldSearchShortTermMemories {
		combinedEmbedding = combineEmbeddings(shortTermMemories)
	}

	memoriesRequested := req.TotalRequestedMemories()
	seenMemories := make(map[string]struct{}, memoriesRequested)
	rootMemories := make([]rp.Memory, 0, memoriesRequested)
	for _, mem := range shortTermMemories {
		seenMemories[mem.ID] = struct{}{}
	}
	if req.LongTerm.PairID != "" {
		seenMemories[req.LongTerm.PairID] = struct{}{}
	}
	var mutex sync.RWMutex
	memoriesRef := cl.conversationsRef(req.UserID)
	{
		g, ctx := errgroup.WithContext(ctx)
		g.Go(func() error {
			slog.Debug("performing vector search with target vector", "userID", req.UserID)
			vectorQuery := memoriesRef.FindNearest("embedding_prompt_5",
				baseEmbedding,
				req.LongTerm.NodeCounts[0],
				firestore.DistanceMeasureDotProduct,
				&firestore.FindNearestOptions{
					DistanceResultField: "vector_distance",
					DistanceThreshold:   firestore.Ptr(req.LongTerm.NodeThresholds[0]),
				})
			querySnapshot, err := vectorQuery.Documents(ctx).GetAll()
			if err != nil {
				return fmt.Errorf("failed to query long term memories with target vector: %w", err)
			}
			for _, doc := range querySnapshot {
				var mem rp.Memory
				if err := doc.DataTo(&mem); err != nil {
					return fmt.Errorf("failed to unmarshal memory: %s, err: %w", doc.Ref.ID, err)
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
				} else if err := mem.PresignImages(ctx, req.UserID, cl.fsClient); err != nil {
					return fmt.Errorf("failed to presign images: %w", err)
				}
				mutex.Lock()
				seenMemories[mem.ID] = struct{}{}
				rootMemories = append(rootMemories, mem)
				mutex.Unlock()
			}
			return nil
		})

		if len(combinedEmbedding) > 0 {
			g.Go(func() error {
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
							return fmt.Errorf("failed to unmarshal memory from combined vector: %s, err: %w", doc.Ref.ID, err)
						}
						mem.ID = doc.Ref.ID
						mem.Depth = 0
						mem.FormatResponse()
						slog.Debug("adding root memory from combined vector", "id", mem.ID, "distance", mem.VectorDistance)
						if req.StripImages {
							mem.StripImages()
						} else if err := mem.PresignImages(ctx, req.UserID, cl.fsClient); err != nil {
							return fmt.Errorf("failed to presign images: %w", err)
						}
						mutex.Lock()
						seenMemories[mem.ID] = struct{}{}
						rootMemories = append(rootMemories, mem)
						mutex.Unlock()
					}
				}
				return nil
			})
		}
		err := g.Wait()
		if err != nil {
			return nil, err
		}
		slog.Debug("found root memories", "count", len(rootMemories))
		if len(req.LongTerm.NodeCounts) == 1 {
			return rootMemories, nil
		}
	}

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

func combineEmbeddings(shortTermMemories []rp.Memory) firestore.Vector32 {
	vectors := make([]firestore.Vector32, 0, len(shortTermMemories)+1)
	for _, mem := range shortTermMemories {
		if len(mem.EmbeddingPrompt5) > 0 {
			vectors = append(vectors, mem.EmbeddingPrompt5)
		}
		if len(mem.EmbeddingResponse5) > 0 {
			vectors = append(vectors, mem.EmbeddingResponse5)
		}
	}
	return combineVectors(vectors)
}
