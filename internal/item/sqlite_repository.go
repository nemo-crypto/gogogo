package item

import (
	"context"
	"database/sql"
	"errors"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteRepository struct {
	db *sql.DB
}

func NewSQLiteRepository(db *sql.DB) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func OpenSQLite(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}

	if err := initSQLiteSchema(ctx, db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func initSQLiteSchema(ctx context.Context, db *sql.DB) error {
	_, err := db.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	created_at DATETIME NOT NULL,
	updated_at DATETIME NOT NULL
);
`)
	return err
}

func (r *SQLiteRepository) List(ctx context.Context) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `
SELECT id, name, description, created_at, updated_at
FROM items
ORDER BY id ASC;
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]Item, 0)
	for rows.Next() {
		current, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, current)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *SQLiteRepository) Get(ctx context.Context, id int64) (Item, error) {
	row := r.db.QueryRowContext(ctx, `
SELECT id, name, description, created_at, updated_at
FROM items
WHERE id = ?;
`, id)

	current, err := scanItem(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Item{}, ErrNotFound
		}
		return Item{}, err
	}

	return current, nil
}

func (r *SQLiteRepository) Create(ctx context.Context, request CreateRequest) (Item, error) {
	if err := request.Validate(); err != nil {
		return Item{}, err
	}

	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
INSERT INTO items (name, description, created_at, updated_at)
VALUES (?, ?, ?, ?);
`, request.Name, request.Description, now, now)
	if err != nil {
		return Item{}, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return Item{}, err
	}

	return Item{
		ID:          id,
		Name:        request.Name,
		Description: request.Description,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (r *SQLiteRepository) Update(ctx context.Context, id int64, request UpdateRequest) (Item, error) {
	if err := request.Validate(); err != nil {
		return Item{}, err
	}

	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `
UPDATE items
SET name = ?, description = ?, updated_at = ?
WHERE id = ?;
`, request.Name, request.Description, now, id)
	if err != nil {
		return Item{}, err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return Item{}, err
	}
	if affected == 0 {
		return Item{}, ErrNotFound
	}

	return r.Get(ctx, id)
}

func (r *SQLiteRepository) Delete(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `
DELETE FROM items
WHERE id = ?;
`, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ErrNotFound
	}

	return nil
}

type itemScanner interface {
	Scan(dest ...any) error
}

func scanItem(scanner itemScanner) (Item, error) {
	var current Item
	if err := scanner.Scan(
		&current.ID,
		&current.Name,
		&current.Description,
		&current.CreatedAt,
		&current.UpdatedAt,
	); err != nil {
		return Item{}, err
	}
	return current, nil
}
