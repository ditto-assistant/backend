package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/ditto-assistant/backend/pkg/envs"
	"github.com/ditto-assistant/backend/pkg/secr"
	"github.com/tursodatabase/go-libsql"
)

var (
	db *sql.DB
)

func Setup(ctx context.Context, shutdown *sync.WaitGroup) error {
	shutdown.Add(1)
	dbName := "local.db"
	dir, err := os.MkdirTemp("", "libsql-*")
	if err != nil {
		fmt.Println("Error creating temporary directory:", err)
		os.Exit(1)
	}

	dbPath := filepath.Join(dir, dbName)
	connector, err := libsql.NewEmbeddedReplicaConnector(dbPath, envs.DB_URL_DITTO_EXAMPLES,
		libsql.WithAuthToken(secr.TURSO_AUTH_TOKEN_DITTO_EXAMPLES),
		libsql.WithEncryption(secr.LIBSQL_ENCRYPTION_KEY),
	)
	if err != nil {
		return fmt.Errorf("error creating connector: %w", err)
	}

	db = sql.OpenDB(connector)
	slog.Debug("db connected", "url", envs.DB_URL_DITTO_EXAMPLES)

	if err := migrate(ctx); err != nil {
		return fmt.Errorf("error creating tables: %w", err)
	}

	go func() {
		<-ctx.Done()
		slog.Debug("shutting down libsql db")
		db.Close()
		connector.Close()
		os.RemoveAll(dir)
		shutdown.Done()
	}()

	// ai.DefineIndexer("custom", "example-indexer", func(ctx context.Context, ir *ai.IndexerRequest) error {
	// 	return nil
	// })

	return nil
}

func migrate(ctx context.Context) error {
	// Check if the migrations table exists
	var tableExists bool
	var name string
	err := db.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='migrations'").Scan(&name)
	tableExists = (name == "migrations")
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("error checking if migrations table exists: %w", err)
	}
	if !tableExists {
		slog.Debug("migrations table does not exist, running initial migration")
	} else {
		// Check if we have at least one migration
		var count int
		err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM migrations").Scan(&count)
		if err != nil {
			return fmt.Errorf("error checking migrations: %w", err)
		} else if count > 0 {
			slog.Debug("tables already migrated", "migrations", count)
			return nil
		} else {
			slog.Debug("migrations table exists but no migrations found")
		}
	}

	tx, err := db.BeginTx(ctx, nil)
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
