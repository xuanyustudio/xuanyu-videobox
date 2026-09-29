# narrato-go

NarratoAI 的纯 Go 客户端，与仓库根目录下的 Python + Streamlit 版并行开发，**不替换**现有 `webui.py` 工作流。

默认界面为**原生 Fyne 窗口**（需本机/仓库内的 MinGW gcc）。仓库已附带便携工具链：`tools/w64devkit`（GCC 14，与 Go CGO 兼容）。

## 进度

- **Task 1–12**：已完成（配置、脚本/工程模型、media、LLM/TTS、pipeline、Fyne 原生 UI）。
- **Phase-1 MVP**：可本地新建工程、生成脚本、跑合成流水线。

## 依赖

- **Go** 1.21+（开发构建）
- **ffmpeg / ffprobe**：成片探测与合成必需。发布包需将可执行文件放入 `dist/`；开发时请在系统 PATH 中可用，或在 `config.yaml` 中配置 `ffmpeg.path` / `ffprobe.path`。
- **原生窗口构建**：需要 `gcc`（推荐 `tools/w64devkit`，**勿用 GCC 15 的 bigobj 版**，Go CGO 无法解析）。

## 配置

本地用户配置使用 **`config.yaml`**（与 Python 版 `config.toml` 部分键兼容），存放于用户数据目录或工程旁。密钥仅保存在本机。

「爆款复刻」页签使用 OpenAI 兼容的视觉模型（默认 ModelScope `Qwen/Qwen3-VL-8B-Instruct`），在「设置」中填写 `replicate.base_url` / `replicate.api_key` / `replicate.model`（对应 Python 版 `QWEN_API_BASE_URL` / `QWEN_API_KEY` / `QWEN_MODEL_ID`）；`replicate.frames` 为默认关键帧数（8）。结果（关键帧 + SORA2 中英文提示词）保存在源视频同目录 `replicate_<时间戳>/`。

工程目录语义对齐 Python 版：`resource/projects/<slug>/project.json` 及 `exports/` 结构，便于互导与对照验收。

## 构建与运行

### Windows 一键打包

```powershell
cd narrato-go
.\scripts\build_windows.ps1
```

产出：

| 文件 | 说明 |
|------|------|
| `dist/narrato.exe` | 默认产品：原生 Fyne 窗口 |
| `dist/narrato-cli.exe` | 控制台版（无 Fyne） |
| `dist/narrato-web.exe` | 浏览器 UI 兜底（无 CGO） |

分发前请将 **ffmpeg.exe** 与 **ffprobe.exe** 复制到 `dist/`（与 exe 同目录）。

### 手动构建

**原生 Fyne（推荐）：**

```powershell
$env:Path = "$PWD\tools\w64devkit\bin;$env:Path"
$env:CGO_ENABLED = "1"
go build -tags fyne_gui -ldflags "-H windowsgui -s -w" -o dist/narrato.exe ./cmd/narrato
```

**浏览器 UI 兜底（无需 gcc）：**

```powershell
go build -ldflags "-H windowsgui -s -w" -o dist/narrato-web.exe ./cmd/narrato
```

**开发运行：**

```powershell
$env:Path = "$PWD\tools\w64devkit\bin;$env:Path"
$env:CGO_ENABLED = "1"
go run -tags fyne_gui ./cmd/narrato
```

## 测试

**Pipeline 冒烟（单元/集成，可无 ffmpeg 时部分 Skip）：**

```powershell
go test ./internal/pipeline/
```

全量包测试：

```powershell
go test ./...
```

## Task 13：串联验收（人工）

**准备：** 本机 ffmpeg；可选打开 `resource/projects/女儿`；配置 LLM Key。

1. 新建工程 → 添加 1 个短视频 + srt → 保存  
2. 生成或粘贴最小 3 行脚本（含 1 段 OST=1）  
3. 确认预计时长；将目标改为 60s，若超长应出现提示  
4. 合成出 mp4；原声段无烧录解说字幕、无明显模糊条  
5. 重开工程，路径与脚本应恢复  

## 设计文档

见仓库 `docs/superpowers/specs/2026-09-24-narrato-golang-client-design.md` 与 `docs/plans/2026-09-24-narrato-golang-client.md`。
