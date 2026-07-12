package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"gogogo/internal/marketdata"
	"gogogo/internal/storage"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		dsn   = flag.String("dsn", env("DATABASE_DSN", "/Users/guilinzhou/Desktop/test-nemo/gogogo/data.db"), "sqlite database path")
		since = flag.String("since", time.Now().UTC().Add(-24*time.Hour).Format(time.RFC3339), "report start time")
	)
	flag.Parse()
	start, err := time.Parse(time.RFC3339, *since)
	if err != nil {
		return fmt.Errorf("parse since: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	db, err := marketdata.OpenSQLite(ctx, *dsn)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	defer db.Close()
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		return fmt.Errorf("init schema: %w", err)
	}

	fmt.Printf("daily_report since=%s\n", start.Format(time.RFC3339))
	for _, item := range []struct {
		name  string
		query string
	}{
		{"orders", "SELECT COUNT(*) FROM orders WHERE created_at >= ?"},
		{"risk_events", "SELECT COUNT(*) FROM risk_events WHERE created_at >= ?"},
		{"signals", "SELECT COUNT(*) FROM signals WHERE created_at >= ?"},
		{"strategy_runs", "SELECT COUNT(*) FROM strategy_runs WHERE created_at >= ?"},
		{"performance_snapshots", "SELECT COUNT(*) FROM performance_snapshots WHERE created_at >= ?"},
		{"balances", "SELECT COUNT(*) FROM balances WHERE created_at >= ?"},
		{"positions", "SELECT COUNT(*) FROM positions WHERE created_at >= ?"},
		{"margin_snapshots", "SELECT COUNT(*) FROM margin_snapshots WHERE created_at >= ?"},
	} {
		count, err := scalarCount(ctx, db, item.query, start)
		if err != nil {
			return fmt.Errorf("count %s: %w", item.name, err)
		}
		fmt.Printf("%s=%d\n", item.name, count)
	}
	return nil
}

func scalarCount(ctx context.Context, db *sql.DB, query string, arg any) (int64, error) {
	var count int64
	if err := db.QueryRowContext(ctx, query, arg).Scan(&count); err != nil {
		return 0, err
	}
	return count, nil
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
