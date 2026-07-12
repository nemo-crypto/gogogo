package item

import (
	"context"
	"database/sql"
	"testing"
)

func TestSQLiteRepositoryCRUD(t *testing.T) {
	t.Parallel()

	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if err := initSQLiteSchema(ctx, db); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	repo := NewSQLiteRepository(db)

	created, err := repo.Create(ctx, CreateRequest{
		Name:        "sqlite item",
		Description: "stored in sqlite",
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.Name != "sqlite item" {
		t.Fatalf("got name = %q, want sqlite item", got.Name)
	}

	updated, err := repo.Update(ctx, created.ID, UpdateRequest{
		Name:        "updated sqlite item",
		Description: "updated in sqlite",
	})
	if err != nil {
		t.Fatalf("update item: %v", err)
	}
	if updated.Name != "updated sqlite item" {
		t.Fatalf("updated name = %q, want updated sqlite item", updated.Name)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("items length = %d, want 1", len(items))
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}
}
