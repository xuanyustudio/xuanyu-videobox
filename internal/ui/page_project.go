//go:build fyne_gui

package ui

import (
	"fmt"
	"image/color"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/project"
)

func newProjectPage(state *AppState) (fyne.CanvasObject, func()) {
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance

	grid := container.NewGridWrap(fyne.NewSize(300, 168))
	scroll := container.NewVScroll(container.NewPadded(grid))

	openProject := func(p *project.Project) {
		state.Project = p
		status.SetText("当前工程: " + p.Name)
		state.triggerRefreshAssets()
		state.triggerRefreshScript()
		state.enterWorkspace()
	}

	var refreshList func()

	createProject := func(name string) {
		name = strings.TrimSpace(name)
		p, err := project.Create(state.ProjectsRoot, name)
		if err != nil {
			status.SetText("新建失败: " + err.Error())
			return
		}
		openProject(p)
		refreshList()
	}

	showNewDialog := func() {
		nameEntry := widget.NewEntry()
		nameEntry.SetPlaceHolder("工程名称")
		d := dialog.NewForm(
			"新建短剧项目",
			"创建",
			"取消",
			[]*widget.FormItem{widget.NewFormItem("名称", nameEntry)},
			func(ok bool) {
				if !ok {
					return
				}
				createProject(nameEntry.Text)
			},
			state.Win,
		)
		d.Resize(fyne.NewSize(420, 180))
		d.Show()
	}

	importProject := func() {
		dir, err := pickDirectory("选择要导入的工程文件夹")
		if err != nil {
			status.SetText(err.Error())
			return
		}
		if dir == "" {
			return
		}
		p, err := project.ImportFromDir(state.ProjectsRoot, dir)
		if err != nil {
			status.SetText("导入失败: " + err.Error())
			return
		}
		openProject(p)
		refreshList()
	}

	renameProject := func(p project.Project) {
		nameEntry := widget.NewEntry()
		nameEntry.SetText(p.Name)
		d := dialog.NewForm(
			"重命名工程",
			"保存",
			"取消",
			[]*widget.FormItem{widget.NewFormItem("名称", nameEntry)},
			func(ok bool) {
				if !ok {
					return
				}
				loaded, err := project.Load(state.ProjectsRoot, p.Slug)
				if err != nil {
					status.SetText(err.Error())
					return
				}
				loaded.Name = strings.TrimSpace(nameEntry.Text)
				if loaded.Name == "" {
					loaded.Name = loaded.Slug
				}
				if err := project.Save(state.ProjectsRoot, loaded); err != nil {
					status.SetText(err.Error())
					return
				}
				if state.Project != nil && state.Project.Slug == loaded.Slug {
					state.Project = loaded
				}
				refreshList()
			},
			state.Win,
		)
		d.Show()
	}

	deleteProject := func(p project.Project) {
		dialog.ShowConfirm(
			"删除工程",
			fmt.Sprintf("确定删除「%s」？此操作不可恢复。", p.Name),
			func(ok bool) {
				if !ok {
					return
				}
				if err := project.Delete(state.ProjectsRoot, p.Slug); err != nil {
					status.SetText(err.Error())
					return
				}
				if state.Project != nil && state.Project.Slug == p.Slug {
					state.Project = nil
					state.triggerRefreshAssets()
					state.triggerRefreshScript()
				}
				refreshList()
			},
			state.Win,
		)
	}

	openFolder := func(p project.Project) {
		dir := project.Dir(state.ProjectsRoot, p.Slug)
		if err := media.OpenFile(dir); err != nil {
			status.SetText(err.Error())
		}
	}

	refreshList = func() {
		list, err := project.List(state.ProjectsRoot)
		if err != nil {
			status.SetText("列表错误: " + err.Error())
			return
		}
		items := []fyne.CanvasObject{newQuickStartCard(showNewDialog, importProject)}
		for _, p := range list {
			pp := p
			active := state.Project != nil && state.Project.Slug == pp.Slug
			items = append(items, newProjectCard(pp, active, func() {
				loaded, err := project.Load(state.ProjectsRoot, pp.Slug)
				if err != nil {
					status.SetText(err.Error())
					return
				}
				openProject(loaded)
				refreshList()
			}, func() { openFolder(pp) }, func() { renameProject(pp) }, func() { deleteProject(pp) }))
		}
		grid.Objects = items
		grid.Refresh()
		if state.Project != nil {
			status.SetText("当前工程: " + state.Project.Name)
		} else {
			status.SetText("点击卡片打开工程，或新建一个")
		}
	}

	header := container.NewBorder(nil, nil, widget.NewLabelWithStyle("短剧解说", fyne.TextAlignLeading, fyne.TextStyle{Bold: true}), nil, status)
	return container.NewBorder(container.NewPadded(header), nil, nil, nil, scroll), refreshList
}

func newQuickStartCard(onNew, onImport func()) fyne.CanvasObject {
	title := canvas.NewText("快速开始", color.NRGBA{R: 0x5b, G: 0x4b, B: 0xc4, A: 0xff})
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.Alignment = fyne.TextAlignCenter
	title.TextSize = 15

	newBtn := widget.NewButton("+ 新建短剧项目", onNew)
	newBtn.Importance = widget.HighImportance
	importBtn := widget.NewButton("导入短剧项目", onImport)
	importBtn.Importance = widget.LowImportance

	body := container.NewVBox(
		container.NewCenter(title),
		container.NewPadded(container.NewGridWithColumns(2, newBtn, importBtn)),
	)

	bg := canvas.NewRectangle(color.NRGBA{R: 0xf7, G: 0xf5, B: 0xff, A: 0xff})
	bg.CornerRadius = 10
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = color.NRGBA{R: 0x8b, G: 0x7c, B: 0xe0, A: 0xff}
	border.StrokeWidth = 1.5
	border.CornerRadius = 10

	return container.NewStack(bg, border, container.NewPadded(body))
}

func newProjectCard(p project.Project, active bool, onOpen, onFolder, onRename, onDelete func()) fyne.CanvasObject {
	titleBtn := widget.NewButton(p.Name, onOpen)
	titleBtn.Importance = widget.LowImportance
	titleBtn.Alignment = widget.ButtonAlignLeading
	if len([]rune(p.Name)) > 20 {
		titleBtn.SetText(string([]rune(p.Name)[:20]) + "…")
	}

	desc := "暂无描述"
	if n := len(p.ScriptItems); n > 0 {
		desc = fmt.Sprintf("%d 条脚本", n)
	} else if n := len(p.VideoPaths); n > 0 {
		desc = fmt.Sprintf("%d 个视频", n)
	}
	descLabel := widget.NewLabel(desc)
	descLabel.Importance = widget.LowImportance

	tags := container.NewHBox(
		makeTag("草稿", color.NRGBA{R: 0xee, G: 0xf0, B: 0xf4, A: 0xff}, color.NRGBA{R: 0x5a, G: 0x64, B: 0x72, A: 0xff}),
		makeTag(fmt.Sprintf("视频 %d", len(p.VideoPaths)), color.NRGBA{R: 0xeb, G: 0xe6, B: 0xff, A: 0xff}, color.NRGBA{R: 0x5b, G: 0x4b, B: 0xc4, A: 0xff}),
		makeTag(fmt.Sprintf("字幕 %d", countNonEmpty(p.SubtitlePaths)), color.NRGBA{R: 0xff, G: 0xef, B: 0xd6, A: 0xff}, color.NRGBA{R: 0xb8, G: 0x6a, B: 0x00, A: 0xff}),
	)

	folderBtn := widget.NewButtonWithIcon("", theme.FolderOpenIcon(), onFolder)
	folderBtn.Importance = widget.LowImportance
	editBtn := widget.NewButtonWithIcon("", theme.DocumentCreateIcon(), onRename)
	editBtn.Importance = widget.LowImportance
	delBtn := widget.NewButtonWithIcon("", theme.DeleteIcon(), onDelete)
	delBtn.Importance = widget.DangerImportance
	actions := container.NewHBox(folderBtn, editBtn, delBtn)

	ts := formatProjectTime(p.UpdatedAt)
	if ts == "" {
		ts = formatProjectTime(p.CreatedAt)
	}
	timeLabel := canvas.NewText(ts, color.NRGBA{R: 0x8a, G: 0x96, B: 0xa6, A: 0xff})
	timeLabel.TextSize = 12

	top := container.NewBorder(nil, nil, nil, actions, titleBtn)
	body := container.NewVBox(top, descLabel, tags, timeLabel)

	bg := canvas.NewRectangle(color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	bg.CornerRadius = 10
	if active {
		bg.FillColor = color.NRGBA{R: 0xef, G: 0xf5, B: 0xff, A: 0xff}
	}
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = color.NRGBA{R: 0xe2, G: 0xe8, B: 0xf0, A: 0xff}
	border.StrokeWidth = 1
	border.CornerRadius = 10
	if active {
		border.StrokeColor = color.NRGBA{R: 0x1a, G: 0x73, B: 0xe8, A: 0xff}
		border.StrokeWidth = 1.5
	}

	return container.NewStack(bg, border, container.NewPadded(body))
}


func makeTag(text string, bgColor, fgColor color.Color) fyne.CanvasObject {
	label := canvas.NewText(text, fgColor)
	label.TextSize = 11
	bg := canvas.NewRectangle(bgColor)
	bg.CornerRadius = 8
	return container.NewStack(bg, container.NewPadded(label))
}

func formatProjectTime(iso string) string {
	iso = strings.TrimSpace(iso)
	if iso == "" {
		return ""
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02T15:04:05-07:00",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, iso); err == nil {
			return t.Format("2006/01/02 15:04")
		}
	}
	if len(iso) >= 16 {
		return strings.ReplaceAll(iso[:16], "T", " ")
	}
	return iso
}

func countNonEmpty(paths []string) int {
	n := 0
	for _, p := range paths {
		if strings.TrimSpace(p) != "" {
			n++
		}
	}
	return n
}
