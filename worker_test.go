package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDownloadPendingMediaDoesNotMarkDisabledImagesDownloaded(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	withTestConfig(t, Config{
		MediaDir:       t.TempDir(),
		DownloadVideos: true,
		DownloadImages: false,
	})
	require.NoError(t, InitDB(":memory:"))

	tweet := TweetModel{
		ID:         "tweet-1",
		ScreenName: "alice",
		CreatedAt:  time.Now(),
		Media: []MediaModel{
			{
				ID:      "media-1",
				TweetID: "tweet-1",
				Type:    "photo",
				URL:     "https://pbs.twimg.com/media/test.jpg",
			},
		},
	}
	require.NoError(t, DB.Create(&tweet).Error)

	DownloadPendingMedia()

	var media MediaModel
	require.NoError(t, DB.First(&media, "id = ?", "media-1").Error)
	assert.False(t, media.Downloaded)
	assert.False(t, media.Failed)
	assert.Equal(t, 0, media.RetryCount)
}
