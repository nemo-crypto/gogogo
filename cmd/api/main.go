package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"gogogo/internal/httpapi"
	"gogogo/internal/item"
)

func main() {
	addr := env("HTTP_ADDR", ":8080")
	dsn := env("DATABASE_DSN", "data.db")
	apiToken := os.Getenv("API_TOKEN")
	if apiToken == "" {
		log.Fatal("API_TOKEN is required")
	}

	db, err := item.OpenSQLite(context.Background(), dsn)
	if err != nil {
		log.Fatalf("open sqlite database: %v", err)
	}
	defer db.Close()

	repo := item.NewSQLiteRepository(db)
	server := httpapi.NewServer(repo, apiToken)

	log.Printf("server listening on %s, sqlite dsn=%s", addr, dsn)
	if err := http.ListenAndServe(addr, server.Routes()); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

func env(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
