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
	"github.com/firebase/genkit/go/ai"
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
	go func() {
		<-ctx.Done()
		slog.Debug("shutting down libsql db")
		db.Close()
		connector.Close()
		os.RemoveAll(dir)
		shutdown.Done()
	}()

	ai.DefineIndexer("custom", "example-indexer", func(ctx context.Context, ir *ai.IndexerRequest) error {
		return nil
	})

	return nil
}

func createTables(ctx context.Context) error {
	return nil
}

const createExampleStore = `
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
