package download

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/store"
)

func TestFindAndRemoveTweetsWithDeletedMedia(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})
	require.NoError(t, store.Init(":memory:"))

	mediaDir := t.TempDir()
	cfg := config.Default()
	cfg.MediaDir = mediaDir
	t.Cleanup(config.SwapForTest(cfg))

	createdAt := time.Date(2025, 3, 1, 12, 0, 0, 0, time.Local)
	tweets := []store.TweetModel{
		{ // File still on disk → kept.
			ID: "1", ScreenName: "alice", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m1", TweetID: "1", Type: "photo", URL: "https://pbs.twimg.com/media/a.jpg", Downloaded: true},
			},
		},
		{ // File deleted → removed.
			ID: "2", ScreenName: "bob", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m2", TweetID: "2", Type: "photo", URL: "https://pbs.twimg.com/media/b.jpg", Downloaded: true},
			},
		},
		{ // One of two files remains → kept.
			ID: "3", ScreenName: "carol", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m3", TweetID: "3", Index: 0, Type: "photo", URL: "https://pbs.twimg.com/media/c.jpg", Downloaded: true},
				{ID: "m4", TweetID: "3", Index: 1, Type: "photo", URL: "https://pbs.twimg.com/media/d.jpg", Downloaded: true},
			},
		},
		{ // Nothing downloaded → never a candidate.
			ID: "4", ScreenName: "dave", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m5", TweetID: "4", Type: "video", URL: "https://video.twimg.com/v.mp4", Downloaded: false},
			},
		},
		{ID: "5", ScreenName: "erin", CreatedAt: createdAt}, // Text-only → never a candidate.
	}
	require.NoError(t, store.DB.Create(&tweets).Error)

	stamp := createdAt.Format("20060102-150405")
	for _, name := range []string{
		"twitter-@alice-" + stamp + "-1.jpg",
		"twitter-@carol-" + stamp + "-3-0.jpg",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(mediaDir, name), []byte("x"), 0o644))
	}

	deleted, err := FindTweetsWithDeletedMedia()
	require.NoError(t, err)
	require.Len(t, deleted, 1)
	assert.Equal(t, "2", deleted[0].ID)

	require.NoError(t, RemoveTweets(deleted))

	var tweetCount, mediaCount int64
	require.NoError(t, store.DB.Model(&store.TweetModel{}).Count(&tweetCount).Error)
	require.NoError(t, store.DB.Model(&store.MediaModel{}).Count(&mediaCount).Error)
	assert.EqualValues(t, 4, tweetCount)
	assert.EqualValues(t, 4, mediaCount) // m2 gone with its tweet

	var remaining []string
	require.NoError(t, store.DB.Model(&store.TweetModel{}).Order("id").Pluck("id", &remaining).Error)
	assert.Equal(t, []string{"1", "3", "4", "5"}, remaining)
}

func TestFindTweetsWithDeletedMediaMissingDir(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})
	require.NoError(t, store.Init(":memory:"))

	cfg := config.Default()
	cfg.MediaDir = filepath.Join(t.TempDir(), "does-not-exist")
	t.Cleanup(config.SwapForTest(cfg))

	_, err := FindTweetsWithDeletedMedia()
	require.Error(t, err)
}
