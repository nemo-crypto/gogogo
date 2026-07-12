package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"gogogo/internal/item"
)

type Server struct {
	repo     item.Repository
	apiToken string
}

func NewServer(repo item.Repository, apiToken string) *Server {
	return &Server{repo: repo, apiToken: apiToken}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("GET /items", s.listItems)
	mux.HandleFunc("POST /items", s.createItem)
	mux.HandleFunc("GET /items/{id}", s.getItem)
	mux.HandleFunc("PUT /items/{id}", s.updateItem)
	mux.HandleFunc("DELETE /items/{id}", s.deleteItem)
	return logRequests(mux)
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listItems(w http.ResponseWriter, r *http.Request) {
	items, err := s.repo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "list items failed")
		return
	}

	writeJSON(w, http.StatusOK, items)
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}

	current, err := s.repo.Get(r.Context(), id)
	if err != nil {
		if errors.Is(err, item.ErrNotFound) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "get item failed")
		return
	}

	writeJSON(w, http.StatusOK, current)
}

func (s *Server) createItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	var request item.CreateRequest
	if !decodeJSON(w, r, &request) {
		return
	}

	current, err := s.repo.Create(r.Context(), request)
	if err != nil {
		if errors.Is(err, item.ErrInvalidItem) {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		writeError(w, http.StatusInternalServerError, "create item failed")
		return
	}

	writeJSON(w, http.StatusCreated, current)
}

func (s *Server) updateItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	id, ok := parseID(w, r)
	if !ok {
		return
	}

	var request item.UpdateRequest
	if !decodeJSON(w, r, &request) {
		return
	}

	current, err := s.repo.Update(r.Context(), id, request)
	if err != nil {
		switch {
		case errors.Is(err, item.ErrInvalidItem):
			writeError(w, http.StatusBadRequest, "name is required")
		case errors.Is(err, item.ErrNotFound):
			writeError(w, http.StatusNotFound, "item not found")
		default:
			writeError(w, http.StatusInternalServerError, "update item failed")
		}
		return
	}

	writeJSON(w, http.StatusOK, current)
}

func (s *Server) deleteItem(w http.ResponseWriter, r *http.Request) {
	if !s.requireAuth(w, r) {
		return
	}

	id, ok := parseID(w, r)
	if !ok {
		return
	}

	if err := s.repo.Delete(r.Context(), id); err != nil {
		if errors.Is(err, item.ErrNotFound) {
			writeError(w, http.StatusNotFound, "item not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "delete item failed")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireAuth(w http.ResponseWriter, r *http.Request) bool {
	expected := "Bearer " + s.apiToken
	authHeader := r.Header.Get("Authorization")
	if s.apiToken == "" || subtle.ConstantTimeCompare([]byte(authHeader), []byte(expected)) != 1 {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return false
	}
	return true
}

func parseID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(w, http.StatusBadRequest, "invalid item id")
		return 0, false
	}
	return id, true
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) bool {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(target); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json body")
		return false
	}

	return true
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	if err := json.NewEncoder(w).Encode(value); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	writeJSON(w, statusCode, map[string]string{"error": message})
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		next.ServeHTTP(w, r)
	})
}
