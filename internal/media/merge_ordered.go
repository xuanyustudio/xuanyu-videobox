package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type clipGeometry struct {
	Width    int
	Height   int
	FPS      string
	HasAudio bool
	Duration float64
}

type mergeTarget struct {
	Width  int
	Height int
	FPS    string
}

// MergeOrderedVideos joins videos in the given order into one MP4.
// Clips may differ in size, frame rate, and whether they have audio.
// The output uses the largest clip's size and frame rate; smaller frames are padded.
func MergeOrderedVideos(ctx context.Context, bins FFmpegBins, clipPaths []string, outPath string, onProgress func(percent int)) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("找不到 ffmpeg")
	}
	if len(clipPaths) < 2 {
		return fmt.Errorf("至少需要 2 个视频")
	}
	for _, p := range clipPaths {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			return fmt.Errorf("找不到视频：%s", p)
		}
	}

	geos := make([]clipGeometry, len(clipPaths))
	var total float64
	for i, p := range clipPaths {
		if err := ctx.Err(); err != nil {
			return err
		}
		g, err := probeClipGeometry(ctx, bins, p)
		if err != nil {
			return fmt.Errorf("%s：%w", filepath.Base(p), err)
		}
		if g.Width <= 0 || g.Height <= 0 {
			return fmt.Errorf("%s 没有可用的画面", filepath.Base(p))
		}
		if g.Duration <= 0 {
			return fmt.Errorf("%s 时长无效", filepath.Base(p))
		}
		geos[i] = g
		total += g.Duration
	}

	target := targetFromClips(geos)
	filter := buildMergeFilter(geos, target)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("创建输出目录失败：%w", err)
	}
	script, err := os.CreateTemp(filepath.Dir(outPath), "merge-filter-*.txt")
	if err != nil {
		return fmt.Errorf("写入滤镜脚本失败：%w", err)
	}
	scriptPath := script.Name()
	if _, err := script.WriteString(filter); err != nil {
		script.Close()
		os.Remove(scriptPath)
		return fmt.Errorf("写入滤镜脚本失败：%w", err)
	}
	if err := script.Close(); err != nil {
		os.Remove(scriptPath)
		return fmt.Errorf("写入滤镜脚本失败：%w", err)
	}
	defer os.Remove(scriptPath)

	args := []string{"-y"}
	for _, p := range clipPaths {
		args = append(args, "-i", p)
	}
	for _, g := range geos {
		if g.HasAudio {
			continue
		}
		args = append(args,
			"-f", "lavfi",
			"-t", formatSeconds(g.Duration),
			"-i", "anullsrc=channel_layout=stereo:sample_rate=44100",
		)
	}
	args = append(args, "-filter_complex_script", scriptPath, "-map", "[vout]", "-map", "[aout]")
	args = append(args, defaultVideoEncodeArgs()...)
	args = append(args, defaultAudioEncodeArgs()...)
	args = append(args, "-movflags", "+faststart")
	if onProgress != nil {
		args = append(args, "-progress", "pipe:1", "-nostats")
	}
	args = append(args, outPath)

	runErr := error(nil)
	if onProgress == nil {
		out, err := Run(ctx, bins.FFmpeg, args...)
		if err != nil {
			runErr = fmt.Errorf("%w\n%s", err, tailText(out, 500))
		}
	} else {
		var last int
		stderr, err := RunInDirLines(ctx, "", bins.FFmpeg, func(line string) {
			pct, ok := mergeProgressPercent(line, total)
			if !ok || pct == last {
				return
			}
			last = pct
			onProgress(pct)
		}, args...)
		if err != nil {
			runErr = fmt.Errorf("%w\n%s", err, tailText(stderr, 500))
		} else {
			onProgress(100)
		}
	}
	if runErr != nil {
		if ctx.Err() != nil {
			os.Remove(outPath)
			return ctx.Err()
		}
		return fmt.Errorf("合并失败：%w", runErr)
	}
	return nil
}

func targetFromClips(clips []clipGeometry) mergeTarget {
	best := clips[0]
	bestArea := best.Width * best.Height
	for _, c := range clips[1:] {
		area := c.Width * c.Height
		if area > bestArea {
			best = c
			bestArea = area
		}
	}
	w := evenDim(best.Width)
	h := evenDim(best.Height)
	if w < 2 || h < 2 {
		w, h = 1920, 1080
	}
	fps := best.FPS
	if fps == "" {
		fps = "30"
	}
	return mergeTarget{Width: w, Height: h, FPS: fps}
}

func buildMergeFilter(clips []clipGeometry, target mergeTarget) string {
	var b strings.Builder
	labels := make([]string, 0, len(clips))
	silenceAt := len(clips)
	for i, c := range clips {
		dur := formatSeconds(c.Duration)
		fmt.Fprintf(&b,
			"[%d:v]scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2:color=black,setsar=1,fps=%s,format=yuv420p,setpts=PTS-STARTPTS[v%d];",
			i, target.Width, target.Height, target.Width, target.Height, target.FPS, i,
		)
		audioFrom := i
		if !c.HasAudio {
			audioFrom = silenceAt
			silenceAt++
		}
		fmt.Fprintf(&b,
			"[%d:a]aformat=sample_rates=44100:channel_layouts=stereo,apad=whole_dur=%s,atrim=duration=%s,asetpts=PTS-STARTPTS[a%d];",
			audioFrom, dur, dur, i,
		)
		labels = append(labels, fmt.Sprintf("[v%d][a%d]", i, i))
	}
	b.WriteString(strings.Join(labels, ""))
	fmt.Fprintf(&b, "concat=n=%d:v=1:a=1[vout][aout]", len(clips))
	return b.String()
}

func probeClipGeometry(ctx context.Context, bins FFmpegBins, path string) (clipGeometry, error) {
	probe := bins.FFprobe
	if probe == "" {
		var err error
		probe, err = ResolveFFprobe("", "")
		if err != nil {
			return clipGeometry{}, fmt.Errorf("找不到 ffprobe：%w", err)
		}
		bins.FFprobe = probe
	}
	out, err := Run(ctx, probe,
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height,avg_frame_rate,duration",
		"-of", "csv=p=0",
		path,
	)
	if err != nil {
		return clipGeometry{}, fmt.Errorf("读取画面失败：%w", err)
	}
	w, h, fps, dur, err := parseVideoStreamCSV(out)
	if err != nil {
		return clipGeometry{}, err
	}
	if dur <= 0 {
		dur, err = ProbeDurationSec(ctx, bins, path)
		if err != nil || dur <= 0 {
			return clipGeometry{}, fmt.Errorf("读取时长失败")
		}
	}
	hasAudio, err := probeAudioStream(ctx, bins, path)
	if err != nil {
		return clipGeometry{}, err
	}
	return clipGeometry{
		Width:    w,
		Height:   h,
		FPS:      fps,
		HasAudio: hasAudio,
		Duration: dur,
	}, nil
}

func parseVideoStreamCSV(out string) (width, height int, fps string, duration float64, err error) {
	line := ""
	for _, raw := range strings.Split(out, "\n") {
		raw = strings.TrimSpace(raw)
		if raw != "" {
			line = raw
			break
		}
	}
	if line == "" {
		return 0, 0, "", 0, fmt.Errorf("没有可用的画面")
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return 0, 0, "", 0, fmt.Errorf("无法读取画面信息：%s", line)
	}
	width, _ = strconv.Atoi(strings.TrimSpace(parts[0]))
	height, _ = strconv.Atoi(strings.TrimSpace(parts[1]))
	if width <= 0 || height <= 0 {
		return 0, 0, "", 0, fmt.Errorf("没有可用的画面")
	}
	if len(parts) >= 3 {
		fps = normalizeFPS(parts[2])
	} else {
		fps = "30"
	}
	if len(parts) >= 4 {
		duration, _ = strconv.ParseFloat(strings.TrimSpace(parts[3]), 64)
	}
	return width, height, fps, duration, nil
}

func normalizeFPS(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "0/0" || strings.EqualFold(raw, "N/A") {
		return "30"
	}
	value := 0.0
	if strings.Contains(raw, "/") {
		parts := strings.Split(raw, "/")
		if len(parts) != 2 {
			return "30"
		}
		num, errN := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		den, errD := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if errN != nil || errD != nil || den == 0 {
			return "30"
		}
		value = num / den
	} else {
		parsed, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return "30"
		}
		value = parsed
	}
	if value < 1 || value > 120 {
		return "30"
	}
	text := strconv.FormatFloat(value, 'f', 3, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		return "30"
	}
	return text
}

func evenDim(n int) int {
	if n <= 0 {
		return 0
	}
	if n%2 != 0 {
		n++
	}
	return n
}

func formatSeconds(d float64) string {
	if d < 0 {
		d = 0
	}
	text := strconv.FormatFloat(d, 'f', 3, 64)
	text = strings.TrimRight(text, "0")
	text = strings.TrimRight(text, ".")
	if text == "" {
		return "0"
	}
	return text
}

func probeAudioStream(ctx context.Context, bins FFmpegBins, path string) (bool, error) {
	probe := bins.FFprobe
	if probe == "" {
		var err error
		probe, err = ResolveFFprobe("", "")
		if err != nil {
			return false, fmt.Errorf("找不到 ffprobe：%w", err)
		}
	}
	out, err := Run(ctx, probe,
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_type",
		"-of", "csv=p=0",
		path,
	)
	if err != nil {
		return false, fmt.Errorf("读取音轨失败：%w", err)
	}
	return strings.TrimSpace(out) == "audio", nil
}

func mergeProgressPercent(line string, duration float64) (int, bool) {
	line = strings.TrimSpace(line)
	if line == "progress=end" {
		return 100, true
	}
	const prefix = "out_time="
	if duration <= 0 || !strings.HasPrefix(line, prefix) {
		return 0, false
	}
	sec, ok := parseProgressClock(strings.TrimPrefix(line, prefix))
	if !ok {
		return 0, false
	}
	pct := int(sec / duration * 100)
	if pct < 0 {
		pct = 0
	}
	if pct > 99 {
		pct = 99
	}
	return pct, true
}

func parseProgressClock(v string) (float64, bool) {
	parts := strings.Split(v, ":")
	if len(parts) != 3 {
		return 0, false
	}
	h, errH := strconv.ParseFloat(parts[0], 64)
	m, errM := strconv.ParseFloat(parts[1], 64)
	s, errS := strconv.ParseFloat(parts[2], 64)
	if errH != nil || errM != nil || errS != nil {
		return 0, false
	}
	return h*3600 + m*60 + s, true
}
