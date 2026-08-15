package download

import (
	"encoding/binary"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/config"
	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
	"twitter-bookmarks-downloader/internal/twitter"
)

// BackfillMediaMetadata fills in width/height (and, for videos, duration) for
// media rows that lack them, first from the stored raw tweet JSON and
// otherwise by reading the header of the downloaded file itself. Returns how
// many rows were filled. Rows whose metadata cannot be determined are left at
// 0 and retried on the next run, which is cheap because only headers are read.
func BackfillMediaMetadata() (int, error) {
	var tweets []store.TweetModel
	if err := store.DB.
		Where(`EXISTS (SELECT 1 FROM media WHERE media.tweet_id = tweets.id
			AND (media.width <= 0 OR media.height <= 0
				OR (media.type = 'video' AND media.duration_ms <= 0)))`).
		Preload("Media").
		Find(&tweets).Error; err != nil {
		return 0, eris.Wrap(err, "failed to query media lacking metadata")
	}

	mediaDir := config.Current().MediaDir
	filled := 0

	for i := range tweets {
		tweet := &tweets[i]
		var entities []twitter.MediaEntity
		if tweet.RawJSON != "" {
			entities, _ = twitter.MediaEntitiesFromRawTweet(tweet.RawJSON)
		}

		for _, media := range tweet.Media {
			filePath := ""
			if media.Downloaded {
				if parsedURL, err := url.Parse(media.URL); err == nil {
					name := BuildFilename(tweet, media.Index, len(tweet.Media), parsedURL)
					filePath = filepath.Join(mediaDir, name)
				}
			}
			entity, hasEntity := MatchingMediaEntity(media, entities)

			updates := map[string]interface{}{}
			if media.Width <= 0 || media.Height <= 0 {
				width, height := 0, 0
				if hasEntity {
					width, height = entity.OriginalInfo.Width, entity.OriginalInfo.Height
				}
				if (width <= 0 || height <= 0) && filePath != "" {
					width, height = fileDimensions(filePath)
				}
				if width > 0 && height > 0 {
					updates["width"], updates["height"] = width, height
				}
			}
			if media.Type == "video" && media.DurationMs <= 0 {
				duration := 0
				if hasEntity {
					duration = entity.VideoInfo.DurationMillis
				}
				if duration <= 0 && filePath != "" {
					duration = fileDurationMs(filePath)
				}
				if duration > 0 {
					updates["duration_ms"] = duration
				}
			}
			if len(updates) == 0 {
				continue
			}

			if err := store.DB.Model(&store.MediaModel{}).
				Where("id = ?", media.ID).
				Updates(updates).Error; err != nil {
				return filled, eris.Wrapf(err, "failed to update metadata for media %s", media.ID)
			}
			filled++
		}
	}

	return filled, nil
}

// fileDimensions reads pixel dimensions from a media file's header. Returns
// zeros when the file is absent or the format is not recognized.
func fileDimensions(path string) (int, int) {
	f, err := os.Open(path)
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	if strings.EqualFold(filepath.Ext(path), ".mp4") {
		return mp4Dimensions(f)
	}

	cfg, _, err := image.DecodeConfig(f)
	if err != nil {
		logx.Warnf("Could not read image dimensions from %s: %v", filepath.Base(path), err)
		return 0, 0
	}
	return cfg.Width, cfg.Height
}

// fileDurationMs reads the duration of an MP4 file in milliseconds. Returns 0
// when the file is absent or the header cannot be parsed.
func fileDurationMs(path string) int {
	if !strings.EqualFold(filepath.Ext(path), ".mp4") {
		return 0
	}
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	return mp4DurationMs(f)
}

// mp4DurationMs walks the MP4 box tree (moov → mvhd) and converts the movie
// duration from timescale units to milliseconds.
func mp4DurationMs(r io.ReadSeeker) int {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return 0
	}
	moov, moovSize, ok := findBox(r, 0, size, "moov")
	if !ok {
		return 0
	}
	mvhd, mvhdSize, ok := findBox(r, moov, moov+moovSize, "mvhd")
	if !ok {
		return 0
	}
	buf := make([]byte, mvhdSize)
	if _, err := r.Seek(mvhd, io.SeekStart); err != nil {
		return 0
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0
	}
	// Version 0: 32-bit timescale at byte 12, duration at 16.
	// Version 1: 64-bit times push timescale to byte 20, duration to 24.
	var timescale, duration uint64
	if len(buf) > 0 && buf[0] == 1 {
		if len(buf) < 32 {
			return 0
		}
		timescale = uint64(binary.BigEndian.Uint32(buf[20:]))
		duration = binary.BigEndian.Uint64(buf[24:])
	} else {
		if len(buf) < 20 {
			return 0
		}
		timescale = uint64(binary.BigEndian.Uint32(buf[12:]))
		duration = uint64(binary.BigEndian.Uint32(buf[16:]))
	}
	if timescale == 0 {
		return 0
	}
	return int(duration * 1000 / timescale)
}

// mp4Dimensions walks the MP4 box tree (moov → trak → tkhd) and returns the
// first non-zero track dimensions, i.e. the video track.
func mp4Dimensions(r io.ReadSeeker) (int, int) {
	size, err := r.Seek(0, io.SeekEnd)
	if err != nil {
		return 0, 0
	}
	moov, moovSize, ok := findBox(r, 0, size, "moov")
	if !ok {
		return 0, 0
	}
	for offset := moov; offset < moov+moovSize; {
		trak, trakSize, ok := findBox(r, offset, moov+moovSize, "trak")
		if !ok {
			return 0, 0
		}
		if tkhd, tkhdSize, ok := findBox(r, trak, trak+trakSize, "tkhd"); ok {
			if w, h := parseTkhd(r, tkhd, tkhdSize); w > 0 && h > 0 {
				return w, h
			}
		}
		offset = trak + trakSize
	}
	return 0, 0
}

// findBox scans [start, end) for the first box with the given type and returns
// the offset and size of its payload.
func findBox(r io.ReadSeeker, start, end int64, boxType string) (int64, int64, bool) {
	for offset := start; offset+8 <= end; {
		header := make([]byte, 8)
		if _, err := r.Seek(offset, io.SeekStart); err != nil {
			return 0, 0, false
		}
		if _, err := io.ReadFull(r, header); err != nil {
			return 0, 0, false
		}
		boxSize := int64(binary.BigEndian.Uint32(header[:4]))
		payload := offset + 8
		if boxSize == 1 { // 64-bit largesize follows the header
			large := make([]byte, 8)
			if _, err := io.ReadFull(r, large); err != nil {
				return 0, 0, false
			}
			boxSize = int64(binary.BigEndian.Uint64(large))
			payload = offset + 16
		}
		if boxSize < 8 || offset+boxSize > end {
			return 0, 0, false
		}
		if string(header[4:8]) == boxType {
			return payload, offset + boxSize - payload, true
		}
		offset += boxSize
	}
	return 0, 0, false
}

// parseTkhd reads the 16.16 fixed-point width/height at the end of a tkhd box.
func parseTkhd(r io.ReadSeeker, offset, size int64) (int, int) {
	buf := make([]byte, size)
	if _, err := r.Seek(offset, io.SeekStart); err != nil {
		return 0, 0
	}
	if _, err := io.ReadFull(r, buf); err != nil {
		return 0, 0
	}
	// Version 0 boxes place width at byte 76, version 1 at byte 88.
	pos := 76
	if len(buf) > 0 && buf[0] == 1 {
		pos = 88
	}
	if len(buf) < pos+8 {
		return 0, 0
	}
	width := int(binary.BigEndian.Uint32(buf[pos:]) >> 16)
	height := int(binary.BigEndian.Uint32(buf[pos+4:]) >> 16)
	return width, height
}
