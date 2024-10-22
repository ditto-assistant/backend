package envs

import (
	"bufio"
	"embed"
	"errors"
	"log/slog"
	"os"
	"strings"
)

//go:embed .env.*
var fs embed.FS

// Environment Variables
var (
	PROJECT_ID     string
	DITTO_ENV      Env
	DB_URL_DITTO   string
	GCLOUD_PROJECT string
)

type Env string

const (
	EnvLocal   Env = "local"
	EnvStaging Env = "staging"
	EnvProd    Env = "prod"
)

func (e Env) String() string {
	return string(e)
}

type EnvFile string

func (e Env) EnvFile() EnvFile {
	return EnvFile(".env." + e.String())
}

func (e EnvFile) Load() error {
	file, err := fs.Open(string(e))
	if err != nil {
		return err
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		os.Setenv(key, value)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	return nil
}

var didLoad = false

func Load() error {
	if didLoad {
		return nil
	}
	env, ok := os.LookupEnv("DITTO_ENV")
	if !ok {
		slog.Info("DITTO_ENV not set, using local")
		env = "local"
	}
	DITTO_ENV = Env(env)
	_, ok = os.LookupEnv("GCLOUD_PROJECT")
	if !ok {
		eFile := DITTO_ENV.EnvFile()
		slog.Info("Loading environment from file", "file", eFile)
		err := eFile.Load()
		if err != nil {
			return err
		}
	}
	PROJECT_ID, ok = os.LookupEnv("PROJECT_ID")
	if !ok {
		return errors.New("env PROJECT_ID not set")
	}
	DB_URL_DITTO, ok = os.LookupEnv("DB_URL_DITTO")
	if !ok {
		return errors.New("env DB_URL_DITTO not set")
	}
	GCLOUD_PROJECT, ok = os.LookupEnv("GCLOUD_PROJECT")
	if !ok {
		return errors.New("env GCLOUD_PROJECT not set")
	}
	didLoad = true
	slog.Debug("Loaded environment variables", "PROJECT_ID", PROJECT_ID, "DITTO_ENV", DITTO_ENV, "DB_URL_DITTO", DB_URL_DITTO, "GCLOUD_PROJECT", GCLOUD_PROJECT)
	return nil
}
