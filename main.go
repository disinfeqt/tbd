package main

import (
	"context"
	"fmt"
	"log"

	twitterscraper "github.com/imperatrona/twitter-scraper"
)

const MAX_BOOKMARKS_COUNT = 5

func main() {
	cookies := parseCookie()

	scraper := twitterscraper.New()
	scraper.SetCookies(cookies)

	// Call IsLoggedIn method to perform authentication
	if !scraper.IsLoggedIn() {
		log.Fatal("Invalid cookies")
	}

	for tweet := range scraper.GetBookmarks(context.Background(), MAX_BOOKMARKS_COUNT) {
		if tweet.Error != nil {
			log.Println("Error getting bookmark:", tweet.Error)
			continue // Continue process next bookmark
		}
		fmt.Printf("%s: %s\n", tweet.Username, tweet.Text)
	}
}
