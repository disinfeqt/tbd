package download

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/store"
)

func TestShouldDownloadURLUsesSettings(t *testing.T) {
	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: false,
	}))

	assert.True(t, ShouldDownloadURL("https://video.twimg.com/ext_tw_video/1/vid/1280x720/test.mp4?tag=12"))
	assert.False(t, ShouldDownloadURL("https://pbs.twimg.com/media/test.jpg?name=orig"))
	assert.False(t, ShouldDownloadURL(""))
}

func TestShouldDownloadURLAllowsImagesWhenEnabled(t *testing.T) {
	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: true,
	}))

	assert.True(t, ShouldDownloadURL("https://pbs.twimg.com/media/test.jpg?name=orig"))
}

func TestShouldDownloadUsesMediaType(t *testing.T) {
	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: false,
	}))

	assert.False(t, ShouldDownload(store.MediaModel{
		Type: "photo",
		URL:  "https://pbs.twimg.com/media/test.jpg?name=orig",
	}))
	assert.True(t, ShouldDownload(store.MediaModel{
		Type: "video",
		URL:  "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/thumb.jpg",
	}))
}
