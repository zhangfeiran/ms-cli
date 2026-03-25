package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
	"github.com/vigo999/ms-cli/configs"
)

func TestHandleMe_IncludesServerAPIKey(t *testing.T) {
	t.Setenv("MSCLI_API_KEY", "server-runtime-key")

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	NewMux(store, []configs.TokenEntry{
		{Token: "test-token", User: "alice", Role: "member"},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status code = %d, want %d", got, want)
	}

	var got struct {
		User   string `json:"user"`
		Role   string `json:"role"`
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode /me response: %v", err)
	}
	if got.User != "alice" || got.Role != "member" {
		t.Fatalf("unexpected identity payload: %+v", got)
	}
	if got.APIKey != "server-runtime-key" {
		t.Fatalf("api_key = %q, want %q", got.APIKey, "server-runtime-key")
	}
}

func TestHandleMe_LogsWhenServerAPIKeyMissing(t *testing.T) {
	t.Setenv("MSCLI_API_KEY", "")

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite db: %v", err)
	}
	defer db.Close()

	store, err := NewStore(db)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}

	var gotLog string
	origLogf := serverLogf
	serverLogf = func(format string, args ...any) {
		gotLog = fmt.Sprintf(format, args...)
	}
	defer func() { serverLogf = origLogf }()

	req := httptest.NewRequest(http.MethodGet, "/me", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	req.Header.Set("Authorization", "Bearer test-token")
	rec := httptest.NewRecorder()

	NewMux(store, []configs.TokenEntry{
		{Token: "test-token", User: "alice", Role: "member"},
	}).ServeHTTP(rec, req)

	if got, want := rec.Code, http.StatusOK; got != want {
		t.Fatalf("status code = %d, want %d", got, want)
	}
	if !strings.Contains(gotLog, "returned no api_key") {
		t.Fatalf("log = %q, want missing api_key diagnostic", gotLog)
	}
	if !strings.Contains(gotLog, "MSCLI_API_KEY is not set") {
		t.Fatalf("log = %q, want MSCLI_API_KEY hint", gotLog)
	}
}
