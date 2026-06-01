package main

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/rotisserie/eris"
)

func ExportUniqueHandles(outputPath string) (int, error) {
	var screenNames []string
	if err := DB.Model(&TweetModel{}).
		Where("screen_name <> ?", "").
		Pluck("screen_name", &screenNames).Error; err != nil {
		return 0, eris.Wrap(err, "failed to query screen names")
	}

	seen := make(map[string]struct{}, len(screenNames))
	handles := make([]string, 0, len(screenNames))
	for _, screenName := range screenNames {
		normalized := strings.ToLower(strings.TrimSpace(screenName))
		if normalized == "" {
			continue
		}

		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		handles = append(handles, "@"+normalized)
	}

	sort.Strings(handles)

	content, err := json.MarshalIndent(handles, "", "  ")
	if err != nil {
		return 0, eris.Wrap(err, "failed to encode handles")
	}

	if err := os.WriteFile(outputPath, append(content, '\n'), 0o644); err != nil {
		return 0, eris.Wrap(err, "failed to write handles export")
	}

	return len(handles), nil
}
