# 视频加字幕 Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 桌面端增加「加字幕」页，可单选或多选视频与字幕，按主文件名配对，对不上的视频用现有语音识别生成字幕，再按页面样式烧进画面。

**Architecture:** `internal/subtitleburn` 放配对、唯一输出路径、样式字符串和烧录。这些函数不启动 ffmpeg、不连接识别服务，由单元测试覆盖。桌面页只负责选文件、样式控件、调用识别和显示状态。导航在「音频分离」和「设置」之间。

**Tech Stack:** Go，现有 Fyne 桌面页、`internal/media`、`internal/asr`。

规格：`docs/superpowers/specs/2026-11-13-add-subtitle-design.md`

---

### Task 1: 按主文件名配对

**Files:**
- Create: `internal/subtitleburn/pair.go`
- Test: `internal/subtitleburn/pair_test.go`

**Step 1: Write the failing test**

`internal/subtitleburn/pair_test.go`，包名 `subtitleburn_test`：

```go
package subtitleburn_test

import (
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestPair(t *testing.T) {
	video := filepath.Join("videos", "A.MP4")
	srt := filepath.Join("subs", "a.srt")
	items, unused := subtitleburn.Pair([]string{video}, []string{srt})
	if len(items) != 1 || items[0].Video != video || items[0].Subtitle != srt || len(unused) != 0 {
		t.Fatalf("case pair: items=%v unused=%v", items, unused)
	}

	ass := filepath.Join("subs", "a.ass")
	items, unused = subtitleburn.Pair([]string{video}, []string{ass, srt})
	if items[0].Subtitle != srt || len(unused) != 1 || unused[0] != ass {
		t.Fatalf("srt wins: items=%v unused=%v", items, unused)
	}

	first := filepath.Join("one", "a.srt")
	second := filepath.Join("two", "a.srt")
	items, unused = subtitleburn.Pair([]string{video}, []string{first, second})
	if items[0].Subtitle != first || len(unused) != 1 || unused[0] != second {
		t.Fatalf("earlier wins: items=%v unused=%v", items, unused)
	}

	same := filepath.Join("otherdir", "A.mov")
	items, unused = subtitleburn.Pair([]string{video, same}, []string{srt})
	if items[0].Subtitle != srt || items[1].Subtitle != "" || len(unused) != 0 {
		t.Fatalf("same stem: items=%v unused=%v", items, unused)
	}

	other := filepath.Join("videos", "B.mkv")
	items, unused = subtitleburn.Pair([]string{video, other}, []string{srt})
	if items[0].Subtitle != srt || items[1].Subtitle != "" || len(unused) != 0 {
		t.Fatalf("unmatched video: items=%v unused=%v", items, unused)
	}

	orphan := filepath.Join("subs", "c.vtt")
	items, unused = subtitleburn.Pair([]string{video}, []string{srt, orphan})
	if items[0].Subtitle != srt || len(unused) != 1 || unused[0] != orphan {
		t.Fatalf("orphan: items=%v unused=%v", items, unused)
	}

	items, unused = subtitleburn.Pair([]string{video, other}, nil)
	if items[0].Subtitle != "" || items[1].Subtitle != "" || len(unused) != 0 {
		t.Fatalf("no subs: items=%v unused=%v", items, unused)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/subtitleburn/ -count=1 -run TestPair`

Expected: FAIL，包 `subtitleburn` 不存在。

**Step 3: Write minimal implementation**

`internal/subtitleburn/pair.go`：

```go
package subtitleburn

import (
	"path/filepath"
	"strings"
)

// Item is one video and the subtitle selected for it.
// An empty Subtitle means the caller must transcribe first.
type Item struct {
	Video    string
	Subtitle string
}

// Pair matches subtitles to videos by base name, ignoring extension and case.
// Subtitle priority is srt, ass, ssa, vtt, then anything else.
// The same priority keeps the earlier path. Only the first video of a name gets the subtitle.
func Pair(videos, subtitles []string) (items []Item, unused []string) {
	winners := map[string]int{}
	for i, sub := range subtitles {
		key := stemKey(sub)
		if prev, ok := winners[key]; ok {
			if extPriority(sub) < extPriority(subtitles[prev]) {
				winners[key] = i
			}
			continue
		}
		winners[key] = i
	}
	used := map[int]bool{}
	for _, video := range videos {
		item := Item{Video: video}
		if idx, ok := winners[stemKey(video)]; ok && !used[idx] {
			item.Subtitle = subtitles[idx]
			used[idx] = true
		}
		items = append(items, item)
	}
	for i, sub := range subtitles {
		if !used[i] {
			unused = append(unused, sub)
		}
	}
	return items, unused
}

func stemKey(path string) string {
	base := filepath.Base(path)
	return strings.ToLower(strings.TrimSuffix(base, filepath.Ext(base)))
}

func extPriority(path string) int {
	switch strings.ToLower(strings.TrimPrefix(filepath.Ext(path), ".")) {
	case "srt":
		return 0
	case "ass":
		return 1
	case "ssa":
		return 2
	case "vtt":
		return 3
	default:
		return 4
	}
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/subtitleburn/ -count=1 -run TestPair`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/subtitleburn/pair.go internal/subtitleburn/pair_test.go
git commit -m "按主文件名把字幕配给视频。"
```

---

### Task 2: 不覆盖的输出路径

**Files:**
- Create: `internal/subtitleburn/output.go`
- Test: `internal/subtitleburn/output_test.go`

**Step 1: Write the failing test**

```go
package subtitleburn_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestUniquePath(t *testing.T) {
	dir := t.TempDir()
	base := filepath.Join(dir, "clip_sub.mp4")
	got, err := subtitleburn.UniquePath(base)
	if err != nil || got != base {
		t.Fatalf("missing: got %q err %v", got, err)
	}
	if err := os.WriteFile(base, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = subtitleburn.UniquePath(base)
	if err != nil || got != filepath.Join(dir, "clip_sub_2.mp4") {
		t.Fatalf("second: got %q err %v", got, err)
	}
	if err := os.WriteFile(got, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err = subtitleburn.UniquePath(base)
	if err != nil || got != filepath.Join(dir, "clip_sub_3.mp4") {
		t.Fatalf("third: got %q err %v", got, err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/subtitleburn/ -count=1 -run TestUniquePath`

Expected: FAIL，`UniquePath` 未定义。

**Step 3: Write minimal implementation**

`internal/subtitleburn/output.go`：

```go
package subtitleburn

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UniquePath returns path when it does not exist.
// Otherwise it inserts _2, _3, ... before the extension, starting at 2.
func UniquePath(path string) (string, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return path, nil
		}
		return "", err
	}
	ext := filepath.Ext(path)
	stem := strings.TrimSuffix(path, ext)
	for i := 2; i < 10000; i++ {
		candidate := fmt.Sprintf("%s_%d%s", stem, i, ext)
		if _, err := os.Stat(candidate); err != nil {
			if os.IsNotExist(err) {
				return candidate, nil
			}
			return "", err
		}
	}
	return "", fmt.Errorf("没有可用的输出文件名: %s", path)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/subtitleburn/ -count=1 -run TestUniquePath`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/subtitleburn/output.go internal/subtitleburn/output_test.go
git commit -m "成片和识别字幕重名时递增文件名。"
```

---

### Task 3: 字幕样式字符串

**Files:**
- Create: `internal/subtitleburn/style.go`
- Test: `internal/subtitleburn/style_test.go`

不引用 `internal/original`。字段和默认值与 `original.SubtitleStyle`、`DefaultSubtitleStyle` 相同。距底部为 0 时保留 0。

**Step 1: Write the failing test**

```go
package subtitleburn_test

import (
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestForceStyle(t *testing.T) {
	got := subtitleburn.ForceStyle(subtitleburn.DefaultStyle())
	want := "FontSize=22,FontName=Arial,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=2,Shadow=1,MarginV=36"
	if got != want {
		t.Fatalf("default\n got %s\nwant %s", got, want)
	}

	style := subtitleburn.DefaultStyle()
	style.MarginV = 0
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, "MarginV=0") {
		t.Fatalf("margin 0: %s", got)
	}

	style = subtitleburn.DefaultStyle()
	style.PrimaryColor = "blue"
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, "PrimaryColour=&H00FFFFFF") {
		t.Fatalf("bad color: %s", got)
	}

	style = subtitleburn.DefaultStyle()
	style.PrimaryColor = "#FF0000"
	style.OutlineColor = "#00FF00"
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, "PrimaryColour=&H000000FF") || !strings.Contains(got, "OutlineColour=&H0000FF00") {
		t.Fatalf("bgr: %s", got)
	}

	style = subtitleburn.DefaultStyle()
	style.FontName = "O'Hara"
	got = subtitleburn.ForceStyle(style)
	if !strings.Contains(got, `FontName=O\'Hara`) {
		t.Fatalf("quote: %s", got)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/subtitleburn/ -count=1 -run TestForceStyle`

Expected: FAIL，`ForceStyle` 未定义。

**Step 3: Write minimal implementation**

`internal/subtitleburn/style.go`：

```go
package subtitleburn

import (
	"fmt"
	"strings"
)

// Style is the ASS force_style used when burning a subtitle.
type Style struct {
	FontSize     int
	FontName     string
	PrimaryColor string // #RRGGBB
	OutlineColor string
	Outline      int
	Shadow       int
	MarginV      int
}

// DefaultStyle matches original.DefaultSubtitleStyle.
func DefaultStyle() Style {
	return Style{
		FontSize:     22,
		FontName:     "Arial",
		PrimaryColor: "#FFFFFF",
		OutlineColor: "#000000",
		Outline:      2,
		Shadow:       1,
		MarginV:      36,
	}
}

// ForceStyle builds the ffmpeg force_style value.
// MarginV 0 stays 0. Only a negative margin falls back to 36.
func ForceStyle(s Style) string {
	d := DefaultStyle()
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
	if s.MarginV < 0 {
		s.MarginV = d.MarginV
	}
	fn := strings.ReplaceAll(s.FontName, "'", `\'`)
	return fmt.Sprintf("FontSize=%d,FontName=%s,PrimaryColour=%s,OutlineColour=%s,Outline=%d,Shadow=%d,MarginV=%d",
		clamp(s.FontSize, 10, 72), fn, hexToAssBGR(s.PrimaryColor), hexToAssBGR(s.OutlineColor),
		clamp(s.Outline, 0, 8), clamp(s.Shadow, 0, 8), clamp(s.MarginV, 0, 200))
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
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/subtitleburn/ -count=1 -run TestForceStyle`

Expected: PASS

**Step 5: Commit**

```bash
git add internal/subtitleburn/style.go internal/subtitleburn/style_test.go
git commit -m "烧录字幕时按页面样式生成 force_style。"
```

---

### Task 4: 烧录滤镜

**Files:**
- Modify: `internal/media/burn.go`
- Create: `internal/subtitleburn/burn.go`
- Test: `internal/subtitleburn/burn_test.go`

导出路径转义，不改 `BurnSubtitles` 的参数和滤镜字符串。新烧录加上引号和 `force_style`。

**Step 1: Write the failing test**

```go
package subtitleburn_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/subtitleburn"
)

func TestSubtitlesFilter(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "clip.mp4")
	srt := filepath.Join(dir, "clip.srt")
	got := subtitleburn.SubtitlesFilter(video, srt, subtitleburn.DefaultStyle())
	want := "subtitles='clip.srt':force_style='FontSize=22,FontName=Arial,PrimaryColour=&H00FFFFFF,OutlineColour=&H00000000,Outline=2,Shadow=1,MarginV=36'"
	if got != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestBurnRequiresFFmpeg(t *testing.T) {
	err := subtitleburn.Burn(context.Background(), media.FFmpegBins{}, "a.mp4", "a.srt", "out.mp4", subtitleburn.DefaultStyle())
	if err == nil || !strings.Contains(err.Error(), "ffmpeg binary required") {
		t.Fatal(err)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/subtitleburn/ -count=1 -run "TestSubtitlesFilter|TestBurnRequiresFFmpeg"`

Expected: FAIL，`SubtitlesFilter` 未定义。

**Step 3: Write minimal implementation**

在 `internal/media/burn.go` 增加导出包装，函数体仍调用现有的 `escapeSubtitlesFilterPath`：

```go
// EscapeSubtitlesFilterPath normalizes paths for ffmpeg's subtitles filter (Windows-safe).
func EscapeSubtitlesFilterPath(videoPath, srtPath string) string {
	return escapeSubtitlesFilterPath(videoPath, srtPath)
}
```

`internal/subtitleburn/burn.go`：

```go
package subtitleburn

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/narratoai/narrato-go/internal/media"
)

// SubtitlesFilter is the ffmpeg -vf value for one burned subtitle.
func SubtitlesFilter(videoPath, subtitlePath string, style Style) string {
	escaped := media.EscapeSubtitlesFilterPath(videoPath, subtitlePath)
	return fmt.Sprintf("subtitles='%s':force_style='%s'", escaped, ForceStyle(style))
}

// Burn writes outPath with the subtitle burned in using style.
func Burn(ctx context.Context, bins media.FFmpegBins, videoPath, subtitlePath, outPath string, style Style) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	args := []string{
		"-y",
		"-i", videoPath,
		"-vf", SubtitlesFilter(videoPath, subtitlePath, style),
		"-c:v", "libx264", "-preset", "ultrafast", "-crf", "23",
		"-pix_fmt", "yuv420p",
		"-c:a", "copy",
		"-movflags", "+faststart",
		outPath,
	}
	out, err := media.RunInDir(ctx, filepath.Dir(videoPath), bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(out))
	}
	return nil
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/subtitleburn/ ./internal/media/ -count=1`

Expected: PASS。`media.BurnSubtitles` 的现有测试仍通过。

**Step 5: Commit**

```bash
git add internal/media/burn.go internal/subtitleburn/burn.go internal/subtitleburn/burn_test.go
git commit -m "用带样式的 subtitles 滤镜把字幕烧进视频。"
```

---

### Task 5: 加字幕页面

**Files:**
- Create: `internal/ui/page_subtitle.go`
- Modify: `internal/ui/app.go`

页面构建标签是 `//go:build fyne_gui`。交互对齐 `page_audio.go`，样式控件对齐 `page_original.go` 里的字号、字体、颜色、描边、阴影、距底部。不要把 BGM 放进来。

**Step 1: 写页面**

`newSubtitlePage(state *AppState) fyne.CanvasObject`：

- 标题「加字幕」。说明：「选择一个或多个视频，再选择字幕。同名的字幕会烧进去，对不上的视频用设置里的语音识别。」
- 按钮：「选择视频」「选择字幕」「清除字幕」「开始」「停止」「打开所在文件夹」。开始是 `HighImportance`。清除字幕和停止是 `LowImportance`。停止一开始禁用。打开文件夹一开始禁用。
- 「选择视频」调用 `pickVideoFiles`，替换视频列表。1 个显示完整路径，多个显示「已选 N 个视频」，没有显示「未选择视频」。
- 「选择字幕」调用 `pickSubtitleFiles`，替换字幕列表。文案对应「已选 N 个字幕」和「未选择字幕」。取消对话框时列表不变。
- 「清除字幕」把字幕列表清空。
- 样式用 `subtitleburn.DefaultStyle()` 做初值。字号滑块 10–72，描边 0–8，阴影 0–8，距底部 0–200，步长都是 1。字体下拉是 Arial、Microsoft YaHei、SimHei、SimSun。主色和描边色是输入框。滑块放在 `fixedWidthLayout`，宽度 220。数值标签跟在滑块后面。

开始时在界面线程读完样式、视频和字幕，再进 goroutine：

1. 视频列表为空：`dialog.ShowInformation("未选择视频", "请先选择要加字幕的视频。", state.Win)`，返回。
2. `resolveFFmpeg(state)` 失败：`dialog.ShowError`，返回。
3. `subtitleburn.Pair`。`unused` 非空时，用 `dialog.NewInformation` 列出 `filepath.Base`，标题「有字幕未配对」，正文「以下字幕没有匹配的视频，将不会使用：」加换行文件名。`SetOnClosed` 里再开始处理。这个对话框没有取消。
4. 处理中禁用选择、清除、开始，启用停止。

每条按 `Pair` 的顺序。状态用 `fyne.Do` 更新，序号从 1 开始，文件名用 `filepath.Base`：

- 字幕路径为空：状态「正在识别 i/n 文件名…」。先取 `asr.Options`，字段与 `page_assets.go` 相同。有任意一条要识别时，循环前调用一次 `asr.RequireReady`。未就绪则这些条各自记失败，错误用 `RequireReady` 的返回值，已配对的条继续烧录。识别路径是视频旁边的 `原名_asr.srt`，经 `UniquePath`。调用 `asr.Transcribe`。返回错误则该条失败，不烧录。
- 识别返回后如果 `ctx.Err() != nil`：不烧录这一条，也不再继续。识别函数没有 context，停止只能在它返回后生效。
- 烧录前状态「正在烧录 i/n 文件名…」。成片是视频旁边的 `原名_sub.mp4`，经 `UniquePath`。调用 `subtitleburn.Burn`。`原名` 是视频文件名去掉扩展名，保留原来的大小写。

结束后：

- `ctx.Err() != nil`：状态「已停止，已完成 N 个」。N 是成功成片数。
- 没有失败且 N 为 1：状态「已保存 」加成片路径。
- 没有失败且 N 大于 1：状态「已保存 N 个，最后一个：」加最后一条成片路径。
- 有失败：状态「成功 N 个，失败 M 个：」加第一条错误原文。N 为 0 也用这句。
- 至少一条成功时记住最后一条路径，并启用「打开所在文件夹」。它调用 `media.OpenFile` 打开该成片所在目录。

停止调用这次任务的 `context.CancelFunc`。

**Step 2: 注册导航**

`app.go` 在 `audioPage := newAudioSplitPage(state)` 后面增加 `subtitlePage := newSubtitlePage(state)`。`newRail` 在音频分离和设置之间插入：

```go
{Label: "加字幕", Icon: theme.FileIcon(), Page: subtitlePage},
```

不改其它页面。

**Step 3: 编译**

Run: `go test ./internal/subtitleburn/ ./internal/media/ -count=1`

Expected: PASS

Run: `go test -tags fyne_gui ./internal/ui/ -count=1`

Expected: PASS。若本机缺少 Fyne 的 CGO 依赖，记下错误，不要为了编译去改无关页面。

**Step 4: Commit**

```bash
git add internal/ui/page_subtitle.go internal/ui/app.go
git commit -m "桌面端可以给一个或多个视频加字幕。"
```

---

## 做完后的核对

- `go test ./internal/subtitleburn/ ./internal/media/ -count=1` 通过。
- `A.MP4` 配 `a.srt`；同时有 `a.ass` 时用 srt，ass 算未使用。
- 同名的第二支视频走识别，不占用上一支的字幕。
- 已有 `clip_sub.mp4` 和 `clip_sub_2.mp4` 时，新成片是 `clip_sub_3.mp4`。
- 距底部 0 的样式字符串含 `MarginV=0`。
- 左侧导航在音频分离和设置之间出现「加字幕」。
- 不改一键原创，也不改 `media.BurnSubtitles` 的现有调用。
