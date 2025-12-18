package main

import (
	"encoding/json"
	"net/http"

	"github.com/rotisserie/eris"
)

// InputTweet represents the simplified JSON structure for indexing
type InputTweet struct {
	ID           string       `json:"id_str"`
	FullText     string       `json:"full_text"`
	Name         string       `json:"name"`
	ScreenName   string       `json:"screen_name"`
	CreatedAt    int64        `json:"created_at"` // Unix timestamp
	PermanentURL string       `json:"permanent_url"`
	Media        []InputMedia `json:"media"`
}

type InputMedia struct {
	ID   string `json:"id_str"`
	Type string `json:"type"` // photo, video, animated_gif
	URL  string `json:"media_url_https"`
}

type SyncResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	StopRequired bool   `json:"stop_required"`
	SavedCount   int    `json:"saved_count"`
}

func StartServer() {
	// Enable CORS for Userscript
	http.HandleFunc("/api/sync", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "https://x.com")
		w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		handleSync(w, r)
	})

	PrintInfo("Server started at http://localhost:8080")
	if err := http.ListenAndServe(":8080", nil); err != nil {
		FatalError(eris.Wrap(err, "Server failed"))
	}
}

func handleSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 核心优化：接收原始 JSON 消息片段数组
	var rawMessages []json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawMessages); err != nil {
		http.Error(w, "Invalid JSON: "+err.Error(), http.StatusBadRequest)
		return
	}

	response := ProcessSync(rawMessages)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
