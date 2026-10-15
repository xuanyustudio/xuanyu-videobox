package intro

import (
	"path/filepath"
	"time"
)

// OutputDir is the folder next to the uploaded image for one intro session.
func OutputDir(imagePath string, now time.Time) string {
	return filepath.Join(filepath.Dir(imagePath), "intro_"+now.Format("20060102_150405"))
}
