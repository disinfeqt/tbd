package download

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/store"
)

func TestProcessQueueDoesNotMarkDisabledImagesDownloaded(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})

	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       t.TempDir(),
		DownloadVideos: true,
		DownloadImages: false,
	}))
	require.NoError(t, store.Init(":memory:"))

	tweet := store.TweetModel{
		ID:         "tweet-1",
		ScreenName: "alice",
		CreatedAt:  time.Now(),
		Media: []store.MediaModel{
			{
				ID:      "media-1",
				TweetID: "tweet-1",
				Type:    "photo",
				URL:     "https://pbs.twimg.com/media/test.jpg",
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweet).Error)

	ProcessQueue()

	var media store.MediaModel
	require.NoError(t, store.DB.First(&media, "id = ?", "media-1").Error)
	assert.False(t, media.Downloaded)
	assert.False(t, media.Failed)
	assert.Equal(t, 0, media.RetryCount)
}

// Download settings are per account: one account can have videos off while
// another keeps them on, and the queue has to respect both at once.
func TestPendingQueueFollowsAccountSettings(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})

	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       t.TempDir(),
		DownloadVideos: true,
		DownloadImages: true,
	}))
	require.NoError(t, store.Init(":memory:"))

	require.NoError(t, store.DB.Create(&[]store.AccountModel{
		{ID: "keeps-video", DownloadVideos: true, DownloadImages: true},
		{ID: "no-video", DownloadVideos: false, DownloadImages: true},
	}).Error)
	require.NoError(t, accounts.Load())

	tweets := []store.TweetModel{
		{
			ID: "t1", AccountID: "keeps-video", ScreenName: "alice", CreatedAt: time.Now(),
			Media: []store.MediaModel{
				{ID: "video-on", TweetID: "t1", Type: "video", URL: "https://video.twimg.com/a.mp4"},
				{ID: "photo-on", TweetID: "t1", Type: "photo", URL: "https://pbs.twimg.com/a.jpg"},
			},
		},
		{
			ID: "t2", AccountID: "no-video", ScreenName: "bob", CreatedAt: time.Now(),
			Media: []store.MediaModel{
				{ID: "video-off", TweetID: "t2", Type: "video", URL: "https://video.twimg.com/b.mp4"},
				{ID: "photo-still-on", TweetID: "t2", Type: "photo", URL: "https://pbs.twimg.com/b.jpg"},
				// Unknown types must land on the same side of the fence as
				// ShouldDownloadFor: a row the query returns but the worker
				// refuses would wedge every Limit(5) batch forever.
				{ID: "mystery-mp4-off", TweetID: "t2", Type: "mystery", URL: "https://video.twimg.com/c.mp4?tag=1"},
				{ID: "mystery-img-on", TweetID: "t2", Type: "mystery", URL: "https://pbs.twimg.com/c.bin"},
				{ID: "untyped-blank", TweetID: "t2", Type: "", URL: ""},
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweets).Error)

	var queued []string
	require.NoError(t, pendingQuery().Order("id").Pluck("id", &queued).Error)
	assert.Equal(t, []string{"mystery-img-on", "photo-on", "photo-still-on", "video-on"}, queued)

	assert.True(t, ShouldDownloadFor("keeps-video", store.MediaModel{Type: "video"}))
	assert.False(t, ShouldDownloadFor("no-video", store.MediaModel{Type: "video"}))
	assert.True(t, ShouldDownloadFor("no-video", store.MediaModel{Type: "photo"}))
	// The query and the worker agree on every unknown-type row above.
	assert.False(t, ShouldDownloadFor("no-video", store.MediaModel{Type: "mystery", URL: "https://video.twimg.com/c.mp4?tag=1"}))
	assert.True(t, ShouldDownloadFor("no-video", store.MediaModel{Type: "mystery", URL: "https://pbs.twimg.com/c.bin"}))
	assert.False(t, ShouldDownloadFor("no-video", store.MediaModel{Type: "", URL: ""}))
}
