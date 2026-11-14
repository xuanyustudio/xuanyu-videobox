package subtitleburn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// OutputDir is the folder under a video's directory that receives new files.
func OutputDir(videoPath string) string {
	return filepath.Join(filepath.Dir(videoPath), "srt")
}

// PrepareOutput creates OutputDir and returns a non-colliding path inside it.
// suffix includes the underscore and extension, such as "_sub.mp4" or "_asr.srt".
func PrepareOutput(videoPath, suffix string) (string, error) {
	dir := OutputDir(videoPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	stem := strings.TrimSuffix(filepath.Base(videoPath), filepath.Ext(videoPath))
	return UniquePath(filepath.Join(dir, stem+suffix))
}

// UniquePath returns path when it does not exist.
// Otherwise it inserts _2, _3, ... before the extension, starting at 2.
func UniquePath(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("没有可用的输出文件名: %s", path)
}
