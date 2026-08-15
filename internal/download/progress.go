package download

import (
	"path/filepath"
	"sort"
	"sync"

	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/store"
)

// ActiveDownload is a file currently being written, with live byte counts.
type ActiveDownload struct {
	File    string `json:"file"`
	Written int64  `json:"written"`
	Total   int64  `json:"total"` // 0 when the server sent no Content-Length
}

// QueueStatus is a snapshot of the download queue for the dashboard.
type QueueStatus struct {
	Total      int64            `json:"total"`
	Downloaded int64            `json:"downloaded"`
	Pending    int64            `json:"pending"`
	Paused     int64            `json:"paused"` // excluded by download settings
	Failed     int64            `json:"failed"`
	Active     []ActiveDownload `json:"active"`
}

var (
	activeMu sync.Mutex
	active   = map[string]*ActiveDownload{}
)

// trackDownload registers an in-flight download and returns its cleanup.
func trackDownload(path string, total int64) func() {
	activeMu.Lock()
	active[path] = &ActiveDownload{File: filepath.Base(path), Total: total}
	activeMu.Unlock()
	return func() {
		activeMu.Lock()
		delete(active, path)
		activeMu.Unlock()
	}
}

func trackProgress(path string, written int64) {
	activeMu.Lock()
	if entry, ok := active[path]; ok {
		entry.Written = written
	}
	activeMu.Unlock()
}

// Status reports queue counts and the files currently downloading.
func Status() (QueueStatus, error) {
	var s QueueStatus
	if err := store.DB.Model(&store.MediaModel{}).Count(&s.Total).Error; err != nil {
		return s, eris.Wrap(err, "failed to count media")
	}
	if err := store.DB.Model(&store.MediaModel{}).
		Where("downloaded = ?", true).Count(&s.Downloaded).Error; err != nil {
		return s, eris.Wrap(err, "failed to count downloaded media")
	}
	if err := store.DB.Model(&store.MediaModel{}).
		Where("failed = ?", true).Count(&s.Failed).Error; err != nil {
		return s, eris.Wrap(err, "failed to count failed media")
	}
	if err := pendingQuery().Count(&s.Pending).Error; err != nil {
		return s, eris.Wrap(err, "failed to count pending media")
	}
	s.Paused = s.Total - s.Downloaded - s.Failed - s.Pending
	if s.Paused < 0 {
		s.Paused = 0
	}

	activeMu.Lock()
	for _, entry := range active {
		s.Active = append(s.Active, *entry)
	}
	activeMu.Unlock()
	sort.Slice(s.Active, func(i, j int) bool { return s.Active[i].File < s.Active[j].File })
	return s, nil
}
