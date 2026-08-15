package download

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/store"
)

func TestRepairVideoMediaURLs(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})

	require.NoError(t, store.Init(":memory:"))

	rawJSON := `{
		"legacy": {
			"extended_entities": {
				"media": [{
					"id_str": "media-1",
					"type": "video",
					"media_url_https": "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/thumb.jpg",
					"video_info": {
						"variants": [
							{"content_type": "application/x-mpegURL", "url": "https://video.twimg.com/ext_tw_video/1/pl/test.m3u8"},
							{"bitrate": 256000, "content_type": "video/mp4", "url": "https://video.twimg.com/ext_tw_video/1/vid/320x180/low.mp4"},
							{"bitrate": 2176000, "content_type": "video/mp4", "url": "https://video.twimg.com/ext_tw_video/1/vid/1280x720/high.mp4"}
						]
					}
				}]
			}
		}
	}`

	tweet := store.TweetModel{
		ID:         "tweet-1",
		ScreenName: "user",
		CreatedAt:  time.Now(),
		RawJSON:    rawJSON,
		Media: []store.MediaModel{
			{
				ID:         "media-1",
				TweetID:    "tweet-1",
				Index:      0,
				Type:       "video",
				URL:        "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/thumb.jpg",
				Downloaded: true,
				Failed:     true,
				RetryCount: 2,
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweet).Error)

	repaired, err := RepairVideoMediaURLs()
	require.NoError(t, err)
	assert.Equal(t, 1, repaired)

	var media store.MediaModel
	require.NoError(t, store.DB.First(&media, "id = ?", "media-1").Error)
	assert.Equal(t, "https://video.twimg.com/ext_tw_video/1/vid/1280x720/high.mp4", media.URL)
	assert.False(t, media.Downloaded)
	assert.False(t, media.Failed)
	assert.Equal(t, 0, media.RetryCount)
}
