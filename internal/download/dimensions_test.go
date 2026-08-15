package download

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/store"
)

func writeBox(buf *bytes.Buffer, boxType string, payload []byte) {
	var header [4]byte
	binary.BigEndian.PutUint32(header[:], uint32(len(payload)+8))
	buf.Write(header[:])
	buf.WriteString(boxType)
	buf.Write(payload)
}

func syntheticMP4(width, height int) []byte {
	tkhd := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhd[76:], uint32(width)<<16)
	binary.BigEndian.PutUint32(tkhd[80:], uint32(height)<<16)

	var trak bytes.Buffer
	writeBox(&trak, "tkhd", tkhd)
	var moov bytes.Buffer
	writeBox(&moov, "trak", trak.Bytes())
	var file bytes.Buffer
	writeBox(&file, "ftyp", []byte("isom0000"))
	writeBox(&file, "moov", moov.Bytes())
	return file.Bytes()
}

func TestBackfillMediaDimensions(t *testing.T) {
	originalDB := store.DB
	t.Cleanup(func() {
		store.DB = originalDB
	})
	require.NoError(t, store.Init(":memory:"))

	mediaDir := t.TempDir()
	cfg := config.Default()
	cfg.MediaDir = mediaDir
	t.Cleanup(config.SwapForTest(cfg))

	createdAt := time.Date(2025, 5, 1, 12, 0, 0, 0, time.Local)
	stamp := createdAt.Format("20060102-150405")

	tweets := []store.TweetModel{
		{ // Dimensions available in raw JSON — no file read needed.
			ID: "1", ScreenName: "alice", CreatedAt: createdAt,
			RawJSON: `{"legacy":{"extended_entities":{"media":[{"id_str":"m1","type":"photo","original_info":{"width":640,"height":480}}]}}}`,
			Media: []store.MediaModel{
				{ID: "m1", TweetID: "1", Type: "photo", URL: "https://pbs.twimg.com/media/a.jpg", Downloaded: true},
			},
		},
		{ // No raw JSON — dimensions read from the PNG on disk.
			ID: "2", ScreenName: "bob", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m2", TweetID: "2", Type: "photo", URL: "https://pbs.twimg.com/media/b.png", Downloaded: true},
			},
		},
		{ // No raw JSON — dimensions read from the MP4 header.
			ID: "3", ScreenName: "carol", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m3", TweetID: "3", Type: "video", URL: "https://video.twimg.com/v.mp4", Downloaded: true},
			},
		},
		{ // Nothing to go on: no raw JSON, not downloaded — stays 0.
			ID: "4", ScreenName: "dave", CreatedAt: createdAt,
			Media: []store.MediaModel{
				{ID: "m4", TweetID: "4", Type: "photo", URL: "https://pbs.twimg.com/media/d.jpg"},
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweets).Error)

	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 320, 200))))
	require.NoError(t, os.WriteFile(
		filepath.Join(mediaDir, "twitter-@bob-"+stamp+"-2.png"), pngBuf.Bytes(), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mediaDir, "twitter-@carol-"+stamp+"-3.mp4"), syntheticMP4(1280, 720), 0o644))

	filled, err := BackfillMediaDimensions()
	require.NoError(t, err)
	assert.Equal(t, 3, filled)

	dims := func(id string) (int, int) {
		var m store.MediaModel
		require.NoError(t, store.DB.First(&m, "id = ?", id).Error)
		return m.Width, m.Height
	}
	w, h := dims("m1")
	assert.Equal(t, [2]int{640, 480}, [2]int{w, h})
	w, h = dims("m2")
	assert.Equal(t, [2]int{320, 200}, [2]int{w, h})
	w, h = dims("m3")
	assert.Equal(t, [2]int{1280, 720}, [2]int{w, h})
	w, h = dims("m4")
	assert.Equal(t, [2]int{0, 0}, [2]int{w, h})

	// Second run is a no-op for everything already filled.
	filled, err = BackfillMediaDimensions()
	require.NoError(t, err)
	assert.Equal(t, 0, filled)
}
