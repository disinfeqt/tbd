package main

import (
	"time"
)

const DUPLICATE_THRESHOLD = 5 // Stop if 5 duplicates found in a row

func ProcessSync(inputTweets []InputTweet) SyncResponse {
	savedCount := 0
	duplicateStreak := 0
	stopRequired := false

	for _, it := range inputTweets {
		// Check if exists
		var exists int64
		DB.Model(&TweetModel{}).Where("id = ?", it.ID).Count(&exists)

		if exists > 0 {
			duplicateStreak++
			PrintInfoF("Skipping existing tweet: %s (Streak: %d)", it.ID, duplicateStreak)
			if duplicateStreak >= DUPLICATE_THRESHOLD {
				stopRequired = true
			}
			continue
		}

		// Reset streak on new tweet
		duplicateStreak = 0

		// Convert and Save
		model := convertInputToModel(it)
		if err := DB.Create(model).Error; err != nil {
			PrintError(err)
			continue
		}
		savedCount++
		PrintInfoF("Saved new tweet: %s", it.ID)
	}

	return SyncResponse{
		Success:      true,
		Message:      "Sync processed",
		StopRequired: stopRequired,
		SavedCount:   savedCount,
	}
}

func convertInputToModel(it InputTweet) *TweetModel {
	tm := &TweetModel{
		ID:           it.ID,
		FullText:     it.FullText,
		Name:         it.Name,
		ScreenName:   it.ScreenName,
		CreatedAt:    time.Unix(it.CreatedAt, 0),
		PermanentURL: it.PermanentURL,
		SyncedAt:     time.Now(),
	}

	for _, im := range it.Media {
		tm.Media = append(tm.Media, MediaModel{
			ID:        im.ID,
			TweetID:   it.ID,
			Type:      im.Type,
			URL:       im.URL,
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		})
	}
	return tm
}
