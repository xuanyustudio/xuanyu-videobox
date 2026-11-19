package hookopen

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
)

func transcribeVideo(ctx context.Context, cfg *config.Config, bins media.FFmpegBins, videoPath, workDir string, hasAudio bool, progress Progress) ([]Cue, string, error) {
	srtPath := filepath.Join(workDir, "transcript.srt")
	if raw, err := os.ReadFile(srtPath); err == nil && len(raw) > 0 {
		cues := parseSRT(string(raw))
		if len(cues) > 0 {
			return cues, formatTranscript(cues), nil
		}
	}
	if !hasAudio {
		return nil, "（原片没有音轨）", nil
	}
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	wavPath := filepath.Join(workDir, "audio.wav")
	if _, err := os.Stat(wavPath); err != nil {
		progress(-1, "正在抽出音轨…")
		if err := extractWAV(ctx, bins, videoPath, wavPath); err != nil {
			return nil, "", err
		}
	}
	opt := asrOptions(cfg)
	if err := asr.RequireReady(opt); err != nil {
		return nil, "", err
	}
	progress(-1, "正在用「"+asr.BackendLabel(opt.Backend)+"」转写台词…")
	if err := asr.Transcribe(wavPath, srtPath, opt); err != nil {
		return nil, "", fmt.Errorf("转写失败: %w", err)
	}
	raw, err := os.ReadFile(srtPath)
	if err != nil {
		return nil, "", err
	}
	cues := parseSRT(string(raw))
	text := formatTranscript(cues)
	_ = os.WriteFile(filepath.Join(workDir, "TRANSCRIPT.md"), []byte("# 台词转写\n\n"+text), 0o644)
	return cues, text, nil
}

func asrOptions(cfg *config.Config) asr.Options {
	opt := asr.Options{EnableSpk: true}
	if cfg != nil {
		opt.Backend = cfg.ASR.Backend
		opt.APIURL = cfg.ASR.APIURL
		opt.FireRedAPIURL = cfg.ASR.FireRedAPIURL
		opt.BailianAPIKey = cfg.ASR.APIKey
		opt.Hotword = cfg.ASR.Hotword
		if cfg.ASR.EnableSpk {
			opt.EnableSpk = true
		}
	}
	return opt
}
