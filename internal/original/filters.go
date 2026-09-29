package original

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
)

// FilterInput is everything BuildFilters needs besides op settings.
type FilterInput struct {
	Settings        []OpSettings
	SubtitleSRT     string
	Style           SubtitleStyle
	DurationSec     float64 // 0 if unknown; used by edge_pad fade-out
	WatermarkText   string
	WatermarkCorner string // tl|tr|bl|br
}

// FilterChain is the ffmpeg -vf / -af lists plus a learning summary.
type FilterChain struct {
	Video   []string
	Audio   []string
	Summary string // human-readable
	VFJoin  string
	AFJoin  string
}

// BuildFilters maps enabled settings to standard ffmpeg filters.
func BuildFilters(in FilterInput) FilterChain {
	settings, _ := NormalizeSettings(in.Settings)
	var vf, af []string
	var parts []string

	has := func(id string) (OpSettings, bool) {
		for _, s := range settings {
			if s.ID == id && s.Enabled {
				return s, true
			}
		}
		return OpSettings{}, false
	}

	if s, ok := has("fps"); ok {
		t := intensity01(s.Intensity)
		if t >= 0.01 {
			speed := 1.0 + 0.05*t // 1.00–1.05
			vf = append(vf, fmt.Sprintf("setpts=%.6f*PTS", 1.0/speed))
			af = append(af, fmt.Sprintf("atempo=%.6f", speed))
			parts = append(parts, LabelFor("fps"))
		}
	}
	if _, ok := has("hflip"); ok {
		vf = append(vf, "hflip")
		parts = append(parts, LabelFor("hflip"))
	}
	if s, ok := has("zoom_in"); ok {
		t := intensity01(s.Intensity)
		keep := 1.0 - 0.12*t
		if keep > 0.999 {
			keep = 0.999
		}
		vf = append(vf,
			fmt.Sprintf("crop=trunc(iw*%.4f/2)*2:trunc(ih*%.4f/2)*2", keep, keep),
			fmt.Sprintf("scale=trunc(iw/%.4f/2)*2:trunc(ih/%.4f/2)*2", keep, keep),
		)
		parts = append(parts, LabelFor("zoom_in"))
	}
	if s, ok := has("zoom_out"); ok {
		t := intensity01(s.Intensity)
		scale := 1.0 - 0.12*t
		if scale < 0.5 {
			scale = 0.5
		}
		vf = append(vf,
			fmt.Sprintf("scale=trunc(iw*%.4f/2)*2:trunc(ih*%.4f/2)*2", scale, scale),
			fmt.Sprintf("pad=trunc(iw/%.4f/2)*2:trunc(ih/%.4f/2)*2:(ow-iw)/2:(oh-ih)/2:black", scale, scale),
		)
		parts = append(parts, LabelFor("zoom_out"))
	}
	if s, ok := has("rotate"); ok {
		t := intensity01(s.Intensity)
		deg := 0.05 + 0.25*t
		rad := deg * math.Pi / 180
		vf = append(vf, fmt.Sprintf("rotate=a=%.6f:ow=iw:oh=ih:c=black", rad))
		parts = append(parts, LabelFor("rotate"))
	}
	if s, ok := has("brightness"); ok {
		t := intensity01(s.Intensity)
		vf = append(vf, fmt.Sprintf("eq=brightness=%.4f:contrast=%.4f:saturation=%.4f",
			0.08*t, 1.0+0.08*t, 1.0+0.12*t))
		parts = append(parts, LabelFor("brightness"))
	}
	if s, ok := has("hue"); ok {
		t := intensity01(s.Intensity)
		vf = append(vf, fmt.Sprintf("hue=h=%.3f", 8*t))
		parts = append(parts, LabelFor("hue"))
	}
	if s, ok := has("noise"); ok {
		t := intensity01(s.Intensity)
		n := 1 + int(12*t+0.5)
		if n < 1 {
			n = 1
		}
		vf = append(vf, fmt.Sprintf("noise=alls=%d:allf=t+u", n))
		parts = append(parts, LabelFor("noise"))
	}
	if s, ok := has("overlay"); ok {
		t := intensity01(s.Intensity)
		// angle PI/8 .. PI/4
		angle := math.Pi/8 + (math.Pi/4-math.Pi/8)*t
		vf = append(vf, fmt.Sprintf("vignette=angle=%.4f", angle))
		parts = append(parts, LabelFor("overlay"))
	}
	if s, ok := has("border"); ok {
		t := intensity01(s.Intensity)
		th := 2 + int(10*t+0.5)
		alpha := 0.10 + 0.25*t
		c := fmt.Sprintf("white@%.3f", alpha)
		vf = append(vf,
			fmt.Sprintf("drawbox=x=0:y=0:w=iw:h=%d:color=%s:t=fill", th, c),
			fmt.Sprintf("drawbox=x=0:y=ih-%d:w=iw:h=%d:color=%s:t=fill", th, th, c),
			fmt.Sprintf("drawbox=x=0:y=0:w=%d:h=ih:color=%s:t=fill", th, c),
			fmt.Sprintf("drawbox=x=iw-%d:y=0:w=%d:h=ih:color=%s:t=fill", th, th, c),
		)
		parts = append(parts, LabelFor("border"))
	}
	if s, ok := has("flying"); ok {
		vf = append(vf, buildFlyingFilters(intensity01(s.Intensity))...)
		parts = append(parts, LabelFor("flying"))
	}
	if s, ok := has("watermark"); ok {
		t := intensity01(s.Intensity)
		text := strings.TrimSpace(in.WatermarkText)
		if text == "" {
			text = DefaultWatermarkText
		}
		text = escapeDrawText(text)
		op := 0.15 + 0.50*t
		x, y := watermarkXY(in.WatermarkCorner)
		fontOpt := ""
		if ff := findFontFile(); ff != "" {
			fontOpt = ":fontfile='" + escapeFilterPath(ff) + "'"
		}
		vf = append(vf, fmt.Sprintf(
			"drawtext=text='%s'%s:fontsize=24:fontcolor=white@%.3f:x=%s:y=%s",
			text, fontOpt, op, x, y,
		))
		parts = append(parts, LabelFor("watermark"))
	}
	if s, ok := has("freeze"); ok {
		t := intensity01(s.Intensity)
		d := 0.2 + 0.3*t
		if t >= 0.5 {
			vf = append(vf, fmt.Sprintf("tpad=start_mode=clone:start_duration=%.3f:stop_mode=clone:stop_duration=%.3f", d, d))
			af = append(af, fmt.Sprintf("adelay=%.0f|%.0f", d*1000, d*1000))
			af = append(af, fmt.Sprintf("apad=pad_dur=%.3f", d))
		} else {
			vf = append(vf, fmt.Sprintf("tpad=stop_mode=clone:stop_duration=%.3f", d))
			af = append(af, fmt.Sprintf("apad=pad_dur=%.3f", d))
		}
		parts = append(parts, LabelFor("freeze"))
	}
	if s, ok := has("edge_pad"); ok {
		t := intensity01(s.Intensity)
		d := 0.08 + 0.22*t
		vf = append(vf, fmt.Sprintf("fade=t=in:st=0:d=%.3f", d))
		dur := in.DurationSec
		if dur > d*2+0.05 {
			vf = append(vf, fmt.Sprintf("fade=t=out:st=%.3f:d=%.3f", dur-d, d))
		}
		parts = append(parts, LabelFor("edge_pad"))
	}
	if s, ok := has("audio_micro"); ok {
		t := intensity01(s.Intensity)
		vol := 1.0 + 0.06*(2*t-1)
		gain := 1.5 * (2*t - 1)
		af = append(af, fmt.Sprintf("volume=%.4f", vol))
		af = append(af, fmt.Sprintf("equalizer=f=1000:t=q:w=1:g=%.3f", gain))
		parts = append(parts, LabelFor("audio_micro"))
	}

	srt := strings.TrimSpace(in.SubtitleSRT)
	if srt != "" {
		if _, err := os.Stat(srt); err == nil {
			escaped := escapeFilterPath(srt)
			vf = append(vf, fmt.Sprintf("subtitles='%s':force_style='%s'", escaped, buildForceStyle(in.Style)))
			parts = append(parts, "字幕烧录")
		}
	}

	// Even dimensions + yuv420p when any video filter present.
	if len(vf) > 0 {
		vf = append(vf, "scale=trunc(iw/2)*2:trunc(ih/2)*2", "format=yuv420p")
	}

	summary := strings.Join(parts, " + ")
	if summary == "" {
		summary = "（无视频/音频滤镜）"
	}
	return FilterChain{
		Video:   vf,
		Audio:   af,
		Summary: summary,
		VFJoin:  strings.Join(vf, ","),
		AFJoin:  strings.Join(af, ","),
	}
}

func intensity01(i int) float64 {
	return float64(clampInt(i, 0, 100)) / 100
}

func watermarkXY(corner string) (x, y string) {
	switch strings.ToLower(strings.TrimSpace(corner)) {
	case "tr":
		return "w-tw-24", "24"
	case "bl":
		return "24", "h-th-24"
	case "br":
		return "w-tw-24", "h-th-24"
	default:
		return "24", "24" // tl
	}
}

func escapeDrawText(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `'`, `\'`)
	s = strings.ReplaceAll(s, ":", `\:`)
	return s
}

func buildFlyingFilters(t float64) []string {
	obj := flyingObjects[rand.Intn(len(flyingObjects))]
	fontOpt := ""
	if ff := findFontFile(); ff != "" {
		fontOpt = ":fontfile='" + escapeFilterPath(ff) + "'"
	}
	type inst struct {
		scale, opacity float64
		speed, offset  int
		y, amp, freq   float64
	}
	instances := []inst{
		{1.0, 0.40, 88, 0, 0.20, 0.12, 1.8},
		{0.70, 0.28, 58, 350, 0.52, 0.10, 1.2},
		{0.50, 0.18, 115, 700, 0.78, 0.07, 2.5},
	}
	n := 1
	if t >= 0.35 {
		n = 2
	}
	if t >= 0.7 {
		n = 3
	}
	var filters []string
	for i := 0; i < n && i < len(instances); i++ {
		c := instances[i]
		op := c.opacity * (0.5 + 0.5*t)
		sz := int(float64(obj.size)*c.scale*(0.7+0.3*t) + 0.5)
		xExpr := fmt.Sprintf("w-mod(t*%d+%d\\,w+200)", c.speed, c.offset)
		yExpr := fmt.Sprintf("h*%g+sin(t*%g)*h*%g", c.y, c.freq, c.amp)
		filters = append(filters, fmt.Sprintf(
			"drawtext=text='%s'%s:fontsize=%d:fontcolor=%s@%g:x='%s':y='%s'",
			obj.char, fontOpt, sz, obj.color, op, xExpr, yExpr,
		))
	}
	return filters
}

func findFontFile() string {
	windir := os.Getenv("WINDIR")
	if windir == "" {
		windir = `C:\Windows`
	}
	for _, name := range []string{"arial.ttf", "simhei.ttf", "msyh.ttf"} {
		p := filepath.Join(windir, "Fonts", name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func escapeFilterPath(p string) string {
	p = filepath.ToSlash(p)
	p = strings.ReplaceAll(p, `\`, `/`)
	p = strings.ReplaceAll(p, ":", `\:`)
	p = strings.ReplaceAll(p, "'", `\'`)
	return p
}

func buildForceStyle(s SubtitleStyle) string {
	d := DefaultSubtitleStyle()
	if s.FontSize <= 0 {
		s.FontSize = d.FontSize
	}
	if s.FontName == "" {
		s.FontName = d.FontName
	}
	if s.PrimaryColor == "" {
		s.PrimaryColor = d.PrimaryColor
	}
	if s.OutlineColor == "" {
		s.OutlineColor = d.OutlineColor
	}
	if s.Outline < 0 {
		s.Outline = d.Outline
	}
	if s.Shadow < 0 {
		s.Shadow = d.Shadow
	}
	if s.MarginV <= 0 {
		s.MarginV = d.MarginV
	}
	fs := clamp(s.FontSize, 10, 72)
	oc := clamp(s.Outline, 0, 8)
	sh := clamp(s.Shadow, 0, 8)
	mv := clamp(s.MarginV, 0, 200)
	fn := strings.ReplaceAll(s.FontName, "'", `\'`)
	return fmt.Sprintf("FontSize=%d,FontName=%s,PrimaryColour=%s,OutlineColour=%s,Outline=%d,Shadow=%d,MarginV=%d",
		fs, fn, hexToAssBGR(s.PrimaryColor), hexToAssBGR(s.OutlineColor), oc, sh, mv)
}

func hexToAssBGR(hex string) string {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) != 6 {
		return "&H00FFFFFF"
	}
	rr, gg, bb := hex[0:2], hex[2:4], hex[4:6]
	return "&H00" + strings.ToUpper(bb+gg+rr)
}

func clamp(v, lo, hi int) int {
	return clampInt(v, lo, hi)
}
