package main

import (
	"encoding/json"
	"os"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/rotisserie/eris"
)

type Config struct {
	MediaDir       string `json:"media_dir"`
	DownloadVideos bool   `json:"download_videos"`
	DownloadImages bool   `json:"download_images"`
}

const configPath = "config.json"

var isReleaseBuild bool
var configMu sync.RWMutex

var config Config = defaultConfig()

func defaultConfig() Config {
	return Config{
		MediaDir:       "media",
		DownloadVideos: true,
		DownloadImages: true,
	}
}

func init() {
	bi, ok := debug.ReadBuildInfo()
	if ok {
		for _, setting := range bi.Settings {
			if setting.Key == "-tags" && strings.Contains(setting.Value, "release") {
				isReleaseBuild = true
				break
			}
		}
	}
}

func LoadConfig(configPath string) error {
	content, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return UpdateConfig(configPath, defaultConfig())
		}
		return eris.Wrap(err, "failed to read config")
	}

	next := defaultConfig()
	if err := json.Unmarshal(content, &next); err != nil {
		return eris.Wrap(err, "failed to parse config")
	}

	return UpdateConfig(configPath, next)
}

func CurrentConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()

	return config
}

func UpdateConfig(configPath string, next Config) error {
	if next.MediaDir == "" {
		next.MediaDir = "media"
	}

	configMu.Lock()
	defer configMu.Unlock()

	if err := SaveConfig(configPath, next); err != nil {
		return err
	}

	config = next
	return nil
}

func SaveConfig(configPath string, next Config) error {
	content, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return eris.Wrap(err, "failed to encode config")
	}

	return eris.Wrap(os.WriteFile(configPath, append(content, '\n'), 0o644), "failed to write config")
}

func shouldDownloadMediaURL(rawURL string) bool {
	if rawURL == "" {
		return false
	}

	current := CurrentConfig()
	if isMP4MediaURL(rawURL) {
		return current.DownloadVideos
	}

	return current.DownloadImages
}

func shouldDownloadMedia(media MediaModel) bool {
	current := CurrentConfig()

	switch media.Type {
	case "photo":
		return current.DownloadImages
	case "video", "animated_gif":
		return current.DownloadVideos
	default:
		return shouldDownloadMediaURL(media.URL)
	}
}
