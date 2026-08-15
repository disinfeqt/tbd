// Package store owns the SQLite database handle and the persisted models.
package store

import (
	"os"

	"github.com/rotisserie/eris"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var DB *gorm.DB

// Reset deletes the database and its WAL sidecar files (stale -wal files would
// otherwise replay old data into a fresh database). Media files are
// intentionally out of scope — this only ever touches the SQLite files.
func Reset(dbPath string) error {
	for _, path := range []string{dbPath, dbPath + "-wal", dbPath + "-shm"} {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return eris.Wrapf(err, "failed to remove %s", path)
		}
	}
	return nil
}

func Init(dbPath string) error {
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
