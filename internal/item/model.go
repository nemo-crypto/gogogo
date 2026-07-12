package item

import (
	"errors"
	"strings"
	"time"
)

var ErrInvalidItem = errors.New("invalid item")

type Item struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type UpdateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (r CreateRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return ErrInvalidItem
	}
	return nil
}

func (r UpdateRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return ErrInvalidItem
	}
	return nil
}
