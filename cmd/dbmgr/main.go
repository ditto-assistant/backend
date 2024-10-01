package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/ditto-assistant/backend/pkg/consts"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/secr"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/vertexai"
	"golang.org/x/sync/errgroup"
)

type ToolExample struct {
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Description string    `json:"description"`
	Examples    []Example `json:"examples"`
}

// EmbedBatch embeds all the examples in the tool example.
func (te ToolExample) EmbedBatch(ctx context.Context, embedder ai.Embedder) error {
	docs := make([]*ai.Document, 0, len(te.Examples))
	for _, example := range te.Examples {
		docs = append(docs, &ai.Document{
			Content: []*ai.Part{
				{
					Kind: ai.PartText,
					Text: example.Prompt,
				},
				{
					Kind: ai.PartText,
					Text: example.Response,
				},
			},
			Metadata: map[string]any{
				"tool_name":   te.Name,
				"description": te.Description,
			},
		})
		// docs = append(docs, &ai.Document{
		// 	Content: []*ai.Part{
		// 		{
		// 			Kind: ai.PartText,
		// 			Text: example.Prompt,
		// 		},
		// 	},
		// 	Metadata: map[string]any{
		// 		"tool_name":   te.Name,
		// 		"description": te.Description,
		// 	},
		// })
		// docs = append(docs, &ai.Document{
		// 	Content: []*ai.Part{
		// 		{
		// 			Kind: ai.PartText,
		// 			Text: example.Response,
		// 		},
		// 	},
		// 	Metadata: map[string]any{
		// 		"tool_name":   te.Name,
		// 		"description": te.Description,
		// 	},
		// })
	}
	rs, err := embedder.Embed(ctx, &ai.EmbedRequest{
		Documents: docs,
	})
	if err != nil {
		return fmt.Errorf("error embedding: %w", err)
	}
	for i := range te.Examples {
		// te.Examples[i].EmPromptResp = rs.Embeddings[i*3].Embedding
		// te.Examples[i].EmPrompt = rs.Embeddings[i*3+1].Embedding
		// te.Examples[i].EmResponse = rs.Embeddings[i*3+2].Embedding
		te.Examples[i].EmPromptResp = rs.Embeddings[i].Embedding
	}
	return nil
}

type Embedding []float32

func (e Embedding) Binary() []byte {
	var buf bytes.Buffer
	buf.Grow(len(e) * 4)
	for _, v := range e {
		err := binary.Write(&buf, binary.LittleEndian, v)
		if err != nil {
			log.Fatalf("error converting float32 to bytes: %s", err)
		}
	}
	return buf.Bytes()
}

type Example struct {
	Prompt       string    `json:"prompt"`
	Response     string    `json:"response"`
	EmPrompt     Embedding `json:"-"`
	EmResponse   Embedding `json:"-"`
	EmPromptResp Embedding `json:"-" db:"type:blob"`
}

type Mode int

const (
	ModeIngest Mode = iota
	ModeSearch
)

var (
	folder string
	mode   Mode
	dryRun bool
	query  string
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <command> [args]", os.Args[0])
	}

	switch os.Args[1] {
	case "ingest":
		mode = ModeIngest
		fset := flag.NewFlagSet("ingest", flag.ExitOnError)
		fset.StringVar(&folder, "folder", "cmd/dbmgr/tool-examples", "folder to ingest")
		fset.BoolVar(&dryRun, "dry-run", false, "dry run")
		fset.Parse(os.Args[2:])
		slog.Debug("ingest mode", "folder", folder, "dry-run", dryRun)
	case "search":
		mode = ModeSearch
		if len(os.Args) < 3 {
			log.Fatalf("usage: %s search <query>", os.Args[0])
		}
		query = strings.Join(os.Args[2:], " ")
		slog.Debug("search mode", "query", query)
	default:
		log.Fatalf("unknown command: %s", os.Args[1])
	}

	if err := secr.Setup(ctx); err != nil {
		log.Fatalf("failed to initialize secrets: %s", err)
	}
	var shutdown sync.WaitGroup
	if err := db.Setup(ctx, &shutdown); err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}
	if err := migrate(ctx); err != nil {
		log.Fatalf("failed to migrate database: %s", err)
	}
	switch mode {
	case ModeIngest:
		if err := ingestPromptExamples(ctx, folder, dryRun); err != nil {
			log.Fatalf("failed to ingest prompt examples: %s", err)
		}
	case ModeSearch:
		if err := testSearch(ctx, query); err != nil {
			log.Fatalf("failed to test search: %s", err)
		}
	}
	cancel()
	shutdown.Wait()
}

func testSearch(ctx context.Context, query string) error {
	if err := vertexai.Init(ctx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		return fmt.Errorf("error initializing vertexai: %w", err)
	}
	embedder := vertexai.Embedder(consts.ModelTextEmbedding004.String())
	if embedder == nil {
		return errors.New("embedder not found")
	}
	emQuery, err := embedder.Embed(ctx, &ai.EmbedRequest{
		Documents: []*ai.Document{
			{
				Content: []*ai.Part{
					{
						Kind: ai.PartText,
						Text: query,
					},
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("error embedding query: %w", err)
	}
	emBytes := Embedding(emQuery.Embeddings[0].Embedding).Binary()
	rows, err := db.D.QueryContext(ctx,
		"SELECT prompt, response FROM examples ORDER BY vector_distance_cos(em_prompt_response, ?) LIMIT 5",
		emBytes)
	if err != nil {
		return fmt.Errorf("error querying database: %w", err)
	}
	slog.Info("query results", "query", query)
	defer rows.Close()
	for rows.Next() {
		var example Example
		err = rows.Scan(&example.Prompt, &example.Response)
		if err != nil {
			return fmt.Errorf("error scanning row: %w", err)
		}
		slog.Info("example", "prompt", example.Prompt, "response", example.Response)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating over rows: %w", err)
	}

	return nil
}

func ingestPromptExamples(ctx context.Context, folder string, dryRun bool) error {
	if err := vertexai.Init(ctx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		return fmt.Errorf("error initializing vertexai: %w", err)
	}
	files, err := filepath.Glob(filepath.Join(folder, "*.json"))
	if err != nil {
		return fmt.Errorf("error reading folder: %w", err)
	}
	fileSlice := make([]ToolExample, 0, len(files))
	for _, file := range files {
		file, err := os.Open(file)
		if err != nil {
			return fmt.Errorf("error opening file: %w", err)
		}
		var toolExamples ToolExample
		if err := json.NewDecoder(file).Decode(&toolExamples); err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}
		fileSlice = append(fileSlice, toolExamples)
	}

	if dryRun {
		slog.Debug("dry run, skipping database operations", "toolCount", len(fileSlice))
		return nil
	}

	embedder := vertexai.Embedder(consts.ModelTextEmbedding004.String())
	if embedder == nil {
		return errors.New("embedder not found")
	}
	group, embedCtx := errgroup.WithContext(ctx)
	for _, tool := range fileSlice {
		group.Go(func() error {
			if err := tool.EmbedBatch(embedCtx, embedder); err != nil {
				return fmt.Errorf("error embedding tool: %w", err)
			}
			return nil
		})
	}
	if err := group.Wait(); err != nil {
		return fmt.Errorf("error embedding tools: %w", err)
	}

	// Start a transaction
	tx, err := db.D.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer tx.Rollback()

	for _, tool := range fileSlice {
		// Insert into tools table
		result, err := tx.ExecContext(ctx,
			"INSERT INTO tools (name, description, version) VALUES (?, ?, ?)",
			tool.Name, tool.Description, tool.Version)
		if err != nil {
			return fmt.Errorf("error inserting tool: %w", err)
		}

		toolID, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("error getting last insert ID: %w", err)
		}

		// Insert examples for this tool
		for _, example := range tool.Examples {
			emPromptRespBytes := example.EmPromptResp.Binary()
			_, err := tx.ExecContext(ctx,
				"INSERT INTO examples (tool_id, prompt, response, em_prompt_response) VALUES (?, ?, ?, ?)",
				toolID, example.Prompt, example.Response, emPromptRespBytes)
			if err != nil {
				return fmt.Errorf("error inserting example: %w", err)
			}
		}

		slog.Info("Inserted tool and examples", "tool", tool.Name, "exampleCount", len(tool.Examples))
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing transaction: %w", err)
	}

	slog.Info("Successfully ingested all tools and examples", "toolCount", len(fileSlice))
	return nil
}
func migrate(ctx context.Context) error {
	// Check if the migrations table exists
	var tableExists bool
	var name string
	err := db.D.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='migrations'").Scan(&name)
	tableExists = (name == "migrations")
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error checking if migrations table exists: %w", err)
	}
	if !tableExists {
		slog.Debug("migrations table does not exist, running initial migration")
	} else {
		// Check if we have at least one migration
		var count int
		err = db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations").Scan(&count)
		if err != nil {
			return fmt.Errorf("error checking migrations: %w", err)
		} else if count > 0 {
			slog.Debug("tables already migrated", "migrations", count)
			return nil
		} else {
			slog.Debug("migrations table exists but no migrations found")
		}
	}

	tx, err := db.D.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction: %w", err)
	}
	defer tx.Rollback()
	// Create the initial migration
	_, err = tx.ExecContext(ctx, createExampleStore)
	if err != nil {
		return fmt.Errorf("error creating tables: %w", err)
	}
	slog.Debug("tables migrated")
	return tx.Commit()
}

const createExampleStore = `
CREATE TABLE IF NOT EXISTS migrations (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  migration_name TEXT,
  migration_date TEXT DEFAULT (datetime('now'))
);
INSERT INTO migrations (migration_name) 
SELECT 'table_init' 
WHERE NOT EXISTS (SELECT 1 FROM migrations);


CREATE TABLE IF NOT EXISTS tools (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT,
  description TEXT,
  version TEXT
);

CREATE TABLE IF NOT EXISTS examples (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  tool_id INTEGER,
  prompt TEXT,
  response TEXT,
  -- 768-dimensional f32 vector embeddings
  em_prompt F32_BLOB(768), 
  em_response F32_BLOB(768), 
  em_prompt_response F32_BLOB(768)
);
`
