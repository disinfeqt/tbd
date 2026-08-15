package server

import (
	_ "embed"
	"net/http"
	"net/url"
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
)

//go:embed explore.html
var exploreHTML []byte

func registerExploreRoutes() {
	http.HandleFunc("/", handleExploreHome)
	http.HandleFunc("/api/explore/stats", handleExploreStats)
	http.HandleFunc("/api/explore/tweets", handleExploreTweets)
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
	sortOrder := "created_at DESC"
	if r.URL.Query().Get("sort") == "oldest" {
		sortOrder = "created_at ASC"
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	applyFilters := func() *gorm.DB {
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

	var total int64
	if err := applyFilters().Count(&total).Error; err != nil {
		logx.Error(eris.Wrap(err, "Failed to count explore tweets"))
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	var tweets []store.TweetModel
	if err := applyFilters().
		Select("id", "name", "screen_name", "full_text", "created_at", "permanent_url").
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
		tweet := &tweets[i]
		item := exploreTweetItem{
			ID:           tweet.ID,
			Name:         tweet.Name,
			ScreenName:   tweet.ScreenName,
			FullText:     tweet.FullText,
			CreatedAt:    tweet.CreatedAt,
			PermanentURL: tweet.PermanentURL,
			Media:        []exploreMediaItem{},
		}
		for _, media := range tweet.Media {
			mi := exploreMediaItem{Type: media.Type, Downloaded: media.Downloaded}
			if media.Downloaded {
				if parsedURL, err := url.Parse(media.URL); err == nil {
					mi.File = download.BuildFilename(tweet, media.Index, len(tweet.Media), parsedURL)
				}
			}
			item.Media = append(item.Media, mi)
		}
		items = append(items, item)
	}

	writeJSON(w, map[string]any{
		"total":     total,
		"page":      page,
		"page_size": explorePageSize,
		"items":     items,
	})
}
