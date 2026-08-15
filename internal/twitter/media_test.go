package twitter

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBestMP4VariantURLSelectsHighestBitrate(t *testing.T) {
	variants := []VideoVariant{
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

	assert.Equal(t, "https://video.twimg.com/ext_tw_video/1/vid/1280x720/high.mp4", BestMP4VariantURL(variants))
}

func TestDownloadURLForMediaUsesVideoVariant(t *testing.T) {
	media := MediaEntity{
		Type:          "video",
		MediaURLHttps: "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/thumb.jpg",
		VideoInfo: VideoInfo{
			Variants: []VideoVariant{
				{
					Bitrate:     832000,
					ContentType: "video/mp4",
					URL:         "https://video.twimg.com/ext_tw_video/1/vid/640x360/video.mp4?tag=12",
				},
			},
		},
	}

	assert.Equal(t, "https://video.twimg.com/ext_tw_video/1/vid/640x360/video.mp4?tag=12", DownloadURLForMedia(media))
}

func TestDownloadURLForMediaFallsBackToImageURL(t *testing.T) {
	media := MediaEntity{
		Type:          "photo",
		MediaURLHttps: "https://pbs.twimg.com/media/test.jpg",
	}

	assert.Equal(t, "https://pbs.twimg.com/media/test.jpg", DownloadURLForMedia(media))
}

func TestIsMP4MediaURL(t *testing.T) {
	assert.True(t, IsMP4MediaURL("https://video.twimg.com/ext_tw_video/1/vid/1280x720/test.mp4?tag=12"))
	assert.False(t, IsMP4MediaURL("https://pbs.twimg.com/media/test.jpg?name=orig"))
}
