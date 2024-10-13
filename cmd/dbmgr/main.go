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
	"slices"
	"strings"
	"sync"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/vertexai"
	_ "github.com/tursodatabase/go-libsql"
	"golang.org/x/sync/errgroup"
)

var (
	dittoEnv envs.Env
	folder   string
	dryRun   bool
	query    string
	mode     Mode
)

type Mode int

const (
	ModeUnknown Mode = iota
	ModeMigrate
	ModeRollback
	ModeSearch
	ModeIngest
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	var shutdown sync.WaitGroup
	defer shutdown.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Parse global flags
	globalFlags := flag.NewFlagSet("global", flag.ExitOnError)
	envFlag := globalFlags.String("env", envs.EnvLocal.String(), "ditto environment")
	globalFlags.Parse(os.Args[1:])
	dittoEnv = envs.Env(*envFlag)
	envs.DITTO_ENV = dittoEnv
	err := dittoEnv.EnvFile().Load()
	if err != nil {
		log.Fatalf("error loading environment file: %s", err)
	}
	if globalFlags.NArg() < 1 {
		log.Fatalf("usage: %s [-env <environment>] <command> [args]", os.Args[0])
	}
	subcommand := globalFlags.Arg(0)

	// Parse subcommand flags
	var version string
	switch subcommand {
	case "migrate":
		mode = ModeMigrate

	case "rollback":
		mode = ModeRollback
		rollbackFlags := flag.NewFlagSet("rollback", flag.ExitOnError)
		rollbackFlags.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: dbmgr [-env <environment>] rollback <version>\n")
		}
		rollbackFlags.Parse(globalFlags.Args()[1:])
		if rollbackFlags.NArg() < 1 {
			rollbackFlags.Usage()
			os.Exit(1)
		}
		version = rollbackFlags.Arg(0)

	case "ingest":
		mode = ModeIngest
		ingestFlags := flag.NewFlagSet("ingest", flag.ExitOnError)
		ingestFlags.StringVar(&folder, "folder", "cmd/dbmgr/tool-examples", "folder to ingest")
		ingestFlags.BoolVar(&dryRun, "dry-run", false, "dry run")
		ingestFlags.Parse(globalFlags.Args()[1:])

	case "search":
		mode = ModeSearch
		searchFlags := flag.NewFlagSet("search", flag.ExitOnError)
		searchFlags.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: dbmgr [-env <environment>] search <query>\n")
		}
		searchFlags.Parse(globalFlags.Args()[1:])
		if searchFlags.NArg() < 1 {
			searchFlags.Usage()
			os.Exit(1)
		}
		query = strings.Join(searchFlags.Args(), " ")
		slog.Debug("search mode", "query", query, "env", dittoEnv)

	default:
		log.Fatalf("unknown command: %s", subcommand)
	}

	if err := secr.Setup(ctx); err != nil {
		log.Fatalf("failed to initialize secrets: %s", err)
	}
	if err := db.Setup(ctx, &shutdown); err != nil {
		log.Fatalf("failed to initialize database: %s", err)
	}

	switch mode {
	case ModeMigrate:
		if err := migrate(ctx); err != nil {
			log.Fatalf("failed to migrate database: %s", err)
		}
	case ModeRollback:
		if err := rollback(ctx, version); err != nil {
			log.Fatalf("failed to rollback database: %s", err)
		}
	case ModeIngest:
		if err := ingestPromptExamples(ctx, folder, dryRun); err != nil {
			log.Fatalf("failed to ingest prompt examples: %s", err)
		}
	case ModeSearch:
		if err := testSearch(ctx, query); err != nil {
			log.Fatalf("failed to test search: %s", err)
		}
	}
}

func testSearch(ctx context.Context, query string) error {
	slog.Debug("test search", "query", query)
	minVersion := "v0.0.1"
	latestVersion, err := getLatestVersion(ctx)
	if err != nil {
		return fmt.Errorf("error getting latest version: %w", err)
	}
	if latestVersion < minVersion {
		return fmt.Errorf("version %s is not applied, please apply at least version %s before searching", latestVersion, minVersion)
	}
	if err := vertexai.Init(ctx, &vertexai.Config{
		ProjectID: "ditto-app-dev",
		Location:  "us-central1",
	}); err != nil {
		return fmt.Errorf("error initializing vertexai: %w", err)
	}
	embedder := vertexai.Embedder(llm.ModelTextEmbedding004.String())
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
	slog.Info("ingesting prompt examples", "folder", folder, "dry-run", dryRun)
	minVersion := "v0.0.1"
	latestVersion, err := getLatestVersion(ctx)
	if err != nil {
		return fmt.Errorf("error getting latest version: %w", err)
	}
	if latestVersion < minVersion {
		return fmt.Errorf("version %s is not applied, please apply at least version %s before embedding", latestVersion, minVersion)
	}
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
	fileSlice := make([]ToolExamples, 0, len(files))
	for _, file := range files {
		file, err := os.Open(file)
		if err != nil {
			return fmt.Errorf("error opening file: %w", err)
		}
		var toolExamples ToolExamples
		if err := json.NewDecoder(file).Decode(&toolExamples); err != nil {
			return fmt.Errorf("error reading file: %w", err)
		}
		fileSlice = append(fileSlice, toolExamples)
	}

	if dryRun {
		slog.Debug("dry run, skipping database operations", "toolCount", len(fileSlice))
		return nil
	}

	embedder := vertexai.Embedder(llm.ModelTextEmbedding004.String())
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
			"INSERT INTO tools (name, description, version, cost_per_call, cost_multiplier, base_tokens) VALUES (?, ?, ?, ?, ?, ?)",
			tool.Name, tool.Description, tool.Version, tool.CostPerCall, tool.CostMultiplier, tool.BaseTokens)
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

type ToolExamples struct {
	db.Tool
	Examples []db.Example `json:"examples"`
}

// EmbedBatch embeds all the examples in the tool example.
func (te ToolExamples) EmbedBatch(ctx context.Context, embedder ai.Embedder) error {
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
	slog.Info("migrating database")
	_, err := db.D.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS migrations (
			name TEXT PRIMARY KEY,
			date TEXT DEFAULT (datetime('now')),
			version TEXT NOT NULL
		)
	`)
	if err != nil {
		return fmt.Errorf("error creating migrations table: %w", err)
	}

	versionFolders, err := filepath.Glob("cmd/dbmgr/migrations/v*")
	if err != nil {
		return fmt.Errorf("error reading migration version folders: %w", err)
	}

	for _, versionFolder := range versionFolders {
		files, err := filepath.Glob(filepath.Join(versionFolder, "*.sql"))
		if err != nil {
			return fmt.Errorf("error reading migrations in %s: %w", versionFolder, err)
		}

		version := filepath.Base(versionFolder)
		for _, file := range files {
			if err := applyMigration(ctx, file, version); err != nil {
				return err
			}
		}
	}

	slog.Info("all migrations completed successfully")
	return nil
}

func applyMigration(ctx context.Context, file, version string) error {
	migrationName := strings.TrimSuffix(filepath.Base(file), ".sql")
	var count int
	err := db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations WHERE name = ?", migrationName).Scan(&count)
	if err != nil {
		return fmt.Errorf("error checking migration status: %w", err)
	}
	if count > 0 {
		slog.Debug("migration already applied, skipping", "file", migrationName)
		return nil
	}

	contents, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading migration file %s: %w", file, err)
	}
	tx, err := db.D.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction for %s: %w", file, err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, string(contents))
	if err != nil {
		return fmt.Errorf("error applying migration %s: %w", file, err)
	}

	// Record this migration as applied
	_, err = tx.ExecContext(ctx, "INSERT INTO migrations (name, version) VALUES (?, ?)", migrationName, version)
	if err != nil {
		return fmt.Errorf("error recording migration %s: %w", file, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing migration %s: %w", file, err)
	}

	slog.Debug("migration applied successfully", "file", migrationName)
	return nil
}

func rollback(ctx context.Context, version string) error {
	slog.Info("rolling back database", "version", version)
	versionFolders, err := filepath.Glob(filepath.Join("cmd/dbmgr/rollbacks/v*"))
	if err != nil {
		return fmt.Errorf("error reading rollback version folders: %w", err)
	}
	slices.Reverse(versionFolders)

	for _, folder := range versionFolders {
		folderVersion := filepath.Base(folder)
		if folderVersion <= version {
			break // Stop rolling back once we reach the target version
		}

		files, err := filepath.Glob(filepath.Join(folder, "*.sql"))
		if err != nil {
			return fmt.Errorf("error reading rollback files in %s: %w", folder, err)
		}
		slices.Reverse(files)

		for _, file := range files {
			if err := applyRollback(ctx, file); err != nil {
				return err
			}
		}
	}
	slog.Info("database rolled back successfully")
	return nil
}

func applyRollback(ctx context.Context, file string) error {
	var tableExists bool
	err := db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='migrations'").Scan(&tableExists)
	if err != nil {
		return fmt.Errorf("error checking if migrations table exists: %w", err)
	}
	if !tableExists {
		return errors.New("migrations table does not exist, cannot apply rollback")
	}
	rollbackName := strings.TrimSuffix(filepath.Base(file), ".sql")
	var count int
	err = db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations WHERE name = ?", rollbackName).Scan(&count)
	if err != nil {
		return fmt.Errorf("error checking migration status: %w", err)
	}
	if count == 0 {
		slog.Debug("migration not applied, skipping rollback", "file", rollbackName)
		return nil
	}

	contents, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading rollback file %s: %w", file, err)
	}
	tx, err := db.D.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("error beginning transaction for %s: %w", file, err)
	}
	defer tx.Rollback()

	_, err = tx.ExecContext(ctx, string(contents))
	if err != nil {
		return fmt.Errorf("error rolling back migration %s: %w", file, err)
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM migrations WHERE name = ? AND EXISTS (SELECT 1 FROM migrations LIMIT 1)", rollbackName)
	if err != nil {
		if strings.Contains(err.Error(), "no such table") {
			slog.Warn("migrations table does not exist, skipping deletion", "name", rollbackName)
		} else {
			return fmt.Errorf("error deleting migration record for %s: %w", rollbackName, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("error committing rollback for %s: %w", rollbackName, err)
	}

	slog.Debug("rollback applied successfully", "file", rollbackName)
	return nil
}

func getLatestVersion(ctx context.Context) (string, error) {
	var version string
	err := db.D.QueryRowContext(ctx, "SELECT version FROM migrations ORDER BY date DESC, version DESC LIMIT 1").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("error getting latest version: %w", err)
	}
	return version, nil
}
