package main

import (
	"context"
	"gogogo/internal/config"
	"log"
	"time"

	"gogogo/internal/marketdata"
	"gogogo/internal/storage"
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
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		log.Fatalf("init full quant sqlite schema: %v", err)
	}

	log.Printf("quant sqlite schema ready, sqlite dsn=%s", dsn)
}

func env(key, fallback string) string {
	return config.Env(key, fallback)
}
