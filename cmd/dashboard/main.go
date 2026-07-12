package main

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"os"
	"time"

	"gogogo/internal/dashboard"
	"gogogo/internal/storage"

	_ "modernc.org/sqlite"
)

func main() {
	addr := env("HTTP_ADDR", ":8081")
	dsn := env("DATABASE_DSN", "data.db")
	haltFile := env("HALT_FILE", ".runtime/halt")

	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		log.Fatalf("ping sqlite database: %v", err)
	}
	if err := storage.InitSQLiteSchema(ctx, db); err != nil {
		log.Fatalf("init sqlite schema: %v", err)
	}

	server := dashboard.NewServer(db, haltFile)
	log.Printf("dashboard listening on http://localhost%s, sqlite dsn=%s", addr, dsn)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatalf("dashboard stopped: %v", err)
	}
}

func env(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
