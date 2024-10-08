package envs

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// Environment Variables
var (
	PROJECT_ID   string
	DITTO_ENV    Env
	DB_URL_DITTO string
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
	file, err := os.Open(string(e))
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
	var ok bool
	PROJECT_ID, ok = os.LookupEnv("PROJECT_ID")
	if !ok {
		return errors.New("env PROJECT_ID not set")
	}
	env, ok := os.LookupEnv("DITTO_ENV")
	if !ok {
		return errors.New("env DITTO_ENV not set")
	}
	DITTO_ENV = Env(env)
	DB_URL_DITTO, ok = os.LookupEnv("DB_URL_DITTO")
	if !ok {
		return errors.New("env DB_URL_DITTO not set")
	}
	didLoad = true
	return nil
}
