package server

import (
	_ "embed"
	"net/http"
)

// The explorer's stylesheet and script live beside explore.html as their own
// files, and the manifest and icons let the page install to a home screen as
// a standalone app.

//go:embed explore.css
var exploreCSS []byte

//go:embed explore.js
var exploreJS []byte

//go:embed manifest.webmanifest
var pwaManifest []byte

//go:embed icon-192.png
var icon192 []byte

//go:embed icon-512.png
var icon512 []byte

//go:embed apple-touch-icon.png
var appleTouchIcon []byte

func registerAssetRoutes() {
	http.HandleFunc("/explore.css", serveAsset("text/css; charset=utf-8", exploreCSS))
	http.HandleFunc("/explore.js", serveAsset("text/javascript; charset=utf-8", exploreJS))
	http.HandleFunc("/manifest.webmanifest", serveAsset("application/manifest+json", pwaManifest))
	http.HandleFunc("/icon-192.png", serveAsset("image/png", icon192))
	http.HandleFunc("/icon-512.png", serveAsset("image/png", icon512))
	http.HandleFunc("/apple-touch-icon.png", serveAsset("image/png", appleTouchIcon))
}

func serveAsset(contentType string, body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", contentType)
		// The page must never outrun its stylesheet or script after a binary
		// update, and revalidation against a local server is free.
		w.Header().Set("Cache-Control", "no-cache")
		_, _ = w.Write(body)
	}
}
