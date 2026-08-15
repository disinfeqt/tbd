package server

import (
	_ "embed"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rotisserie/eris"
	"gorm.io/gorm"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/download"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
	"twitter-bookmarks-downloader/internal/twitter"
)

//go:embed explore.html
var exploreHTML []byte

func registerExploreRoutes() {
	http.HandleFunc("/", handleExploreHome)
	http.HandleFunc("/api/explore/stats", handleExploreStats)
	http.HandleFunc("/api/explore/tweets", handleExploreTweets)
	http.HandleFunc("/api/explore/tweets/delete", handleExploreDeleteTweet)
	http.HandleFunc("/api/explore/missing", handleExploreMissing)
	http.HandleFunc("/api/explore/missing/fix", handleExploreMissingFix)
	http.HandleFunc("/api/explore/reveal", handleExploreReveal)
	http.HandleFunc("/media/", handleMediaFile)
}

func handleExploreHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(exploreHTML)
}

func handleMediaFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Resolve the media dir per request so settings changes apply immediately.
	fs := http.FileServer(http.Dir(config.Current().MediaDir))
	http.StripPrefix("/media/", fs).ServeHTTP(w, r)
}

type authorStat struct {
	ScreenName string `json:"screen_name"`
	Name       string `json:"name"`
	Count      int    `json:"count"`
}

type monthStat struct {
	Month string `json:"month"` // "2006-01"
	Count int    `json:"count"`
}

type typeStat struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

type exploreStats struct {
	Totals struct {
		Bookmarks       int64 `json:"bookmarks"`
		Authors         int   `json:"authors"`
		TweetsWithMedia int64 `json:"tweets_with_media"`
		MediaTotal      int64 `json:"media_total"`
		MediaDownloaded int64 `json:"media_downloaded"`
		MediaFailed     int64 `json:"media_failed"`
	} `json:"totals"`
	FirstBookmark string       `json:"first_bookmark,omitempty"`
	LastBookmark  string       `json:"last_bookmark,omitempty"`
	Monthly       []monthStat  `json:"monthly"`
	Hours         [24]int      `json:"hours"`
	Weekdays      [7]int       `json:"weekdays"` // Monday-first
	TopAuthors    []authorStat `json:"top_authors"`
	MediaTypes    []typeStat   `json:"media_types"`
}

func handleExploreStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// The archive is small (thousands of rows), so pull the light columns once
	// and aggregate in Go — no SQL-dialect date math, correct local timezones.
	type tweetRow struct {
		ScreenName string
		Name       string
		CreatedAt  time.Time
	}
	var rows []tweetRow
	if err := store.DB.Model(&store.TweetModel{}).
		Select("screen_name", "name", "created_at").
		Find(&rows).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to query tweets for stats"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	var stats exploreStats
	stats.Totals.Bookmarks = int64(len(rows))
	stats.Monthly = []monthStat{}
	stats.TopAuthors = []authorStat{}
	stats.MediaTypes = []typeStat{}

	authorCounts := map[string]*authorStat{}
	monthCounts := map[string]int{}
	var first, last time.Time

	for _, row := range rows {
		local := row.CreatedAt.In(time.Local)
		if first.IsZero() || local.Before(first) {
			first = local
		}
		if last.IsZero() || local.After(last) {
			last = local
		}

		monthCounts[local.Format("2006-01")]++
		stats.Hours[local.Hour()]++
		stats.Weekdays[(int(local.Weekday())+6)%7]++ // Monday-first

		key := strings.ToLower(row.ScreenName)
		if key == "" {
			continue
		}
		entry, ok := authorCounts[key]
		if !ok {
			entry = &authorStat{ScreenName: row.ScreenName, Name: row.Name}
			authorCounts[key] = entry
		}
		entry.Count++
		if entry.Name == "" {
			entry.Name = row.Name
		}
	}

	stats.Totals.Authors = len(authorCounts)
	if !first.IsZero() {
		stats.FirstBookmark = first.Format(time.RFC3339)
		stats.LastBookmark = last.Format(time.RFC3339)

		// Fill the month range continuously so the timeline has no gaps.
		cursor := time.Date(first.Year(), first.Month(), 1, 0, 0, 0, 0, time.Local)
		end := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.Local)
		for !cursor.After(end) {
			key := cursor.Format("2006-01")
			stats.Monthly = append(stats.Monthly, monthStat{Month: key, Count: monthCounts[key]})
			cursor = cursor.AddDate(0, 1, 0)
		}
	}

	for _, entry := range authorCounts {
		stats.TopAuthors = append(stats.TopAuthors, *entry)
	}
	sort.Slice(stats.TopAuthors, func(i, j int) bool {
		if stats.TopAuthors[i].Count != stats.TopAuthors[j].Count {
			return stats.TopAuthors[i].Count > stats.TopAuthors[j].Count
		}
		return strings.ToLower(stats.TopAuthors[i].ScreenName) < strings.ToLower(stats.TopAuthors[j].ScreenName)
	})
	if len(stats.TopAuthors) > 10 {
		stats.TopAuthors = stats.TopAuthors[:10]
	}

	// Media aggregates are cheap COUNT queries.
	if err := store.DB.Model(&store.MediaModel{}).Count(&stats.Totals.MediaTotal).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count media"))
	}
	if err := store.DB.Model(&store.MediaModel{}).Where("downloaded = ?", true).Count(&stats.Totals.MediaDownloaded).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count downloaded media"))
	}
	if err := store.DB.Model(&store.MediaModel{}).Where("failed = ?", true).Count(&stats.Totals.MediaFailed).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count failed media"))
	}
	if err := store.DB.Model(&store.MediaModel{}).
		Distinct("tweet_id").Count(&stats.Totals.TweetsWithMedia).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count tweets with media"))
	}

	type typeRow struct {
		Type  string
		Count int
	}
	var typeRows []typeRow
	if err := store.DB.Model(&store.MediaModel{}).
		Select("type, COUNT(*) as count").Group("type").Order("count DESC").
		Find(&typeRows).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count media types"))
	}
	for _, tr := range typeRows {
		stats.MediaTypes = append(stats.MediaTypes, typeStat{Type: tr.Type, Count: tr.Count})
	}

	writeJSON(w, stats)
}

type exploreMediaItem struct {
	Type       string `json:"type"`
	Downloaded bool   `json:"downloaded"`
	File       string `json:"file,omitempty"`
	Missing    bool   `json:"missing,omitempty"` // downloaded, but the file is gone from disk
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
}

type exploreTweetItem struct {
	ID           string             `json:"id"`
	Name         string             `json:"name"`
	ScreenName   string             `json:"screen_name"`
	FullText     string             `json:"full_text"`
	CreatedAt    time.Time          `json:"created_at"`
	PermanentURL string             `json:"permanent_url"`
	Media        []exploreMediaItem `json:"media"`
}

const explorePageSize = 24

func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

func handleExploreTweets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	author := strings.TrimSpace(r.URL.Query().Get("author"))
	mediaType := r.URL.Query().Get("type")
	sortOrder := "created_at DESC"
	if r.URL.Query().Get("sort") == "oldest" {
		sortOrder = "created_at ASC"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	// Search and author filters, without the media-type filter, so the same
	// base can also produce the per-type tab counts.
	applyBaseFilters := func() *gorm.DB {
		query := store.DB.Model(&store.TweetModel{})
		if q != "" {
			like := "%" + escapeLike(q) + "%"
			query = query.Where(
				`(full_text LIKE ? ESCAPE '\' OR screen_name LIKE ? ESCAPE '\' OR name LIKE ? ESCAPE '\')`,
				like, like, like,
			)
		}
		if author != "" {
			query = query.Where("LOWER(screen_name) = LOWER(?)", author)
		}
		return query
	}
	applyTypeFilter := func(query *gorm.DB, mediaType string) *gorm.DB {
		switch mediaType {
		case "photo", "video":
			return query.Where("EXISTS (SELECT 1 FROM media WHERE media.tweet_id = tweets.id AND media.type = ?)", mediaType)
		case "gif":
			return query.Where("EXISTS (SELECT 1 FROM media WHERE media.tweet_id = tweets.id AND media.type = ?)", "animated_gif")
		case "text":
			return query.Where("NOT EXISTS (SELECT 1 FROM media WHERE media.tweet_id = tweets.id)")
		}
		return query
	}
	applyFilters := func() *gorm.DB {
		return applyTypeFilter(applyBaseFilters(), mediaType)
	}

	var total int64
	if err := applyFilters().Count(&total).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count explore tweets"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	counts := map[string]int64{}
	for _, key := range []string{"all", "photo", "video", "gif", "text"} {
		if key == mediaType || (key == "all" && mediaType == "") {
			counts[key] = total
			continue
		}
		var n int64
		if err := applyTypeFilter(applyBaseFilters(), key).Count(&n).Error; err != nil {
			logx.Error(eris.Wrap(err, "Failed to count explore tweets by type"))
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
		counts[key] = n
	}

	var tweets []store.TweetModel
	if err := applyFilters().
		Select("id", "name", "screen_name", "full_text", "created_at", "permanent_url", "raw_json").
		Preload("Media", func(db *gorm.DB) *gorm.DB { return db.Order(`"index" ASC`) }).
		Order(sortOrder).
		Offset((page - 1) * explorePageSize).
		Limit(explorePageSize).
		Find(&tweets).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to query explore tweets"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	items := make([]exploreTweetItem, 0, len(tweets))
	for i := range tweets {
		items = append(items, exploreItemFromTweet(&tweets[i]))
	}

	writeJSON(w, map[string]any{
		"total":     total,
		"page":      page,
		"page_size": explorePageSize,
		"counts":    counts,
		"items":     items,
	})
}

func exploreItemFromTweet(tweet *store.TweetModel) exploreTweetItem {
	item := exploreTweetItem{
		ID:           tweet.ID,
		Name:         tweet.Name,
		ScreenName:   tweet.ScreenName,
		FullText:     tweet.FullText,
		CreatedAt:    tweet.CreatedAt,
		PermanentURL: tweet.PermanentURL,
		Media:        []exploreMediaItem{},
	}
	// Media dimensions come from the stored raw tweet JSON; the frontend uses
	// them to reserve each card's aspect ratio before the file loads.
	var entities []twitter.MediaEntity
	if tweet.RawJSON != "" {
		entities, _ = twitter.MediaEntitiesFromRawTweet(tweet.RawJSON)
	}
	for _, media := range tweet.Media {
		mi := exploreMediaItem{Type: media.Type, Downloaded: media.Downloaded}
		if media.Downloaded {
			if parsedURL, err := url.Parse(media.URL); err == nil {
				mi.File = download.BuildFilename(tweet, media.Index, len(tweet.Media), parsedURL)
			}
		}
		if media.Width > 0 && media.Height > 0 {
			mi.Width, mi.Height = media.Width, media.Height
		} else if entity, ok := download.MatchingMediaEntity(media, entities); ok {
			mi.Width = entity.OriginalInfo.Width
			mi.Height = entity.OriginalInfo.Height
		}
		item.Media = append(item.Media, mi)
	}
	return item
}

// handleExploreMissing lists bookmarks whose downloaded media files are all
// gone from disk (each vanished file flagged). A bookmark with at least one
// surviving file is not considered missing — same rule as -fix-deleted-media.
func handleExploreMissing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mediaDir := config.Current().MediaDir
	if info, err := os.Stat(mediaDir); err != nil || !info.IsDir() {
		http.Error(w, "Media folder is not accessible", http.StatusInternalServerError)
		return
	}

	var tweets []store.TweetModel
	if err := store.DB.
		Where("EXISTS (SELECT 1 FROM media WHERE media.tweet_id = tweets.id AND media.downloaded = ?)", true).
		Preload("Media", func(db *gorm.DB) *gorm.DB { return db.Order(`"index" ASC`) }).
		Order("created_at DESC").
		Find(&tweets).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to query tweets for missing media"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	items := []exploreTweetItem{}
	for i := range tweets {
		item := exploreItemFromTweet(&tweets[i])
		verified := false
		present := false
		for j := range item.Media {
			mi := &item.Media[j]
			if mi.File == "" {
				if mi.Downloaded {
					present = true // cannot compute the filename, so cannot prove it is gone
				}
				continue
			}
			if _, err := os.Stat(filepath.Join(mediaDir, mi.File)); errors.Is(err, fs.ErrNotExist) {
				mi.Missing = true
				verified = true
			} else {
				present = true
			}
		}
		if verified && !present {
			items = append(items, item)
		}
	}

	writeJSON(w, map[string]any{
		"total": len(items),
		"items": items,
	})
}

// handleExploreMissingFix removes every bookmark whose downloaded media files
// are all gone from disk — the dashboard equivalent of -fix-deleted-media.
func handleExploreMissingFix(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	tweets, err := download.FindTweetsWithDeletedMedia()
	if err != nil {
		logx.Error(eris.Wrap(err, "Failed to find bookmarks with deleted media"))
		http.Error(w, "Media folder is not accessible", http.StatusInternalServerError)
		return
	}
	if len(tweets) > 0 {
		if err := download.RemoveTweets(tweets); err != nil {
			logx.Error(eris.Wrap(err, "Failed to remove bookmarks with deleted media"))
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, map[string]any{"removed": len(tweets)})
}

// revealInFileManager shows the file in the OS file manager. A var so tests
// can stub it out.
var revealInFileManager = func(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Run()
	case "windows":
		// explorer exits nonzero even on success, so ignore its status.
		_ = exec.Command("explorer", "/select,"+path).Run()
		return nil
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Run()
	}
}

// handleExploreReveal reveals a downloaded media file in Finder / Explorer.
func handleExploreReveal(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		File string `json:"file"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil ||
		req.File == "" || filepath.Base(req.File) != req.File {
		http.Error(w, "Invalid file name", http.StatusBadRequest)
		return
	}

	path := filepath.Join(config.Current().MediaDir, req.File)
	if _, err := os.Stat(path); err != nil {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	if err := revealInFileManager(path); err != nil {
		logx.Error(eris.Wrap(err, "Failed to reveal media file"))
		http.Error(w, "Failed to reveal file", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{"revealed": true})
}

// handleExploreDeleteTweet removes a bookmark and its media records, and
// optionally its downloaded files from the media folder.
func handleExploreDeleteTweet(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ID          string `json:"id"`
		DeleteFiles bool   `json:"delete_files"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ID == "" {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	var tweet store.TweetModel
	if err := store.DB.Preload("Media").First(&tweet, "id = ?", req.ID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			http.Error(w, "Bookmark not found", http.StatusNotFound)
			return
		}
		logx.Error(eris.Wrap(err, "Failed to load tweet for deletion"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	filesRemoved := 0
	if req.DeleteFiles {
		mediaDir := config.Current().MediaDir
		for _, media := range tweet.Media {
			if !media.Downloaded {
				continue
			}
			parsedURL, err := url.Parse(media.URL)
			if err != nil {
				continue
			}
			name := download.BuildFilename(&tweet, media.Index, len(tweet.Media), parsedURL)
			if err := os.Remove(filepath.Join(mediaDir, name)); err == nil {
				filesRemoved++
			} else if !errors.Is(err, fs.ErrNotExist) {
				logx.Warnf("Failed to delete media file %s: %v", name, err)
			}
		}
	}

	if err := download.RemoveTweets([]store.TweetModel{tweet}); err != nil {
		logx.Error(eris.Wrap(err, "Failed to delete bookmark"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, map[string]any{
		"deleted":       true,
		"files_removed": filesRemoved,
	})
}
