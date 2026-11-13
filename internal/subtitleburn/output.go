package subtitleburn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

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
