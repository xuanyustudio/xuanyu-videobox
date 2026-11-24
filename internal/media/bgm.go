package media

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var bgmExts = map[string]struct{}{
	".mp3": {}, ".wav": {}, ".flac": {}, ".m4a": {}, ".aac": {}, ".ogg": {}, ".wma": {},
}

// IsAudioPath reports whether path looks like a BGM/SFX audio file.
func IsAudioPath(path string) bool {
	_, ok := bgmExts[strings.ToLower(filepath.Ext(path))]
	return ok
}

// BGMEntry is one track in a local BGM library.
type BGMEntry struct {
	Path  string
	Title string
}

// DefaultBGMLibraryDirs returns likely folders to scan for a BGM library.
func DefaultBGMLibraryDirs(configured, exeDir, projectsRoot string) []string {
	var dirs []string
	add := func(p string) {
		p = strings.TrimSpace(p)
		if p == "" {
			return
		}
		for _, d := range dirs {
			if strings.EqualFold(filepath.Clean(d), filepath.Clean(p)) {
				return
			}
		}
		dirs = append(dirs, p)
	}
	add(configured)
	if exeDir != "" {
		add(filepath.Join(exeDir, "bgm"))
		add(filepath.Join(exeDir, "resource", "bgm"))
	}
	if projectsRoot != "" {
		add(filepath.Join(filepath.Dir(projectsRoot), "bgm"))
	}
	if home, err := os.UserHomeDir(); err == nil {
		add(filepath.Join(home, "Narrato", "bgm"))
	}
	return dirs
}

// ListBGMLibrary lists audio files in the first existing directory from dirs,
// plus any additional existing dirs. Files are sorted by title.
func ListBGMLibrary(dirs ...string) []BGMEntry {
	seen := map[string]struct{}{}
	var out []BGMEntry
	for _, dir := range dirs {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if !IsAudioPath(full) {
				continue
			}
			key := strings.ToLower(filepath.Clean(full))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			title := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			out = append(out, BGMEntry{Path: full, Title: title})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Title) < strings.ToLower(out[j].Title)
	})
	return out
}
