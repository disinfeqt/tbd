package download

import (
	"github.com/rotisserie/eris"

	"twitter-bookmarks-downloader/internal/logx"
	"twitter-bookmarks-downloader/internal/store"
	"twitter-bookmarks-downloader/internal/twitter"
)

func MatchingMediaEntity(media store.MediaModel, entities []twitter.MediaEntity) (twitter.MediaEntity, bool) {
	if media.ID != "" {
		for _, entity := range entities {
			if entity.IDStr == media.ID {
				return entity, true
			}
		}
	}

	if media.Index >= 0 && media.Index < len(entities) {
		return entities[media.Index], true
	}

	return twitter.MediaEntity{}, false
}

func RepairVideoMediaURLs() (int, error) {
	var mediaList []store.MediaModel
	if err := store.DB.
		Where("type IN ? AND url NOT LIKE ?", []string{"video", "animated_gif"}, "%.mp4%").
		Find(&mediaList).Error; err != nil {
		return 0, eris.Wrap(err, "failed to query video media for repair")
	}

	repaired := 0

	for _, media := range mediaList {
		var tweet store.TweetModel
		if err := store.DB.Select("id", "raw_json").First(&tweet, "id = ?", media.TweetID).Error; err != nil {
			logx.Warnf("Skipping video media repair for %s: raw tweet JSON not found", media.ID)
			continue
		}

		entities, err := twitter.MediaEntitiesFromRawTweet(tweet.RawJSON)
		if err != nil {
			logx.Warnf("Skipping video media repair for %s: raw tweet JSON could not be parsed", media.ID)
			continue
		}

		entity, ok := MatchingMediaEntity(media, entities)
		if !ok {
			continue
		}

		nextURL := twitter.DownloadURLForMedia(entity)
		if nextURL == "" || nextURL == media.URL {
			continue
		}

		if err := store.DB.Model(&store.MediaModel{}).
			Where("id = ?", media.ID).
			Updates(map[string]interface{}{
				"url":         nextURL,
				"downloaded":  false,
				"failed":      false,
				"retry_count": 0,
			}).Error; err != nil {
			return repaired, eris.Wrapf(err, "failed to update media %s", media.ID)
		}

		repaired++
	}

	return repaired, nil
}
