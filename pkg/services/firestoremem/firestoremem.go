package firestoremem

import (
	"context"
	"fmt"

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
	if len(req.LongTerm.NodeCounts) > 1 {
		return nil, fmt.Errorf("AT THIS TIME only one node count is supported for long term memories")
	}
	memoriesRef := cl.firestore.Collection("memory").Doc(req.UserID).Collection("conversations")
	vectorQuery := memoriesRef.FindNearest("embedding_vector",
		req.LongTerm.Vector,
		req.LongTerm.NodeCounts[0],
		firestore.DistanceMeasureCosine,
		&firestore.FindNearestOptions{
			DistanceResultField: "vector_distance",
		})
	querySnapshot, err := vectorQuery.Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to query long term memories: %w", err)
	}
	memories := make([]rp.Memory, 0, len(querySnapshot))
	for _, doc := range querySnapshot {
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
