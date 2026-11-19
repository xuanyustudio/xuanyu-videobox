# 玄宇视频工具箱

仓库：[xuanyustudio/xuanyu-videobox](https://github.com/xuanyustudio/xuanyu-videobox)

本人做解说和漫剧过程中，发现需要改动的周边工具，于是把日常会反复用到的能力收进这个 Windows 桌面工具箱。打开后窗口标题是「玄宇视频工具箱」。

默认界面是原生窗口（Fyne）。剪辑、合成和转码依赖本机的 **ffmpeg / ffprobe**；编译原生窗口需要 **gcc**（仓库已附带 `tools/w64devkit`，请勿使用 GCC 15 的 bigobj 版）。

本地密钥写在本机的 `config.yaml` 里，不会进入本仓库。

## 功能

侧栏从上到下依次是：

- **短剧解说**：用工程组织素材、脚本和成片，跑解说向的合成流水线
- **一键原创**：勾选画面手法、调节强度，先预览再按标准编码导出
- **提示词提取**：可一次多选视频，按队列做字幕、抽帧并生成提示词
- **视频逆向**：用设置里的视觉模型和内置 Pi Agent，拆出角色表、分镜和复刻方案，支持从断点继续
- **爆点开头**：结合画面、台词和音量，选出大约 3 到 8 秒最抓眼球的片段，加转场接到片头，合成新视频
- **链接下载**：粘贴分享链接，解析并下载无水印视频，支持抖音、快手、西瓜视频、微博等，也可解析作者主页后批量下载
- **AB 拼接**：用 A 素材（图片或视频）与 B 画面视频按设定拼接导出
- **手法图鉴**：查阅各层画面手法的说明和观感
- **口播视频**：上传角色图，生成四视图，再做成对着镜头介绍新片的口播视频；可选用参考音色或上传自己的参考音
- **克隆音频**：用 MiniMax 根据参考音复刻音色，也可查询、试听账号里已有的音色，再拿来合成口播
- **音频分离**：从视频抽出音轨，保存为同目录下的 MP3，支持批量
- **加字幕**：为视频烧录字幕；对不上的文件会走设置里的语音识别，成片和识别出的字幕写到视频目录下的 `srt` 文件夹
- **视频合并**：按列表顺序合并多段视频；画面尺寸不一致时按最大的一段对齐，其余补黑边
- **设置**：配置视觉模型、语音识别、MiniMax 等接口

## 依赖

- **Go** 1.25（见 `go.mod`）
- **ffmpeg / ffprobe**：成片探测与合成必需。发布时请把可执行文件放到 `dist/`；开发时可放在系统 PATH，或在 `config.yaml` 中配置路径
- **gcc**：原生窗口构建需要（推荐仓库内的 `tools/w64devkit`）

## 构建与运行

### Windows 一键打包

```powershell
cd xuanyu-videobox
.\scripts\build_windows.ps1
```

产出：

| 文件 | 说明 |
|------|------|
| `dist/玄宇视频工具箱.exe` | 默认产品：原生窗口 |
| `dist/narrato-cli.exe` | 控制台版（无 Fyne） |
| `dist/narrato-web.exe` | 浏览器 UI 兜底（无需 CGO） |

分发前请将 **ffmpeg.exe** 与 **ffprobe.exe** 复制到 `dist/`（与 exe 同目录）。

### 手动构建

原生窗口（推荐）：

```powershell
$env:Path = "$PWD\tools\w64devkit\bin;$env:Path"
$env:CGO_ENABLED = "1"
go build -tags fyne_gui -ldflags "-H windowsgui -s -w" -o dist/玄宇视频工具箱.exe ./cmd/narrato
```

浏览器 UI 兜底（无需 gcc）：

```powershell
go build -ldflags "-H windowsgui -s -w" -o dist/narrato-web.exe ./cmd/narrato
```

开发运行：

```powershell
$env:Path = "$PWD\tools\w64devkit\bin;$env:Path"
$env:CGO_ENABLED = "1"
go run -tags fyne_gui ./cmd/narrato
```

## 测试

```powershell
go test ./internal/pipeline/
go test ./...
```

## 联系方式

微信：qiangui8
