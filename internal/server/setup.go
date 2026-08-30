package server

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/download"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
)

type accountView struct {
	ID     string `json:"id"`
	Handle string `json:"handle"`
	// MediaDir is the stored setting — empty means "follow the global default".
	// MediaDirResolved is where files actually land right now; the settings
	// form must edit the stored value, or one save would pin the default.
	MediaDir         string     `json:"media_dir"`
	MediaDirResolved string     `json:"media_dir_resolved"`
	DownloadVideos   bool       `json:"download_videos"`
	DownloadImages   bool       `json:"download_images"`
	Bookmarks        int64      `json:"bookmarks"`
	LastSyncAt       *time.Time `json:"last_sync_at"`
}

// accountViews lists every account with the size of its archive, newest sync
// first — the switcher and the settings view both render from this.
func accountViews() ([]accountView, error) {
	var counts []struct {
		AccountID string
		Total     int64
	}
	if err := store.DB.Model(&store.AccountBookmarkModel{}).
		Select("account_id, COUNT(*) AS total").
		Group("account_id").
		Find(&counts).Error; err != nil {
		return nil, eris.Wrap(err, "failed to count bookmarks per account")
	}
	byAccount := make(map[string]int64, len(counts))
	for _, row := range counts {
		byAccount[row.AccountID] = row.Total
	}

	rows := accounts.All()
	views := make([]accountView, 0, len(rows))
	for _, row := range rows {
		views = append(views, accountView{
			ID:               row.ID,
			Handle:           row.Handle,
			MediaDir:         row.MediaDir,
			MediaDirResolved: accounts.MediaDir(row.ID),
			DownloadVideos:   row.DownloadVideos,
			DownloadImages:   row.DownloadImages,
			Bookmarks:        byAccount[row.ID],
			LastSyncAt:       nonZeroTime(row.LastSyncAt),
		})
	}
	return views, nil
}

// handleExploreAccounts lists the accounts and saves changes to one of them.
// An empty id in a POST edits the global defaults new accounts start from.
func handleExploreAccounts(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeAccounts(w)
	case http.MethodPost:
		var patch struct {
			ID             string  `json:"id"`
			MediaDir       *string `json:"media_dir"`
			DownloadVideos *bool   `json:"download_videos"`
			DownloadImages *bool   `json:"download_images"`
		}
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, "Invalid JSON", http.StatusBadRequest)
			return
		}

		if patch.ID == "" {
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
				logx.Error(eris.Wrap(err, "Failed to save default settings"))
				http.Error(w, "Failed to save settings", http.StatusInternalServerError)
				return
			}
			writeAccounts(w)
			return
		}

		if _, err := accounts.Update(patch.ID, patch.MediaDir, patch.DownloadVideos, patch.DownloadImages); err != nil {
			logx.Error(eris.Wrap(err, "Failed to save account settings"))
			http.Error(w, "Failed to save settings", http.StatusInternalServerError)
			return
		}
		writeAccounts(w)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func writeAccounts(w http.ResponseWriter) {
	views, err := accountViews()
	if err != nil {
		logx.Error(err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	current := config.Current()
	writeJSON(w, map[string]any{
		"items":   views,
		"default": accounts.Default(),
		"defaults": map[string]any{
			"media_dir":       current.MediaDir,
			"download_videos": current.DownloadVideos,
			"download_images": current.DownloadImages,
		},
	})
}

// handleExploreSetup tells the dashboard whether the userscript is installed,
// current, and syncing — everything its getting-started view needs.
func handleExploreSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	views, err := accountViews()
	if err != nil {
		logx.Error(err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	var bookmarks int64
	if err := store.DB.Model(&store.TweetModel{}).Count(&bookmarks).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count bookmarks"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"script":      currentUserscriptStatus(),
		"accounts":    views,
		"bookmarks":   bookmarks,
		"install_url": "/tbd.user.js",
		"sync_url":    "https://x.com/i/history",
	})
}

// removeBookmarks drops tweets from one account's archive and reports how many
// it actually held. A tweet another account still bookmarks keeps its row and
// its files; only this account's membership goes.
func removeBookmarks(accountID string, tweets []store.TweetModel) (int, error) {
	if accountID == "" || len(tweets) == 0 {
		return 0, nil
	}

	ids := make([]string, 0, len(tweets))
	for _, tweet := range tweets {
		ids = append(ids, tweet.ID)
	}

	removed := 0
	const chunkSize = 500 // stay under SQLite's bound-parameter limit
	for start := 0; start < len(ids); start += chunkSize {
		chunk := ids[start:min(start+chunkSize, len(ids))]

		var held []string
		if err := store.DB.Model(&store.AccountBookmarkModel{}).
			Where("account_id = ? AND tweet_id IN ?", accountID, chunk).
			Pluck("tweet_id", &held).Error; err != nil {
			return removed, eris.Wrap(err, "failed to list the account's bookmarks")
		}
		if len(held) == 0 {
			continue
		}

		if err := store.DB.Where("account_id = ? AND tweet_id IN ?", accountID, held).
			Delete(&store.AccountBookmarkModel{}).Error; err != nil {
			return removed, eris.Wrap(err, "failed to remove the account's bookmarks")
		}
		removed += len(held)

		// Whatever nobody holds any more can leave the database entirely.
		var orphans []string
		if err := store.DB.Model(&store.TweetModel{}).
			Where(`id IN ? AND NOT EXISTS (SELECT 1 FROM account_bookmarks
				WHERE account_bookmarks.tweet_id = tweets.id)`, held).
			Pluck("id", &orphans).Error; err != nil {
			return removed, eris.Wrap(err, "failed to find unheld bookmarks")
		}
		if len(orphans) == 0 {
			continue
		}
		models := make([]store.TweetModel, 0, len(orphans))
		for _, id := range orphans {
			models = append(models, store.TweetModel{ID: id})
		}
		if err := download.RemoveTweets(models); err != nil {
			return removed, err
		}
	}
	return removed, nil
}

// nonZeroTime keeps "never happened" out of the JSON: a zero time.Time still
// marshals as year 1, which reads as a real date in the dashboard.
func nonZeroTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}
