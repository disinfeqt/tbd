// Package server exposes the local HTTP API used by the userscript and the
// bookmark explorer dashboard.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/syncer"
)

func Start(addr string) error {
	http.HandleFunc("/api/sync-raw", handleSyncRaw)
	http.HandleFunc("/api/settings", handleSettings)
	registerExploreRoutes()

	logx.Infof("TBD is ready — dashboard at http://localhost:41008 · keep this window running")
	return http.ListenAndServe(addr, nil)
}

func writeJSON(w http.ResponseWriter, payload any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		logx.Error(eris.Wrap(err, "Failed to encode JSON response"))
	}
}

func handleSyncRaw(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	logx.Info("Received a batch of bookmarks from the browser")

	var fullResponse json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fullResponse); err != nil {
		logx.Error(eris.Wrap(err, "Failed to decode raw sync payload"))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := syncer.ProcessSyncRaw(fullResponse)
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
		writeJSON(w, config.Current())
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
		logx.Error(eris.Wrap(err, "Failed to decode settings payload"))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	next := config.Current()
	if patch.MediaDir != nil {
		next.MediaDir = *patch.MediaDir
	}
	if patch.DownloadVideos != nil {
		next.DownloadVideos = *patch.DownloadVideos
	}
	if patch.DownloadImages != nil {
		next.DownloadImages = *patch.DownloadImages
	}

	if err := config.Update(config.DefaultPath, next); err != nil {
		logx.Error(eris.Wrap(err, "Failed to save settings"))
		http.Error(w, "Failed to save settings", http.StatusInternalServerError)
		return
	}

	writeJSON(w, config.Current())
}
