package store

import (
	"time"

	"gorm.io/gorm"
)

// AccountModel is one X account TBD archives for. Each account keeps its own
// media folder and download settings, so two accounts never write over each
// other's files.
type AccountModel struct {
	ID     string `gorm:"primaryKey" json:"id"` // X user id, read from the twid cookie
	Handle string `json:"handle"`               // @screen_name, a label only

	// MediaDir is where this account's downloads land. Empty means the global
	// default from config.json.
	MediaDir       string `json:"media_dir"`
	DownloadVideos bool   `json:"download_videos"`
	DownloadImages bool   `json:"download_images"`

	CreatedAt  time.Time `json:"created_at"`
	LastSyncAt time.Time `json:"last_sync_at"`
}

func (AccountModel) TableName() string {
	return "accounts"
}

// AccountBookmarkModel records that an account has a tweet bookmarked. Two
// accounts can bookmark the same tweet, so the tweet is stored once and each
// account gets its own membership row pointing at it.
type AccountBookmarkModel struct {
	AccountID string    `gorm:"primaryKey" json:"account_id"`
	TweetID   string    `gorm:"primaryKey;index" json:"tweet_id"`
	SyncedAt  time.Time `json:"synced_at"`
}

func (AccountBookmarkModel) TableName() string {
	return "account_bookmarks"
}

type TweetModel struct {
	ID           string    `gorm:"primaryKey" json:"id_str"` // Twitter uses id_str
	Name         string    `json:"name"`
	ScreenName   string    `json:"screen_name"`
	FullText     string    `json:"full_text"`
	CreatedAt    time.Time `json:"created_at"`
	PermanentURL string    `json:"permanent_url"`

	// Relationships
	Media []MediaModel `gorm:"foreignKey:TweetID" json:"media"`

	// AccountID is the account whose media folder holds this tweet's files — the
	// first one to bookmark it. Who currently has it bookmarked is a separate
	// question, answered by account_bookmarks.
	AccountID string `gorm:"index" json:"account_id"`

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
	return db.AutoMigrate(&AccountModel{}, &AccountBookmarkModel{}, &TweetModel{}, &MediaModel{})
}
