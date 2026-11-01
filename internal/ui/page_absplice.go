//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/media/absplice"
)

func newABSplicePage(state *AppState) fyne.CanvasObject {
	var (
		mu       sync.Mutex
		cancelFn context.CancelFunc
		running  bool
		lastOut  string
	)

	aPath := ""
	bPath := ""
	outPath := ""
	workDir := ""

	aLabel := widget.NewLabel("未选择 A 素材（图片或视频）")
	aLabel.Truncation = fyne.TextTruncateEllipsis
	bLabel := widget.NewLabel("未选择 B 画面（视频）")
	bLabel.Truncation = fyne.TextTruncateEllipsis
	outLabel := widget.NewLabel("未选择输出路径（留空自动命名）")
	outLabel.Truncation = fyne.TextTruncateEllipsis
	outLabel.Importance = widget.LowImportance

	startEntry := widget.NewEntry()
	startEntry.SetText("0")
	startEntry.SetPlaceHolder("秒，例如 1.5")
	durEntry := widget.NewEntry()
	durEntry.SetText("1.0")
	durEntry.SetPlaceHolder("秒")
	fpsEntry := widget.NewEntry()
	fpsEntry.SetText("30")
	crfEntry := widget.NewEntry()
	crfEntry.SetText("23")
	crfEntry.SetPlaceHolder("18 极佳 ~ 28 省体积")
	gopEntry := widget.NewEntry()
	gopEntry.SetText("30")
	widthEntry := widget.NewEntry()
	widthEntry.SetText("1920")
	heightEntry := widget.NewEntry()
	heightEntry.SetText("1080")
	presetSelect := widget.NewSelect(
		[]string{"ultrafast", "superfast", "veryfast", "faster", "fast", "medium", "slow", "slower", "veryslow"},
		nil,
	)
	presetSelect.SetSelected("medium")
	keepTempCheck := widget.NewCheck("保留临时 .h264 文件（便于调试 NAL）", nil)

	idrCountEntry := widget.NewEntry()
	idrCountEntry.SetText("10")
	idrCountEntry.SetPlaceHolder("手填个数，例如 10")
	idrHint := widget.NewLabel("")
	idrHint.Importance = widget.LowImportance
	idrAutoCheck := widget.NewCheck("自动（约每秒 1 个 I 帧；图片 A 固定 1 个）", nil)
	idrAutoCheck.SetChecked(true)

	parseFloatOrSafe := func(s string, def float64) float64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || v < 0 {
			return def
		}
		return v
	}
	parseIntOrSafe := func(s string, def int) int {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v <= 0 {
			return def
		}
		return v
	}
	refreshIDRHint := func() {
		dur := parseFloatOrSafe(durEntry.Text, 1.0)
		if aPath != "" && absplice.IsImagePath(aPath) {
			idrHint.SetText("当前 A 是图片 → 将使用 1 个 IDR")
			idrCountEntry.Disable()
			return
		}
		if idrAutoCheck.Checked {
			n := absplice.SuggestIDRCount(dur, 0, 1)
			idrHint.SetText(fmt.Sprintf("自动模式：拼接 %.1fs → 预计 %d 个 A-IDR（均匀替换到 B）", dur, n))
			idrCountEntry.Disable()
		} else {
			idrCountEntry.Enable()
			n := parseIntOrSafe(idrCountEntry.Text, 10)
			idrHint.SetText(fmt.Sprintf("手动模式：将使用 %d 个 A-IDR", n))
		}
	}
	idrAutoCheck.OnChanged = func(bool) { refreshIDRHint() }
	durEntry.OnChanged = func(string) { refreshIDRHint() }
	idrCountEntry.OnChanged = func(string) { refreshIDRHint() }
	refreshIDRHint()

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	statusLabel := widget.NewLabel("准备就绪")
	statusLabel.Importance = widget.LowImportance
	logEntry := widget.NewMultiLineEntry()
	logEntry.SetMinRowsVisible(12)
	logEntry.Wrapping = fyne.TextWrapWord

	appendLog := func(line string) {
		now := time.Now().Format("15:04:05")
		cur := logEntry.Text
		if cur == "" {
			logEntry.SetText("[" + now + "] " + line)
		} else {
			logEntry.SetText(cur + "\n[" + now + "] " + line)
		}
		logEntry.CursorRow = len(logEntry.Text)
	}
	setProgress := func(p float64, msg string) {
		progress.SetValue(p)
		statusLabel.SetText(msg)
	}

	var autoFillOutput func()
	autoFillOutput = func() {
		if outPath != "" {
			return
		}
		base := ""
		if bPath != "" {
			base = strings.TrimSuffix(filepath.Base(bPath), filepath.Ext(bPath))
		}
		dir := ""
		if bPath != "" {
			dir = filepath.Dir(bPath)
		} else if aPath != "" {
			dir = filepath.Dir(aPath)
		} else {
			dir, _ = os.Getwd()
		}
		name := "ab_splice_"
		if base != "" {
			name += base + "_"
		}
		name += time.Now().Format("20060102_150405") + ".mp4"
		outPath = filepath.Join(dir, name)
		outLabel.SetText(outPath)
	}

	pickA := widget.NewButton("选择 A（图片/视频）", func() {
		p, err := pickAMediaFile()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if p == "" {
			return
		}
		aPath = p
		kind := "视频"
		if absplice.IsImagePath(p) {
			kind = "图片"
		}
		aLabel.SetText(filepath.Base(p) + "（" + kind + "）")
		appendLog("A 素材 = " + p + "（" + kind + "）")
		refreshIDRHint()
		autoFillOutput()
	})
	pickB := widget.NewButton("选择 B 画面视频", func() {
		p, err := pickOneVideoFile()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if p == "" {
			return
		}
		bPath = p
		bLabel.SetText(filepath.Base(p))
		appendLog("B 视频(PB帧) = " + p)
		autoFillOutput()
	})
	analyzeB := widget.NewButton("分析", nil)
	analyzeB.Importance = widget.MediumImportance
	pickOut := widget.NewButton("选择输出 MP4", func() {
		def := "ab_splice_" + time.Now().Format("20060102_150405") + ".mp4"
		p, err := pickSaveFile("输出拼接结果 MP4", def, "mp4")
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if p == "" {
			return
		}
		outPath = p
		outLabel.SetText(p)
		appendLog("输出 = " + p)
	})
	openLast := widget.NewButton("用默认播放器打开输出", func() {
		if lastOut == "" {
			dialog.ShowInformation("提示", "还没有成功生成的输出", state.Win)
			return
		}
		openExplorerOrFile(lastOut)
	})
	openLast.Disable()
	showInFolder := widget.NewButton("在文件夹显示", func() {
		if lastOut == "" {
			return
		}
		openExplorerOrFile(filepath.Dir(lastOut))
	})
	showInFolder.Disable()

	startBtn := widget.NewButton("开始拼接", nil)
	startBtn.Importance = widget.HighImportance
	stopBtn := widget.NewButton("停止", nil)
	stopBtn.Disable()
	stopBtn.Importance = widget.DangerImportance

	resolveBins := func() (media.FFmpegBins, error) {
		bins := media.FFmpegBins{
			FFmpeg:  state.Cfg.FFmpeg.Path,
			FFprobe: state.Cfg.FFmpeg.FFprobePath,
		}
		beside := ""
		if exe, e := os.Executable(); e == nil {
			beside = filepath.Dir(exe)
		}
		var err error
		if bins.FFmpeg == "" {
			bins.FFmpeg, err = media.ResolveFFmpeg("", beside)
			if err != nil {
				return bins, fmt.Errorf("找不到 ffmpeg：%w（请在「设置」里配置 ffmpeg 路径）", err)
			}
			state.Cfg.FFmpeg.Path = bins.FFmpeg
		}
		if bins.FFprobe == "" {
			bins.FFprobe, err = media.ResolveFFprobe("", beside)
			if err != nil {
				bins.FFprobe = ""
			} else {
				state.Cfg.FFmpeg.FFprobePath = bins.FFprobe
			}
		}
		return bins, nil
	}

	parseFloatOr := func(s string, def float64) float64 {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil || v < 0 {
			return def
		}
		return v
	}
	parseIntOr := func(s string, def int) int {
		v, err := strconv.Atoi(strings.TrimSpace(s))
		if err != nil || v <= 0 {
			return def
		}
		return v
	}

	analyzeB.OnTapped = func() {
		if bPath == "" {
			dialog.ShowInformation("未选择视频", "请先点击「选择 B 画面视频」选好文件，再点分析。", state.Win)
			return
		}
		if _, err := os.Stat(bPath); err != nil {
			dialog.ShowError(fmt.Errorf("B 视频不存在：%w", err), state.Win)
			return
		}
		bins, err := resolveBins()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		if bins.FFprobe == "" {
			dialog.ShowError(fmt.Errorf("找不到 ffprobe，无法分析（请在「设置」配置 ffprobe 路径，或放到 ffmpeg 同目录）"), state.Win)
			return
		}
		spliceDur := parseFloatOr(durEntry.Text, 1.0)
		outFPS := parseIntOr(fpsEntry.Text, 30)
		analyzeB.Disable()
		appendLog("开始分析 B 视频：" + bPath)
		statusLabel.SetText("正在分析视频结构（长视频可能较慢）…")
		go func() {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			t0 := time.Now()
			info, err := absplice.AnalyzeVideo(ctx, bins, bPath)
			fyne.Do(func() {
				analyzeB.Enable()
				if err != nil {
					statusLabel.SetText("分析失败")
					appendLog("分析失败：" + err.Error())
					dialog.ShowError(fmt.Errorf("分析失败：%w", err), state.Win)
					return
				}
				dt := time.Since(t0).Round(time.Millisecond)
				report := info.FormatReport(spliceDur, outFPS)
				appendLog(fmt.Sprintf("分析完成（%s）：关键帧≈IDR %d，GOP %d，总帧 %d", dt, info.KeyFrames, info.GOPCount, info.FrameCount))
				statusLabel.SetText(fmt.Sprintf("分析完成: 关键帧 %d, GOP %d, 总帧 %d", info.KeyFrames, info.GOPCount, info.FrameCount))

				body := widget.NewMultiLineEntry()
				body.SetText(report)
				body.Wrapping = fyne.TextWrapWord
				scroll := container.NewScroll(body)
				scroll.SetMinSize(fyne.NewSize(520, 420))
				d := dialog.NewCustom("B 视频分析结果", "关闭", scroll, state.Win)
				d.Resize(fyne.NewSize(560, 500))
				d.Show()
			})
		}()
	}

	setBusy := func(b bool) {
		mu.Lock()
		running = b
		mu.Unlock()
		if b {
			startBtn.Disable()
			stopBtn.Enable()
		} else {
			startBtn.Enable()
			stopBtn.Disable()
		}
	}

	stopBtn.OnTapped = func() {
		mu.Lock()
		fn := cancelFn
		mu.Unlock()
		if fn != nil {
			fn()
		}
	}

	startBtn.OnTapped = func() {
		if aPath == "" {
			dialog.ShowInformation("缺少 A 素材", "请先选择 A（图片或视频，将抽帧做成多个 IDR）", state.Win)
			return
		}
		if bPath == "" {
			dialog.ShowInformation("缺少 B 画面", "请先选择 B 画面视频（P/B 帧来源）", state.Win)
			return
		}
		if _, err := os.Stat(aPath); err != nil {
			dialog.ShowError(fmt.Errorf("A 素材不存在：%w", err), state.Win)
			return
		}
		if _, err := os.Stat(bPath); err != nil {
			dialog.ShowError(fmt.Errorf("B 视频不存在：%w", err), state.Win)
			return
		}
		autoFillOutput()
		if outPath == "" {
			dialog.ShowInformation("缺少输出路径", "请选择输出 MP4 的保存位置", state.Win)
			return
		}
		if err := absplice.EnsureDir(outPath); err != nil {
			dialog.ShowError(fmt.Errorf("创建输出目录失败：%w", err), state.Win)
			return
		}
		bins, err := resolveBins()
		if err != nil {
			dialog.ShowError(err, state.Win)
			return
		}
		cfg := absplice.DefaultEncodeConfig(bins)
		cfg.Width = parseIntOr(widthEntry.Text, 1920)
		cfg.Height = parseIntOr(heightEntry.Text, 1080)
		cfg.FPS = parseIntOr(fpsEntry.Text, 30)
		cfg.CRF = parseIntOr(crfEntry.Text, 23)
		cfg.GOPLength = parseIntOr(gopEntry.Text, 30)
		if presetSelect.Selected != "" {
			cfg.Preset = presetSelect.Selected
		}
		startB := parseFloatOr(startEntry.Text, 0)
		dur := parseFloatOr(durEntry.Text, 1.0)
		if dur <= 0 {
			dur = 1.0
			durEntry.SetText("1.0")
		}
		idrManual := 0
		if !idrAutoCheck.Checked && !absplice.IsImagePath(aPath) {
			idrManual = parseIntOr(idrCountEntry.Text, 10)
		}
		wDir := workDir
		if wDir == "" {
			wDir = filepath.Dir(outPath)
		}
		opts := absplice.ABSpliceOptions{
			SourceAPath:   aPath,
			ImageAPath:    aPath,
			VideoBPath:    bPath,
			StartTimeB:    startB,
			Duration:      dur,
			OutPath:       outPath,
			WorkDir:       wDir,
			EncodeCfg:     cfg,
			KeepTempFiles: keepTempCheck.Checked,
			IDRCount:      idrManual,
			IDRPerSec:     1,
		}
		nPlan := absplice.SuggestIDRCount(dur, idrManual, 1)
		if absplice.IsImagePath(aPath) {
			nPlan = 1
		}
		appendLog(fmt.Sprintf(
			"开始任务：A=%s, B=%s @%.3fs +%.3fs, A-IDR×%d, 尺寸=%dx%d@%dfps CRF=%d preset=%s gop=%d",
			filepath.Base(aPath), filepath.Base(bPath),
			startB, dur, nPlan,
			cfg.Width, cfg.Height, cfg.FPS, cfg.CRF, cfg.Preset, cfg.GOPLength,
		))
		appendLog("输出 = " + outPath)
		setProgress(1, "准备中…")
		setBusy(true)
		ctx, cancel := context.WithCancel(context.Background())
		mu.Lock()
		cancelFn = cancel
		mu.Unlock()

		go func() {
			defer cancel()
			t0 := time.Now()
			setProgress(5, fmt.Sprintf("编码 A 的 %d 个 IDR + 切分 B…", nPlan))
			appendLog(fmt.Sprintf("从 A 均匀抽 %d 帧做成 IDR，B 切成 %d 段并替换段首 I 帧", nPlan, nPlan))
			result, err := absplice.ABSplice(ctx, opts)
			fyne.Do(func() {
				defer setBusy(false)
				if err != nil {
					setProgress(0, "失败")
					appendLog("失败：" + err.Error())
					dialog.ShowError(fmt.Errorf("拼接失败：%w", err), state.Win)
					return
				}
				dt := time.Since(t0).Round(time.Millisecond)
				setProgress(100, "完成 "+dt.String())
				appendLog(fmt.Sprintf("完成！A-IDR×%d，耗时 %s → %s", result.IDRCount, dt, result.OutputPath))
				lastOut = result.OutputPath
				openLast.Enable()
				showInFolder.Enable()
				if opts.KeepTempFiles {
					appendLog("临时文件已保留（用于调试）：")
					for i, p := range result.TempAIDRs {
						appendLog(fmt.Sprintf("  A IDR[%d] = %s", i, p))
					}
					for i, p := range result.TempBSegs {
						appendLog(fmt.Sprintf("  B 段[%d] = %s", i, p))
					}
					appendLog("  拼接裸流   = " + result.H264Path)
					appendLog("  音频AAC    = " + result.TempAudio)
				}
				dialog.ShowConfirm("完成",
					fmt.Sprintf("拼接成功（A-IDR×%d），耗时 %s。\n输出：%s\n\n是否用默认播放器打开验证？", result.IDRCount, dt, result.OutputPath),
					func(yes bool) {
						if yes {
							openExplorerOrFile(result.OutputPath)
						}
					}, state.Win)
			})
		}()

		go func() {
			for {
				mu.Lock()
				isRun := running
				mu.Unlock()
				if !isRun {
					return
				}
				time.Sleep(250 * time.Millisecond)
				fyne.Do(func() {
					cur := progress.Value
					if cur >= 98 {
						return
					}
					steps := []struct {
						at float64
						t  string
					}{
						{10, fmt.Sprintf("从 A 抽取并编码 %d 个 IDR…", nPlan)},
						{40, fmt.Sprintf("将 B 切成 %d 段并编码 P 帧…", nPlan)},
						{70, "重写 slice header 并拼接裸流…"},
						{90, "封装 MP4 + 合并音频…"},
					}
					for _, s := range steps {
						if cur < s.at {
							setProgress(cur+0.8, s.t)
							return
						}
					}
					setProgress(cur+0.4, "处理中…")
				})
			}
		}()
	}

	// ─── 布局 ──────────────────────────────────────────────────
	aRow := container.NewBorder(nil, nil, pickA, nil, aLabel)
	bBtns := container.NewHBox(pickB, analyzeB)
	bRow := container.NewBorder(nil, nil, bBtns, nil, bLabel)
	outRow := container.NewBorder(nil, nil, pickOut, nil, outLabel)

	paramsCard := widget.NewCard("参数", "", container.NewVBox(
		widget.NewForm(
			widget.NewFormItem("B 视频起始（秒）", startEntry),
			widget.NewFormItem("拼接持续（秒）", durEntry),
			widget.NewFormItem("输出帧率 (fps)", fpsEntry),
			widget.NewFormItem("CRF 质量（越小越清晰）", crfEntry),
			widget.NewFormItem("GOP 长度（帧）", gopEntry),
			widget.NewFormItem("输出宽度（px）", widthEntry),
			widget.NewFormItem("输出高度（px）", heightEntry),
			widget.NewFormItem("x264 preset", presetSelect),
			widget.NewFormItem("A-IDR 个数", idrCountEntry),
		),
		idrAutoCheck,
		idrHint,
		keepTempCheck,
	))

	fileCard := widget.NewCard("输入 / 输出",
		"A：图片或视频（均匀抽 N 帧做成 IDR）；B：视频（切成 N 段，每段开头的 I 换成对应 A 帧）",
		container.NewVBox(
			aRow, bRow, outRow,
			widget.NewSeparator(),
			container.NewGridWithColumns(2, startBtn, stopBtn),
			progress,
			statusLabel,
		),
	)

	hint := widget.NewLabel("验证：用「分析」看输出关键帧应约等于 A-IDR 个数且内容来自 A；播放时第 0/段首为 A，中间为 B（码流替换后段首附近可能有花屏属正常）。")
	hint.Wrapping = fyne.TextWrapWord
	hint.Importance = widget.WarningImportance
	btnsRow := container.NewGridWithColumns(2, openLast, showInFolder)

	return container.NewPadded(container.NewBorder(
		container.NewVBox(fileCard, paramsCard, hint, btnsRow),
		nil, nil, nil,
		widget.NewCard("运行日志", "", logEntry),
	))
}

func openExplorerOrFile(p string) {
	if p == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", p)
	case "darwin":
		cmd = exec.Command("open", p)
	default:
		cmd = exec.Command("xdg-open", p)
	}
	_ = cmd.Start()
}
