package webui

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/replicate"
)

// handleReplicateRun runs 提示词提取 synchronously (like /api/script/generate).
func (s *server) handleReplicateRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	var body struct {
		Video      string `json:"video"`
		Frames     int    `json:"frames"`
		Template   string `json:"template"`
		SegmentSec int    `json:"segment_sec"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	s.mu.Lock()
	if s.replicating {
		s.mu.Unlock()
		http.Error(w, "正在提取中", 409)
		return
	}
	cfg := *s.cfg
	s.replicating = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.replicating = false
		s.mu.Unlock()
	}()

	params := replicate.Params{
		VideoPath:  strings.Trim(strings.TrimSpace(body.Video), `"`),
		Frames:     body.Frames,
		Template:   replicate.Template(body.Template),
		BaseURL:    cfg.Replicate.BaseURL,
		APIKey:     cfg.Replicate.APIKey,
		Model:      cfg.Replicate.Model,
		SegmentSec: body.SegmentSec,
		ASR: asr.Options{
			Backend:       cfg.ASR.Backend,
			APIURL:        cfg.ASR.APIURL,
			FireRedAPIURL: cfg.ASR.FireRedAPIURL,
			BailianAPIKey: cfg.ASR.APIKey,
			Hotword:       cfg.ASR.Hotword,
			EnableSpk:     cfg.ASR.EnableSpk,
		},
		Bins: media.FFmpegBins{
			FFmpeg:  cfg.FFmpeg.Path,
			FFprobe: cfg.FFmpeg.FFprobePath,
		},
		OnProgress: func(percent int, msg string) {
			s.appendLog("[提示词提取] " + msg)
		},
	}
	if err := replicate.Validate(params); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	// Persist the chosen window before the long run. cfg is a value copy, so
	// the live config must be updated under the lock; Save stays outside it.
	norm := replicate.NormalizeSegmentSec(body.SegmentSec)
	s.mu.Lock()
	s.cfg.Replicate.SegmentSec = norm
	s.mu.Unlock()
	if err := config.Save(s.cfgPath, s.cfg); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	res, err := replicate.Run(r.Context(), params)
	if err != nil {
		http.Error(w, "提示词提取失败: "+err.Error(), 500)
		return
	}
	s.mu.Lock()
	s.replicateLast = res
	s.mu.Unlock()
	segments := make([]map[string]any, 0, len(res.Segments))
	for _, seg := range res.Segments {
		segments = append(segments, map[string]any{
			"index":       seg.Index,
			"start_sec":   seg.StartSec,
			"end_sec":     seg.EndSec,
			"frame_start": seg.FrameStart,
			"frame_count": len(seg.Frames),
			"prompt_en":   seg.PromptEN,
			"prompt_zh":   seg.PromptZH,
			"analysis":    seg.Analysis,
		})
	}
	writeJSON(w, map[string]any{
		"output_dir":        res.OutputDir,
		"frames":            len(res.Frames),
		"duration_sec":      res.DurationSec,
		"prompt":            res.Prompt,
		"prompt_en":         res.PromptEN,
		"prompt_zh":         res.PromptZH,
		"analysis":          res.Analysis,
		"subtitle_path":     res.SubtitlePath,
		"character_lock_zh": res.CharacterLockZH,
		"character_lock_en": res.CharacterLockEN,
		"character_ref":     res.CharacterRef,
		"segments":          segments,
	})
}

// handleReplicateFrame serves keyframe ?i=N of the last 提示词提取 result.
func (s *server) handleReplicateFrame(w http.ResponseWriter, r *http.Request) {
	i, err := strconv.Atoi(r.URL.Query().Get("i"))
	s.mu.Lock()
	res := s.replicateLast
	s.mu.Unlock()
	if err != nil || res == nil || i < 0 || i >= len(res.Frames) {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	http.ServeFile(w, r, res.Frames[i])
}

// handleReplicateOpen opens the last result folder in the system file browser.
func (s *server) handleReplicateOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", 405)
		return
	}
	s.mu.Lock()
	res := s.replicateLast
	s.mu.Unlock()
	if res == nil {
		http.Error(w, "暂无结果", 400)
		return
	}
	if err := media.OpenFile(res.OutputDir); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	writeJSON(w, map[string]string{"ok": "1"})
}
