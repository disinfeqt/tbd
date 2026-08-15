package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/config"
)

func TestHandleSettingsGetAndPost(t *testing.T) {
	t.Cleanup(config.SwapForTest(config.Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: true,
	}))

	originalCwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() {
		require.NoError(t, os.Chdir(originalCwd))
	})

	getRecorder := httptest.NewRecorder()
	handleSettings(getRecorder, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	require.Equal(t, http.StatusOK, getRecorder.Code)
	assert.Contains(t, getRecorder.Body.String(), `"download_images":true`)

	postRecorder := httptest.NewRecorder()
	body := strings.NewReader(`{"download_images":false}`)
	req := httptest.NewRequest(http.MethodPost, "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	handleSettings(postRecorder, req)
	require.Equal(t, http.StatusOK, postRecorder.Code)
	assert.Contains(t, postRecorder.Body.String(), `"download_images":false`)
	assert.False(t, config.Current().DownloadImages)

	_, err = os.Stat(config.DefaultPath)
	require.NoError(t, err)
}
