package tts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/difyz9/edge-tts-go/pkg/communicate"
)

// Edge synthesizes speech via Microsoft Edge online TTS (requires network).
// Output is MPEG audio (mp3); prefer a .mp3 outPath.
type Edge struct{}

func (Edge) Synthesize(ctx context.Context, text, voice, outPath string) (float64, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, fmt.Errorf("edge tts: empty text")
	}
	if voice == "" {
		voice = "zh-CN-XiaoxiaoNeural"
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}

	comm, err := communicate.NewCommunicate(
		text,
		voice,
		"+0%",
		"+0%",
		"+0Hz",
		"",
		10,
		60,
	)
	if err != nil {
		return 0, fmt.Errorf("edge tts: %w", err)
	}
	if err := comm.Save(ctx, outPath, ""); err != nil {
		return 0, fmt.Errorf("edge tts 网络合成失败（需能访问微软 Edge TTS）: %w", err)
	}

	st, err := os.Stat(outPath)
	if err != nil {
		return 0, err
	}
	// Real clips are typically tens of KB; reject tiny/corrupt placeholders.
	if st.Size() < 2048 {
		return 0, fmt.Errorf("edge tts: 输出过小 (%d bytes)，合成可能失败", st.Size())
	}

	return EstimateDurationSec(text), nil
}
