package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/narratoai/narrato-go/internal/media"
	"github.com/narratoai/narrato-go/internal/media/absplice"
)

func main() {
	var (
		imageA    = flag.String("a", "", "A 画面图片路径（I 帧来源）")
		videoB    = flag.String("b", "", "B 画面视频路径（P/B 帧来源）")
		audioSrc  = flag.String("audio", "", "音频来源（可选，默认使用 B 视频的音频）")
		startB    = flag.Float64("start", 0.0, "B 视频起始时间（秒）")
		duration  = flag.Float64("dur", 1.0, "拼接持续时间（秒），不含 I 帧 A 本身时长")
		out       = flag.String("o", "ab_splice_out.mp4", "输出 MP4 路径")
		workDir   = flag.String("work", "", "临时工作目录（默认与输出目录相同）")
		width     = flag.Int("w", 1920, "输出宽度")
		height    = flag.Int("h", 1080, "输出高度")
		fps       = flag.Int("fps", 30, "输出帧率")
		crf       = flag.Int("crf", 23, "x264 CRF 质量")
		preset    = flag.String("preset", "medium", "x264 预设 (ultrafast, fast, medium, slow...)")
		gop       = flag.Int("gop", 30, "GOP 参考长度（PB 帧编码用的大 keyint）")
		keepTemp  = flag.Bool("keep", false, "保留临时文件（用于调试）")
		ffmpegBin = flag.String("ffmpeg", "", "ffmpeg 二进制路径（留空自动查找）")
		ffprobeBin = flag.String("ffprobe", "", "ffprobe 二进制路径（留空自动查找）")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ABSplice — H.264 码流级 AB 画面拼接工具（市场常见的 I/P 帧切换方案）\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n  %s -a A.jpg -b B.mp4 -start 1.5 -dur 1.0 -o out.mp4\n\n选项:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n原理:\n")
		fmt.Fprintf(os.Stderr, "  1. A 素材 → x264 编码 1 个 I 帧 (IDR NAL)\n")
		fmt.Fprintf(os.Stderr, "  2. B 素材 → x264 编码长 GOP P/B 帧序列（禁用场景切换/IDR）\n")
		fmt.Fprintf(os.Stderr, "  3. 解析两组 Annexb 裸流，提取 SPS/PPS/VCL NAL\n")
		fmt.Fprintf(os.Stderr, "  4. 重写 B 端每个 P/B 帧 slice header: frame_num / pic_order_cnt_lsb 连续对齐到 A 的 I 帧\n")
		fmt.Fprintf(os.Stderr, "  5. 按 [SPS, PPS, IDR(A), PB1, PB2..PBn, EOSeq] 顺序拼接成新的 Annexb H.264\n")
		fmt.Fprintf(os.Stderr, "  6. FFmpeg 以 copy 视频 + 重编码/复制音频 封装 MP4\n")
		fmt.Fprintf(os.Stderr, "  7. 用 VLC 打开输出拖动验证 画面A→画面B 连续无花屏\n")
	}
	flag.Parse()

	if *imageA == "" || *videoB == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\n错误: 必须指定 -a (A 图) 和 -b (B 视频)")
		os.Exit(2)
	}
	if _, err := os.Stat(*imageA); err != nil {
		fmt.Fprintf(os.Stderr, "A 画面文件不存在: %s (%v)\n", *imageA, err)
		os.Exit(1)
	}
	if _, err := os.Stat(*videoB); err != nil {
		fmt.Fprintf(os.Stderr, "B 视频文件不存在: %s (%v)\n", *videoB, err)
		os.Exit(1)
	}
	bins := media.FFmpegBins{FFmpeg: *ffmpegBin, FFprobe: *ffprobeBin}
	var err error
	if bins.FFmpeg == "" {
		beside := ""
		if exe, e := os.Executable(); e == nil {
			beside = filepath.Dir(exe)
		}
		dist := filepath.Join(rootOfProject(), "dist")
		if s, e := os.Stat(dist); e == nil && s.IsDir() {
			if bins.FFmpeg, err = media.ResolveFFmpeg(filepath.Join(dist, "ffmpeg.exe"), beside); err != nil {
				bins.FFmpeg, err = media.ResolveFFmpeg("", beside)
			}
		} else {
			bins.FFmpeg, err = media.ResolveFFmpeg("", beside)
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "找不到 ffmpeg: %v\n请通过 -ffmpeg 指定路径，或将 ffmpeg.exe 放入 dist/ 目录或 PATH 中。\n", err)
			os.Exit(1)
		}
	}
	if bins.FFprobe == "" {
		beside := ""
		if exe, e := os.Executable(); e == nil {
			beside = filepath.Dir(exe)
		}
		bins.FFprobe, _ = media.ResolveFFprobe("", beside)
	}
	fmt.Fprintf(os.Stdout, "[ABSplice] ffmpeg  = %s\n", bins.FFmpeg)
	fmt.Fprintf(os.Stdout, "[ABSplice] ffprobe = %s\n", bins.FFprobe)
	fmt.Fprintf(os.Stdout, "[ABSplice] A 图片  = %s\n", *imageA)
	fmt.Fprintf(os.Stdout, "[ABSplice] B 视频  = %s @ %.3fs +%.3fs\n", *videoB, *startB, *duration)
	fmt.Fprintf(os.Stdout, "[ABSplice] 输出    = %s (%dx%d @ %dfps CRF=%d %s)\n", *out, *width, *height, *fps, *crf, *preset)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	cfg := absplice.DefaultEncodeConfig(bins)
	cfg.Width = *width
	cfg.Height = *height
	cfg.FPS = *fps
	cfg.CRF = *crf
	cfg.Preset = *preset
	cfg.GOPLength = *gop
	opts := absplice.ABSpliceOptions{
		ImageAPath:    *imageA,
		VideoBPath:    *videoB,
		AudioSource:   *audioSrc,
		StartTimeB:    *startB,
		Duration:      *duration,
		OutPath:       *out,
		WorkDir:       *workDir,
		EncodeCfg:     cfg,
		KeepTempFiles: *keepTemp,
	}
	t0 := time.Now()
	result, err := absplice.ABSplice(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[ABSplice] 失败: %v\n", err)
		os.Exit(1)
	}
	dt := time.Since(t0)
	fmt.Fprintf(os.Stdout, "\n[ABSplice] 完成！用时 %s\n", dt.Round(time.Millisecond))
	fmt.Fprintf(os.Stdout, "  输出文件: %s\n", result.OutputPath)
	if *keepTemp {
		fmt.Fprintf(os.Stdout, "  临时 I帧:  %s\n", result.TempIFrame)
		fmt.Fprintf(os.Stdout, "  临时 PB帧: %s\n", result.TempPBFrames)
		fmt.Fprintf(os.Stdout, "  拼接264:   %s\n", result.H264Path)
		fmt.Fprintf(os.Stdout, "  音频AAC:   %s\n", result.TempAudio)
	}
	fmt.Fprintln(os.Stdout, "\n下一步: 用 VLC / ffplay 打开输出，拖动到起点前后验证 A→B 是否无缝无花屏。")
}

func rootOfProject() string {
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	if exe, err := os.Executable(); err == nil {
		return filepath.Dir(filepath.Dir(exe))
	}
	return "."
}
