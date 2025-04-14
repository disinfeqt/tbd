package main

import (
	"context"
	"fmt"
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
	scraper := initScraper()

	for tweet := range scraper.GetBookmarks(context.Background(), config.MaxBookmarksCount) {
		if tweet.Error != nil {
			err := eris.Wrap(tweet.Error, "Error getting bookmark")
			PrintError(err)
			continue // Continue process next bookmark
		}

		fmt.Printf("Processing Tweet: %s\n", tweet.ID)
		err := saveTweet(&tweet.Tweet, config)
		if err != nil {
			PrintError(eris.Wrap(err, "Error:"))
		}
		err = downloadPhotos(&tweet.Tweet, config)
		if err != nil {
			PrintError(err)
		}
	}
}

func initScraper() *twitterscraper.Scraper {
	cookies, err := parseCookie()
	if err != nil {
		err = eris.Wrap(err, "Error parsing cookies")
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
