package server

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"
)

type meResponse struct {
	User   string `json:"user"`
	Role   string `json:"role"`
	APIKey string `json:"api_key,omitempty"`
}

var serverLogf = log.Printf

func HandleMe(store *Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := UserFromContext(r.Context())
		apiKey := strings.TrimSpace(os.Getenv("MSCLI_API_KEY"))
		if apiKey == "" {
			serverLogf("warning: /me for user %s from %s returned no api_key because MSCLI_API_KEY is not set", user, r.RemoteAddr)
		}
		store.TouchSession(user)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(meResponse{
			User:   user,
			Role:   RoleFromContext(r.Context()),
			APIKey: apiKey,
		})
	}
}
