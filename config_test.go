package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func withTestConfig(t *testing.T, next Config) {
	t.Helper()

	original := CurrentConfig()
	configMu.Lock()
	config = next
	configMu.Unlock()

	t.Cleanup(func() {
		configMu.Lock()
		config = original
		configMu.Unlock()
	})
}

func TestShouldDownloadMediaURLUsesSettings(t *testing.T) {
	withTestConfig(t, Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: false,
	})

	assert.True(t, shouldDownloadMediaURL("https://video.twimg.com/ext_tw_video/1/vid/1280x720/test.mp4?tag=12"))
	assert.False(t, shouldDownloadMediaURL("https://pbs.twimg.com/media/test.jpg?name=orig"))
	assert.False(t, shouldDownloadMediaURL(""))
}

func TestShouldDownloadMediaURLAllowsImagesWhenEnabled(t *testing.T) {
	withTestConfig(t, Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: true,
	})

	assert.True(t, shouldDownloadMediaURL("https://pbs.twimg.com/media/test.jpg?name=orig"))
}

func TestDefaultConfigDownloadsImagesAndVideos(t *testing.T) {
	assert.True(t, config.DownloadVideos)
	assert.True(t, config.DownloadImages)
}
