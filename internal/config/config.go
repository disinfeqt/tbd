// Package config owns the persisted app settings and their in-memory copy.
package config

import (
	"encoding/json"
	"os"
	"sync"

	"github.com/rotisserie/eris"
)

type Config struct {
	MediaDir       string `json:"media_dir"`
	DownloadVideos bool   `json:"download_videos"`
	DownloadImages bool   `json:"download_images"`
}

// DefaultPath is where the app reads and writes its settings file.
const DefaultPath = "config.json"

var mu sync.RWMutex

var current Config = Default()

func Default() Config {
	return Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: true,
	}
}

func Load(path string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Update(path, Default())
		}
		return eris.Wrap(err, "failed to read config")
	}

	next := Default()
	if err := json.Unmarshal(content, &next); err != nil {
		return eris.Wrap(err, "failed to parse config")
	}

	return Update(path, next)
}

func Current() Config {
	mu.RLock()
	defer mu.RUnlock()

	return current
}

func Update(path string, next Config) error {
	if next.MediaDir == "" {
		next.MediaDir = "media"
	}

	mu.Lock()
	defer mu.Unlock()

	if err := Save(path, next); err != nil {
		return err
	}

	current = next
	return nil
}

func Save(path string, next Config) error {
	content, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return eris.Wrap(err, "failed to encode config")
	}

	return eris.Wrap(os.WriteFile(path, append(content, '\n'), 0o644), "failed to write config")
}

// SwapForTest replaces the in-memory config without touching disk and returns a
// restore function. Test helper only.
func SwapForTest(next Config) (restore func()) {
	mu.Lock()
	previous := current
	current = next
	mu.Unlock()

	return func() {
		mu.Lock()
		current = previous
		mu.Unlock()
	}
}
