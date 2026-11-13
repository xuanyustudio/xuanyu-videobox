package subtitleburn

import (
	"path/filepath"
	"strings"
)

// Item is one video and the subtitle selected for it.
// An empty Subtitle means the caller must transcribe first.
type Item struct {
	Video    string
	Subtitle string
}

// Pair matches subtitles to videos by base name, ignoring extension and case.
// Subtitle priority is srt, ass, ssa, vtt, then anything else.
// The same priority keeps the earlier path. Only the first video of a name gets the subtitle.
func Pair(videos, subtitles []string) (items []Item, unused []string) {
	winners := map[string]int{}
	for i, sub := range subtitles {
		key := stemKey(sub)
		if prev, ok := winners[key]; ok {
			if extPriority(sub) < extPriority(subtitles[prev]) {
				winners[key] = i
			}
			continue
		}
		winners[key] = i
	}
	used := map[int]bool{}
	for _, video := range videos {
		item := Item{Video: video}
		if idx, ok := winners[stemKey(video)]; ok && !used[idx] {
			item.Subtitle = subtitles[idx]
			used[idx] = true
		}
		items = append(items, item)
	}
	for i, sub := range subtitles {
		if !used[i] {
			unused = append(unused, sub)
		}
	}
	return items, unused
}

func stemKey(path string) string {
	base := filepath.Base(path)
	return strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
}

func extPriority(path string) int {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "srt":
		return 0
	case "ass":
		return 1
	case "ssa":
		return 2
	case "vtt":
		return 3
	default:
		return 4
	}
}
