package config

import (
	"bufio"
	"os"
	"strings"
)

func Env(key string, fallback string) string {
	loadLocalEnv()
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

var localEnvLoaded bool

func loadLocalEnv() {
	if localEnvLoaded {
		return
	}
	localEnvLoaded = true
	for _, path := range []string{".env.local", ".env"} {
		_ = loadEnvFile(path)
	}
}

func loadEnvFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		if key == "" || os.Getenv(key) != "" {
			continue
		}
		_ = os.Setenv(key, value)
	}
	return scanner.Err()
}
