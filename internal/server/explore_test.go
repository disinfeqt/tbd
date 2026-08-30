package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
)

func seedExploreDB(t *testing.T) {
	t.Helper()

	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})
	require.NoError(t, store.Init(":memory:"))

	// SyncedAt (time added) deliberately disagrees with CreatedAt (tweet
	// date): tweet 1 is the oldest tweet but the most recently added.
	tweets := []store.TweetModel{
		{
			ID:         "1",
			Name:       "Alice",
			ScreenName: "alice",
			FullText:   "hello go generics",
			CreatedAt:  time.Date(2025, 1, 10, 9, 30, 0, 0, time.Local),
			SyncedAt:   time.Date(2025, 6, 3, 0, 0, 0, 0, time.Local),
		},
		{
			ID:         "2",
			Name:       "Alice",
			ScreenName: "alice",
			FullText:   "salt &amp; pepper",
			CreatedAt:  time.Date(2025, 2, 5, 22, 0, 0, 0, time.Local),
			SyncedAt:   time.Date(2025, 6, 1, 0, 0, 0, 0, time.Local),
		},
		{
			ID:         "3",
			Name:       "Bob",
			ScreenName: "bob",
			FullText:   "unrelated",
			CreatedAt:  time.Date(2025, 3, 1, 12, 0, 0, 0, time.Local),
			SyncedAt:   time.Date(2025, 6, 2, 0, 0, 0, 0, time.Local),
			RawJSON:    `{"legacy":{"extended_entities":{"media":[{"id_str":"m1","type":"photo","media_url_https":"https://pbs.twimg.com/media/a.jpg","original_info":{"width":1200,"height":800}}]}}}`,
			Media: []store.MediaModel{
				{ID: "m1", TweetID: "3", Type: "photo", URL: "https://pbs.twimg.com/media/a.jpg", Downloaded: true},
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweets).Error)
	// Runs the startup migration, so these tweets belong to an account the way
	// they would after a real upgrade.
	require.NoError(t, accounts.Load())
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
	// Most recently added first by default — tweet 1 was added last even
	// though it is the oldest tweet.
	assert.Equal(t, "1", items[0].(map[string]any)["id"])

	search := get("?q=generics")
	assert.EqualValues(t, 1, search["total"])

	// Stored entities are decoded for display and matchable with a raw "&".
	amp := get("?q=" + url.QueryEscape("salt & pepper"))
	assert.EqualValues(t, 1, amp["total"])
	assert.Equal(t, "salt & pepper", amp["items"].([]any)[0].(map[string]any)["full_text"])

	byAuthor := get("?author=ALICE")
	assert.EqualValues(t, 2, byAuthor["total"])

	oldest := get("?sort=oldest")
	assert.Equal(t, "2", oldest["items"].([]any)[0].(map[string]any)["id"])

	// Tweet-date sorts ignore when the bookmark was added.
	tweetNewest := get("?sort=tweet-newest")
	assert.Equal(t, "3", tweetNewest["items"].([]any)[0].(map[string]any)["id"])
	tweetOldest := get("?sort=tweet-oldest")
	assert.Equal(t, "1", tweetOldest["items"].([]any)[0].(map[string]any)["id"])

	// Month filter uses the tweet date.
	january := get("?month=2025-01")
	assert.EqualValues(t, 1, january["total"])
	assert.Equal(t, "1", january["items"].([]any)[0].(map[string]any)["id"])
	assert.EqualValues(t, 0, get("?month=2025-06")["total"])

	// Downloaded media gets a computed local filename.
	withMedia := get("?author=bob")
	media := withMedia["items"].([]any)[0].(map[string]any)["media"].([]any)
	require.Len(t, media, 1)
	assert.Equal(t, true, media[0].(map[string]any)["downloaded"])
	assert.Contains(t, media[0].(map[string]any)["file"], "twitter-@bob-")
	// Dimensions parsed from the stored raw tweet JSON.
	assert.EqualValues(t, 1200, media[0].(map[string]any)["width"])
	assert.EqualValues(t, 800, media[0].(map[string]any)["height"])
}

func TestHandleExploreTweetsTypeFilter(t *testing.T) {
	seedExploreDB(t)
	// Give one of Alice's tweets a (not yet downloaded) video plus a photo,
	// making it a mixed-media tweet.
	require.NoError(t, store.DB.Create(&[]store.MediaModel{
		{ID: "m2", TweetID: "2", Type: "video", URL: "https://video.twimg.com/v.mp4"},
		{ID: "m3", TweetID: "2", Index: 1, Type: "photo", URL: "https://pbs.twimg.com/media/b.jpg"},
	}).Error)

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
	counts := all["counts"].(map[string]any)
	assert.EqualValues(t, 3, counts["all"])
	assert.EqualValues(t, 1, counts["photo"])
	assert.EqualValues(t, 1, counts["video"])
	assert.EqualValues(t, 0, counts["gif"])
	assert.EqualValues(t, 1, counts["text"])

	// Photos is photo-only: the mixed photo+video tweet 2 is excluded.
	photos := get("?type=photo")
	assert.EqualValues(t, 1, photos["total"])
	assert.Equal(t, "3", photos["items"].([]any)[0].(map[string]any)["id"])

	videos := get("?type=video")
	assert.EqualValues(t, 1, videos["total"])
	assert.Equal(t, "2", videos["items"].([]any)[0].(map[string]any)["id"])

	texts := get("?type=text")
	assert.EqualValues(t, 1, texts["total"])
	assert.Equal(t, "1", texts["items"].([]any)[0].(map[string]any)["id"])

	// Type filter composes with the author filter; counts respect the author.
	aliceVideos := get("?author=alice&type=video")
	assert.EqualValues(t, 1, aliceVideos["total"])
	aliceCounts := aliceVideos["counts"].(map[string]any)
	assert.EqualValues(t, 2, aliceCounts["all"])
	assert.EqualValues(t, 0, aliceCounts["photo"])
	assert.EqualValues(t, 1, aliceCounts["video"])
	assert.EqualValues(t, 1, aliceCounts["text"])
}

func TestHandleExploreTweetsLengthSort(t *testing.T) {
	seedExploreDB(t)
	// Tweet 1 gets a short video, tweet 2 a long one; tweet 3 stays photo-only.
	require.NoError(t, store.DB.Create(&[]store.MediaModel{
		{ID: "v1", TweetID: "1", Type: "video", URL: "https://video.twimg.com/a.mp4", DurationMs: 5000},
		{ID: "v2", TweetID: "2", Type: "video", URL: "https://video.twimg.com/b.mp4", DurationMs: 30000},
	}).Error)

	get := func(query string) []string {
		rec := httptest.NewRecorder()
		handleExploreTweets(rec, httptest.NewRequest(http.MethodGet, "/api/explore/tweets"+query, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		ids := []string{}
		for _, item := range payload["items"].([]any) {
			ids = append(ids, item.(map[string]any)["id"].(string))
		}
		return ids
	}

	assert.Equal(t, []string{"2", "1", "3"}, get("?sort=longest"))
	// Tweets with no known duration go last on shortest too.
	assert.Equal(t, []string{"1", "2", "3"}, get("?sort=shortest"))

	// Duration is exposed on the media payload.
	rec := httptest.NewRecorder()
	handleExploreTweets(rec, httptest.NewRequest(http.MethodGet, "/api/explore/tweets?type=video&sort=longest", nil))
	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	media := payload["items"].([]any)[0].(map[string]any)["media"].([]any)
	assert.EqualValues(t, 30000, media[0].(map[string]any)["duration_ms"])
}

func TestHandleExploreMissingAndDelete(t *testing.T) {
	seedExploreDB(t)
	mediaDir := t.TempDir()
	cfg := config.Default()
	cfg.MediaDir = mediaDir
	t.Cleanup(config.SwapForTest(cfg))

	getMissing := func() map[string]any {
		rec := httptest.NewRecorder()
		handleExploreMissing(rec, httptest.NewRequest(http.MethodGet, "/api/explore/missing", nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var payload map[string]any
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		return payload
	}

	// Carol has two downloaded photos and one survives on disk → not missing.
	carolCreated := time.Date(2025, 4, 1, 12, 0, 0, 0, time.Local)
	require.NoError(t, store.DB.Create(&store.TweetModel{
		ID: "4", Name: "Carol", ScreenName: "carol", CreatedAt: carolCreated,
		Media: []store.MediaModel{
			{ID: "m4a", TweetID: "4", Index: 0, Type: "photo", URL: "https://pbs.twimg.com/media/c.jpg", Downloaded: true},
			{ID: "m4b", TweetID: "4", Index: 1, Type: "photo", URL: "https://pbs.twimg.com/media/d.jpg", Downloaded: true},
		},
	}).Error)
	carolFile := "twitter-@carol-" + carolCreated.Format("20060102-150405") + "-4-0.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(mediaDir, carolFile), []byte("x"), 0o644))

	// Only Bob is listed: his downloaded photo is fully gone from disk.
	payload := getMissing()
	assert.EqualValues(t, 1, payload["total"])
	item := payload["items"].([]any)[0].(map[string]any)
	assert.Equal(t, "3", item["id"])
	assert.Equal(t, true, item["media"].([]any)[0].(map[string]any)["missing"])

	// Restore the file → nothing is missing anymore.
	name := "twitter-@bob-" +
		time.Date(2025, 3, 1, 12, 0, 0, 0, time.Local).Format("20060102-150405") + "-3.jpg"
	require.NoError(t, os.WriteFile(filepath.Join(mediaDir, name), []byte("x"), 0o644))
	assert.EqualValues(t, 0, getMissing()["total"])

	// Delete the bookmark along with its file.
	rec := httptest.NewRecorder()
	handleExploreDeleteTweet(rec, httptest.NewRequest(http.MethodPost,
		"/api/explore/tweets/delete", strings.NewReader(`{"id":"3","delete_files":true}`)))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.EqualValues(t, 1, resp["files_removed"])

	_, err := os.Stat(filepath.Join(mediaDir, name))
	assert.True(t, os.IsNotExist(err))
	var tweetCount, mediaCount int64
	require.NoError(t, store.DB.Model(&store.TweetModel{}).Count(&tweetCount).Error)
	require.NoError(t, store.DB.Model(&store.MediaModel{}).Count(&mediaCount).Error)
	assert.EqualValues(t, 3, tweetCount)
	assert.EqualValues(t, 2, mediaCount) // Carol's media rows remain

	// Unknown id → 404.
	rec404 := httptest.NewRecorder()
	handleExploreDeleteTweet(rec404, httptest.NewRequest(http.MethodPost,
		"/api/explore/tweets/delete", strings.NewReader(`{"id":"nope"}`)))
	assert.Equal(t, http.StatusNotFound, rec404.Code)
}

func TestHandleExploreDownloads(t *testing.T) {
	seedExploreDB(t)
	// One video pending download alongside bob's downloaded photo.
	require.NoError(t, store.DB.Create(&store.MediaModel{
		ID: "m2", TweetID: "2", Type: "video", URL: "https://video.twimg.com/v.mp4",
	}).Error)

	rec := httptest.NewRecorder()
	handleExploreDownloads(rec, httptest.NewRequest(http.MethodGet, "/api/explore/downloads", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.EqualValues(t, 2, payload["total"])
	assert.EqualValues(t, 1, payload["downloaded"])
	assert.EqualValues(t, 1, payload["pending"])
	assert.EqualValues(t, 0, payload["failed"])
}

func TestHandleExploreLogs(t *testing.T) {
	logx.Info("hello from the test")

	rec := httptest.NewRecorder()
	handleExploreLogs(rec, httptest.NewRequest(http.MethodGet, "/api/explore/logs", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var payload struct {
		Items  []logx.Entry `json:"items"`
		LastID int64        `json:"last_id"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	require.NotEmpty(t, payload.Items)
	last := payload.Items[len(payload.Items)-1]
	assert.Equal(t, "info", last.Level)
	assert.Equal(t, "hello from the test", last.Msg)
	assert.Equal(t, last.ID, payload.LastID)

	// after=<last id> returns nothing new.
	rec2 := httptest.NewRecorder()
	handleExploreLogs(rec2, httptest.NewRequest(http.MethodGet,
		"/api/explore/logs?after="+strconv.FormatInt(payload.LastID, 10), nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	var payload2 struct {
		Items []logx.Entry `json:"items"`
	}
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &payload2))
	assert.Empty(t, payload2.Items)
}

func TestHandleExploreAuthors(t *testing.T) {
	seedExploreDB(t)

	rec := httptest.NewRecorder()
	handleExploreAuthors(rec, httptest.NewRequest(http.MethodGet, "/api/explore/authors", nil))
	require.Equal(t, http.StatusOK, rec.Code)

	var payload map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
	assert.EqualValues(t, 2, payload["total"])
	items := payload["items"].([]any)
	require.Len(t, items, 2)
	first := items[0].(map[string]any)
	assert.Equal(t, "alice", first["screen_name"])
	assert.EqualValues(t, 2, first["count"])
	assert.Equal(t, "bob", items[1].(map[string]any)["screen_name"])
}

func TestHandleExploreReveal(t *testing.T) {
	seedExploreDB(t)
	mediaDir := t.TempDir()
	cfg := config.Default()
	cfg.MediaDir = mediaDir
	t.Cleanup(config.SwapForTest(cfg))

	originalReveal := revealInFileManager
	t.Cleanup(func() { revealInFileManager = originalReveal })
	var revealed string
	revealInFileManager = func(path string) error {
		revealed = path
		return nil
	}

	post := func(body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		handleExploreReveal(rec, httptest.NewRequest(http.MethodPost,
			"/api/explore/reveal", strings.NewReader(body)))
		return rec
	}

	require.NoError(t, os.WriteFile(filepath.Join(mediaDir, "a.jpg"), []byte("x"), 0o644))

	rec := post(`{"file":"a.jpg"}`)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, filepath.Join(mediaDir, "a.jpg"), revealed)

	assert.Equal(t, http.StatusNotFound, post(`{"file":"nope.jpg"}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`{"file":"../secret.txt"}`).Code)
	assert.Equal(t, http.StatusBadRequest, post(`{"file":""}`).Code)
}

func TestHandleExploreMissingFix(t *testing.T) {
	seedExploreDB(t)
	mediaDir := t.TempDir()
	cfg := config.Default()
	cfg.MediaDir = mediaDir
	t.Cleanup(config.SwapForTest(cfg))

	// Bob's downloaded photo is gone from disk → removed in one call.
	rec := httptest.NewRecorder()
	handleExploreMissingFix(rec, httptest.NewRequest(http.MethodPost, "/api/explore/missing/fix", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	var resp map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &resp))
	assert.EqualValues(t, 1, resp["removed"])

	var tweetCount int64
	require.NoError(t, store.DB.Model(&store.TweetModel{}).Count(&tweetCount).Error)
	assert.EqualValues(t, 2, tweetCount)

	// Second call is a no-op.
	rec2 := httptest.NewRecorder()
	handleExploreMissingFix(rec2, httptest.NewRequest(http.MethodPost, "/api/explore/missing/fix", nil))
	require.Equal(t, http.StatusOK, rec2.Code)
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &resp))
	assert.EqualValues(t, 0, resp["removed"])
}

// Media lives under /media/<account>/<file> because every account has its own
// folder, and the same tweet saved twice produces the same file name in both.
func TestHandleMediaFileServesPerAccount(t *testing.T) {
	dir := t.TempDir()
	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       dir,
		DownloadVideos: true,
		DownloadImages: true,
	}))
	seedExploreDB(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.jpg"), []byte("jpeg"), 0o644))

	rec := httptest.NewRecorder()
	handleMediaFile(rec, httptest.NewRequest(http.MethodGet, "/media/"+accounts.LegacyID+"/a.jpg", nil))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "jpeg", rec.Body.String())

	// A path that climbs out of the folder is not a file name.
	rec = httptest.NewRecorder()
	handleMediaFile(rec, httptest.NewRequest(http.MethodGet, "/media/"+accounts.LegacyID+"/../../etc/hosts", nil))
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

// Every number in the header belongs to the account on screen. Media counts
// reach an account through the tweet they hang off, and used to span all of them.
func TestHandleExploreStatsCountsOnlyThisAccount(t *testing.T) {
	seedExploreDB(t)

	// A second account with its own bookmark and its own two media files.
	require.NoError(t, store.DB.Create(&store.AccountModel{
		ID: "other", Handle: "bob", DownloadVideos: true, DownloadImages: true,
	}).Error)
	require.NoError(t, store.DB.Create(&store.TweetModel{
		ID: "9", AccountID: "other", ScreenName: "carol", CreatedAt: time.Now(), SyncedAt: time.Now(),
		Media: []store.MediaModel{
			{ID: "m9a", TweetID: "9", Type: "video", URL: "https://video.twimg.com/9.mp4", Downloaded: true},
			{ID: "m9b", TweetID: "9", Type: "photo", URL: "https://pbs.twimg.com/9.jpg", Downloaded: true},
		},
	}).Error)
	require.NoError(t, store.DB.Create(&store.AccountBookmarkModel{
		AccountID: "other", TweetID: "9", SyncedAt: time.Now(),
	}).Error)
	require.NoError(t, accounts.Load())

	statsFor := func(account string) exploreStats {
		rec := httptest.NewRecorder()
		handleExploreStats(rec, httptest.NewRequest(http.MethodGet, "/api/explore/stats?account="+account, nil))
		require.Equal(t, http.StatusOK, rec.Code)
		var out exploreStats
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &out))
		return out
	}

	first := statsFor(accounts.LegacyID)
	assert.Equal(t, int64(3), first.Totals.Bookmarks)
	assert.Equal(t, int64(1), first.Totals.MediaTotal, "the other account's files are not this account's")
	assert.Equal(t, int64(1), first.Totals.MediaDownloaded)
	assert.Equal(t, int64(1), first.Totals.TweetsWithMedia)
	assert.Len(t, first.MediaTypes, 1)

	second := statsFor("other")
	assert.Equal(t, int64(1), second.Totals.Bookmarks)
	assert.Equal(t, int64(2), second.Totals.MediaTotal)
	assert.Len(t, second.MediaTypes, 2)
}
