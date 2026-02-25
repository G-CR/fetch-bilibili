package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"fetch-bilibili/internal/creator"
	"fetch-bilibili/internal/repo"
)

type stubCreatorService struct {
	last   creator.Entry
	result repo.Creator
	err    error
}

func (s *stubCreatorService) Upsert(ctx context.Context, entry creator.Entry) (repo.Creator, error) {
	s.last = entry
	if s.err != nil {
		return repo.Creator{}, s.err
	}
	return s.result, nil
}

func TestHealthz(t *testing.T) {
	r := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ok" {
		t.Fatalf("expected body ok, got %s", w.Body.String())
	}
}

func TestReadyz(t *testing.T) {
	r := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if w.Body.String() != "ready" {
		t.Fatalf("expected body ready, got %s", w.Body.String())
	}
}

func TestNotFound(t *testing.T) {
	r := NewRouter(nil)
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateCreator(t *testing.T) {
	service := &stubCreatorService{
		result: repo.Creator{ID: 1, UID: "123", Name: "name", Platform: "bilibili", Status: "active"},
	}
	r := NewRouter(service)

	body := map[string]string{"uid": "123", "name": "name"}
	payload, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader(payload))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if service.last.UID != "123" {
		t.Fatalf("expected service to be called")
	}
}

func TestCreateCreatorMissingService(t *testing.T) {
	r := NewRouter(nil)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader([]byte(`{"uid":"1"}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}
}

func TestCreateCreatorBadJSON(t *testing.T) {
	service := &stubCreatorService{}
	r := NewRouter(service)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader([]byte("{bad")))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateCreatorMissingFields(t *testing.T) {
	service := &stubCreatorService{}
	r := NewRouter(service)
	req := httptest.NewRequest(http.MethodPost, "/creators", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateCreatorMethodNotAllowed(t *testing.T) {
	service := &stubCreatorService{}
	r := NewRouter(service)
	req := httptest.NewRequest(http.MethodGet, "/creators", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}
