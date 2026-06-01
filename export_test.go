package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportUniqueHandles(t *testing.T) {
	originalDB := DB
	t.Cleanup(func() {
		DB = originalDB
	})

	require.NoError(t, InitDB(":memory:"))
	require.NoError(t, DB.Create(&[]TweetModel{
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
	count, err := ExportUniqueHandles(outputPath)
	require.NoError(t, err)
	assert.Equal(t, 2, count)

	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)

	var handles []string
	require.NoError(t, json.Unmarshal(content, &handles))
	assert.Equal(t, []string{"@alice", "@bob"}, handles)
}
