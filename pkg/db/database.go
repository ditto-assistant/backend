package db

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	"github.com/tursodatabase/go-libsql"
)

type ConnectionMode string

const (
	ModeCloud   ConnectionMode = "cloud"
	ModeReplica ConnectionMode = "replica"
)

var D *sql.DB

func Setup(ctx context.Context, shutdown *sync.WaitGroup, mode ConnectionMode) error {
	shutdown.Add(1)
	if envs.DITTO_ENV == envs.EnvLocal {
		return setupCloud(ctx, shutdown)
	}

	switch mode {
	case ModeCloud:
		return setupCloud(ctx, shutdown)
	case ModeReplica:
		return setupReplica(ctx, shutdown)
	default:
		return fmt.Errorf("invalid connection mode: %s", mode)
	}
}

func setupCloud(ctx context.Context, shutdown *sync.WaitGroup) error {
	dbUrl := envs.DB_URL_DITTO
	if envs.DITTO_ENV != envs.EnvLocal {
		dbUrl += "?authToken=" + secr.TURSO_AUTH_TOKEN.String()
	}

	var err error
	D, err = sql.Open("libsql", dbUrl)
	if err != nil {
		return fmt.Errorf("error opening cloud db: %w", err)
	}
	D.SetConnMaxIdleTime(9 * time.Second)

	go func() {
		<-ctx.Done()
		err := D.Close()
		if err != nil {
			slog.Error("error closing libsql db", "error", err)
		} else {
			slog.Debug("closed libsql db")
		}
		shutdown.Done()
	}()

	slog.Debug("db connected in cloud mode", "url", envs.DB_URL_DITTO)
	return nil
}

func setupReplica(ctx context.Context, shutdown *sync.WaitGroup) error {
	dbName := "local.db"
	dir, err := os.MkdirTemp("", "libsql-*")
	if err != nil {
		return fmt.Errorf("error creating temporary directory: %w", err)
	}
	dbPath := filepath.Join(dir, dbName)
	slog.Debug("creating embedded replica connector", "url", envs.DB_URL_DITTO)
	connector, err := libsql.NewEmbeddedReplicaConnector(dbPath, envs.DB_URL_DITTO,
		libsql.WithEncryption(secr.LIBSQL_ENCRYPTION_KEY.String()),
		libsql.WithSyncInterval(time.Minute),
		libsql.WithAuthToken(secr.TURSO_AUTH_TOKEN.String()),
	)
	if err != nil {
		return fmt.Errorf("error creating connector: %w", err)
	}
	D = sql.OpenDB(connector)
	slog.Debug("db connected in replica mode", "url", envs.DB_URL_DITTO)

	go func() {
		<-ctx.Done()
		slog.Debug("shutting down libsql db")
		D.Close()
		connector.Close()
		os.RemoveAll(dir)
		shutdown.Done()
	}()

	return nil
}
