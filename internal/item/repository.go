package item

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"
)

var ErrNotFound = errors.New("item not found")

type Repository interface {
	List(ctx context.Context) ([]Item, error)
	Get(ctx context.Context, id int64) (Item, error)
	Create(ctx context.Context, request CreateRequest) (Item, error)
	Update(ctx context.Context, id int64, request UpdateRequest) (Item, error)
	Delete(ctx context.Context, id int64) error
}

type MemoryRepository struct {
	mu     sync.RWMutex
	nextID int64
	items  map[int64]Item
}

func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{
		nextID: 1,
		items:  make(map[int64]Item),
	}
}

func (r *MemoryRepository) List(ctx context.Context) ([]Item, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	items := make([]Item, 0, len(r.items))
	for _, current := range r.items {
		items = append(items, current)
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].ID < items[j].ID
	})

	return items, nil
}

func (r *MemoryRepository) Get(ctx context.Context, id int64) (Item, error) {
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	current, ok := r.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}

	return current, nil
}

func (r *MemoryRepository) Create(ctx context.Context, request CreateRequest) (Item, error) {
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	if err := request.Validate(); err != nil {
		return Item{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	current := Item{
		ID:          r.nextID,
		Name:        request.Name,
		Description: request.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	r.items[current.ID] = current
	r.nextID++

	return current, nil
}

func (r *MemoryRepository) Update(ctx context.Context, id int64, request UpdateRequest) (Item, error) {
	if err := ctx.Err(); err != nil {
		return Item{}, err
	}
	if err := request.Validate(); err != nil {
		return Item{}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	current, ok := r.items[id]
	if !ok {
		return Item{}, ErrNotFound
	}

	current.Name = request.Name
	current.Description = request.Description
	current.UpdatedAt = time.Now().UTC()
	r.items[id] = current

	return current, nil
}

func (r *MemoryRepository) Delete(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.items[id]; !ok {
		return ErrNotFound
	}

	delete(r.items, id)
	return nil
}
