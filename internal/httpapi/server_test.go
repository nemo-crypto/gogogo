package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"gogogo/internal/item"
)

func TestItemRoutesCRUD(t *testing.T) {
	t.Parallel()

	server := NewServer(item.NewMemoryRepository(), "test-token")

	createBody := bytes.NewBufferString(`{"name":"phone","description":"demo item"}`)
	createResp := httptest.NewRecorder()
	createReq := httptest.NewRequest(http.MethodPost, "/items", createBody)
	createReq.Header.Set("Authorization", "Bearer test-token")
	server.Routes().ServeHTTP(createResp, createReq)

	if createResp.Code != http.StatusCreated {
		t.Fatalf("create status = %d, want %d, body: %s", createResp.Code, http.StatusCreated, createResp.Body.String())
	}

	var created item.Item
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode created item: %v", err)
	}

	getResp := httptest.NewRecorder()
	getReq := httptest.NewRequest(http.MethodGet, "/items/1", nil)
	server.Routes().ServeHTTP(getResp, getReq)

	if getResp.Code != http.StatusOK {
		t.Fatalf("get status = %d, want %d", getResp.Code, http.StatusOK)
	}

	updateBody := bytes.NewBufferString(`{"name":"laptop","description":"updated item"}`)
	updateResp := httptest.NewRecorder()
	updateReq := httptest.NewRequest(http.MethodPut, "/items/1", updateBody)
	updateReq.Header.Set("Authorization", "Bearer test-token")
	server.Routes().ServeHTTP(updateResp, updateReq)

	if updateResp.Code != http.StatusOK {
		t.Fatalf("update status = %d, want %d, body: %s", updateResp.Code, http.StatusOK, updateResp.Body.String())
	}

	deleteResp := httptest.NewRecorder()
	deleteReq := httptest.NewRequest(http.MethodDelete, "/items/1", nil)
	deleteReq.Header.Set("Authorization", "Bearer test-token")
	server.Routes().ServeHTTP(deleteResp, deleteReq)

	if deleteResp.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want %d", deleteResp.Code, http.StatusNoContent)
	}
}

func TestCreateItemRequiresName(t *testing.T) {
	t.Parallel()

	server := NewServer(item.NewMemoryRepository(), "test-token")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{"description":"missing name"}`))
	req.Header.Set("Authorization", "Bearer test-token")

	server.Routes().ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusBadRequest)
	}
}

func TestListItemsDoesNotRequireAuth(t *testing.T) {
	t.Parallel()

	repo := item.NewMemoryRepository()
	if _, err := repo.Create(context.Background(), item.CreateRequest{Name: "public item"}); err != nil {
		t.Fatalf("seed item: %v", err)
	}

	server := NewServer(repo, "test-token")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/items", nil)

	server.Routes().ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusOK)
	}
}

func TestWriteRoutesRequireAuth(t *testing.T) {
	t.Parallel()

	server := NewServer(item.NewMemoryRepository(), "test-token")

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "create", method: http.MethodPost, path: "/items", body: `{"name":"phone"}`},
		{name: "update", method: http.MethodPut, path: "/items/1", body: `{"name":"phone"}`},
		{name: "delete", method: http.MethodDelete, path: "/items/1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := httptest.NewRecorder()
			req := httptest.NewRequest(tt.method, tt.path, bytes.NewBufferString(tt.body))

			server.Routes().ServeHTTP(resp, req)

			if resp.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
			}
		})
	}
}

func TestWriteRoutesRejectInvalidToken(t *testing.T) {
	t.Parallel()

	server := NewServer(item.NewMemoryRepository(), "test-token")
	resp := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/items", bytes.NewBufferString(`{"name":"phone"}`))
	req.Header.Set("Authorization", "Bearer wrong-token")

	server.Routes().ServeHTTP(resp, req)

	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", resp.Code, http.StatusUnauthorized)
	}
}
