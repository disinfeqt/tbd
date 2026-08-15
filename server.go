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
	http.HandleFunc("/api/settings", handleSettings)

	PrintInfoF("TBD is ready — open https://x.com/i/history and keep this window running (listening on http://localhost%s)", addr)
	return http.ListenAndServe(addr, nil)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		PrintError(eris.Wrap(err, "Failed to encode JSON response"))
	}
}

func handleSyncRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	PrintInfo("Received a batch of bookmarks from the browser")

	var fullResponse json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fullResponse); err != nil {
		PrintError(eris.Wrap(err, "Failed to decode raw sync payload"))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := ProcessSyncRaw(fullResponse)
	writeJSON(w, response)
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	if r.Method == http.MethodGet {
		writeJSON(w, CurrentConfig())
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var patch struct {
		MediaDir       *string `json:"media_dir"`
		DownloadVideos *bool   `json:"download_videos"`
		DownloadImages *bool   `json:"download_images"`
	}
	if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
		PrintError(eris.Wrap(err, "Failed to decode settings payload"))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	next := CurrentConfig()
	if patch.MediaDir != nil {
		next.MediaDir = *patch.MediaDir
	}
	if patch.DownloadVideos != nil {
		next.DownloadVideos = *patch.DownloadVideos
	}
	if patch.DownloadImages != nil {
		next.DownloadImages = *patch.DownloadImages
	}

	if err := UpdateConfig(configPath, next); err != nil {
		PrintError(eris.Wrap(err, "Failed to save settings"))
		http.Error(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}

	writeJSON(w, CurrentConfig())
}
