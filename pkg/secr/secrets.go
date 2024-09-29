package secr

import (
	"os"
	"path/filepath"
)

func GetString(key string) (string, error) {
	val, err := os.ReadFile(filepath.Join("secrets", key))
	if err != nil {
		return "", err
	}
	return string(val), nil
}

func GetBytes(key string) ([]byte, error) {
	val, err := os.ReadFile(filepath.Join("secrets", key))
	if err != nil {
		return nil, err
	}
	return val, nil
}
