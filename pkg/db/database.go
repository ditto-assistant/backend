package db

import (
	"context"
	"database/sql"
	"log/slog"
	"sync"
	"time"

	"github.com/ditto-assistant/backend/cfg/envs"
	"github.com/ditto-assistant/backend/cfg/secr"
	_ "github.com/tursodatabase/go-libsql"
)

var D *sql.DB

func Setup(ctx context.Context, shutdown *sync.WaitGroup) (err error) {
	shutdown.Add(1)
	dbUrl := envs.DB_URL_DITTO
	if envs.DITTO_ENV != envs.EnvLocal {
		dbUrl += "?authToken=" + secr.TURSO_AUTH_TOKEN.String()
	}
	D, err = sql.Open("libsql", dbUrl)
	if err != nil {
		return
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
	return nil
}
