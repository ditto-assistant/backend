package firestoremem

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"cloud.google.com/go/firestore"
	"github.com/ditto-assistant/backend/pkg/services/filestorage"
	"github.com/ditto-assistant/backend/types/rp"
	"github.com/ditto-assistant/backend/types/rq"
	"golang.org/x/sync/errgroup"
)

type Client struct {
	firestore *firestore.Client
	fsClient  *filestorage.Client
}

func NewClient(firestore *firestore.Client, fsClient *filestorage.Client) *Client {
	return &Client{firestore: firestore, fsClient: fsClient}
}

func (cl *Client) GetMemoriesV2(ctx context.Context, req *rq.GetMemoriesV2) (rp.MemoriesV2, error) {
	var rsp rp.MemoriesV2
	group, ctx := errgroup.WithContext(ctx)
	group.Go(func() error {
		short, err := cl.getShort(ctx, req)
		if err != nil {
			return err
		}
		rsp.ShortTerm = short
		return nil
	})
	group.Go(func() error {
		long, err := cl.getLong(ctx, req)
		if err != nil {
			return err
		}
		rsp.LongTerm = long
		return nil
	})
	if err := group.Wait(); err != nil {
		return rsp, err
	}
	return rsp, nil
}

func (cl *Client) getShort(ctx context.Context, req *rq.GetMemoriesV2) ([]rp.Memory, error) {
	if req.ShortTerm == nil {
		return nil, nil
	}
	if req.ShortTerm.K == 0 {
		return nil, nil
	}
	memoriesRef := cl.firestore.Collection("memory").Doc(req.UserID).Collection("conversations")
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

func (cl *Client) getLong(ctx context.Context, req *rq.GetMemoriesV2) ([]rp.Memory, error) {
	if req.LongTerm == nil {
		return nil, nil
	}
	if len(req.LongTerm.NodeCounts) == 0 {
		return nil, fmt.Errorf("no node counts provided")
	}

	// Keep track of seen memory IDs to avoid duplicates
	seenMemories := make(map[string]struct{}, req.LongTerm.NodeCounts[0])
	var mutex sync.RWMutex

	// First level search
	memoriesRef := cl.firestore.Collection("memory").Doc(req.UserID).Collection("conversations")
	vectorQuery := memoriesRef.FindNearest("embedding_vector",
		req.LongTerm.Vector,
		req.LongTerm.NodeCounts[0],
		firestore.DistanceMeasureDotProduct,
		&firestore.FindNearestOptions{
			DistanceResultField: "vector_distance",
			DistanceThreshold:   firestore.Ptr(0.75),
		})
	querySnapshot, err := vectorQuery.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to query long term memories: %w", err)
	}

	// Process first level results
	rootMemories := make([]rp.Memory, 0, len(querySnapshot))
	for _, doc := range querySnapshot {
		var mem rp.Memory
		if err := doc.DataTo(&mem); err != nil {
			return nil, fmt.Errorf("failed to unmarshal memory: %s, err: %w", doc.Ref.ID, err)
		}
		mem.ID = doc.Ref.ID
		mem.Depth = 0
		mem.FormatResponse()
		slog.Debug("adding root memory", "id", mem.ID, "distance", mem.VectorDistance)
		if req.StripImages {
			mem.StripImages()
		} else {
			if err := mem.PresignImages(ctx, req.UserID, cl.fsClient); err != nil {
				return nil, fmt.Errorf("failed to presign images: %w", err)
			}
		}
		seenMemories[mem.ID] = struct{}{}
		rootMemories = append(rootMemories, mem)
	}

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

		nodeCount := req.LongTerm.NodeCounts[depth]
		// Request more memories than needed since we might skip some duplicates
		adjustedNodeCount := nodeCount * 2
		vectorQuery := memoriesRef.FindNearest("embedding_vector",
			parent.EmbeddingVector,
			adjustedNodeCount,
			firestore.DistanceMeasureDotProduct,
			&firestore.FindNearestOptions{
				DistanceResultField: "vector_distance",
				DistanceThreshold:   firestore.Ptr(0.3),
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
			// Break if we have enough unique children
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
	return rootMemories, nil
}
