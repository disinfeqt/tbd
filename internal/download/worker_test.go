package download

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
