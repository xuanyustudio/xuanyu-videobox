package media

import (
	"os"
	"path/filepath"
	"strings"
)

var videoExts = map[string]struct{}{
	".mp4": {}, ".mov": {}, ".mkv": {}, ".avi": {}, ".m4v": {},
	".webm": {}, ".ts": {}, ".flv": {}, ".wmv": {},
}

var subtitleExts = map[string]struct{}{
	".srt": {}, ".ass": {}, ".ssa": {}, ".vtt": {},
}

// IsVideoPath reports whether path looks like a video file by extension.
func IsVideoPath(path string) bool {
	_, ok := videoExts[strings.ToLower(filepath.Ext(path))]
	return ok
}

// IsSubtitlePath reports whether path looks like a subtitle file by extension.
func IsSubtitlePath(path string) bool {
	_, ok := subtitleExts[strings.ToLower(filepath.Ext(path))]
	return ok
}

// ListVideosInDir returns video files directly under dir (non-recursive).
func ListVideosInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if IsVideoPath(full) {
			out = append(out, full)
		}
	}
	return out, nil
}

// ListSubtitlesInDir returns subtitle files directly under dir (non-recursive).
func ListSubtitlesInDir(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		full := filepath.Join(dir, e.Name())
		if IsSubtitlePath(full) {
			out = append(out, full)
		}
	}
	return out, nil
}

// FindSidecarSubtitle looks for a subtitle next to the video with the same stem.
func FindSidecarSubtitle(videoPath string) string {
	dir := filepath.Dir(videoPath)
	stem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	for ext := range subtitleExts {
		candidate := filepath.Join(dir, stem+ext)
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}
	}
	return ""
}

// MatchSidecarSubtitles returns existing sidecar subtitle paths for the given videos.
func MatchSidecarSubtitles(videoPaths []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, v := range videoPaths {
		sub := FindSidecarSubtitle(v)
		if sub == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(sub))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, sub)
	}
	return out
}

// AlignSubtitlesToVideos returns one subtitle path per video (empty if unmatched).
// Prefer same-stem match, then fall back to the same index when still free.
func AlignSubtitlesToVideos(videos, subs []string) []string {
	out := make([]string, len(videos))
	used := map[string]struct{}{}
	for i, v := range videos {
		stem := strings.TrimSuffix(filepath.Base(v), filepath.Ext(v))
		for _, s := range subs {
			s = strings.TrimSpace(s)
			if s == "" {
				continue
			}
			key := strings.ToLower(filepath.Clean(s))
			if _, ok := used[key]; ok {
				continue
			}
			sStem := strings.TrimSuffix(filepath.Base(s), filepath.Ext(s))
			if strings.EqualFold(stem, sStem) {
				out[i] = s
				used[key] = struct{}{}
				break
			}
		}
	}
	for i := range videos {
		if out[i] != "" {
			continue
		}
		if i >= len(subs) {
			continue
		}
		s := strings.TrimSpace(subs[i])
		if s == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(s))
		if _, ok := used[key]; ok {
			continue
		}
		out[i] = s
		used[key] = struct{}{}
	}
	return out
}

// AppendUniquePaths appends paths not already present (case-insensitive on Windows).
func AppendUniquePaths(dst []string, add ...string) []string {
	seen := map[string]struct{}{}
	for _, p := range dst {
		seen[strings.ToLower(filepath.Clean(p))] = struct{}{}
	}
	for _, p := range add {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(filepath.Clean(p))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		dst = append(dst, p)
	}
	return dst
}
