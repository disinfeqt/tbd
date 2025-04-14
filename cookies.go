package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/rotisserie/eris"
)

const COOKIES_FILENAME = "x.com_cookies.json"

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

func parseCookie() ([]*http.Cookie, error) {
	f, err := os.Open(COOKIES_FILENAME)
	if err != nil {
		return nil, eris.Wrap(err, "error opening cookies file")
	}
	defer CloseResource(f)

	var jsonCookies []jsonCookie
	err = json.NewDecoder(f).Decode(&jsonCookies)
	if err != nil {
		return nil, eris.Wrap(err, "invalid cookies file format")
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
			fmt.Printf("Warning: Unknown SameSite value '%s', defaulting to SameSiteDefaultMode\n", jc.SameSite)
			cookie.SameSite = http.SameSiteDefaultMode
		}
		cookies = append(cookies, cookie)
	}

	return cookies, nil
}

func convertExpirationDate(timestamp float64) time.Time {
	seconds := int64(timestamp)
	nanoseconds := int64((timestamp - float64(seconds)) * 1e9)
	return time.Unix(seconds, nanoseconds)
}
