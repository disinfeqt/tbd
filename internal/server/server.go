// Package server exposes the local HTTP API used by the userscript and the
// bookmark explorer dashboard.
package server

import (
	"encoding/json"
	"net/http"

	"github.com/rotisserie/eris"

	tbd "twitter-bookmarks-downloader"
	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/syncer"
)

func Start(addr string) error {
	http.HandleFunc("/api/sync-raw", handleSyncRaw)
	http.HandleFunc("/api/settings", handleSettings)
	http.HandleFunc("/tbd.user.js", handleUserscriptFile)
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

	accountID := noteUserscript(r, true)
	logx.Info("Received a batch of bookmarks from the browser")

	var fullResponse json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&fullResponse); err != nil {
		logx.Error(eris.Wrap(err, "Failed to decode raw sync payload"))
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	response := syncer.ProcessSyncRaw(fullResponse, accountID)
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

	// Settings belong to the account the browser is signed in as; the global
	// config is only the fallback and the template for new accounts.
	accountID := noteUserscript(r, false)

	if r.Method == http.MethodGet {
		writeJSON(w, settingsPayload(accountID))
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

	if _, known := accounts.Get(accountID); known {
		if _, err := accounts.Update(accountID, patch.MediaDir, patch.DownloadVideos, patch.DownloadImages); err != nil {
			logx.Error(eris.Wrap(err, "Failed to save account settings"))
			http.Error(w, "Failed to save settings", http.StatusInternalServerError)
			return
		}
		writeJSON(w, settingsPayload(accountID))
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

	writeJSON(w, settingsPayload(accountID))
}

// settingsPayload keeps the shape the userscript expects and adds the account
// the values came from, so the panel can name it.
func settingsPayload(accountID string) map[string]any {
	current := config.Current()
	payload := map[string]any{
		"media_dir":       current.MediaDir,
		"download_videos": current.DownloadVideos,
		"download_images": current.DownloadImages,
		// The script asks on every page load, which is the only moment it can
		// find out it is older than the app it is talking to.
		"latest_script_version": tbd.UserscriptVersion(),
	}
	if account, ok := accounts.Get(accountID); ok {
		payload["media_dir"] = accounts.MediaDir(account.ID)
		payload["download_videos"] = account.DownloadVideos
		payload["download_images"] = account.DownloadImages
		payload["account"] = map[string]any{"id": account.ID, "handle": account.Handle}
	}
	return payload
}
