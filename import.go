package main

import (
	"encoding/json"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"

	"github.com/rotisserie/eris"
	"gorm.io/gorm/clause"
)

// LegacyTweet matches the JSON structure from the old twitter-scraper.
type LegacyTweet struct {
	ID           string    `json:"ID"`
	Username     string    `json:"Username"`
	Name         string    `json:"Name"`
	Text         string    `json:"Text"`
	TimeParsed   time.Time `json:"TimeParsed"`
	PermanentURL string    `json:"PermanentURL"`
	OrderedMedia []struct {
		ID   string `json:"ID"`
		Type string `json:"Type"` // "photo", "video", "animated_gif"
		URL  string `json:"URL"`
	} `json:"OrderedMedia"`
}

func ImportLegacyData(tweetsDir string) error {
	PrintInfoF("Starting legacy import from: %s", tweetsDir)

	files, err := os.ReadDir(tweetsDir)
	if err != nil {
		return eris.Wrap(err, "failed to read tweets directory")
	}

	count := 0
	skipped := 0

	for _, file := range files {
		if file.IsDir() || filepath.Ext(file.Name()) != ".json" {
			continue
		}

		filePath := filepath.Join(tweetsDir, file.Name())
		if err := processLegacyFile(filePath); err != nil {
			PrintError(eris.Wrapf(err, "Failed to import %s", file.Name()))
			continue
		}
		count++
	}

	PrintInfoF("Legacy import complete. Processed: %d, Skipped (existing): %d", count, skipped)
	return nil
}

func processLegacyFile(filePath string) error {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return eris.Wrap(err, "failed to read file")
	}

	var lt LegacyTweet
	if err := json.Unmarshal(content, &lt); err != nil {
		return eris.Wrap(err, "failed to unmarshal legacy json")
	}

	// 1. Create TweetModel
	tweet := TweetModel{
		ID:           lt.ID,
		Name:         lt.Name,
		ScreenName:   lt.Username,
		FullText:     lt.Text,
		CreatedAt:    lt.TimeParsed,
		PermanentURL: lt.PermanentURL,
		RawJSON:      string(content), // Store the legacy JSON as is
		SyncedAt:     time.Now(),
	}

	// 2. Prepare MediaModels and Check File Existence
	for i, m := range lt.OrderedMedia {
		if !shouldDownloadMediaURL(m.URL) {
			continue
		}

		media := MediaModel{
			ID:      m.ID,
			TweetID: lt.ID,
			Index:   i,
			URL:     m.URL,
			Type:    m.Type,
		}

		// Self-healing: Check if the file already exists on disk
		if exists, err := checkMediaFileExists(&tweet, i, len(lt.OrderedMedia), m.URL); err == nil && exists {
			media.Downloaded = true
		}

		tweet.Media = append(tweet.Media, media)
	}

	// 3. Save to DB (Ignore duplicates)
	// We use Clauses to ignore if the tweet ID already exists.
	// However, we might want to update the RawJSON if it was empty?
	// For simplicity, we skip existing tweets to avoid overwriting newer data.
	if err := DB.Clauses(clause.OnConflict{DoNothing: true}).Create(&tweet).Error; err != nil {
		return eris.Wrap(err, "db create failed")
	}

	return nil
}

func checkMediaFileExists(tweet *TweetModel, index int, total int, mediaURL string) (bool, error) {
	parsedURL, err := url.Parse(mediaURL)
	if err != nil {
		return false, err
	}

	filename := buildFilename(tweet, index, total, parsedURL)
	fullPath := path.Join(config.MediaDir, filename)

	if info, err := os.Stat(fullPath); err == nil && !info.IsDir() {
		// File exists
		// Optional: We could check file size if we had Content-Length, but we don't.
		// Assuming existence is enough for legacy migration.
		return true, nil
	}

	return false, nil
}
