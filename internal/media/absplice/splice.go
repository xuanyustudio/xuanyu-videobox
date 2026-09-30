package absplice

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/narratoai/narrato-go/internal/media"
)

type ABSpliceOptions struct {
	// SourceAPath is A media: still image or video (preferred).
	SourceAPath string
	// ImageAPath is deprecated alias for SourceAPath (kept for older callers).
	ImageAPath string
	VideoBPath string
	AudioSource string
	StartTimeB  float64
	Duration    float64
	OutPath     string
	WorkDir     string
	EncodeCfg   EncodeConfig
	KeepTempFiles bool

	// IDRCount: number of A-IDRs to inject. 0 = auto (~1 per second of Duration).
	IDRCount int
	// IDRPerSec used when IDRCount==0 (default 1).
	IDRPerSec float64
}

type ABSpliceResult struct {
	OutputPath   string
	H264Path     string
	TempIFrame   string   // first A IDR (compat)
	TempPBFrames string   // first B segment (compat)
	TempAudio    string
	TempAIDRs    []string
	TempBSegs    []string
	IDRCount     int
}

func (o ABSpliceOptions) resolvedA() string {
	if o.SourceAPath != "" {
		return o.SourceAPath
	}
	return o.ImageAPath
}

func ABSplice(ctx context.Context, opts ABSpliceOptions) (*ABSpliceResult, error) {
	aPath := opts.resolvedA()
	if aPath == "" || opts.VideoBPath == "" || opts.OutPath == "" {
		return nil, fmt.Errorf("A 素材、B 视频和输出路径均必填")
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Dir(opts.OutPath)
	}
	if opts.WorkDir == "" {
		opts.WorkDir = "."
	}
	if err := os.MkdirAll(opts.WorkDir, 0o755); err != nil {
		return nil, fmt.Errorf("create work dir: %w", err)
	}
	if err := EnsureDir(opts.OutPath); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}

	fps := opts.EncodeCfg.FPS
	if fps <= 0 {
		fps = 30
		opts.EncodeCfg.FPS = fps
	}
	if opts.Duration <= 0 {
		opts.Duration = 1
	}

	n := opts.IDRCount
	if IsImagePath(aPath) {
		n = 1
	} else {
		n = SuggestIDRCount(opts.Duration, opts.IDRCount, opts.IDRPerSec)
	}

	result := &ABSpliceResult{
		OutputPath: opts.OutPath,
		TempAudio:  filepath.Join(opts.WorkDir, "absplice_mux_audio.aac"),
		H264Path:   filepath.Join(opts.WorkDir, "absplice_concat_vonly.h264"),
		IDRCount:   n,
	}
	result.TempAIDRs = make([]string, n)
	result.TempBSegs = make([]string, n)
	for i := 0; i < n; i++ {
		result.TempAIDRs[i] = filepath.Join(opts.WorkDir, fmt.Sprintf("absplice_A_%02d.h264", i))
		result.TempBSegs[i] = filepath.Join(opts.WorkDir, fmt.Sprintf("absplice_B_%02d.h264", i))
	}
	result.TempIFrame = result.TempAIDRs[0]
	result.TempPBFrames = result.TempBSegs[0]
	if !opts.KeepTempFiles {
		defer cleanupTempFiles(result)
	}

	// Sample times on A
	aTimes := make([]float64, n)
	if IsImagePath(aPath) {
		aTimes[0] = 0
	} else {
		aDur, err := ProbeMediaDurationSec(ctx, opts.EncodeCfg.FFmpegBins, aPath)
		if err != nil {
			return nil, fmt.Errorf("probe A duration: %w", err)
		}
		aTimes = EvenSampleTimes(aDur, n)
	}

	windows := SegmentWindows(opts.StartTimeB, opts.Duration, n)
	maxSeg := 0.0
	for _, w := range windows {
		if w[1] > maxSeg {
			maxSeg = w[1]
		}
	}
	EnsureSegmentGOP(&opts.EncodeCfg, maxSeg)

	// 1) Encode N IDRs from A
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := EncodeIDRFromMedia(ctx, opts.EncodeCfg, aPath, aTimes[i], result.TempAIDRs[i]); err != nil {
			return nil, fmt.Errorf("encode A IDR #%d: %w", i, err)
		}
	}

	// 2) Encode N B segments
	for i := 0; i < n; i++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		start, dur := windows[i][0], windows[i][1]
		if err := EncodePBFrames(ctx, opts.EncodeCfg, opts.VideoBPath, start, dur, result.TempBSegs[i]); err != nil {
			return nil, fmt.Errorf("encode B segment #%d: %w", i, err)
		}
	}

	// 3) Combine all pairs
	if err := CombineMultiStreams(result.TempAIDRs, result.TempBSegs, result.H264Path); err != nil {
		return nil, fmt.Errorf("combine annexb streams: %w", err)
	}

	audioSrc := opts.AudioSource
	if audioSrc == "" {
		audioSrc = opts.VideoBPath
	}
	audioDur := opts.Duration + (1.0 / float64(fps))
	if err := ExtractAudio(ctx, opts.EncodeCfg.FFmpegBins, audioSrc, opts.StartTimeB, audioDur, result.TempAudio); err != nil {
		return nil, fmt.Errorf("extract audio: %w", err)
	}
	if err := MuxMP4WithAudio(ctx, opts.EncodeCfg.FFmpegBins, result.H264Path, result.TempAudio, opts.OutPath, fps, audioDur); err != nil {
		return nil, fmt.Errorf("mux MP4 with audio: %w", err)
	}
	return result, nil
}

// CombineStreams is the single-pair helper (A IDR + B P-frames).
func CombineStreams(iFramePath, pbFramesPath, outH264 string, cfg EncodeConfig) error {
	_ = cfg
	return CombineMultiStreams([]string{iFramePath}, []string{pbFramesPath}, outH264)
}

// CombineMultiStreams builds:
//
//	SPS/PPS(from first B) + for each i: IDR(Ai) + rewritten non-IDR VCL(Bi)
func CombineMultiStreams(aIDRPaths, bSegPaths []string, outH264 string) error {
	if len(aIDRPaths) == 0 || len(aIDRPaths) != len(bSegPaths) {
		return fmt.Errorf("A IDR paths and B segment paths length mismatch")
	}

	type pair struct {
		aNALs, bNALs []NALUnit
	}
	pairs := make([]pair, len(aIDRPaths))
	var spsNAL, ppsNAL *NALUnit
	var sps *SPSParams

	for i := range aIDRPaths {
		aData, err := os.ReadFile(aIDRPaths[i])
		if err != nil {
			return fmt.Errorf("read A IDR #%d: %w", i, err)
		}
		bData, err := os.ReadFile(bSegPaths[i])
		if err != nil {
			return fmt.Errorf("read B segment #%d: %w", i, err)
		}
		pairs[i].aNALs = ParseNALUnits(aData)
		pairs[i].bNALs = ParseNALUnits(bData)
		if len(pairs[i].aNALs) == 0 {
			return fmt.Errorf("no NAL in A IDR #%d", i)
		}
		if len(pairs[i].bNALs) == 0 {
			return fmt.Errorf("no NAL in B segment #%d", i)
		}
		if spsNAL == nil {
			spsNAL = FindNALByType(pairs[i].bNALs, NALTypeSPS)
			ppsNAL = FindNALByType(pairs[i].bNALs, NALTypePPS)
			if spsNAL == nil {
				spsNAL = FindNALByType(pairs[i].aNALs, NALTypeSPS)
			}
			if ppsNAL == nil {
				ppsNAL = FindNALByType(pairs[i].aNALs, NALTypePPS)
			}
		}
	}
	if spsNAL == nil || ppsNAL == nil {
		return fmt.Errorf("missing SPS or PPS")
	}
	var err error
	sps, err = ParseSPS(spsNAL)
	if err != nil {
		return fmt.Errorf("parse SPS: %w", err)
	}

	maxFN := sps.MaxFrameNum()
	if maxFN == 0 {
		maxFN = 16
	}
	maxPOC := sps.MaxPicOrderCntLsb()
	if maxPOC == 0 {
		maxPOC = 256
	}

	var outBuf bytes.Buffer
	startCode4 := []byte{0x00, 0x00, 0x00, 0x01}
	writeNAL := func(n NALUnit) {
		outBuf.Write(startCode4)
		outBuf.WriteByte(n.HeaderByte)
		outBuf.Write(n.Payload)
	}
	writeNAL(*spsNAL)
	writeNAL(*ppsNAL)

	totalP := 0
	for i, p := range pairs {
		idr := FindNALByType(p.aNALs, NALTypeIDR)
		if idr == nil {
			return fmt.Errorf("no IDR in A stream #%d", i)
		}
		writeNAL(*idr)

		frameIdx := uint32(1)
		segP := 0
		for _, n := range p.bNALs {
			if !n.IsVCL() || n.Type == NALTypeIDR {
				continue
			}
			fn := frameIdx % maxFN
			poc := (frameIdx * 2) % maxPOC
			rewritten, err := RewriteSliceHeaderForRef(&n, sps, fn, poc)
			if err != nil {
				return fmt.Errorf("rewrite segment #%d frame_num=%d: %w", i, fn, err)
			}
			writeNAL(*rewritten)
			frameIdx++
			segP++
		}
		if segP == 0 {
			return fmt.Errorf("B segment #%d has no P frames after dropping IDR", i)
		}
		totalP += segP
	}
	if totalP == 0 {
		return fmt.Errorf("no P frames written")
	}

	if err := EnsureDir(outH264); err != nil {
		return err
	}
	if err := os.WriteFile(outH264, outBuf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write combined H.264: %w", err)
	}
	return nil
}

func MuxMP4WithAudio(ctx context.Context, bins media.FFmpegBins, videoPath, audioPath, outPath string, fps int, durationSec float64) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	if fps <= 0 {
		fps = 30
	}
	if durationSec <= 0 {
		durationSec = 1.0 / float64(fps)
	}
	if err := EnsureDir(outPath); err != nil {
		return err
	}
	args := []string{
		"-y",
		"-fflags", "+genpts",
		"-f", "h264",
		"-r", fmt.Sprintf("%d", fps),
		"-i", videoPath,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "aac",
		"-ar", "44100",
		"-ac", "2",
		"-b:a", "192k",
		"-t", fmt.Sprintf("%.3f", durationSec),
		"-map", "0:v:0",
		"-map", "1:a:0",
		"-movflags", "+faststart",
		"-max_muxing_queue_size", "4096",
		outPath,
	}
	out, err := media.Run(ctx, bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("mux MP4 failed: %w, output: %s", err, out)
	}
	return nil
}

func cleanupTempFiles(r *ABSpliceResult) {
	paths := []string{r.TempAudio, r.H264Path}
	paths = append(paths, r.TempAIDRs...)
	paths = append(paths, r.TempBSegs...)
	// compat singles may duplicate list entries — remove is fine
	if r.TempIFrame != "" {
		paths = append(paths, r.TempIFrame)
	}
	if r.TempPBFrames != "" {
		paths = append(paths, r.TempPBFrames)
	}
	for _, p := range paths {
		if p != "" {
			_ = os.Remove(p)
		}
	}
}
