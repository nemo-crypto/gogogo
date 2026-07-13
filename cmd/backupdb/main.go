package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"gogogo/internal/config"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	source := env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db")
	backupDir := env("BACKUP_PATH", "./backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	target := filepath.Join(backupDir, fmt.Sprintf("data-%s.db", time.Now().UTC().Format("20060102T150405Z")))
	if err := copyFile(source, target); err != nil {
		return err
	}
	fmt.Printf("backup_created source=%s target=%s\n", source, target)
	return nil
}

func copyFile(source string, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return fmt.Errorf("open source database: %w", err)
	}
	defer in.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create backup database: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy database: %w", err)
	}
	return out.Sync()
}

func env(key string, fallback string) string {
	return config.Env(key, fallback)
}
