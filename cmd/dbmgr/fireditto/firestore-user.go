package fireditto

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/googai"
	"github.com/ditto-assistant/backend/types/rp"
)

func (f *Command) printUser(ctx context.Context) error {
	if err := f.initFirebase(ctx); err != nil {
		return err
	}
	if err := f.getUserByEmail(ctx); err != nil {
		return err
	}
	convDocs, err := f.fs.
		Collection("memory").
		Doc(f.UID).
		Collection("conversations").
		OrderBy("timestamp", f.Order()).
		Limit(f.User.Limit).
		Offset(f.User.Offset).
		Documents(ctx).
		GetAll()
	if err != nil {
		return fmt.Errorf("error getting conversations from Firestore: %w", err)
	}
	slog.Info("User conversations", "userID", f.UID, "count", len(convDocs))
	for _, doc := range convDocs {
		var conv struct {
			Prompt    string    `firestore:"prompt"`
			Response  string    `firestore:"response"`
			Timestamp time.Time `firestore:"timestamp"`
		}
		if err := doc.DataTo(&conv); err != nil {
			slog.Error("Error unmarshaling conversation", "error", err, "docID", doc.Ref.ID)
			continue
		}
		var promptLinks []string
		rp.TrimStuff(&conv.Prompt, "![image](", ")", func(s *string) error {
			promptLinks = append(promptLinks, *s)
			return nil
		})
		var respLinks []string
		rp.TrimStuff(&conv.Response, "![DittoImage](", ")", func(s *string) error {
			respLinks = append(respLinks, *s)
			return nil
		})
		if len(conv.Prompt) > 100 {
			conv.Prompt = conv.Prompt[:100] + "..."
		}
		if len(conv.Response) > 100 {
			conv.Response = conv.Response[:100] + "..."
		}
		fmt.Printf("%s\n\033[32mUser: %s\033[0m\n", conv.Timestamp.Format(time.DateTime), conv.Prompt)
		if len(promptLinks) > 0 {
			fmt.Printf("\033[32mUser Images: %v\033[0m\n", promptLinks)
		}
		fmt.Printf("\033[36mDitto: %s\033[0m\n", conv.Response)
		if len(respLinks) > 0 {
			fmt.Printf("\033[36mDitto Images: %v\033[0m\n", respLinks)
		}
		fmt.Println()
	}
	return nil
}

func (f *Command) initFirebase(ctx context.Context) error {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return fmt.Errorf("error creating firebase app: %w", err)
	}
	fs, err := app.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("error getting firestore client: %w", err)
	}
	f.fs = fs
	auth, err := app.Auth(ctx)
	if err != nil {
		return fmt.Errorf("error getting auth client: %w", err)
	}
	f.auth = auth
	return nil
}

func (f *Command) getUserByEmail(ctx context.Context) error {
	if f.Email == "" || f.UID != "" {
		return nil
	}
	user, err := f.auth.GetUserByEmail(ctx, f.Email)
	if err != nil {
		return fmt.Errorf("error getting user by email: %w", err)
	}
	f.UID = user.UID
	slog.Info("User ID", "userID", f.UID)
	return nil
}

func (f *Command) embedMem(ctx context.Context) error {
	if err := f.initFirebase(ctx); err != nil {
		return err
	}
	if err := f.getUserByEmail(ctx); err != nil {
		return err
	}
	if f.UID == "" && !f.Mem.AllUsers {
		return errors.New("user ID is empty")
	}
	slog.Info("embed command", "command", f.Mem.Embed.String())
	model := fmt.Sprintf("text-embedding-00%d", f.Mem.Embed.ModelVersion)
	googaiClient, err := googai.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("error initializing Google AI client: %w", err)
	}
	colRef := f.fs.Collection("memory").Doc(f.UID).Collection("conversations")
	var query firestore.Query
	if !f.Mem.Embed.Start.Time().IsZero() {
		query = colRef.Where("timestamp", ">=", f.Mem.Embed.Start.Time())
	} else {
		query = colRef.Query
	}
	docs, err := query.Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("error getting memory docs: %w", err)
	}
	slog.Info("Found documents to update", "count", len(docs))
	batchSize := 50
	bulkWriter := f.fs.BulkWriter(ctx)
	defer func() {
		bulkWriter.End()
		slog.Info("bulk writer ended")
	}()
	for i := 0; i < len(docs); i += batchSize {
		end := i + batchSize
		if end > len(docs) {
			end = len(docs)
		}
		batch := docs[i:end]
		skipList := make([]bool, len(batch))
		contents := make([]string, 0, len(batch))
		for j, doc := range batch {
			data, err := doc.DataAt(f.Mem.Embed.ContentField)
			if err != nil {
				return fmt.Errorf("error getting data from document: %s: %w", doc.Ref.ID, err)
			}
			str, ok := data.(string)
			if !ok {
				return fmt.Errorf("data: %T is not a string for document: %s", data, doc.Ref.ID)
			}
			switch f.Mem.Embed.ContentField {
			case "prompt":
				rp.TrimStuff(&str, "![image](", ")", nil)
			case "response":
				rp.TrimStuff(&str, "![DittoImage](", ")", nil)
				rp.FormatToolsResponse(&str)
			}
			if str == "" {
				skipList[j] = true
				continue
			}
			contents = append(contents, str)
		}
		if len(contents) == 0 {
			slog.Info("No contents to embed", "batch", i, "end", end)
			continue
		}
		var embedResp googai.EmbedResponse
		err = googaiClient.Embed(ctx, &googai.EmbedRequest{
			Documents: contents,
			Model:     llm.ServiceName(model),
		}, &embedResp)
		if err != nil {
			return fmt.Errorf("error embedding batch starting at %d: %w", i, err)
		}
		responseIndex := 0
		for j, doc := range batch {
			if skipList[j] {
				continue
			}
			_, err := bulkWriter.Update(doc.Ref, []firestore.Update{
				{Path: f.Mem.Embed.EmbedField, Value: firestore.Vector32(embedResp.Embeddings[responseIndex])},
			})
			if err != nil {
				return fmt.Errorf("error updating document %s: %w", doc.Ref.ID, err)
			}
			responseIndex++
		}
		slog.Info("Processed batch", "start", i, "end", end, "total", len(docs))
	}
	return nil
}

func (f *Command) deleteColumn(ctx context.Context) error {
	if err := f.initFirebase(ctx); err != nil {
		return err
	}
	if err := f.getUserByEmail(ctx); err != nil {
		return err
	}
	if f.UID == "" && !f.Mem.AllUsers {
		return errors.New("user ID is empty")
	}
	slog.Info("Starting column deletion", "column", f.Mem.DeleteColumn, "userID", f.UID)
	bulkWriter := f.fs.BulkWriter(ctx)
	defer func() {
		bulkWriter.End()
		slog.Info("Successfully deleted column from all documents", "column", f.Mem.DeleteColumn)
	}()
	docs, err := f.fs.Collection("memory").Doc(f.UID).Collection("conversations").Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("error getting memory docs: %w", err)
	}
	slog.Info("Found documents to update", "count", len(docs))
	for _, doc := range docs {
		_, err := bulkWriter.Update(doc.Ref, []firestore.Update{
			{Path: f.Mem.DeleteColumn, Value: firestore.Delete},
		})
		if err != nil {
			return fmt.Errorf("error updating document %s: %w", doc.Ref.ID, err)
		}
	}
	return nil
}
