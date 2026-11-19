//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/reverse"
)

func newReversePage(state *AppState) fyne.CanvasObject {
	var (
		videoPath string
		outDir    string
		logPath   string
		runMu     sync.Mutex
		cancelRun context.CancelFunc
		busy      bool
		logLines  []string
	)

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100
	progress.SetValue(0)

	piStatus := widget.NewLabel("正在检查 Pi Agent…")
	piStatus.Wrapping = fyne.TextWrapWord
	piStatus.Importance = widget.LowImportance

	videoHint := widget.NewLabel("未选择视频")
	videoHint.Wrapping = fyne.TextWrapWord
	videoHint.Importance = widget.LowImportance

	subtitle := widget.NewLabel("选择一条参考视频，用「设置」里的视觉模型和内置 Pi Agent 拆出角色表、分镜和复刻方案，方便长视频保持人物一致。")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	castEntry := widget.NewMultiLineEntry()
	castEntry.SetPlaceHolder("角色 / 场景 / 道具表将显示在这里…")
	castEntry.Wrapping = fyne.TextWrapWord
	castEntry.SetMinRowsVisible(10)

	shotsEntry := widget.NewMultiLineEntry()
	shotsEntry.SetPlaceHolder("分镜表将显示在这里…")
	shotsEntry.Wrapping = fyne.TextWrapWord
	shotsEntry.SetMinRowsVisible(10)

	remakeEntry := widget.NewMultiLineEntry()
	remakeEntry.SetPlaceHolder("复刻方案和提示词草稿将显示在这里…")
	remakeEntry.Wrapping = fyne.TextWrapWord
	remakeEntry.SetMinRowsVisible(10)

	reportEntry := widget.NewMultiLineEntry()
	reportEntry.SetPlaceHolder("拆解报告将显示在这里…")
	reportEntry.Wrapping = fyne.TextWrapWord
	reportEntry.SetMinRowsVisible(10)

	resultTabs := container.NewAppTabs(
		container.NewTabItem("角色表", castEntry),
		container.NewTabItem("分镜", shotsEntry),
		container.NewTabItem("复刻方案", remakeEntry),
		container.NewTabItem("报告", reportEntry),
	)

	logEntry := widget.NewMultiLineEntry()
	logEntry.SetPlaceHolder("运行日志会显示在这里…")
	logEntry.Wrapping = fyne.TextWrapWord
	logEntry.SetMinRowsVisible(8)

	var startBtn, resumeBtn, cancelBtn, openOutBtn, openLogBtn, installBtn *widget.Button

	appendLog := func(msg string) {
		msg = strings.TrimSpace(msg)
		if msg == "" {
			return
		}
		logLines = append(logLines, time.Now().Format("15:04:05")+"  "+msg)
		if len(logLines) > 400 {
			logLines = logLines[len(logLines)-400:]
		}
		text := strings.Join(logLines, "\n")
		logEntry.SetText(text)
		logEntry.CursorRow = len(logLines)
		logEntry.Refresh()
	}

	refreshResume := func() {
		if resumeBtn == nil {
			return
		}
		if busy || videoPath == "" {
			resumeBtn.Disable()
			return
		}
		dir, summary, ok := reverse.FindResumable(videoPath)
		if !ok {
			resumeBtn.Disable()
			return
		}
		outDir = dir
		openOutBtn.Enable()
		lp := filepath.Join(dir, "run.log")
		logPath = lp
		openLogBtn.Enable()
		resumeBtn.Enable()
		if strings.TrimSpace(status.Text) == "" {
			status.SetText(summary)
		}
	}

	setBusy := func(v bool) {
		busy = v
		if v {
			startBtn.Disable()
			resumeBtn.Disable()
			installBtn.Disable()
			cancelBtn.Enable()
		} else {
			startBtn.Enable()
			installBtn.Enable()
			cancelBtn.Disable()
			refreshResume()
		}
	}

	refreshPi := func() {
		go func() {
			st := reverse.Probe(context.Background())
			fyne.Do(func() {
				if st.Installed {
					ver := st.Version
					if ver == "" {
						ver = "已安装"
					}
					piStatus.SetText("Pi Agent " + ver + " 已就绪，使用「设置」中的视觉模型。")
					installBtn.SetText("重新安装 Pi")
				} else {
					piStatus.SetText("尚未安装 Pi Agent。第一次拆解时会自动安装，也可以点右边按钮先装好。")
					installBtn.SetText("安装 Pi Agent")
				}
			})
		}()
	}

	setResult := func(res *reverse.Result) {
		cast, shots, remake, report := reverse.ReadResultFiles(res)
		castEntry.SetText(cast)
		shotsEntry.SetText(shots)
		remakeEntry.SetText(remake)
		reportEntry.SetText(report)
		if res != nil && res.OutDir != "" {
			outDir = res.OutDir
			openOutBtn.Enable()
		}
		if res != nil && res.LogPath != "" {
			logPath = res.LogPath
			openLogBtn.Enable()
		}
	}

	pickBtn := widget.NewButton("选择视频", func() {
		files, err := pickVideoFiles()
		if err != nil {
			status.SetText("选择视频失败: " + err.Error())
			appendLog("选择视频失败: " + err.Error())
			return
		}
		if len(files) == 0 {
			return
		}
		videoPath = files[0]
		videoHint.SetText("已选: " + videoPath)
		if len(files) > 1 {
			videoHint.SetText("已选: " + videoPath + fmt.Sprintf("（本次只处理第一条，另 %d 条未加入）", len(files)-1))
		}
		status.SetText("")
		appendLog("已选择视频: " + videoPath)
		if dir, summary, ok := reverse.FindResumable(videoPath); ok {
			status.SetText(summary)
			appendLog(summary + "：" + dir)
		}
		refreshResume()
	})

	installBtn = widget.NewButton("安装 Pi Agent", func() {
		runMu.Lock()
		if busy {
			runMu.Unlock()
			return
		}
		setBusy(true)
		runMu.Unlock()
		status.SetText("正在安装 Pi Agent…")
		appendLog("正在安装 Pi Agent…")
		go func() {
			err := reverse.Install(context.Background(), func(msg string) {
				fyne.Do(func() {
					status.SetText(msg)
					appendLog(msg)
				})
			})
			fyne.Do(func() {
				runMu.Lock()
				setBusy(false)
				runMu.Unlock()
				if err != nil {
					status.SetText("安装失败: " + err.Error())
					appendLog("安装失败: " + err.Error())
					return
				}
				status.SetText("Pi Agent 安装完成")
				appendLog("Pi Agent 安装完成")
				refreshPi()
			})
		}()
	})

	openOutBtn = widget.NewButton("打开输出目录", func() {
		if outDir != "" {
			openExplorerOrFile(outDir)
		}
	})
	openOutBtn.Disable()

	openLogBtn = widget.NewButton("打开日志文件", func() {
		if logPath != "" {
			openExplorerOrFile(logPath)
		} else if outDir != "" {
			openExplorerOrFile(outDir)
		}
	})
	openLogBtn.Disable()

	startJob := func(resume bool) {
		if videoPath == "" {
			status.SetText("请先选择一条视频")
			return
		}
		resumeDir := ""
		if resume {
			dir, summary, ok := reverse.FindResumable(videoPath)
			if !ok {
				status.SetText("没有找到可继续的拆解，请先点「开始逆向」")
				return
			}
			resumeDir = dir
			status.SetText(summary)
		}
		runMu.Lock()
		if busy {
			runMu.Unlock()
			return
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancelRun = cancel
		setBusy(true)
		runMu.Unlock()

		progress.SetValue(0)
		if resume {
			status.SetText("从断点继续…")
		} else {
			status.SetText("开始拆解…")
			castEntry.SetText("")
			shotsEntry.SetText("")
			remakeEntry.SetText("")
			reportEntry.SetText("")
		}
		logLines = nil
		logEntry.SetText("")
		if resume {
			appendLog("从断点继续: " + resumeDir)
			state.Log.Append("[视频逆向] 继续: " + resumeDir)
		} else {
			appendLog("开始拆解: " + videoPath)
			state.Log.Append("[视频逆向] 开始: " + videoPath)
		}

		go func(path, dir string, doResume bool) {
			var (
				res *reverse.Result
				err error
			)
			cb := func(pct int, msg string) {
				fyne.Do(func() {
					if pct >= 0 {
						progress.SetValue(float64(pct))
					}
					if msg != "" {
						status.SetText(msg)
						appendLog(msg)
					}
				})
			}
			if doResume {
				res, err = reverse.Resume(ctx, state.Cfg, dir, cb)
			} else {
				res, err = reverse.Run(ctx, state.Cfg, path, cb)
			}
			fyne.Do(func() {
				runMu.Lock()
				cancelRun = nil
				setBusy(false)
				runMu.Unlock()
				if res != nil {
					setResult(res)
				}
				if ctx.Err() != nil {
					status.SetText("已取消。已完成的抽帧和笔记会保留，可点「继续上次」接着跑")
					appendLog("已取消")
					state.Log.Append("[视频逆向] 已取消")
					return
				}
				if err != nil {
					status.SetText("失败: " + err.Error())
					appendLog("失败: " + err.Error())
					state.Log.Append("[视频逆向] 失败: " + err.Error())
					return
				}
				progress.SetValue(100)
				doneDir := ""
				if res != nil {
					doneDir = res.OutDir
				}
				status.SetText("拆解完成，结果在 " + doneDir)
				appendLog("拆解完成，结果在 " + doneDir)
				state.Log.Append("[视频逆向] 完成: " + doneDir)
			})
		}(videoPath, resumeDir, resume)
	}

	startBtn = widget.NewButton("开始逆向", func() { startJob(false) })
	startBtn.Importance = widget.HighImportance
	resumeBtn = widget.NewButton("继续上次", func() { startJob(true) })
	resumeBtn.Disable()
	cancelBtn = widget.NewButton("取消", func() {
		runMu.Lock()
		if cancelRun != nil {
			cancelRun()
		}
		runMu.Unlock()
		appendLog("正在取消…")
	})
	cancelBtn.Disable()

	modelHint := widget.NewLabel("视觉模型与「提示词提取」共用，在「设置」中配置。结果和运行日志保存在视频同目录 reverse_文件名_时间戳/ 下。中途失败或取消后，可点「继续上次」跳过抽帧，从已有笔记接着跑。")
	modelHint.Wrapping = fyne.TextWrapWord
	modelHint.Importance = widget.LowImportance

	toolbar := container.NewHBox(pickBtn, startBtn, resumeBtn, cancelBtn, openOutBtn, openLogBtn, installBtn)
	logScroll := container.NewVScroll(logEntry)
	logScroll.SetMinSize(fyne.NewSize(0, 180))

	body := container.NewVBox(
		pageTitle("视频逆向"),
		subtitle,
		toolbar,
		piStatus,
		videoHint,
		progress,
		status,
		modelHint,
		sectionTitle("运行日志"),
		logScroll,
		sectionTitle("拆解结果"),
		resultTabs,
	)
	refreshPi()
	return container.NewVScroll(body)
}
