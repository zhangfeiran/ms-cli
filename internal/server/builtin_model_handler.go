package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/vigo999/ms-cli/configs"
)

type builtinModelCredentialPayload struct {
	ID     string `json:"id"`
	APIKey string `json:"api_key"`
}

func HandleBuiltinModelCredential(store *Store, refs []configs.BuiltinModelCredentialRef) http.HandlerFunc {
	lookup := make(map[string]configs.BuiltinModelCredentialRef, len(refs))
	for _, ref := range refs {
		id := strings.TrimSpace(ref.ID)
		if id == "" {
			continue
		}
		lookup[strings.ToLower(id)] = ref
	}

	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		store.TouchSession(user)

		id := strings.ToLower(strings.TrimSpace(r.PathValue("id")))
		ref, ok := lookup[id]
		if !ok {
			http.Error(w, `{"error":"builtin model credential not found"}`, http.StatusNotFound)
			return
		}
		if strings.TrimSpace(ref.APIKey) == "" {
			http.Error(w, `{"error":"builtin model credential is empty"}`, http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(builtinModelCredentialPayload{
			ID:     ref.ID,
			APIKey: ref.APIKey,
		})
	}
}
