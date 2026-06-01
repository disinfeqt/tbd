package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestShouldDownloadMediaUsesMediaType(t *testing.T) {
	withTestConfig(t, Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: false,
	})

	assert.False(t, shouldDownloadMedia(MediaModel{
		Type: "photo",
		URL:  "https://pbs.twimg.com/media/test.jpg?name=orig",
	}))
	assert.True(t, shouldDownloadMedia(MediaModel{
		Type: "video",
		URL:  "https://pbs.twimg.com/ext_tw_video_thumb/1/pu/img/thumb.jpg",
	}))
}

func TestDefaultConfigDownloadsImagesAndVideos(t *testing.T) {
	current := CurrentConfig()
	assert.True(t, current.DownloadVideos)
	assert.True(t, current.DownloadImages)
}

func TestLoadConfigCreatesDefaultFileWhenMissing(t *testing.T) {
	withTestConfig(t, defaultConfig())

	configPath := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, LoadConfig(configPath))

	content, err := os.ReadFile(configPath)
	require.NoError(t, err)
	assert.Contains(t, string(content), `"media_dir": "media"`)
	assert.Contains(t, string(content), `"download_videos": true`)
	assert.Contains(t, string(content), `"download_images": true`)
}

func TestUpdateConfigDoesNotMutateMemoryWhenSaveFails(t *testing.T) {
	original := Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: true,
	}
	withTestConfig(t, original)

	err := UpdateConfig(t.TempDir(), Config{
		MediaDir:       "other-media",
		DownloadVideos: false,
		DownloadImages: false,
	})

	require.Error(t, err)
	assert.Equal(t, original, CurrentConfig())
}
