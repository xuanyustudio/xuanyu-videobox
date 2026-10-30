//go:build fyne_gui

package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/media"
)

func defaultDownloadDir(cfg *config.Config) string {
	root := cfg.Paths.ProjectsRoot
	if root == "" {
		root = config.Default().Paths.ProjectsRoot
	}
	return root
}

func newLinkDownloadPage(state *AppState) fyne.CanvasObject {
	var (
		mu       sync.Mutex
		busy     bool
		cancelFn context.CancelFunc
		lastDir  string
		feed     struct {
			posts     []linkdl.AuthorPost
			checked   []bool
			cursor    string
			hasMore   bool
			author    string
			sourceURL string
		}
	)

	subtitle := widget.NewLabel("粘贴视频分享链接 → 解析 → 下载无水印视频\n支持：抖音、快手、西瓜视频、微博、皮皮虾、好看视频、皮皮搞笑、梨视频\n抖音作者主页在下方解析作品列表，勾选后批量下载")
	subtitle.Wrapping = fyne.TextWrapWord
	subtitle.Importance = widget.LowImportance

	urlEntry := widget.NewMultiLineEntry()
	urlEntry.SetPlaceHolder("每行一个链接，支持分享文本（自动提取URL）\n例如：https://v.douyin.com/xxxxx/\n或：https://www.douyin.com/video/123456")
	urlEntry.SetMinRowsVisible(4)
	urlEntry.Wrapping = fyne.TextWrapWord

	outDirEntry := widget.NewEntry()
	outDirEntry.SetPlaceHolder("下载保存目录（留空使用工程根目录）")
	outDirEntry.SetText(defaultDownloadDir(state.Cfg))

	authorEntry := widget.NewEntry()
	authorEntry.SetPlaceHolder("抖音作者主页链接或分享文本，例如 https://www.douyin.com/user/……")

	status := widget.NewLabel("")
	status.Wrapping = fyne.TextWrapWord
	status.Importance = widget.LowImportance

	authorStatus := widget.NewLabel("解析作者主页后，在这里勾选要下载的作品")
	authorStatus.Wrapping = fyne.TextWrapWord
	authorStatus.Importance = widget.LowImportance

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 100

	logEntry := widget.NewMultiLineEntry()
	logEntry.SetMinRowsVisible(8)
	logEntry.Wrapping = fyne.TextWrapWord
	logEntry.Disable()

	checklist := container.NewVBox()
	checkScroll := container.NewVScroll(checklist)
	checkScroll.SetMinSize(fyne.NewSize(520, 280))

	appendLog := func(line string) {
		fyne.Do(func() {
			logEntry.SetText(logEntry.Text + line + "\n")
		})
	}

	var startBtn, cancelBtn, openOutBtn, parseBtn, loadMoreBtn, downloadSelBtn *widget.Button

	openOutBtn = widget.NewButton("打开输出目录", func() {
		mu.Lock()
		dir := lastDir
		mu.Unlock()
		if dir != "" {
			_ = media.OpenFile(dir)
		}
	})
	openOutBtn.Disable()

	setBusy := func(v bool) {
		busy = v
		if v {
			startBtn.Disable()
			parseBtn.Disable()
			loadMoreBtn.Disable()
			downloadSelBtn.Disable()
			cancelBtn.Enable()
			openOutBtn.Disable()
		} else {
			startBtn.Enable()
			parseBtn.Enable()
			downloadSelBtn.Enable()
			if feed.hasMore {
				loadMoreBtn.Enable()
			} else {
				loadMoreBtn.Disable()
			}
			cancelBtn.Disable()
			if lastDir != "" {
				openOutBtn.Enable()
			}
		}
	}

	refreshChecklist := func() {
		if len(feed.checked) != len(feed.posts) {
			next := make([]bool, len(feed.posts))
			copy(next, feed.checked)
			feed.checked = next
		}
		checklist.Objects = nil
		if len(feed.posts) == 0 {
			empty := widget.NewLabel("作品列表会显示在这里")
			empty.Importance = widget.LowImportance
			checklist.Add(empty)
		}
		for i := range feed.posts {
			idx := i
			post := feed.posts[idx]
			chk := widget.NewCheck(postChecklistTitle(post), nil)
			chk.SetChecked(feed.checked[idx])
			chk.OnChanged = func(on bool) {
				if idx < len(feed.checked) {
					feed.checked[idx] = on
				}
			}
			meta := widget.NewLabel(postChecklistMeta(post))
			meta.Importance = widget.LowImportance
			meta.Truncation = fyne.TextTruncateEllipsis
			checklist.Add(container.NewVBox(chk, meta))
		}
		checklist.Refresh()
		checkScroll.Refresh()
	}

	updateAuthorStatus := func() {
		name := feed.author
		if name == "" {
			name = "该作者"
		}
		more := "没有更多了"
		if feed.hasMore {
			more = "还可以加载更多"
		}
		authorStatus.SetText(fmt.Sprintf("%s · 已加载 %d 条 · %s", name, len(feed.posts), more))
	}

	outputDir := func() string {
		dir := strings.TrimSpace(outDirEntry.Text)
		if dir == "" {
			dir = defaultDownloadDir(state.Cfg)
		}
		return dir
	}

	startWork := func(timeout time.Duration, work func(ctx context.Context)) {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		cancelFn = cancel
		setBusy(true)
		mu.Unlock()

		go func() {
			defer func() {
				mu.Lock()
				cancelFn = nil
				mu.Unlock()
				fyne.Do(func() {
					mu.Lock()
					setBusy(false)
					mu.Unlock()
				})
			}()
			work(ctx)
		}()
	}

	downloadResolved := func(ctx context.Context, outDir string, labels []string, resolve func(context.Context, int) (*linkdl.VideoInfo, error)) {
		success := 0
		total := len(labels)
		for i, label := range labels {
			if ctx.Err() != nil {
				appendLog("已取消")
				break
			}
			tag := fmt.Sprintf("[%d/%d]", i+1, total)
			appendLog(fmt.Sprintf("%s 解析: %s", tag, label))
			fyne.Do(func() {
				status.SetText(fmt.Sprintf("%s 解析中…", tag))
				progress.SetValue(float64(i) / float64(total) * 100)
			})

			info, err := resolve(ctx, i)
			if err != nil {
				appendLog(fmt.Sprintf("%s 解析失败: %s", tag, err))
				continue
			}
			appendLog(fmt.Sprintf("%s [%s] %s — %s", tag, info.Platform, info.Title, info.Author))
			if info.VideoURL == "" {
				appendLog(fmt.Sprintf("%s 无视频地址（图集？已跳过）", tag))
				continue
			}

			fyne.Do(func() { status.SetText(fmt.Sprintf("%s 下载中…", tag)) })
			path, err := linkdl.Download(ctx, info, outDir, func(p linkdl.Progress) {
				fyne.Do(func() {
					base := float64(i) / float64(total) * 100
					step := float64(p.Percent) / float64(total)
					progress.SetValue(base + step)
				})
			})
			if err != nil {
				appendLog(fmt.Sprintf("%s 下载失败: %s", tag, err))
				continue
			}
			appendLog(fmt.Sprintf("%s 已保存: %s", tag, path))
			success++
			mu.Lock()
			lastDir = outDir
			mu.Unlock()
		}

		fyne.Do(func() {
			progress.SetValue(100)
			status.SetText(fmt.Sprintf("完成: %d/%d 成功", success, total))
		})
	}

	doDownload := func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()

		var urls []string
		for _, line := range strings.Split(urlEntry.Text, "\n") {
			if u := strings.TrimSpace(line); u != "" {
				urls = append(urls, u)
			}
		}
		if len(urls) == 0 {
			status.SetText("请输入至少一个链接")
			return
		}
		outDir := outputDir()
		logEntry.SetText("")
		progress.SetValue(0)
		status.SetText(fmt.Sprintf("开始处理 %d 个链接…", len(urls)))

		links := append([]string(nil), urls...)
		startWork(45*time.Second, func(ctx context.Context) {
			downloadResolved(ctx, outDir, links, func(ctx context.Context, i int) (*linkdl.VideoInfo, error) {
				return linkdl.DetectAndParse(ctx, links[i])
			})
		})
	}

	parseAuthor := func(appendPage bool) {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()

		raw := strings.TrimSpace(authorEntry.Text)
		cursor := ""
		if appendPage {
			raw = feed.sourceURL
			cursor = feed.cursor
			if raw == "" {
				authorStatus.SetText("请先解析作者主页")
				return
			}
		} else if raw == "" {
			authorStatus.SetText("请输入作者主页链接")
			return
		}

		if appendPage {
			authorStatus.SetText("正在加载更多…")
			logEntry.SetText(logEntry.Text + "加载更多…\n")
		} else {
			feed.posts = nil
			feed.checked = nil
			feed.cursor = ""
			feed.hasMore = false
			feed.author = ""
			feed.sourceURL = ""
			refreshChecklist()
			checkScroll.ScrollToTop()
			logEntry.SetText("解析作者主页: " + raw + "\n")
			progress.SetValue(0)
			authorStatus.SetText("正在解析作品列表…")
			status.SetText("正在解析作品列表…")
		}

		profile := raw
		pageCursor := cursor
		startWork(3*time.Minute, func(ctx context.Context) {
			page, err := linkdl.FetchDouyinAuthorPosts(ctx, profile, pageCursor)
			if err != nil {
				if ctx.Err() != nil {
					appendLog("已取消")
					fyne.Do(func() {
						authorStatus.SetText("已取消")
						status.SetText("已取消")
					})
					return
				}
				appendLog("作者主页解析失败: " + err.Error())
				fyne.Do(func() {
					authorStatus.SetText("解析失败: " + err.Error())
					status.SetText("解析失败")
				})
				return
			}

			fyne.Do(func() {
				if !appendPage {
					feed.sourceURL = profile
				}
				seen := map[string]struct{}{}
				for _, p := range feed.posts {
					if p.ContentID != "" {
						seen[p.ContentID] = struct{}{}
					}
				}
				added := 0
				for _, p := range page.Items {
					if p.ContentID != "" {
						if _, ok := seen[p.ContentID]; ok {
							continue
						}
						seen[p.ContentID] = struct{}{}
					}
					feed.posts = append(feed.posts, p)
					feed.checked = append(feed.checked, false)
					added++
				}
				if appendPage && added == 0 {
					feed.hasMore = false
					feed.cursor = ""
					appendLog("没有更多新作品")
				} else {
					feed.cursor = page.Cursor
					feed.hasMore = page.HasMore
					if page.Author != "" {
						feed.author = page.Author
					}
					appendLog(fmt.Sprintf("本页 %d 条，累计 %d 条", added, len(feed.posts)))
				}
				refreshChecklist()
				if !appendPage {
					checkScroll.ScrollToTop()
				}
				updateAuthorStatus()
				status.SetText(authorStatus.Text)
			})
		})
	}

	downloadSelected := func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()

		var selected []linkdl.AuthorPost
		for i, post := range feed.posts {
			if i < len(feed.checked) && feed.checked[i] {
				selected = append(selected, post)
			}
		}
		if len(selected) == 0 {
			status.SetText("请先勾选要下载的作品")
			return
		}
		outDir := outputDir()
		labels := make([]string, len(selected))
		for i, post := range selected {
			labels[i] = post.Title
			if labels[i] == "" {
				labels[i] = post.ContentID
			}
			if labels[i] == "" {
				labels[i] = post.WebURL
			}
		}
		logEntry.SetText("")
		progress.SetValue(0)
		status.SetText(fmt.Sprintf("开始下载所选 %d 个作品…", len(selected)))

		timeout := time.Duration(len(selected)) * 2 * time.Minute
		if timeout < 3*time.Minute {
			timeout = 3 * time.Minute
		}
		posts := append([]linkdl.AuthorPost(nil), selected...)
		startWork(timeout, func(ctx context.Context) {
			downloadResolved(ctx, outDir, labels, func(ctx context.Context, i int) (*linkdl.VideoInfo, error) {
				return linkdl.VideoInfoForPost(ctx, posts[i])
			})
		})
	}

	startBtn = widget.NewButton("解析并下载", func() { doDownload() })
	startBtn.Importance = widget.HighImportance

	cancelBtn = widget.NewButton("取消", func() {
		mu.Lock()
		if cancelFn != nil {
			cancelFn()
		}
		mu.Unlock()
	})
	cancelBtn.Disable()

	clearBtn := widget.NewButton("清空", func() {
		mu.Lock()
		if busy {
			mu.Unlock()
			return
		}
		mu.Unlock()
		urlEntry.SetText("")
		logEntry.SetText("")
		status.SetText("")
		progress.SetValue(0)
		openOutBtn.Disable()
	})
	clearBtn.Importance = widget.LowImportance

	pasteBtn := widget.NewButton("粘贴", func() {
		clip := fyne.CurrentApp().Clipboard().Content()
		if clip != "" {
			cur := urlEntry.Text
			if cur != "" && !strings.HasSuffix(cur, "\n") {
				cur += "\n"
			}
			urlEntry.SetText(cur + clip)
		}
	})
	pasteBtn.Importance = widget.LowImportance

	pasteAuthorBtn := widget.NewButton("粘贴主页", func() {
		clip := strings.TrimSpace(fyne.CurrentApp().Clipboard().Content())
		if clip != "" {
			authorEntry.SetText(clip)
		}
	})
	pasteAuthorBtn.Importance = widget.LowImportance

	parseBtn = widget.NewButton("解析作品列表", func() { parseAuthor(false) })
	parseBtn.Importance = widget.HighImportance

	loadMoreBtn = widget.NewButton("加载更多", func() { parseAuthor(true) })
	loadMoreBtn.Disable()

	selectAllBtn := widget.NewButton("全选", func() {
		if len(feed.posts) == 0 {
			return
		}
		if len(feed.checked) != len(feed.posts) {
			feed.checked = make([]bool, len(feed.posts))
		}
		for i := range feed.checked {
			feed.checked[i] = true
		}
		refreshChecklist()
	})
	selectAllBtn.Importance = widget.LowImportance

	clearSelBtn := widget.NewButton("清除选择", func() {
		for i := range feed.checked {
			feed.checked[i] = false
		}
		refreshChecklist()
	})
	clearSelBtn.Importance = widget.LowImportance

	downloadSelBtn = widget.NewButton("下载所选", func() { downloadSelected() })
	downloadSelBtn.Importance = widget.HighImportance

	refreshChecklist()

	sec := func(text string) *widget.Label {
		return widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	}

	toolbar := container.NewHBox(pasteBtn, startBtn, cancelBtn, openOutBtn, clearBtn)
	authorBar := container.NewHBox(pasteAuthorBtn, parseBtn, loadMoreBtn, selectAllBtn, clearSelBtn, downloadSelBtn)

	body := container.NewVBox(
		subtitle,
		sec("视频链接"),
		urlEntry,
		container.NewBorder(nil, nil, widget.NewLabel("保存目录"), nil, outDirEntry),
		toolbar,
		widget.NewSeparator(),
		sec("抖音作者主页"),
		authorEntry,
		authorBar,
		authorStatus,
		checkScroll,
		progress,
		status,
		sec("日志"),
		logEntry,
	)
	return container.NewVScroll(body)
}

func postChecklistTitle(p linkdl.AuthorPost) string {
	title := strings.Join(strings.Fields(p.Title), " ")
	if title == "" {
		title = "（无标题）"
	}
	r := []rune(title)
	if len(r) > 72 {
		return string(r[:72]) + "…"
	}
	return title
}

func postChecklistMeta(p linkdl.AuthorPost) string {
	id := p.ContentID
	if id == "" {
		id = "无编号"
	}
	parts := []string{id}
	if !p.CreatedAt.IsZero() {
		parts = append(parts, p.CreatedAt.Local().Format("2006-01-02 15:04"))
	}
	switch p.Kind {
	case "", "video":
	case "image_album":
		parts = append(parts, "图集")
	case "live":
		parts = append(parts, "直播")
	default:
		parts = append(parts, p.Kind)
	}
	return strings.Join(parts, " · ")
}
