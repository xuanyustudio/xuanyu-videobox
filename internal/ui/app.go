//go:build fyne_gui

package ui

import (
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/narratoai/narrato-go/internal/config"
	"github.com/narratoai/narrato-go/internal/linkdl"
	"github.com/narratoai/narrato-go/internal/project"
)

// AppState holds shared UI and domain state for all tabs.
type AppState struct {
	Cfg          *config.Config
	CfgPath      string
	Project      *project.Project
	ProjectsRoot string
	Log          *LogBuf
	Win          fyne.Window

	onRefreshProject func()
	onRefreshAssets  func()
	onRefreshScript  func()
	onEnterWorkspace func()
	onLeaveWorkspace func()
}

// DefaultConfigPath prefers config.yaml beside the executable, else user config dir.
func DefaultConfigPath() string {
	if exe, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(exe), "config.yaml")
		if _, err := os.Stat(beside); err == nil {
			return beside
		}
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "config.yaml"
	}
	return filepath.Join(dir, "narrato-go", "config.yaml")
}

// Run starts the Fyne desktop shell (blocks until the window closes).
func Run() {
	cfgPath := DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fyne.LogError("load config", err)
		cfg = config.Default()
	}
	root := cfg.Paths.ProjectsRoot
	if root == "" {
		root = config.Default().Paths.ProjectsRoot
	}
	_ = os.MkdirAll(root, 0o755)

	linkdl.ConfigureDouyin(cfg.Douyin.BaseURL, cfg.Douyin.APIKey)

	// Prefer Windows YaHei before Fyne initializes its font cache.
	preferSystemFontEnv()

	a := app.NewWithID("com.narratoai.narrato-go")
	a.Settings().SetTheme(newNarratoTheme())
	w := a.NewWindow("玄宇视频工具箱")
	w.Resize(fyne.NewSize(1180, 760))
	w.SetMaster()

	state := &AppState{
		Cfg:          cfg,
		CfgPath:      cfgPath,
		ProjectsRoot: root,
		Log:          &LogBuf{},
		Win:          w,
	}

	projectPage, refreshProject := newProjectPage(state)
	assetsPage, refreshAssets := newAssetsPage(state)
	scriptPage, refreshScript := newScriptPage(state)
	renderPage := newRenderPage(state)
	settingsPage := newSettingsPage(state)
	originalPage := newOriginalPage(state)
	replicatePage := newReplicatePage(state)
	reversePage := newReversePage(state)
	hookPage := newHookPage(state)
	abSplicePage := newABSplicePage(state)
	linkDLPage := newLinkDownloadPage(state)

	state.onRefreshProject = refreshProject
	state.onRefreshAssets = refreshAssets
	state.onRefreshScript = refreshScript

	statusBar := widget.NewLabel("就绪。选择或新建工程")
	statusBar.Importance = widget.LowImportance

	projTitle := pageTitle("工程工作台")
	backBtn := widget.NewButton("← 返回工程列表", nil)
	backBtn.Importance = widget.LowImportance

	workspaceTabs := container.NewAppTabs(
		container.NewTabItem("素材", assetsPage),
		container.NewTabItem("脚本", scriptPage),
		container.NewTabItem("合成", renderPage),
	)
	workspaceTabs.SetTabLocation(container.TabLocationTop)

	workspaceHeader := container.NewBorder(nil, nil, backBtn, nil, projTitle)
	workspaceView := container.NewBorder(container.NewPadded(workspaceHeader), nil, nil, nil, workspaceTabs)

	homeStack := container.NewStack(projectPage, workspaceView)
	workspaceView.Hide()

	var showHome, showWorkspace func()
	showHome = func() {
		workspaceView.Hide()
		projectPage.Show()
		homeStack.Refresh()
		refreshProject()
		w.SetTitle("玄宇视频工具箱")
		if state.Project != nil {
			statusBar.SetText("工程列表。可重新打开: " + state.Project.Name)
		} else {
			statusBar.SetText("就绪。选择或新建工程")
		}
	}
	showWorkspace = func() {
		if state.Project == nil {
			return
		}
		name := state.Project.Name
		if name == "" {
			name = state.Project.Slug
		}
		projTitle.SetText(name)
		w.SetTitle("玄宇视频工具箱 - " + name)
		projectPage.Hide()
		workspaceView.Show()
		homeStack.Refresh()
		workspaceTabs.SelectIndex(0)
		refreshAssets()
		refreshScript()
		statusBar.SetText("当前工程: " + name + "。在素材、脚本、合成里继续")
	}

	backBtn.OnTapped = func() { showHome() }
	state.onEnterWorkspace = showWorkspace
	state.onLeaveWorkspace = showHome

	atlasPage := newAtlasPage()
	introPage := newIntroPage(state)
	voiceClonePage := newVoiceClonePage(state)
	audioPage := newAudioSplitPage(state)
	subtitlePage := newSubtitlePage(state)
	mergePage := newMergePage(state)
	rail, content := newRail([]railItem{
		{Label: "短剧解说", Icon: theme.MediaVideoIcon(), Page: homeStack},
		{Label: "一键原创", Icon: theme.ColorChromaticIcon(), Page: originalPage},
		{Label: "提示词提取", Icon: theme.ContentCopyIcon(), Page: replicatePage},
		{Label: "视频逆向", Icon: theme.ViewRefreshIcon(), Page: reversePage},
		{Label: "爆点开头", Icon: theme.MediaPlayIcon(), Page: hookPage},
		{Label: "链接下载", Icon: theme.DownloadIcon(), Page: linkDLPage},
		{Label: "AB 拼接", Icon: theme.ContentCutIcon(), Page: abSplicePage},
		{Label: "手法图鉴", Icon: theme.DocumentIcon(), Page: atlasPage},
		{Label: "口播视频", Icon: theme.MediaPhotoIcon(), Page: introPage},
		{Label: "克隆音频", Icon: theme.MediaMusicIcon(), Page: voiceClonePage},
		{Label: "音频分离", Icon: theme.FileAudioIcon(), Page: audioPage},
		{Label: "加字幕", Icon: theme.FileIcon(), Page: subtitlePage},
		{Label: "视频合并", Icon: theme.MediaFastForwardIcon(), Page: mergePage},
		{Label: "设置", Icon: theme.SettingsIcon(), Page: settingsPage},
	})

	statusRow := container.NewVBox(widget.NewSeparator(), container.NewPadded(statusBar))
	w.SetContent(container.NewBorder(nil, statusRow, rail, nil, content))
	refreshProject()
	trackIME(w)
	w.ShowAndRun()
}

func (s *AppState) triggerRefreshProject() {
	if s != nil && s.onRefreshProject != nil {
		s.onRefreshProject()
	}
}

func (s *AppState) triggerRefreshAssets() {
	if s != nil && s.onRefreshAssets != nil {
		s.onRefreshAssets()
	}
}

func (s *AppState) triggerRefreshScript() {
	if s != nil && s.onRefreshScript != nil {
		s.onRefreshScript()
	}
}

func (s *AppState) enterWorkspace() {
	if s != nil && s.onEnterWorkspace != nil {
		s.onEnterWorkspace()
	}
}

func (s *AppState) leaveWorkspace() {
	if s != nil && s.onLeaveWorkspace != nil {
		s.onLeaveWorkspace()
	}
}
