package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// MP3Path is the audio file written beside a video: same folder, same name, .mp3.
func MP3Path(videoPath string) string {
	dir := filepath.Dir(videoPath)
	base := filepath.Base(videoPath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" || stem == "." {
		stem = "audio"
	}
	out := filepath.Join(dir, stem+".mp3")
	if filepath.Clean(out) == filepath.Clean(videoPath) {
		out = filepath.Join(dir, stem+"_audio.mp3")
	}
	return out
}

// ExtractMP3 writes the video's first audio stream to an MP3 beside the source.
func ExtractMP3(ctx context.Context, bins FFmpegBins, videoPath string) (string, error) {
	if bins.FFmpeg == "" {
		return "", fmt.Errorf("ffmpeg binary required")
	}
	videoPath = strings.TrimSpace(videoPath)
	if videoPath == "" {
		return "", fmt.Errorf("未选择视频")
	}
	if _, err := os.Stat(videoPath); err != nil {
		return "", fmt.Errorf("视频不存在: %w", err)
	}
	out := MP3Path(videoPath)
	log, err := Run(ctx, bins.FFmpeg,
		"-y",
		"-i", videoPath,
		"-map", "0:a:0",
		"-vn",
		"-c:a", "libmp3lame",
		"-b:a", "192k",
		out,
	)
	if err != nil {
		if ctx.Err() != nil {
			_ = os.Remove(out)
			return "", ctx.Err()
		}
		msg := strings.ToLower(log)
		if strings.Contains(msg, "matches no streams") || strings.Contains(msg, "does not contain any stream") {
			return "", fmt.Errorf("这个视频没有音轨")
		}
		detail := tailText(log, 240)
		if detail == "" {
			detail = err.Error()
		}
		return "", fmt.Errorf("分离失败: %s", detail)
	}
	if _, err := os.Stat(out); err != nil {
		return "", fmt.Errorf("分离完成但没有生成文件")
	}
	return out, nil
}

func tailText(s string, n int) string {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return ""
	}
	lines := strings.Split(s, "\n")
	var kept []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		kept = append(kept, line)
	}
	if len(kept) > 3 {
		kept = kept[len(kept)-3:]
	}
	out := strings.Join(kept, " ")
	if len(out) > n {
		out = out[len(out)-n:]
	}
	return strings.TrimSpace(out)
}
