package main

import (
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB(dbPath string) error {
	dsn := dbPath
	if dbPath != ":memory:" {
		// WAL lets the HTTP handler and the download worker write concurrently;
		// busy_timeout retries briefly instead of failing with "database is locked".
		dsn = dbPath + "?_journal_mode=WAL&_busy_timeout=5000"
	}

	var err error
	DB, err = gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	})
	if err != nil {
		return err
	}

	return Migrate(DB)
}
