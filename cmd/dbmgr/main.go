package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sync"

	"github.com/ditto-assistant/backend/pkg/db"
	"github.com/ditto-assistant/backend/pkg/secr"
)

type Mode int

const (
	ModeIngest Mode = iota
)

var (
	folder string
	mode   Mode
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if len(os.Args) < 2 {
		log.Fatalf("usage: %s <command> [args]", os.Args[0])
	}

	if os.Args[1] == "ingest" {
		mode = ModeIngest
		fset := flag.NewFlagSet("ingest", flag.ExitOnError)
		fset.StringVar(&folder, "folder", "cmd/dbmgr/prompt-examples", "folder to ingest")
		fset.Parse(os.Args[2:])
		slog.Debug("ingest mode", "folder", folder)
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
	}
	cancel()
	shutdown.Wait()
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
