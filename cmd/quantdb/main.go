package main

import (
	"context"
	"log"
	"os"
	"time"

	"gogogo/internal/marketdata"
)

func main() {
	dsn := env("DATABASE_DSN", "data.db")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := marketdata.OpenSQLite(ctx, dsn)
	if err != nil {
		log.Fatalf("open quant sqlite database: %v", err)
	}
	defer db.Close()

	log.Printf("quant market data schema ready, sqlite dsn=%s", dsn)
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
