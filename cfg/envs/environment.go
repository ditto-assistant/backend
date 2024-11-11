package envs

import (
	"bufio"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

//go:embed .env.*
var fs embed.FS

// Environment Variables
var (
	BACKBLAZE_KEY_ID       string
	PROJECT_ID             string
	DALL_E_PREFIX          string
	DITTO_ENV              Env
	DITTO_CONTENT_ENDPOINT string
	DITTO_CONTENT_REGION   string
	DITTO_CONTENT_BUCKET   string
	DITTO_CONTENT_PREFIX   string
	DB_URL_DITTO           string
	GCLOUD_PROJECT         string
	PRICE_ID_TOKENS_1B     string
	PRICE_ID_TOKENS_11B    string
	PRICE_ID_TOKENS_30B    string
	PRICE_ID_TOKENS_100B   string
	PRICE_ID_TOKENS_150B   string
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
		// check if .env.common exists
		if _, err := fs.Open(".env.common"); err == nil {
			slog.Info("Loading common environment variables")
			err := EnvFile(".env.common").Load()
			if err != nil {
				return err
			}
		}
	}
	envs := []envLookup{
		{&BACKBLAZE_KEY_ID, "BACKBLAZE_KEY_ID"},
		{&PROJECT_ID, "PROJECT_ID"},
		{&DALL_E_PREFIX, "DALL_E_PREFIX"},
		{&DITTO_CONTENT_ENDPOINT, "DITO_CONTENT_ENDPOINT"},
		{&DITTO_CONTENT_REGION, "DITO_CONTENT_REGION"},
		{&DITTO_CONTENT_BUCKET, "DITO_CONTENT_BUCKET"},
		{&DITTO_CONTENT_PREFIX, "DITO_CONTENT_PREFIX"},
		{&DB_URL_DITTO, "DB_URL_DITTO"},
		{&GCLOUD_PROJECT, "GCLOUD_PROJECT"},
		{&PRICE_ID_TOKENS_1B, "PRICE_ID_TOKENS_1B"},
		{&PRICE_ID_TOKENS_11B, "PRICE_ID_TOKENS_11B"},
		{&PRICE_ID_TOKENS_30B, "PRICE_ID_TOKENS_30B"},
		{&PRICE_ID_TOKENS_100B, "PRICE_ID_TOKENS_100B"},
		{&PRICE_ID_TOKENS_150B, "PRICE_ID_TOKENS_150B"},
	}
	if err := lookupEnvs(envs); err != nil {
		return err
	}
	didLoad = true
	slog.Debug("Loaded environment variables",
		"PROJECT_ID", PROJECT_ID,
		"DITTO_ENV", DITTO_ENV,
		"DB_URL_DITTO", DB_URL_DITTO,
		"GCLOUD_PROJECT", GCLOUD_PROJECT,
	)
	return nil
}

type envLookup struct {
	ptr *string
	key string
}

func lookupEnvs(envs []envLookup) error {
	var errorSlice []error
	for _, env := range envs {
		val, err := lookupEnv(env.key)
		if err != nil {
			errorSlice = append(errorSlice, err)
		}
		*env.ptr = val
	}
	if len(errorSlice) > 0 {
		return fmt.Errorf("failed to lookup envs: %w", errors.Join(errorSlice...))
	}
	return nil
}

func lookupEnv(key string) (string, error) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("env %s not set", key)
	}
	return val, nil
}
