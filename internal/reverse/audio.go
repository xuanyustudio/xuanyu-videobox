package reverse

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/narratoai/narrato-go/internal/asr"
	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/media"
)

const (
	audioFileName = "audio.wav"
	transcriptSRT = "transcript.srt"
	transcriptMD  = "TRANSCRIPT.md"
)

type transcriptLine struct {
	Start float64
	End   float64
	Text  string
}

func ensureTranscript(ctx context.Context, cfg *config.Config, job *Job, writeLog func(string), progress func(int, string)) error {
	if job == nil {
		return fmt.Errorf("缺少任务信息")
	}
	if fileIfExists(job.OutDir, transcriptMD) != "" {
		writeLog("已有台词转写，跳过识别")
		return nil
	}
	if strings.TrimSpace(job.VideoPath) == "" {
		return fmt.Errorf("任务里没有原片路径")
	}
	if _, err := os.Stat(job.VideoPath); err != nil {
		return fmt.Errorf("找不到原片: %w", err)
	}

	bins := resolveFFmpegBins(cfg)
	if strings.TrimSpace(bins.FFmpeg) == "" {
		return fmt.Errorf("未找到 ffmpeg，无法抽出音轨")
	}

	audioPath := filepath.Join(job.OutDir, audioFileName)
	if fileIfExists(job.OutDir, audioFileName) == "" {
		progress(-1, "正在抽出音轨…")
		writeLog("正在用 ffmpeg 抽出音轨")
		if err := extractJobAudio(ctx, bins, job.VideoPath, audioPath); err != nil {
			if isNoAudioErr(err) {
				body := "# 台词转写\n\n原片没有音轨，因此没有可转写的对白。\n"
				if werr := os.WriteFile(filepath.Join(job.OutDir, transcriptMD), []byte(body), 0o644); werr != nil {
					return werr
				}
				writeLog("原片没有音轨，已记下")
				return nil
			}
			return err
		}
		writeLog("音轨已保存为 " + audioFileName)
	}

	opt := asrOptions(cfg)
	if err := asr.RequireReady(opt); err != nil {
		return err
	}
	progress(-1, "正在转写台词…")
	writeLog("正在用「" + asr.BackendLabel(opt.Backend) + "」转写台词")
	srtPath := filepath.Join(job.OutDir, transcriptSRT)
	if err := asr.Transcribe(audioPath, srtPath, opt); err != nil {
		return fmt.Errorf("转写失败: %w", err)
	}
	raw, err := os.ReadFile(srtPath)
	if err != nil {
		return fmt.Errorf("读取转写结果失败: %w", err)
	}
	md := formatTranscriptMD(string(raw))
	if err := os.WriteFile(filepath.Join(job.OutDir, transcriptMD), []byte(md), 0o644); err != nil {
		return err
	}
	n := len(parseSRT(string(raw)))
	writeLog(fmt.Sprintf("台词转写已写入 %s，共 %d 句", transcriptMD, n))
	return nil
}

func resolveFFmpegBins(cfg *config.Config) media.FFmpegBins {
	beside := ""
	if exe, err := os.Executable(); err == nil {
		beside = filepath.Dir(exe)
	}
	var path, probe string
	if cfg != nil {
		path = cfg.FFmpeg.Path
		probe = cfg.FFmpeg.FFprobePath
	}
	ff, _ := media.ResolveFFmpeg(path, beside)
	fp, _ := media.ResolveFFprobe(probe, beside)
	return media.FFmpegBins{FFmpeg: ff, FFprobe: fp}
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

func extractJobAudio(ctx context.Context, bins media.FFmpegBins, videoPath, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	log, err := media.Run(ctx, bins.FFmpeg,
		"-y",
		"-i", videoPath,
		"-map", "0:a:0",
		"-vn",
		"-ac", "1",
		"-ar", "16000",
		"-c:a", "pcm_s16le",
		outPath,
	)
	if err != nil {
		if ctx.Err() != nil {
			_ = os.Remove(outPath)
			return ctx.Err()
		}
		if isNoAudioLog(log) {
			_ = os.Remove(outPath)
			return fmt.Errorf("这个视频没有音轨")
		}
		detail := strings.TrimSpace(log)
		if detail == "" {
			detail = err.Error()
		}
		if len([]rune(detail)) > 240 {
			detail = string([]rune(detail)[len([]rune(detail))-240:])
		}
		return fmt.Errorf("抽音频失败: %s", detail)
	}
	if _, err := os.Stat(outPath); err != nil {
		return fmt.Errorf("抽音频完成但没有生成文件")
	}
	return nil
}

func isNoAudioErr(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "没有音轨")
}

func isNoAudioLog(log string) bool {
	msg := strings.ToLower(log)
	return strings.Contains(msg, "matches no streams") || strings.Contains(msg, "does not contain any stream")
}

func formatTranscriptMD(srt string) string {
	lines := parseSRT(srt)
	var b strings.Builder
	b.WriteString("# 台词转写\n\n")
	b.WriteString("由设置里的字幕识别生成，带时间轴。成稿时对白以这份为准；若观察笔记里清晰可见的屏幕字幕与此不一致，以看得见的字幕为准，并在报告里标明。\n\n")
	if len(lines) == 0 {
		text := strings.TrimSpace(stripSRTTiming(srt))
		if text == "" {
			b.WriteString("（没有识别到对白。）\n")
			return b.String()
		}
		b.WriteString(text)
		b.WriteByte('\n')
		return b.String()
	}
	for _, line := range lines {
		fmt.Fprintf(&b, "- %.2f–%.2f  %s\n", line.Start, line.End, line.Text)
	}
	return b.String()
}

var srtTimeRe = regexp.MustCompile(`(?i)(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})\s*-->\s*(\d{1,2}):(\d{2}):(\d{2})[,.](\d{1,3})`)

func parseSRT(s string) []transcriptLine {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	blocks := regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(s), -1)
	var out []transcriptLine
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		timeIdx := -1
		var start, end float64
		for i, line := range lines {
			m := srtTimeRe.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			start = srtStamp(m[1], m[2], m[3], m[4])
			end = srtStamp(m[5], m[6], m[7], m[8])
			timeIdx = i
			break
		}
		if timeIdx < 0 {
			continue
		}
		text := strings.TrimSpace(strings.Join(lines[timeIdx+1:], " "))
		text = regexp.MustCompile(`(?i)</?[^>]+>`).ReplaceAllString(text, "")
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			continue
		}
		out = append(out, transcriptLine{Start: start, End: end, Text: text})
	}
	return out
}

func srtStamp(h, m, s, frac string) float64 {
	hh, _ := strconv.Atoi(h)
	mm, _ := strconv.Atoi(m)
	ss, _ := strconv.Atoi(s)
	for len(frac) < 3 {
		frac += "0"
	}
	if len(frac) > 3 {
		frac = frac[:3]
	}
	ms, _ := strconv.Atoi(frac)
	return float64(hh)*3600 + float64(mm)*60 + float64(ss) + float64(ms)/1000
}

func stripSRTTiming(s string) string {
	var b strings.Builder
	for _, line := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || srtTimeRe.MatchString(trim) || regexp.MustCompile(`^\d+$`).MatchString(trim) {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(trim)
	}
	return b.String()
}
