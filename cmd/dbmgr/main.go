package main

import (
	"context"
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
	"github.com/ditto-assistant/backend/pkg/envs"
	"github.com/ditto-assistant/backend/pkg/secr"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/vertexai"
	_ "github.com/tursodatabase/go-libsql"
	"golang.org/x/sync/errgroup"
)

type Mode int

const (
	ModeIngest Mode = iota
	ModeSearch
)

var (
	dittoEnv envs.Env
	folder   string
	mode     Mode
	dryRun   bool
	query    string
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new FlagSet for global flags
	globalFlags := flag.NewFlagSet("global", flag.ExitOnError)
	envFlag := globalFlags.String("env", envs.EnvLocal.String(), "ditto environment")

	// Parse global flags
	globalFlags.Parse(os.Args[1:])

	// Set dittoEnv based on the parsed flag
	dittoEnv = envs.Env(*envFlag)
	envs.DITTO_ENV = dittoEnv
	err := dittoEnv.EnvFile().Load()
	if err != nil {
		log.Fatalf("error loading environment file: %s", err)
	}

	// Check if there's a subcommand
	if globalFlags.NArg() < 1 {
		log.Fatalf("usage: %s [-env <environment>] <command> [args]", os.Args[0])
	}

	// Get the subcommand
	subcommand := globalFlags.Arg(0)

	switch subcommand {
	case "ingest":
		mode = ModeIngest
		ingestFlags := flag.NewFlagSet("ingest", flag.ExitOnError)
		ingestFlags.StringVar(&folder, "folder", "cmd/dbmgr/tool-examples", "folder to ingest")
		ingestFlags.BoolVar(&dryRun, "dry-run", false, "dry run")
		ingestFlags.Parse(globalFlags.Args()[1:])
		slog.Debug("ingest mode", "folder", folder, "dry-run", dryRun, "env", dittoEnv)
	case "search":
		mode = ModeSearch
		searchFlags := flag.NewFlagSet("search", flag.ExitOnError)
		searchFlags.Parse(globalFlags.Args()[1:])
		if searchFlags.NArg() < 1 {
			log.Fatalf("usage: %s [-env <environment>] search <query>", os.Args[0])
		}
		query = strings.Join(searchFlags.Args(), " ")
		slog.Debug("search mode", "query", query, "env", dittoEnv)
	default:
		log.Fatalf("unknown command: %s", subcommand)
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
	em := db.Embedding(emQuery.Embeddings[0].Embedding)
	examples, err := db.SearchExamples(ctx, em)
	if err != nil {
		return fmt.Errorf("error searching examples: %w", err)
	}

	slog.Info("query results", "query", query)
	for _, example := range examples {
		slog.Info("example", "prompt", example.Prompt, "response", example.Response)
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
			emPromptBytes := example.EmPrompt.Binary()
			emPromptRespBytes := example.EmPromptResp.Binary()
			_, err := tx.ExecContext(ctx,
				"INSERT INTO examples (tool_id, prompt, response, em_prompt, em_prompt_response) VALUES (?, ?, ?, ?, ?)",
				toolID, example.Prompt, example.Response, emPromptBytes, emPromptRespBytes)
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

type ToolExample struct {
	Name        string       `json:"name"`
	Version     string       `json:"version"`
	Description string       `json:"description"`
	Examples    []db.Example `json:"examples"`
}

// EmbedBatch embeds all the examples in the tool example.
func (te ToolExample) EmbedBatch(ctx context.Context, embedder ai.Embedder) error {
	docs := make([]*ai.Document, 0, len(te.Examples)*2)
	for _, example := range te.Examples {
		// Embed the prompt
		docs = append(docs, &ai.Document{
			Content: []*ai.Part{
				{
					Kind: ai.PartText,
					Text: example.Prompt,
				},
			},
			Metadata: map[string]any{
				"tool_name":   te.Name,
				"description": te.Description,
			},
		})
		// Embed the prompt and response together
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
	}
	rs, err := embedder.Embed(ctx, &ai.EmbedRequest{Documents: docs})
	if err != nil {
		return fmt.Errorf("error embedding: %w", err)
	}
	for i := range te.Examples {
		te.Examples[i].EmPrompt = rs.Embeddings[i*2].Embedding
		te.Examples[i].EmPromptResp = rs.Embeddings[i*2+1].Embedding
	}
	return nil
}

func migrate(ctx context.Context) error {
	// First, ensure the migrations table exists
	_, err := db.D.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS migrations (
			migration_name TEXT PRIMARY KEY,
			migration_date TEXT DEFAULT (datetime('now'))
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating migrations table: %w", err)
	}

	files, err := filepath.Glob("cmd/dbmgr/migrations/*.sql")
	if err != nil {
		return fmt.Errorf("error reading migrations: %w", err)
	}

	for _, file := range files {
		migrationName := filepath.Base(file)

		// Check if this migration has already been applied
		var count int
		err := db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations WHERE migration_name = ?", migrationName).Scan(&count)
		if err != nil {
			return fmt.Errorf("error checking migration status: %w", err)
		}

		if count > 0 {
			slog.Debug("migration already applied, skipping", "file", migrationName)
			continue
		}

		contents, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("error reading migration file %s: %w", file, err)
		}
		slog.Debug("applying migration", "file", migrationName)

		// Start a new transaction for each migration
		tx, err := db.D.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("error beginning transaction for %s: %w", file, err)
		}

		_, err = tx.ExecContext(ctx, string(contents))
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error applying migration %s: %w", file, err)
		}

		// Record this migration as applied
		_, err = tx.ExecContext(ctx, "INSERT INTO migrations (migration_name) VALUES (?)", migrationName)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("error recording migration %s: %w", file, err)
		}

		// Commit the transaction for this migration
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("error committing migration %s: %w", file, err)
		}

		slog.Debug("migration applied successfully", "file", migrationName)
	}

	slog.Info("all migrations completed successfully")
	return nil
}
