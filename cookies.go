package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"time"
)

type jsonCookie struct {
	Name           string  `json:"name"`
	Value          string  `json:"value"`
	Path           string  `json:"path"`
	Domain         string  `json:"domain"`
	ExpirationDate float64 `json:"expirationDate"`
	Secure         bool    `json:"secure"`
	HttpOnly       bool    `json:"httpOnly"`
	SameSite       string  `json:"sameSite"`
}

func parseCookie() []*http.Cookie {
	f, err := os.Open("x.com_cookies.json")
	if err != nil {
		log.Println(err)
		log.Fatal("Error opening cookies file")
	}
	defer func() {
		if err := f.Close(); err != nil {
			log.Fatal("Error closing file:", err)
		}
	}()

	var jsonCookies []jsonCookie
	err = json.NewDecoder(f).Decode(&jsonCookies)
	if err != nil {
		log.Println(err)
		log.Fatal("Invalid cookies file format")
	}

	var cookies []*http.Cookie
	for _, jc := range jsonCookies {

		cookie := &http.Cookie{
			Name:     jc.Name,
			Value:    url.QueryEscape(jc.Value),
			Path:     jc.Path,
			Domain:   jc.Domain,
			Secure:   jc.Secure,
			HttpOnly: jc.HttpOnly,
			Expires:  convertExpirationDate(jc.ExpirationDate),
		}

		// twitter-scraper require domain set to twitter.com to authenticate successfully
		switch cookie.Domain {
		case "x.com":
			cookie.Domain = "twitter.com"
		case ".x.com":
			cookie.Domain = ".twitter.com"
		}

		switch jc.SameSite {
		case "lax":
			cookie.SameSite = http.SameSiteLaxMode
		case "strict":
			cookie.SameSite = http.SameSiteStrictMode
		case "none":
			cookie.SameSite = http.SameSiteNoneMode
		case "unspecified":
			cookie.SameSite = http.SameSiteDefaultMode
		case "no_restriction":
			cookie.SameSite = http.SameSiteNoneMode
		default:
			log.Printf("Warning: Unknown SameSite value '%s', defaulting to SameSiteDefaultMode\n", jc.SameSite)
			cookie.SameSite = http.SameSiteDefaultMode
		}
		cookies = append(cookies, cookie)
	}

	return cookies
}

func convertExpirationDate(timestamp float64) time.Time {
	seconds := int64(timestamp)
	nanoseconds := int64((timestamp - float64(seconds)) * 1e9)
	return time.Unix(seconds, nanoseconds)
}
