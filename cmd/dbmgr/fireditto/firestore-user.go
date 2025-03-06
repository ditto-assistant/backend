package fireditto

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"github.com/ditto-assistant/backend/pkg/services/db"
	"github.com/ditto-assistant/backend/pkg/services/llm"
	"github.com/ditto-assistant/backend/pkg/services/llm/googai"
	"github.com/ditto-assistant/backend/types/rp"
)

func (f *Command) printUser(ctx context.Context) error {
	if err := f.initClients(ctx); err != nil {
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

func (f *Command) initClients(ctx context.Context) error {
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
	f.googai, err = googai.NewClient(ctx)
	if err != nil {
		return fmt.Errorf("error initializing Google AI client: %w", err)
	}
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
	if err := f.initClients(ctx); err != nil {
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
	bulkWriter := f.fs.BulkWriter(ctx)
	defer func() {
		bulkWriter.End()
		slog.Info("firestore bulk writer ended")
	}()
	if f.Mem.AllUsers {
		fmt.Println("ALL USERS, ARE YOU SURE?")
		if !requireConfirmation() {
			return errors.New("operation cancelled by user")
		}
		return f.embedAllUsersMem(ctx, model, bulkWriter)
	} else {
		return f.embedSingleUserMem(ctx, model, bulkWriter)
	}
}

func (f *Command) embedSingleUserMem(ctx context.Context, model string, bulkWriter *firestore.BulkWriter) error {
	colRef := f.fs.Collection("memory").Doc(f.UID).Collection("conversations")
	return f.processUserConversations(ctx, colRef, model, bulkWriter, f.UID)
}

func (f *Command) embedAllUsersMem(ctx context.Context, model string, bulkWriter *firestore.BulkWriter) error {
	memoryRef := f.fs.Collection("memory")
	userDocs, err := memoryRef.DocumentRefs(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("error getting all user memory docs: %w", err)
	}
	slog.Info("Found users to process", "count", len(userDocs))
	for _, userDoc := range userDocs {
		userID := userDoc.ID
		colRef := memoryRef.Doc(userID).Collection("conversations")
		if err := f.processUserConversations(ctx, colRef, model, bulkWriter, userID); err != nil {
			return fmt.Errorf("%w: failed processing conversations; userID: %s", err, userID)
		}
	}
	return nil
}

func (f *Command) processUserConversations(
	ctx context.Context,
	colRef *firestore.CollectionRef,
	model string,
	bulkWriter *firestore.BulkWriter,
	userID string,
) error {
	logger := f.logger.With("userID", userID)
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
	if f.DryRun {
		logger.Info("Dry run, skipping processing", "count", len(docs))
		return nil
	}
	if slices.Contains(f.Mem.SkipUserIDs, userID) {
		logger.Info("Skipping userID")
	}
	var lastAirdropAt sql.NullTime
	var email sql.NullString
	db.D.QueryRowContext(ctx, "SELECT email, last_airdrop_at FROM users WHERE uid = ?", userID).Scan(&email, &lastAirdropAt)
	if !lastAirdropAt.Valid {
		logger.Info("User has not received airdrops, indicating they have not recently used the app")
		return nil
	}
	fmt.Printf("EMBED %s: last active at: %s; conversations: %d", email.String, lastAirdropAt.Time, len(docs))
	if !requireConfirmation() {
		logger.Debug("Embedding skipped")
		return nil
	}
	logger.Info("Processing user conversations", "count", len(docs))
	return f.embedBatch(ctx, docs, model, bulkWriter, logger)
}

const batchSize = 50

// EmbeddingItem tracks an individual content item to be embedded
type EmbeddingItem struct {
	DocIndex   int    // Index of document in the batch
	FieldIndex int    // Index of field in the embed fields
	Content    string // The content to embed
	EmbedPath  string // The path where embedding should be stored
	DocumentID string // Document ID for better logging
}

func (f *Command) embedBatch(
	ctx context.Context,
	docs []*firestore.DocumentSnapshot,
	model string,
	bulkWriter *firestore.BulkWriter,
	logger *slog.Logger,
) error {
	totalEmbeddingCount := 0
	logger.Info("Starting embedding process", "total_documents", len(docs), "batch_size", batchSize, "model", model)
	for i := 0; i < len(docs); i += batchSize {
		end := min(i+batchSize, len(docs))
		batch := docs[i:end]
		embedItems := make([]EmbeddingItem, 0, len(batch)*len(f.Mem.Embed.Fields))
		contents := make([]string, 0, len(batch)*len(f.Mem.Embed.Fields))
		// First pass: collect all items to be embedded
		for j, doc := range batch {
			docEmbedCount := 0
			for k, pair := range f.Mem.Embed.Fields {
				data, err := doc.DataAt(pair.ContentField)
				if err != nil {
					return fmt.Errorf("error getting data from document: %s: %w", doc.Ref.ID, err)
				}
				str, ok := data.(string)
				if !ok {
					return fmt.Errorf("data: %T is not a string for document: %s", data, doc.Ref.ID)
				}
				switch pair.ContentField {
				case "prompt":
					rp.TrimStuff(&str, "![image](", ")", nil)
				case "response":
					rp.TrimStuff(&str, "![DittoImage](", ")", nil)
					rp.FormatToolsResponse(&str)
				}
				if str == "" {
					logger.Debug("Skipping empty content", "docID", doc.Ref.ID, "field", pair.ContentField)
					continue
				}
				item := EmbeddingItem{
					DocIndex:   j,
					FieldIndex: k,
					Content:    str,
					EmbedPath:  pair.EmbeddingField,
					DocumentID: doc.Ref.ID,
				}
				embedItems = append(embedItems, item)
				contents = append(contents, str)
				docEmbedCount++
			}
			logger.Debug("Document prepared for embedding",
				"docID", doc.Ref.ID,
				"fields_to_embed", docEmbedCount,
				"total_fields", len(f.Mem.Embed.Fields))
		}
		if len(contents) == 0 {
			logger.Info("No contents to embed in this batch", "batch_start", i, "batch_end", end)
			continue
		}

		var embedResp googai.EmbedResponse
		err := f.googai.Embed(ctx, &googai.EmbedRequest{
			Documents: contents,
			Model:     llm.ServiceName(model),
		}, &embedResp)
		if err != nil {
			return fmt.Errorf("error embedding batch starting at %d: %w", i, err)
		}
		if len(embedResp.Embeddings) != len(contents) {
			return fmt.Errorf("unexpected number of embeddings received: got %d, expected %d",
				len(embedResp.Embeddings), len(contents))
		}

		docUpdates := make(map[string][]firestore.Update, len(batch))
		for idx, item := range embedItems {
			docID := item.DocumentID
			if _, exists := docUpdates[docID]; !exists {
				docUpdates[docID] = make([]firestore.Update, 0, len(f.Mem.Embed.Fields))
			}
			docUpdates[docID] = append(docUpdates[docID], firestore.Update{
				Path:  item.EmbedPath,
				Value: firestore.Vector32(embedResp.Embeddings[idx]),
			})
		}
		updatedDocs := 0
		for _, doc := range batch {
			docID := doc.Ref.ID
			updates, hasUpdates := docUpdates[docID]
			if !hasUpdates || len(updates) == 0 {
				continue
			}
			_, err := bulkWriter.Update(doc.Ref, updates)
			if err != nil {
				return fmt.Errorf("error updating document %s: %w", docID, err)
			}
			updatedDocs++
			totalEmbeddingCount += len(updates)
		}
		logger.Info("Processed batch",
			"start", i,
			"end", end,
			"documents_updated", updatedDocs,
			"documents_in_batch", len(batch),
			"embeddings_applied", len(contents))
	}

	logger.Info("Completed embedding process",
		"total_documents", len(docs),
		"total_embeddings", totalEmbeddingCount)
	return nil
}

func (f *Command) deleteColumn(ctx context.Context) error {
	if err := f.initClients(ctx); err != nil {
		return err
	}
	if err := f.getUserByEmail(ctx); err != nil {
		return err
	}
	if f.UID == "" && !f.Mem.AllUsers {
		return errors.New("user ID is empty")
	}
	slog.Info("Starting column deletion", "column", f.Mem.DeleteColumn, "userID", f.UID, "allUsers", f.Mem.AllUsers)
	bulkWriter := f.fs.BulkWriter(ctx)
	defer func() {
		bulkWriter.End()
		slog.Info("Successfully deleted column from all documents", "column", f.Mem.DeleteColumn)
	}()

	if f.Mem.AllUsers {
		fmt.Println("ALL USERS, ARE YOU SURE?")
		if !requireConfirmation() {
			return errors.New("operation cancelled by user")
		}
		return f.deleteColumnAllUsers(ctx, bulkWriter)
	}

	return f.deleteColumnSingleUser(ctx, bulkWriter)
}

func (f *Command) deleteColumnSingleUser(ctx context.Context, bulkWriter *firestore.BulkWriter) error {
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

func (f *Command) deleteColumnAllUsers(ctx context.Context, bulkWriter *firestore.BulkWriter) error {
	memoryRef := f.fs.Collection("memory")
	userDocs, err := memoryRef.DocumentRefs(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("error getting all user memory docs: %w", err)
	}
	slog.Info("Found users to process", "count", len(userDocs))
	for _, userDoc := range userDocs {
		userID := userDoc.ID
		if slices.Contains(f.Mem.SkipUserIDs, userID) {
			slog.Info("Skipping userID", "userID", userID)
			continue
		}
		docs, err := memoryRef.Doc(userID).Collection("conversations").Documents(ctx).GetAll()
		if err != nil {
			return fmt.Errorf("error getting conversations for user %s: %w", userID, err)
		}
		slog.Info("Processing user conversations", "userID", userID, "count", len(docs))
		for _, doc := range docs {
			_, err := bulkWriter.Update(doc.Ref, []firestore.Update{
				{Path: f.Mem.DeleteColumn, Value: firestore.Delete},
			})
			if err != nil {
				return fmt.Errorf("error updating document %s for user %s: %w", doc.Ref.ID, userID, err)
			}
		}
	}
	return nil
}

func requireConfirmation() bool {
	fmt.Print(" (y/n) ")
	var response string
	fmt.Scanln(&response)
	response = strings.ToLower(strings.TrimSpace(response))
	return response == "y"
}
