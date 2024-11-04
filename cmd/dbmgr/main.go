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
	"strconv"
	"strings"
	"sync"

	firebase "firebase.google.com/go/v4"
	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/llm"
	"github.com/ditto-assistant/backend/pkg/numfmt"
	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/plugins/vertexai"
	_ "github.com/tursodatabase/go-libsql"
	"golang.org/x/sync/errgroup"
)

type Mode int

const (
	ModeUnknown Mode = iota
	ModeMigrate
	ModeRollback
	ModeSearch
	ModeIngest
	ModeFirestore
	ModeSyncBalance
	ModeSetBalance
)

func main() {
	var (
		dittoEnv envs.Env
		folder   string
		dryRun   bool
		query    string
		mode     Mode
		userID   string
	)
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
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
	var userBalance int64
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

	case "firestore":
		mode = ModeFirestore
		firestoreFlags := flag.NewFlagSet("firestore", flag.ExitOnError)
		firestoreFlags.StringVar(&userID, "uid", "", "user ID")
		firestoreFlags.Parse(globalFlags.Args()[1:])

	case "sync":
		if globalFlags.NArg() < 2 {
			log.Fatalf("usage: dbmgr [-env <environment>] sync <sync_type>")
		}
		switch globalFlags.Arg(1) {
		case "bals":
			mode = ModeSyncBalance
		default:
			log.Fatalf("unknown sync type: %s", globalFlags.Arg(1))
		}

	case "setbal":
		mode = ModeSetBalance
		setbalFlags := flag.NewFlagSet("setbal", flag.ExitOnError)
		setbalFlags.Usage = func() {
			fmt.Fprintf(os.Stderr, "usage: dbmgr [-env <environment>] setbal <uid> <balance>\n")
		}
		setbalFlags.Parse(globalFlags.Args()[1:])
		if setbalFlags.NArg() != 2 {
			setbalFlags.Usage()
			os.Exit(1)
		}
		userID = setbalFlags.Arg(0)
		userBalance, err = strconv.ParseInt(setbalFlags.Arg(1), 10, 64)
		if err != nil {
			log.Fatalf("invalid balance: %s", err)
		}

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
	case ModeFirestore:
		if err := firestorePrintUser(ctx, userID); err != nil {
			log.Fatalf("failed to test firestore: %s", err)
		}
	case ModeSyncBalance:
		if err := syncBalance(ctx); err != nil {
			log.Fatalf("failed to sync balance: %s", err)
		}
	case ModeSetBalance:
		if err := setBalance(ctx, userID, userBalance); err != nil {
			log.Fatalf("failed to set balance: %s", err)
		}
	}
}

func syncBalance(ctx context.Context) error {
	slog.Debug("syncing balance from firestore to database")
	count, err := db.GetDittoTokensPerDollar(ctx)
	if err != nil {
		return fmt.Errorf("error getting ditto tokens per dollar: %w", err)
	}
	slog.Debug("ditto tokens per dollar", "count", numfmt.LargeNumber(count))
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return fmt.Errorf("error creating firebase app: %w", err)
	}
	firestore, err := app.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("error getting firestore client: %w", err)
	}

	balanceQuery := firestore.CollectionGroup("balance")
	docs, err := balanceQuery.Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("error getting balance documents: %w", err)
	}

	for _, doc := range docs {
		var userData struct {
			Balance float64 `firestore:"balance"`
		}
		if err := doc.DataTo(&userData); err != nil {
			slog.Error("Error unmarshaling Firestore document", "docID", doc.Ref.ID, "error", err)
			continue
		}

		userID := doc.Ref.Parent.Parent.ID
		newBalance := int64(userData.Balance * float64(count))
		user := db.User{UID: userID, Balance: newBalance}
		if err := user.InitBalance(ctx); err != nil {
			return fmt.Errorf("error initializing user: %w", err)
		}
		slog.Info("User balance synced",
			"userID", userID,
			"user_dollars", strconv.FormatFloat(userData.Balance, 'f', 2, 64),
			"user_tokens", numfmt.LargeNumber(newBalance),
		)
	}

	return nil
}

func firestorePrintUser(ctx context.Context, userID string) error {
	app, err := firebase.NewApp(ctx, nil)
	if err != nil {
		return fmt.Errorf("error creating firebase app: %w", err)
	}
	firestore, err := app.Firestore(ctx)
	if err != nil {
		return fmt.Errorf("error getting firestore client: %w", err)
	}

	// getBalanceFromFirestore retrieves the balance for a given user ID from Firestore
	// It returns the balance as a float64, or false if the user doesn't exist or an error occurs
	balanceRef := firestore.Collection("users").Doc(userID).Collection("balance")
	docs, err := balanceRef.Documents(ctx).GetAll()
	if err != nil {
		slog.Error("Error getting documents from Firestore users collection", "error", err)
		return fmt.Errorf("error getting documents from Firestore: %w", err)
	}

	if len(docs) == 0 {
		slog.Info("User doesn't exist in Firestore", "userID", userID)
		return nil
	}

	userDoc := docs[0]
	var userData struct {
		Balance float64 `firestore:"balance"`
	}
	if err := userDoc.DataTo(&userData); err != nil {
		slog.Error("Error unmarshaling Firestore document", "error", err)
		return fmt.Errorf("error unmarshaling Firestore document: %w", err)
	}

	if userData.Balance != 0 {
		slog.Info("Retrieved user balance from Firestore", "userID", userID, "balance", userData.Balance)
	} else {
		slog.Info("User balance not found or zero", "userID", userID)
	}
	count, err := db.GetDittoTokensPerDollar(ctx)
	if err != nil {
		return fmt.Errorf("error getting ditto tokens per dollar: %w", err)
	}
	newBalance := int64(userData.Balance * float64(count))
	slog.Info("User tokens", "ditto per dollar", count, "user_dollars", userData.Balance, "user_tokens", numfmt.LargeNumber(newBalance))

	return nil
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
	em := llm.Embedding(emQuery.Embeddings[0].Embedding)
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
			if err := tool.Embed(embedCtx, embedder); err != nil {
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
		var serviceID int64
		err := tx.QueryRowContext(ctx, "SELECT id FROM services WHERE name = ?", tool.ServiceName).Scan(&serviceID)
		if err != nil {
			return fmt.Errorf("error getting service ID for %s: %w", tool.ServiceName, err)
		}

		// Insert into tools table
		result, err := tx.ExecContext(ctx,
			"INSERT INTO tools (name, description, version, service_id) VALUES (?, ?, ?, ?)",
			tool.Name, tool.Description, tool.Version, serviceID)
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
	llm.Tool
	Examples []llm.Example `json:"examples"`
}

// Embed embeds all the examples in the tool example.
func (te ToolExamples) Embed(ctx context.Context, embedder ai.Embedder) error {
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
	slog := slog.With("name", migrationName, "version", version)
	var count int
	err := db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations WHERE name = ?", migrationName).Scan(&count)
	if err != nil {
		return fmt.Errorf("error checking migration status: %w", err)
	}
	if count > 0 {
		slog.Debug("migration already applied, skipping")
		return nil
	}

	contents, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading migration file %s: %w", file, err)
	}
	statements := strings.Split(string(contents), ";\n\n")
	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		_, err = db.D.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("error applying migration %s: %w", file, err)
		}
	}
	_, err = db.D.ExecContext(ctx, "INSERT INTO migrations (name, version) VALUES (?, ?)", migrationName, version)
	if err != nil {
		return fmt.Errorf("error recording migration %s: %w", file, err)
	}

	slog.Debug("migration applied successfully")
	return nil
}

func rollback(ctx context.Context, version string) error {
	slog.Info("rolling back database", "version", version)
	rollbackFiles, err := filepath.Glob("cmd/dbmgr/rollbacks/v*.sql")
	if err != nil {
		return fmt.Errorf("error reading rollback files: %w", err)
	}
	slices.Reverse(rollbackFiles)

	for _, file := range rollbackFiles {
		fileVersion := strings.TrimSuffix(filepath.Base(file), ".sql")
		if fileVersion <= version {
			break // Stop rolling back once we reach the target version
		}

		if err := applyRollback(ctx, file); err != nil {
			return err
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

	rollbackVersion := strings.TrimSuffix(filepath.Base(file), ".sql")
	var count int
	err = db.D.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations WHERE version = ?", rollbackVersion).Scan(&count)
	if err != nil {
		return fmt.Errorf("error checking migration status: %w", err)
	}
	if count == 0 {
		slog.Debug("version not applied, skipping rollback", "version", rollbackVersion)
		return nil
	}

	contents, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("error reading rollback file %s: %w", file, err)
	}

	// Split the contents into statements, respecting SQL strings
	statements := splitSQLStatements(string(contents))

	for _, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		_, err = db.D.ExecContext(ctx, stmt)
		if err != nil {
			return fmt.Errorf("error rolling back version %s: %w", rollbackVersion, err)
		}
	}

	_, err = db.D.ExecContext(ctx, "DELETE FROM migrations WHERE version = ?", rollbackVersion)
	if err != nil {
		return fmt.Errorf("error deleting migration records for version %s: %w", rollbackVersion, err)
	}

	slog.Debug("rollback applied successfully", "version", rollbackVersion)
	return nil
}

// splitSQLStatements splits a SQL script into individual statements,
// respecting SQL strings and comments
func splitSQLStatements(script string) []string {
	var statements []string
	var currentStatement strings.Builder
	currentStatement.Grow(len(script))
	var inString bool
	var stringDelimiter rune
	newlinesInRow := -1

	for _, r := range script {
		currentStatement.WriteRune(r)
		switch r {
		case '\'', '"':
			if !inString {
				inString = true
				stringDelimiter = r
			} else if stringDelimiter == r {
				inString = false
			}
		case ';':
			if !inString {
				newlinesInRow = 0
			}
		case '\n':
			if !inString && newlinesInRow >= 0 {
				newlinesInRow++
				if newlinesInRow > 1 {
					statements = append(statements, strings.TrimSpace(currentStatement.String()))
					currentStatement.Reset()
					newlinesInRow = -1
				}
			}
		default:
			newlinesInRow = -1
		}
	}

	// Add any remaining content as the last statement
	if currentStatement.Len() > 0 {
		statements = append(statements, strings.TrimSpace(currentStatement.String()))
	}

	return statements
}

func getLatestVersion(ctx context.Context) (string, error) {
	var version string
	err := db.D.QueryRowContext(ctx, "SELECT version FROM migrations ORDER BY date DESC, version DESC LIMIT 1").Scan(&version)
	if err != nil {
		return "", fmt.Errorf("error getting latest version: %w", err)
	}
	return version, nil
}

func setBalance(ctx context.Context, uid string, balance int64) error {
	slog.Info("setting user balance", "uid", uid, "balance", numfmt.LargeNumber(balance))

	result, err := db.D.ExecContext(ctx,
		"UPDATE users SET balance = ? WHERE uid = ?",
		balance, uid)
	if err != nil {
		return fmt.Errorf("error updating balance: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("error getting rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("no user found with uid: %s", uid)
	}

	slog.Info("successfully set balance",
		"uid", uid,
		"new_balance", numfmt.LargeNumber(balance))
	return nil
}
