package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/store"
)

func seedExploreDB(t *testing.T) {
	t.Helper()

	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})
	require.NoError(t, store.Init(":memory:"))

	tweets := []store.TweetModel{
		{
			ID:         "1",
			Name:       "Alice",
			ScreenName: "alice",
			FullText:   "hello go generics",
			CreatedAt:  time.Date(2025, 1, 10, 9, 30, 0, 0, time.Local),
		},
		{
			ID:         "2",
			Name:       "Alice",
			ScreenName: "alice",
			FullText:   "another post",
			CreatedAt:  time.Date(2025, 2, 5, 22, 0, 0, 0, time.Local),
		},
		{
			ID:         "3",
			Name:       "Bob",
			ScreenName: "bob",
			FullText:   "unrelated",
			CreatedAt:  time.Date(2025, 3, 1, 12, 0, 0, 0, time.Local),
			Media: []store.MediaModel{
				{ID: "m1", TweetID: "3", Type: "photo", URL: "https://pbs.twimg.com/media/a.jpg", Downloaded: true},
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweets).Error)
}

func TestHandleExploreStats(t *testing.T) {
	seedExploreDB(t)

	rec := httptest.NewRecorder()
	handleExploreStats(rec, httptest.NewRequest(http.MethodGet, "/api/explore/stats", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var stats exploreStats
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &stats))

	assert.Equal(t, int64(3), stats.Totals.Bookmarks)
	assert.Equal(t, 2, stats.Totals.Authors)
	assert.Equal(t, int64(1), stats.Totals.TweetsWithMedia)
	assert.Equal(t, int64(1), stats.Totals.MediaDownloaded)

	// Jan..Mar 2025 with no gaps.
	require.Len(t, stats.Monthly, 3)
	assert.Equal(t, monthStat{Month: "2025-01", Count: 1}, stats.Monthly[0])
	assert.Equal(t, monthStat{Month: "2025-02", Count: 1}, stats.Monthly[1])
	assert.Equal(t, monthStat{Month: "2025-03", Count: 1}, stats.Monthly[2])

	require.NotEmpty(t, stats.TopAuthors)
	assert.Equal(t, "alice", stats.TopAuthors[0].ScreenName)
	assert.Equal(t, 2, stats.TopAuthors[0].Count)

	assert.Equal(t, 1, stats.Hours[9])  // 09:30 local
	assert.Equal(t, 1, stats.Hours[22]) // 22:00 local
}

func TestHandleExploreTweetsSearchAndFilter(t *testing.T) {
	seedExploreDB(t)

	get := func(query string) map[string]any {
		rec := httptest.NewRecorder()
		handleExploreTweets(rec, httptest.NewRequest(http.MethodGet, "/api/explore/tweets"+query, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		return payload
	}

	all := get("")
	assert.EqualValues(t, 3, all["total"])
	items := all["items"].([]any)
	require.Len(t, items, 3)
	// Newest first by default.
	assert.Equal(t, "3", items[0].(map[string]any)["id"])

	search := get("?q=generics")
	assert.EqualValues(t, 1, search["total"])

	byAuthor := get("?author=ALICE")
	assert.EqualValues(t, 2, byAuthor["total"])

	oldest := get("?sort=oldest")
	assert.Equal(t, "1", oldest["items"].([]any)[0].(map[string]any)["id"])

	// Downloaded media gets a computed local filename.
	withMedia := get("?author=bob")
	media := withMedia["items"].([]any)[0].(map[string]any)["media"].([]any)
	require.Len(t, media, 1)
	assert.Equal(t, true, media[0].(map[string]any)["downloaded"])
	assert.Contains(t, media[0].(map[string]any)["file"], "twitter-@bob-")
}
