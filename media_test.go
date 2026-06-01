package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBestMP4VariantURLSelectsHighestBitrate(t *testing.T) {
	variants := []tweetVideoVariant{
		{
			ContentType: "application/x-mpegURL",
			URL:         "https://video.twimg.com/ext_tw_video/1/pl/test.m3u8",
		},
		{
			Bitrate:     256000,
			ContentType: "video/mp4",
			URL:         "https://video.twimg.com/ext_tw_video/1/vid/320x180/low.mp4",
		},
		{
			Bitrate:     2176000,
			ContentType: "video/mp4",
			URL:         "https://video.twimg.com/ext_tw_video/1/vid/1280x720/high.mp4",
		},
		{
			Bitrate:     832000,
			ContentType: "video/mp4",
			URL:         "https://video.twimg.com/ext_tw_video/1/vid/640x360/mid.mp4",
		},
	}

	assert.Equal(t, "https://video.twimg.com/ext_tw_video/1/vid/1280x720/high.mp4", bestMP4VariantURL(variants))
}

func TestDownloadURLForMediaUsesVideoVariant(t *testing.T) {
	media := tweetMediaEntity{
		Type:          "video",
		MediaURLHttps: "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/thumb.jpg",
		VideoInfo: tweetVideoInfo{
			Variants: []tweetVideoVariant{
				{
					Bitrate:     832000,
					ContentType: "video/mp4",
					URL:         "https://video.twimg.com/ext_tw_video/1/vid/640x360/video.mp4?tag=12",
				},
			},
		},
	}

	assert.Equal(t, "https://video.twimg.com/ext_tw_video/1/vid/640x360/video.mp4?tag=12", downloadURLForMedia(media))
}

func TestDownloadURLForMediaFallsBackToImageURL(t *testing.T) {
	media := tweetMediaEntity{
		Type:          "photo",
		MediaURLHttps: "https://pbs.twimg.com/media/test.jpg",
	}

	assert.Equal(t, "https://pbs.twimg.com/media/test.jpg", downloadURLForMedia(media))
}

func TestIsMP4MediaURL(t *testing.T) {
	assert.True(t, isMP4MediaURL("https://video.twimg.com/ext_tw_video/1/vid/1280x720/test.mp4?tag=12"))
	assert.False(t, isMP4MediaURL("https://pbs.twimg.com/media/test.jpg?name=orig"))
}

func TestRepairVideoMediaURLs(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	require.NoError(t, InitDB(":memory:"))

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

	tweet := TweetModel{
		ID:         "tweet-1",
		ScreenName: "user",
		CreatedAt:  time.Now(),
		RawJSON:    rawJSON,
		Media: []MediaModel{
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
	require.NoError(t, DB.Create(&tweet).Error)

	repaired, err := RepairVideoMediaURLs()
	require.NoError(t, err)
	assert.Equal(t, 1, repaired)

	var media MediaModel
	require.NoError(t, DB.First(&media, "id = ?", "media-1").Error)
	assert.Equal(t, "https://video.twimg.com/ext_tw_video/1/vid/1280x720/high.mp4", media.URL)
	assert.False(t, media.Downloaded)
	assert.False(t, media.Failed)
	assert.Equal(t, 0, media.RetryCount)
}
