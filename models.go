package main

import (
	"time"

	"gorm.io/gorm"
)

type TweetModel struct {
	ID           string    `gorm:"primaryKey" json:"id_str"` // Twitter uses id_str
	Name         string    `json:"name"`
	ScreenName   string    `json:"screen_name"`
	FullText     string    `json:"full_text"`
	CreatedAt    time.Time `json:"created_at"`
	PermanentURL string    `json:"permanent_url"`

	// Relationships
	Media []MediaModel `gorm:"foreignKey:TweetID" json:"media"`

	// Metadata
	RawJSON  string    `json:"-"`
	SyncedAt time.Time `json:"synced_at"`
}

func (TweetModel) TableName() string {
	return "tweets"
}

type MediaModel struct {
	ID      string `gorm:"primaryKey" json:"id_str"`
	TweetID string `gorm:"index" json:"tweet_id"`
	URL     string `json:"media_url_https"`
	Type    string `json:"type"` // photo, video, animated_gif

	// Download Status
	Downloaded bool `gorm:"default:false" json:"downloaded"`
	Failed     bool `gorm:"default:false" json:"failed"`
	RetryCount int  `gorm:"default:0" json:"retry_count"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

func (MediaModel) TableName() string {
	return "media"
}

func Migrate(db *gorm.DB) error {
	return db.AutoMigrate(&TweetModel{}, &MediaModel{})
}
