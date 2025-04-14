package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	twitterscraper "github.com/imperatrona/twitter-scraper"
	"github.com/rotisserie/eris"
)

const (
	MAX_BOOKMARKS_COUNT = 5
	MEDIA_DIR           = "media"
)

var isReleaseBuild bool

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
	cookies, err := parseCookie()
	if err != nil {
		err = eris.Wrap(err, "Error parsing cookies")
		FatalError(err)
		return
	}

	scraper := twitterscraper.New()
	scraper.SetCookies(cookies)

	// Call IsLoggedIn method to perform authentication
	if !scraper.IsLoggedIn() {
		FatalError(eris.New("Invalid cookies"))
	}

	for tweet := range scraper.GetBookmarks(context.Background(), MAX_BOOKMARKS_COUNT) {
		if tweet.Error != nil {
			err := eris.Wrap(tweet.Error, "Error getting bookmark")
			PrintError(err)
			continue // Continue process next bookmark
		}

		fmt.Printf("%s: %s\n", tweet.Username, tweet.Text)
		err := downloadPhotos(&tweet.Tweet)
		if err != nil {
			PrintError(err)
		}
	}
}
