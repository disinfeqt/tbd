package main

import (
	"context"
	"runtime/debug"
	"strings"

	twitterscraper "github.com/imperatrona/twitter-scraper"
	"github.com/rotisserie/eris"
)

type Config struct {
	MediaDir          string
	TweetsDir         string
	MaxBookmarksCount int
}

var isReleaseBuild bool

var config Config = Config{
	MediaDir:          "media",
	TweetsDir:         "tweets",
	MaxBookmarksCount: 100000,
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

func main() {
	err := InitLogger("logs")
	if err != nil {
		FatalError(eris.Wrap(err, "Failed to initialize logger"))
	}
	defer CloseLogger()

	scraper := initScraper()

	processedBookmarksCount := 0
	var errors []error

	for tweet := range scraper.GetBookmarks(context.Background(), config.MaxBookmarksCount) {
		if tweet.Error != nil {
			err := eris.Wrap(tweet.Error, "Error getting bookmark")
			PrintError(err)
			continue // Continue process next bookmark
		}

		PrintInfoF("Processing Tweet: %s", tweet.ID)

		err := saveTweet(&tweet.Tweet, config)
		if err != nil {
			err = eris.Wrapf(err, " Tweet: %s", tweet.ID)
			PrintError(err)
			errors = append(errors, err)
		}

		err = downloadMedia(&tweet.Tweet, config)
		if err != nil {
			err = eris.Wrapf(err, " Tweet: %s", tweet.ID)
			PrintError(err)
			errors = append(errors, err)
		}

		processedBookmarksCount += 1
	}

	PrintInfoF("Successfully processed %d bookmarks", processedBookmarksCount)
	if len(errors) > 0 {
		PrintWarningF("  With %d errors", len(errors))
		for _, err := range errors {
			PrintError(err)
		}
	}
}

func initScraper() *twitterscraper.Scraper {
	cookies, err := parseCookie()
	if err != nil {
		err = eris.Wrapf(err, "Failed to parse cookies file: %s", COOKIES_FILENAME)
		FatalError(err)
	}

	scraper := twitterscraper.New()
	scraper.SetCookies(cookies)
	scraper.WithDelay(2)

	// Call IsLoggedIn method to perform authentication
	if !scraper.IsLoggedIn() {
		FatalError(eris.New("Invalid cookies"))
	}

	return scraper
}
