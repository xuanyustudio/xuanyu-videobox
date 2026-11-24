package tts

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/difyz9/edge-tts-go/pkg/communicate"
)

// Edge synthesizes speech via Microsoft Edge online TTS (requires network).
// Output is MPEG audio (mp3); prefer a .mp3 outPath.
type Edge struct {
	Params VoiceParams
}

// DefaultEdgeTimeout is used when the caller did not set a deadline.
// The upstream websocket client uses HandshakeTimeout=0 and does not
// cancel ReadMessage when ctx is done, so we always wrap Save.
const DefaultEdgeTimeout = 45 * time.Second

// PreviewTimeout is the fail-fast limit for 预听 (Edge or MiniMax).
const PreviewTimeout = 20 * time.Second

func (e Edge) Synthesize(ctx context.Context, text, voice, outPath string) (float64, error) {
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

	if ctx == nil {
		ctx = context.Background()
	}
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, DefaultEdgeTimeout)
		defer cancel()
	}

	p := ClampVoiceParams(e.Params)
	comm, err := communicate.NewCommunicate(
		text,
		voice,
		EdgeRatePercent(p.Speed),
		EdgeVolumePercent(p.Volume),
		EdgePitchHz(p.Pitch),
		"",
		10,
		60,
	)
	if err != nil {
		return 0, fmt.Errorf("edge tts: %w", err)
	}
	if err := saveEdge(ctx, comm, outPath); err != nil {
		return 0, fmt.Errorf("edge tts 网络合成失败（需能访问微软 Edge TTS: api.msedgeservices.com）: %w", err)
	}

	st, err := os.Stat(outPath)
	if err != nil {
		return 0, err
	}
	// Real clips are typically tens of KB; reject tiny/corrupt placeholders.
	if st.Size() < 2048 {
		return 0, fmt.Errorf("edge tts: 输出过小 (%d bytes)，合成可能失败", st.Size())
	}

	return EstimateDurationSecAtSpeed(text, p.Speed), nil
}

func saveEdge(ctx context.Context, comm *communicate.Communicate, outPath string) error {
	done := make(chan error, 1)
	go func() {
		done <- comm.Save(ctx, outPath, "")
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return fmt.Errorf("连接微软超时或被取消（请检查是否能访问 api.msedgeservices.com）: %w", ctx.Err())
	}
}
