package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigDownloadsImagesAndVideos(t *testing.T) {
	current := Default()
	assert.True(t, current.DownloadVideos)
	assert.True(t, current.DownloadImages)
}

func TestLoadConfigCreatesDefaultFileWhenMissing(t *testing.T) {
	t.Cleanup(SwapForTest(Default()))

	configPath := filepath.Join(t.TempDir(), "config.json")
	require.NoError(t, Load(configPath))

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
	t.Cleanup(SwapForTest(original))

	err := Update(t.TempDir(), Config{
		MediaDir:       "other-media",
		DownloadVideos: false,
		DownloadImages: false,
	})

	require.Error(t, err)
	assert.Equal(t, original, Current())
}
