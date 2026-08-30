package server

import (
	"context"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/accounts"
	"twitter-bookmarks-downloader/internal/logx"
)

// Poster frames for the video cards in the grid.
//
// iOS draws nothing at all for a video it has not played — not the first
// frame, not a preload, not a media fragment — so a card there cannot get its
// preview out of the <video> element. ffmpeg pulls a single frame per file the
// first time one is asked for, and it is served from disk ever after.

// Frames live in a hidden folder inside the media folder they came from, so
// they follow the media when it moves and are per-account like it is. Nothing
// ever lists that folder — the media it holds is looked up by exact name — so
// the extra directory is invisible to the rest of the app, and hidden keeps it
// out of the way in Finder, which "Reveal in Finder" opens.
const posterSubdir = ".posters"

// posterPath is where the frame for one media file belongs.
func posterPath(accountID, file string) string {
	return filepath.Join(accounts.MediaDir(accountID), posterSubdir, file+".jpg")
}

// posterWidth is generous for a retina card and small enough that several
// hundred frames stay negligible on disk.
const posterWidth = 480

// ffmpeg is optional. Without it this endpoint reports no poster and the cards
// fall back to whatever the video element manages on its own.
var (
	ffmpegOnce sync.Once
	ffmpegPath string
)

func ffmpeg() string {
	ffmpegOnce.Do(func() {
		path, err := exec.LookPath("ffmpeg")
		if err != nil {
			logx.Warn("ffmpeg not found — video cards will have no preview image")
			return
		}
		ffmpegPath = path
		logx.Infof("Poster frames enabled — using %s", path)
	})
	return ffmpegPath
}

// A scroll can ask for dozens of frames at once; run a few at a time so the
// machine stays usable.
var posterSlots = make(chan struct{}, 4)

func handlePoster(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// /poster/<account>/<file>, mirroring the media route it draws from.
	rest := strings.TrimPrefix(r.URL.Path, "/poster/")
	accountID, name, split := strings.Cut(rest, "/")
	if !split {
		accountID, name = accounts.Resolve(""), rest
	}
	decoded, err := url.PathUnescape(name)
	if err != nil || decoded == "" || filepath.Base(decoded) != decoded {
		http.NotFound(w, r)
		return
	}

	source := filepath.Join(accounts.MediaDir(accountID), decoded)
	info, err := os.Stat(source)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	cached := posterPath(accountID, decoded)
	if posterStale(cached, info.ModTime()) {
		if err := extractPoster(r.Context(), source, cached); err != nil {
			logx.Error(eris.Wrapf(err, "Failed to extract a poster from %s", decoded))
			http.NotFound(w, r)
			return
		}
	}

	// A frame never changes once pulled, and its name carries the file it came
	// from, so the browser can hold on to it.
	w.Header().Set("Cache-Control", "public, max-age=604800")
	http.ServeFile(w, r, cached)
}

// posterStale reports whether the cached frame is missing, empty, or older
// than the file it was taken from.
func posterStale(cached string, source time.Time) bool {
	info, err := os.Stat(cached)
	return err != nil || info.Size() == 0 || info.ModTime().Before(source)
}

func extractPoster(ctx context.Context, source, cached string) error {
	bin := ffmpeg()
	if bin == "" {
		return eris.New("ffmpeg is not installed")
	}
	if err := os.MkdirAll(filepath.Dir(cached), 0o755); err != nil {
		return eris.Wrap(err, "failed to create the poster directory")
	}

	posterSlots <- struct{}{}
	defer func() { <-posterSlots }()

	// Another request may have finished this frame while this one queued.
	if info, err := os.Stat(cached); err == nil && info.Size() > 0 {
		return nil
	}

	started := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Written alongside the target and renamed into place, so a half-written
	// frame is never served and two requests racing cannot tear one.
	tmp, err := os.CreateTemp(filepath.Dir(cached), ".poster-*.jpg")
	if err != nil {
		return eris.Wrap(err, "failed to create a temporary file")
	}
	_ = tmp.Close()
	defer func() { _ = os.Remove(tmp.Name()) }()

	// min() keeps a small source at its own size instead of blowing it up; the
	// comma inside it is escaped because commas separate filters.
	cmd := exec.CommandContext(ctx, bin,
		"-nostdin", "-loglevel", "error", "-y",
		"-i", source,
		"-frames:v", "1",
		"-vf", `scale=min(`+strconv.Itoa(posterWidth)+`\,iw):-2`,
		"-q:v", "5",
		"-f", "image2",
		tmp.Name(),
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return eris.Wrapf(err, "ffmpeg failed: %s", strings.TrimSpace(string(out)))
	}
	if err := os.Rename(tmp.Name(), cached); err != nil {
		return eris.Wrap(err, "failed to store the poster")
	}

	// One line per frame, like the download worker: these only appear the
	// first time a video is seen, and go quiet once the archive is covered.
	size := "?"
	if info, err := os.Stat(cached); err == nil {
		size = strconv.FormatInt(info.Size()/1024, 10) + " KB"
	}
	logx.Infof("  [Poster] Extracted: %s (%s, %s)", filepath.Base(source), size,
		time.Since(started).Round(time.Millisecond))
	return nil
}
