package server

import (
	"net/http"
	"strings"
	"sync"
	"time"

	tbd "twitter-bookmarks-downloader"
	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/logx"
)

// The userscript is the only thing that can reach these endpoints, so a call
// from it is proof it is installed, running, and able to talk to us. That is
// what the explorer's setup view reports on.
var (
	scriptMu      sync.RWMutex
	scriptSeenAt  time.Time
	scriptVersion string
)

// knownScriptVersion is the version of the newest script that has called in
// since TBD started; empty until one does.
func knownScriptVersion() string {
	scriptMu.RLock()
	defer scriptMu.RUnlock()
	return scriptVersion
}

// noteUserscript records a call from the userscript and returns the account the
// request belongs to. An unidentified call falls back to the most recently
// synced account so a script too old to send its account still works.
func noteUserscript(r *http.Request, synced bool) string {
	if version := strings.TrimSpace(r.Header.Get("X-TBD-Version")); version != "" {
		scriptMu.Lock()
		scriptSeenAt = time.Now()
		scriptVersion = version
		scriptMu.Unlock()
	}

	id := strings.TrimSpace(r.Header.Get("X-TBD-Account"))
	handle := strings.TrimPrefix(strings.TrimSpace(r.Header.Get("X-TBD-Handle")), "@")
	if id == "" {
		id = accounts.Resolve("")
		if id == "" {
			if !synced {
				// A settings read that names no account and finds none on
				// record has nothing to attach to; the global defaults answer it.
				return ""
			}
			// Bookmarks have to land somewhere: park them under the legacy
			// account, which the first identified sync adopts.
			id = accounts.LegacyID
		}
	}

	record := accounts.Ensure
	if synced {
		record = accounts.Touch
	}
	if _, err := record(id, handle); err != nil {
		logx.Error(err)
	}
	return id
}

// userscriptStatus is what the explorer's setup view renders.
type userscriptStatus struct {
	Installed     bool       `json:"installed"` // has ever called in since TBD started
	Version       string     `json:"version"`
	LatestVersion string     `json:"latest_version"`
	Outdated      bool       `json:"outdated"`
	SeenAt        *time.Time `json:"seen_at"`
}

func currentUserscriptStatus() userscriptStatus {
	scriptMu.RLock()
	seenAt, version := scriptSeenAt, scriptVersion
	scriptMu.RUnlock()

	latest := tbd.UserscriptVersion()
	return userscriptStatus{
		Installed:     !seenAt.IsZero(),
		Version:       version,
		LatestVersion: latest,
		Outdated:      version != "" && latest != "" && version != latest,
		SeenAt:        nonZeroTime(seenAt),
	}
}

// handleUserscriptFile serves the script itself. Tampermonkey recognises a
// .user.js URL and opens its install prompt, so the setup view can link
// straight at it instead of asking for copy and paste.
func handleUserscriptFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	_, _ = w.Write([]byte(tbd.Userscript))
}
