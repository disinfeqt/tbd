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

func syntheticMP4(width, height, durationMs int) []byte {
	tkhd := make([]byte, 84)
	binary.BigEndian.PutUint32(tkhd[76:], uint32(width)<<16)
	binary.BigEndian.PutUint32(tkhd[80:], uint32(height)<<16)

	mvhd := make([]byte, 20)
	binary.BigEndian.PutUint32(mvhd[12:], 1000) // timescale: 1 unit = 1ms
	binary.BigEndian.PutUint32(mvhd[16:], uint32(durationMs))

	var trak bytes.Buffer
	writeBox(&trak, "tkhd", tkhd)
	var moov bytes.Buffer
	writeBox(&moov, "mvhd", mvhd)
	writeBox(&moov, "trak", trak.Bytes())
	var file bytes.Buffer
	writeBox(&file, "ftyp", []byte("isom0000"))
	writeBox(&file, "moov", moov.Bytes())
	return file.Bytes()
}

func TestBackfillMediaMetadata(t *testing.T) {
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
		{ // Video duration and dimensions available in raw JSON.
			ID: "5", ScreenName: "erin", CreatedAt: createdAt,
			RawJSON: `{"legacy":{"extended_entities":{"media":[{"id_str":"m5","type":"video","original_info":{"width":720,"height":1280},"video_info":{"duration_millis":21500}}]}}}`,
			Media: []store.MediaModel{
				{ID: "m5", TweetID: "5", Type: "video", URL: "https://video.twimg.com/e.mp4"},
			},
		},
	}
	require.NoError(t, store.DB.Create(&tweets).Error)

	var pngBuf bytes.Buffer
	require.NoError(t, png.Encode(&pngBuf, image.NewRGBA(image.Rect(0, 0, 320, 200))))
	require.NoError(t, os.WriteFile(
		filepath.Join(mediaDir, "twitter-@bob-"+stamp+"-2.png"), pngBuf.Bytes(), 0o644))
	require.NoError(t, os.WriteFile(
		filepath.Join(mediaDir, "twitter-@carol-"+stamp+"-3.mp4"), syntheticMP4(1280, 720, 7000), 0o644))

	filled, err := BackfillMediaMetadata()
	require.NoError(t, err)
	assert.Equal(t, 4, filled)

	row := func(id string) store.MediaModel {
		var m store.MediaModel
		require.NoError(t, store.DB.First(&m, "id = ?", id).Error)
		return m
	}
	m := row("m1")
	assert.Equal(t, [2]int{640, 480}, [2]int{m.Width, m.Height})
	m = row("m2")
	assert.Equal(t, [2]int{320, 200}, [2]int{m.Width, m.Height})
	m = row("m3") // dimensions and duration both read from the MP4 header
	assert.Equal(t, [2]int{1280, 720}, [2]int{m.Width, m.Height})
	assert.Equal(t, 7000, m.DurationMs)
	m = row("m4")
	assert.Equal(t, [2]int{0, 0}, [2]int{m.Width, m.Height})
	m = row("m5") // dimensions and duration both from raw JSON
	assert.Equal(t, [2]int{720, 1280}, [2]int{m.Width, m.Height})
	assert.Equal(t, 21500, m.DurationMs)

	// Second run is a no-op for everything already filled.
	filled, err = BackfillMediaMetadata()
	require.NoError(t, err)
	assert.Equal(t, 0, filled)
}
