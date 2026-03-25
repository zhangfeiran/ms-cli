package server

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/vigo999/ms-cli/configs"
)

func TestBuiltinModelCredentialRouteReturnsConfiguredKey(t *testing.T) {
	store := newTestStore(t)
	mux := NewMux(store, []configs.TokenEntry{
		{Token: "member-token", User: "alice", Role: "member"},
	}, []configs.BuiltinModelCredentialRef{
		{ID: "kimi-k2.5", APIKey: "free-key"},
	})

	req := httptest.NewRequest(http.MethodGet, "/builtin-models/kimi-k2.5/credential", nil)
	req.Header.Set("Authorization", "Bearer member-token")
	resp := httptest.NewRecorder()

	mux.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}

	var payload struct {
		ID     string `json:"id"`
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got, want := payload.ID, "kimi-k2.5"; got != want {
		t.Fatalf("id = %q, want %q", got, want)
	}
	if got, want := payload.APIKey, "free-key"; got != want {
		t.Fatalf("api_key = %q, want %q", got, want)
	}
}

func TestBuiltinModelCredentialRouteRejectsUnknownPreset(t *testing.T) {
	store := newTestStore(t)
	mux := NewMux(store, []configs.TokenEntry{
		{Token: "member-token", User: "alice", Role: "member"},
	}, []configs.BuiltinModelCredentialRef{
		{ID: "kimi-k2.5", APIKey: "free-key"},
	})

	req := httptest.NewRequest(http.MethodGet, "/builtin-models/unknown/credential", nil)
	req.Header.Set("Authorization", "Bearer member-token")
	resp := httptest.NewRecorder()

	mux.ServeHTTP(resp, req)

	if got, want := resp.Code, http.StatusNotFound; got != want {
		t.Fatalf("status = %d, want %d", got, want)
	}
}

func newTestStore(t *testing.T) *Store {
	t.Helper()

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return store
}
