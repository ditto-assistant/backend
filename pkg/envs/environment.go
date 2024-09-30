package envs

import (
	"fmt"
	"os"
)

// Environment Variables
var (
	PROJECT_ID            string
	DITTO_ENV             Env
	DB_URL_DITTO_EXAMPLES string
)

type Env string

const (
	EnvLocal Env = "local"
	EnvDev   Env = "dev"
	EnvProd  Env = "prod"
)

var didLoad = false

func Load() error {
	if didLoad {
		return nil
	}
	var ok bool
	PROJECT_ID, ok = os.LookupEnv("PROJECT_ID")
	if !ok {
		return fmt.Errorf("env PROJECT_ID not set")
	}
	env, ok := os.LookupEnv("DITTO_ENV")
	if !ok {
		return fmt.Errorf("env DITTO_ENV not set")
	}
	DITTO_ENV = Env(env)
	DB_URL_DITTO_EXAMPLES, ok = os.LookupEnv("DB_URL_DITTO_EXAMPLES")
	if !ok {
		return fmt.Errorf("env DB_URL_DITTO_EXAMPLES not set")
	}
	didLoad = true
	return nil
}
