package item

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryRepositoryCRUD(t *testing.T) {
	t.Parallel()

	repo := NewMemoryRepository()
	ctx := context.Background()

	created, err := repo.Create(ctx, CreateRequest{
		Name:        "first",
		Description: "first item",
	})
	if err != nil {
		t.Fatalf("create item: %v", err)
	}
	if created.ID != 1 {
		t.Fatalf("created id = %d, want 1", created.ID)
	}

	got, err := repo.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get item: %v", err)
	}
	if got.Name != "first" {
		t.Fatalf("got name = %q, want first", got.Name)
	}

	updated, err := repo.Update(ctx, created.ID, UpdateRequest{
		Name:        "updated",
		Description: "updated item",
	})
	if err != nil {
		t.Fatalf("update item: %v", err)
	}
	if updated.Name != "updated" {
		t.Fatalf("updated name = %q, want updated", updated.Name)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("list items: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("list length = %d, want 1", len(items))
	}

	if err := repo.Delete(ctx, created.ID); err != nil {
		t.Fatalf("delete item: %v", err)
	}

	_, err = repo.Get(ctx, created.ID)
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("get deleted item error = %v, want ErrNotFound", err)
	}
}

func TestMemoryRepositoryRejectsEmptyName(t *testing.T) {
	t.Parallel()

	repo := NewMemoryRepository()

	_, err := repo.Create(context.Background(), CreateRequest{Name: " "})
	if !errors.Is(err, ErrInvalidItem) {
		t.Fatalf("create error = %v, want ErrInvalidItem", err)
	}
}
