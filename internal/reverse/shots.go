package reverse

import (
	"context"
	"fmt"
	"image"
	_ "image/jpeg"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/narratoai/narrato-go/internal/media"
)

const (
	thumbFPS          = 2.0
	thumbWidth        = 160
	minShotSec        = 0.35
	hardChangeFrac    = 0.28
	changeFracCut     = 0.20
	changeFracSoft    = 0.14
	meanAbsSoft       = 0.10
	pixDiffThreshold  = 18
	localRadius       = 4
	maxGapSec         = 3.0
	minFrameGapSec    = 0.25
	maxAnalysisFrames = 96
	maxShots          = 96
	shortShotSec      = 0.8
)

// Shot is one automatically detected camera/edit shot.
type Shot struct {
	Index int     `json:"index"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func (s Shot) Duration() float64 { return s.End - s.Start }

// SamplePoint is one analysis frame to extract.
type SamplePoint struct {
	Timestamp float64
	Shot      int
}

func fallbackShots(duration float64) []Shot {
	if duration <= 0 {
		duration = 1
	}
	return []Shot{{Index: 1, Start: 0, End: duration}}
}

func shotsFromCuts(duration float64, cuts []float64) []Shot {
	if duration <= 0 {
		duration = 1
	}
	cleaned := make([]float64, 0, len(cuts))
	last := 0.0
	for _, t := range cuts {
		if t < minShotSec || t > duration-minShotSec {
			continue
		}
		if t-last < minShotSec {
			continue
		}
		cleaned = append(cleaned, t)
		last = t
	}
	bounds := append([]float64{0}, append(cleaned, duration)...)
	shots := make([]Shot, 0, len(bounds)-1)
	for i := 0; i < len(bounds)-1; i++ {
		if bounds[i+1] <= bounds[i] {
			continue
		}
		shots = append(shots, Shot{Index: len(shots) + 1, Start: bounds[i], End: bounds[i+1]})
	}
	if len(shots) == 0 {
		return fallbackShots(duration)
	}
	return mergeToMaxShots(shots, maxShots)
}

func mergeToMaxShots(shots []Shot, max int) []Shot {
	if max < 1 || len(shots) <= max {
		return numberShots(shots)
	}
	out := append([]Shot(nil), shots...)
	for len(out) > max {
		shortest := 0
		for i := 1; i < len(out); i++ {
			if out[i].Duration() < out[shortest].Duration() {
				shortest = i
			}
		}
		if shortest == 0 {
			out[1].Start = out[0].Start
			out = append(out[:1], out[2:]...)
		} else if shortest == len(out)-1 {
			out[shortest-1].End = out[shortest].End
			out = out[:shortest]
		} else if out[shortest-1].Duration() <= out[shortest+1].Duration() {
			out[shortest-1].End = out[shortest].End
			out = append(out[:shortest], out[shortest+1:]...)
		} else {
			out[shortest+1].Start = out[shortest].Start
			out = append(out[:shortest], out[shortest+1:]...)
		}
	}
	return numberShots(out)
}

func numberShots(shots []Shot) []Shot {
	for i := range shots {
		shots[i].Index = i + 1
	}
	return shots
}

func planSampleTimes(shots []Shot, budget int) []SamplePoint {
	if budget < 1 {
		budget = maxAnalysisFrames
	}
	if len(shots) == 0 {
		return nil
	}
	type cand struct {
		t    float64
		shot int
		rank int // 0 = required (at least one per shot), 1 = other end, 2 = mid fill
	}
	var cands []cand
	add := func(t float64, shot, rank int) {
		if t < 0 {
			t = 0
		}
		cands = append(cands, cand{t: t, shot: shot, rank: rank})
	}
	for _, s := range shots {
		dur := s.Duration()
		if dur <= 0 {
			continue
		}
		mid := s.Start + dur/2
		if dur < shortShotSec {
			add(mid, s.Index, 0)
			continue
		}
		head := s.Start + math.Min(0.05, dur/4)
		tail := s.End - math.Min(0.08, dur/4)
		if tail-head < minFrameGapSec {
			add(mid, s.Index, 0)
			continue
		}
		add(head, s.Index, 0)
		add(tail, s.Index, 1)
		if dur > maxGapSec {
			n := int(math.Ceil(dur/maxGapSec)) - 1
			for i := 1; i <= n; i++ {
				t := s.Start + dur*float64(i)/float64(n+1)
				if t-head >= minFrameGapSec && tail-t >= minFrameGapSec {
					add(t, s.Index, 2)
				}
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].rank != cands[j].rank {
			return cands[i].rank < cands[j].rank
		}
		return cands[i].t < cands[j].t
	})
	out := make([]SamplePoint, 0, len(cands))
	for _, c := range cands {
		tooClose := false
		for _, p := range out {
			dt := math.Abs(c.t - p.Timestamp)
			if dt < 0.05 {
				tooClose = true
				break
			}
			if dt < minFrameGapSec && c.shot == p.Shot {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		// Always keep one frame per shot; only mid/tail fills consume leftover budget.
		if c.rank > 0 && len(out) >= budget {
			continue
		}
		if c.rank == 0 && len(out) >= budget && len(out) >= len(shots) {
			continue
		}
		out = append(out, SamplePoint{Timestamp: c.t, Shot: c.shot})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].Shot < out[j].Shot
	})
	return out
}

func detectShots(ctx context.Context, bins media.FFmpegBins, videoPath, workDir string, duration float64, progress func(int, string)) ([]Shot, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	if duration <= 0 {
		return fallbackShots(1), nil
	}
	if bins.FFmpeg == "" {
		bin, err := media.ResolveFFmpeg("", "")
		if err != nil {
			return fallbackShots(duration), err
		}
		bins.FFmpeg = bin
	}
	thumbDir := filepath.Join(workDir, "thumbs_detect")
	if err := os.MkdirAll(thumbDir, 0o755); err != nil {
		return fallbackShots(duration), err
	}
	defer os.RemoveAll(thumbDir)

	progress(-1, "正在扫描画面切点…")
	pattern := filepath.Join(thumbDir, "t_%04d.jpg")
	if _, err := media.Run(ctx, bins.FFmpeg,
		"-y", "-hide_banner", "-loglevel", "error",
		"-i", videoPath,
		"-vf", fmt.Sprintf("fps=%g,scale=%d:-2", thumbFPS, thumbWidth),
		"-q:v", "8",
		pattern,
	); err != nil {
		return fallbackShots(duration), fmt.Errorf("抽缩略图失败: %w", err)
	}

	matches, err := filepath.Glob(filepath.Join(thumbDir, "t_*.jpg"))
	if err != nil {
		return fallbackShots(duration), err
	}
	sort.Strings(matches)
	if len(matches) < 3 {
		return fallbackShots(duration), nil
	}

	lumas := make([][]uint8, 0, len(matches))
	times := make([]float64, 0, len(matches))
	for i, p := range matches {
		if err := ctx.Err(); err != nil {
			return fallbackShots(duration), err
		}
		img, err := decodeJPEG(p)
		if err != nil {
			continue
		}
		lumas = append(lumas, luma(img))
		times = append(times, float64(i)/thumbFPS)
		if (i+1)%40 == 0 {
			progress(-1, fmt.Sprintf("已扫描缩略图 %d/%d", i+1, len(matches)))
		}
	}
	if len(lumas) < 3 {
		return fallbackShots(duration), nil
	}

	fracs := make([]float64, len(lumas))
	means := make([]float64, len(lumas))
	for i := 1; i < len(lumas); i++ {
		fracs[i], means[i] = compareLuma(lumas[i-1], lumas[i])
	}
	var cuts []float64
	for i := 1; i < len(fracs); i++ {
		base := localMedian(fracs, i, localRadius)
		hard := fracs[i] >= hardChangeFrac
		spike := fracs[i] >= changeFracCut && fracs[i] > base*2.5+0.04
		soft := fracs[i] >= changeFracSoft && means[i] >= meanAbsSoft && fracs[i] > base*2.0+0.03
		if hard || spike || soft {
			cuts = append(cuts, clampTime(times[i], duration))
		}
	}
	shots := shotsFromCuts(duration, cuts)
	progress(-1, fmt.Sprintf("画面扫描完成，切出 %d 个镜头", len(shots)))
	return shots, nil
}

func clampTime(t, duration float64) float64 {
	if t < 0 {
		return 0
	}
	if duration > 0 && t > duration {
		return duration
	}
	return t
}

func decodeJPEG(path string) (image.Image, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	img, _, err := image.Decode(f)
	return img, err
}

func luma(img image.Image) []uint8 {
	b := img.Bounds()
	out := make([]uint8, b.Dx()*b.Dy())
	i := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			out[i] = uint8(((299*r + 587*g + 114*bl + 500) / 1000) >> 8)
			i++
		}
	}
	return out
}

func compareLuma(a, b []uint8) (frac, mean float64) {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	if n == 0 {
		return 0, 0
	}
	var sum, changed int
	for i := 0; i < n; i++ {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		sum += d
		if d > pixDiffThreshold {
			changed++
		}
	}
	return float64(changed) / float64(n), float64(sum) / float64(n) / 255.0
}

func localMedian(vals []float64, i, radius int) float64 {
	lo := i - radius
	if lo < 1 {
		lo = 1
	}
	hi := i + radius
	if hi >= len(vals) {
		hi = len(vals) - 1
	}
	buf := make([]float64, 0, hi-lo)
	for j := lo; j <= hi; j++ {
		if j == i {
			continue
		}
		buf = append(buf, vals[j])
	}
	if len(buf) == 0 {
		return 0
	}
	sort.Float64s(buf)
	return buf[len(buf)/2]
}

func extractAt(ctx context.Context, bins media.FFmpegBins, videoPath, outDir string, points []SamplePoint, progress func(int, string)) ([]string, []SamplePoint, error) {
	if progress == nil {
		progress = func(int, string) {}
	}
	if bins.FFmpeg == "" {
		bin, err := media.ResolveFFmpeg("", "")
		if err != nil {
			return nil, nil, fmt.Errorf("未找到 ffmpeg: %w", err)
		}
		bins.FFmpeg = bin
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0, len(points))
	kept := make([]SamplePoint, 0, len(points))
	for i, p := range points {
		if err := ctx.Err(); err != nil {
			return paths, kept, err
		}
		out := filepath.Join(outDir, fmt.Sprintf("frame_%03d.jpg", i))
		args := []string{"-y", "-hide_banner", "-loglevel", "error"}
		if p.Timestamp >= 1 {
			args = append(args, "-ss", fmt.Sprintf("%.3f", p.Timestamp-0.4), "-i", videoPath, "-ss", "0.400")
		} else {
			args = append(args, "-i", videoPath, "-ss", fmt.Sprintf("%.3f", p.Timestamp))
		}
		args = append(args,
			"-vframes", "1",
			"-vf", "scale='min(1024,iw)':-2",
			"-q:v", "5",
			out,
		)
		if _, err := media.Run(ctx, bins.FFmpeg, args...); err != nil {
			progress(-1, fmt.Sprintf("帧 %d @ %.1fs 提取失败: %v", i+1, p.Timestamp, err))
			_ = os.Remove(out)
			continue
		}
		st, err := os.Stat(out)
		if err != nil || st.Size() == 0 {
			_ = os.Remove(out)
			continue
		}
		paths = append(paths, out)
		kept = append(kept, p)
		if len(points) > 0 {
			pct := 15 + (i+1)*20/len(points)
			progress(pct, fmt.Sprintf("提取帧 %d/%d @ %.1fs（镜头 %d）", i+1, len(points), p.Timestamp, p.Shot))
		}
	}
	if len(paths) == 0 {
		return nil, nil, fmt.Errorf("无法提取视频帧，请确保已安装 ffmpeg")
	}
	return paths, kept, nil
}
