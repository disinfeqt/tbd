package export

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/store"
)

func TestExportUniqueHandles(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})

	require.NoError(t, store.Init(":memory:"))
	require.NoError(t, store.DB.Create(&[]store.TweetModel{
		{
			ID:         "1",
			ScreenName: "Alice",
			CreatedAt:  time.Now(),
		},
		{
			ID:         "2",
			ScreenName: "alice",
			CreatedAt:  time.Now(),
		},
		{
			ID:         "3",
			ScreenName: "Bob",
			CreatedAt:  time.Now(),
		},
		{
			ID:        "4",
			CreatedAt: time.Now(),
		},
	}).Error)

	outputPath := filepath.Join(t.TempDir(), "handles.json")
	count, err := UniqueHandles(outputPath)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var handles []string
	require.NoError(t, json.Unmarshal(content, &handles))
	assert.Equal(t, []string{"@alice", "@bob"}, handles)
}
