package main

import (
	"encoding/json"
	"time"
)

const DUPLICATE_THRESHOLD = 5 // Stop if 5 duplicates found in a row

func ProcessSync(rawMessages []json.RawMessage) SyncResponse {
	savedCount := 0
	duplicateStreak := 0
	stopRequired := false

	for _, msg := range rawMessages {
		// 1. 解析核心字段用于排重和模型填充
		var it InputTweet
		if err := json.Unmarshal(msg, &it); err != nil {
			PrintError(err)
			continue
		}

		// 2. 检查是否已存在
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

		// 3. 转换并保存，直接将原始字节转为 string 存入 RawJSON
		model := convertInputToModel(it, string(msg))
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

func convertInputToModel(it InputTweet, rawJSON string) *TweetModel {
	tm := &TweetModel{
		ID:           it.ID,
		FullText:     it.FullText,
		Name:         it.Name,
		ScreenName:   it.ScreenName,
		CreatedAt:    time.Unix(it.CreatedAt, 0),
		PermanentURL: it.PermanentURL,
		RawJSON:      rawJSON, // 完美拿到原始 JSON，无冗余发送
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
