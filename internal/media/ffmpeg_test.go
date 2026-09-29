package media_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
)

func TestResolveBinaryPrefersConfig(t *testing.T) {
	fake := filepath.Join(t.TempDir(), "ffmpeg.exe")
	if err := os.WriteFile(fake, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := media.ResolveFFmpeg(fake, "")
	if err != nil || got != fake {
		t.Fatalf("%q %v", got, err)
	}
}

func TestFFmpegVersion(t *testing.T) {
	bin, err := media.ResolveFFmpeg("", "")
	if err != nil {
		t.Skip(err)
	}
	out, err := media.Run(context.Background(), bin, "-version")
	if err != nil || !strings.Contains(out, "ffmpeg") {
		t.Fatalf("%v %s", err, out)
	}
}
