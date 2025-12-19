package main

import (
	"encoding/json"
	"net/http"

	"github.com/rotisserie/eris"
)

type SyncResponse struct {
	Success               bool   `json:"success"`
	Message               string `json:"message"`
	DuplicateLimitReached bool   `json:"duplicate_limit_reached"`
	SavedCount            int    `json:"saved_count"`
}

func StartServer(addr string) error {
	http.HandleFunc("/api/sync-raw", handleSyncRaw)

	PrintInfoF("Server starting on %s...", addr)
	return http.ListenAndServe(addr, nil)
}

func handleSyncRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	PrintInfoF("Receiving raw GraphQL response from client (%s)...", r.RemoteAddr)

	var fullResponse json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fullResponse); err != nil {
		PrintError(eris.Wrap(err, "Failed to decode raw sync payload"))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := ProcessSyncRaw(fullResponse)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
