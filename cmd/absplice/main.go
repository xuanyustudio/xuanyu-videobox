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
		sourceA    = flag.String("a", "", "A 素材路径（图片或视频；视频将均匀抽 N 帧做 IDR）")
		videoB     = flag.String("b", "", "B 画面视频路径（P 帧来源）")
		audioSrc   = flag.String("audio", "", "音频来源（可选，默认使用 B 视频的音频）")
		startB     = flag.Float64("start", 0.0, "B 视频起始时间（秒）")
		duration   = flag.Float64("dur", 1.0, "拼接持续时间（秒）")
		idrCount   = flag.Int("idr", 0, "A-IDR 个数；0=自动（约每秒 1 个；图片 A 固定 1）")
		out        = flag.String("o", "ab_splice_out.mp4", "输出 MP4 路径")
		workDir    = flag.String("work", "", "临时工作目录（默认与输出目录相同）")
		width      = flag.Int("w", 1920, "输出宽度")
		height     = flag.Int("h", 1080, "输出高度")
		fps        = flag.Int("fps", 30, "输出帧率")
		crf        = flag.Int("crf", 23, "x264 CRF 质量")
		preset     = flag.String("preset", "medium", "x264 预设 (ultrafast, fast, medium, slow...)")
		gop        = flag.Int("gop", 30, "GOP 参考长度（会按分段自动抬高）")
		keepTemp   = flag.Bool("keep", false, "保留临时文件（用于调试）")
		ffmpegBin  = flag.String("ffmpeg", "", "ffmpeg 二进制路径（留空自动查找）")
		ffprobeBin = flag.String("ffprobe", "", "ffprobe 二进制路径（留空自动查找）")
	)
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "ABSplice — 多 IDR 码流级 AB 拼接\n\n")
		fmt.Fprintf(os.Stderr, "用法:\n  %s -a A.mp4 -b B.mp4 -dur 10 -idr 0 -o out.mp4\n\n选项:\n", filepath.Base(os.Args[0]))
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\n原理:\n")
		fmt.Fprintf(os.Stderr, "  从 A 均匀抽 N 帧编成 IDR；B 切成 N 段；每段去掉自带 IDR，换上对应 A-IDR。\n")
	}
	flag.Parse()

	if *sourceA == "" || *videoB == "" {
		flag.Usage()
		fmt.Fprintln(os.Stderr, "\n错误: 必须指定 -a (A 图片/视频) 和 -b (B 视频)")
		os.Exit(2)
	}
	if _, err := os.Stat(*sourceA); err != nil {
		fmt.Fprintf(os.Stderr, "A 素材不存在: %s (%v)\n", *sourceA, err)
		os.Exit(1)
	}
	if _, err := os.Stat(*videoB); err != nil {
		fmt.Fprintf(os.Stderr, "B 视频不存在: %s (%v)\n", *videoB, err)
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
			fmt.Fprintf(os.Stderr, "找不到 ffmpeg: %v\n", err)
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

	nPlan := absplice.SuggestIDRCount(*duration, *idrCount, 1)
	if absplice.IsImagePath(*sourceA) {
		nPlan = 1
	}
	fmt.Fprintf(os.Stdout, "[ABSplice] ffmpeg  = %s\n", bins.FFmpeg)
	fmt.Fprintf(os.Stdout, "[ABSplice] A 素材  = %s\n", *sourceA)
	fmt.Fprintf(os.Stdout, "[ABSplice] B 视频  = %s @ %.3fs +%.3fs\n", *videoB, *startB, *duration)
	fmt.Fprintf(os.Stdout, "[ABSplice] A-IDR   = %d\n", nPlan)
	fmt.Fprintf(os.Stdout, "[ABSplice] 输出    = %s (%dx%d @ %dfps)\n", *out, *width, *height, *fps)

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
		SourceAPath:   *sourceA,
		ImageAPath:    *sourceA,
		VideoBPath:    *videoB,
		AudioSource:   *audioSrc,
		StartTimeB:    *startB,
		Duration:      *duration,
		OutPath:       *out,
		WorkDir:       *workDir,
		EncodeCfg:     cfg,
		KeepTempFiles: *keepTemp,
		IDRCount:      *idrCount,
		IDRPerSec:     1,
	}
	t0 := time.Now()
	result, err := absplice.ABSplice(ctx, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n[ABSplice] 失败: %v\n", err)
		os.Exit(1)
	}
	dt := time.Since(t0)
	fmt.Fprintf(os.Stdout, "\n[ABSplice] 完成！A-IDR×%d 用时 %s\n", result.IDRCount, dt.Round(time.Millisecond))
	fmt.Fprintf(os.Stdout, "  输出文件: %s\n", result.OutputPath)
	if *keepTemp {
		for i, p := range result.TempAIDRs {
			fmt.Fprintf(os.Stdout, "  A[%d]=%s\n", i, p)
		}
		for i, p := range result.TempBSegs {
			fmt.Fprintf(os.Stdout, "  B[%d]=%s\n", i, p)
		}
		fmt.Fprintf(os.Stdout, "  拼接264: %s\n", result.H264Path)
	}
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
