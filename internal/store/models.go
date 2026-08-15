package store

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
	Index   int    `gorm:"default:0" json:"index"` // Original order in tweet
	URL     string `json:"media_url_https"`
	Type    string `json:"type"` // photo, video, animated_gif

	// Pixel dimensions, backfilled from raw tweet JSON or the file on disk;
	// 0 when unknown. The explorer uses them to reserve layout space.
	Width  int `gorm:"default:0" json:"width"`
	Height int `gorm:"default:0" json:"height"`

	// Video duration in milliseconds, same sources as the dimensions;
	// 0 for photos and when unknown.
	DurationMs int `gorm:"default:0" json:"duration_ms"`

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
