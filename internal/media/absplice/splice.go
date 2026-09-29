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
	ImageAPath    string
	VideoBPath    string
	AudioSource   string
	StartTimeB    float64
	Duration      float64
	OutPath       string
	WorkDir       string
	EncodeCfg     EncodeConfig
	KeepTempFiles bool
}

type ABSpliceResult struct {
	OutputPath   string
	H264Path     string
	TempIFrame   string
	TempPBFrames string
	TempAudio    string
}

func ABSplice(ctx context.Context, opts ABSpliceOptions) (*ABSpliceResult, error) {
	if opts.ImageAPath == "" || opts.VideoBPath == "" || opts.OutPath == "" {
		return nil, fmt.Errorf("input paths and output path are required")
	}
	if opts.WorkDir == "" {
		opts.WorkDir = filepath.Dir(opts.OutPath)
	}
	if err := EnsureDir(opts.OutPath); err != nil {
		return nil, fmt.Errorf("create output dir: %w", err)
	}
	result := &ABSpliceResult{
		OutputPath:   opts.OutPath,
		TempIFrame:   filepath.Join(opts.WorkDir, "absplice_A_iframeraw.h264"),
		TempPBFrames: filepath.Join(opts.WorkDir, "absplice_B_pbraw.h264"),
		TempAudio:    filepath.Join(opts.WorkDir, "absplice_mux_audio.aac"),
		H264Path:     filepath.Join(opts.WorkDir, "absplice_combined.h264"),
	}
	if !opts.KeepTempFiles {
		defer cleanupTempFiles(result)
	}
	if err := EncodeIFrame(ctx, opts.EncodeCfg, opts.ImageAPath, result.TempIFrame); err != nil {
		return nil, fmt.Errorf("encode I-frame from A: %w", err)
	}
	if err := EncodePBFrames(ctx, opts.EncodeCfg, opts.VideoBPath, opts.StartTimeB, opts.Duration, result.TempPBFrames); err != nil {
		return nil, fmt.Errorf("encode P/B frames from B: %w", err)
	}
	audioSrc := opts.AudioSource
	if audioSrc == "" {
		audioSrc = opts.VideoBPath
	}
	audioStart := opts.StartTimeB
	audioDur := opts.Duration + (1.0 / float64(opts.EncodeCfg.FPS))
	if err := ExtractAudio(ctx, opts.EncodeCfg.FFmpegBins, audioSrc, audioStart, audioDur, result.TempAudio); err != nil {
		return nil, fmt.Errorf("extract audio: %w", err)
	}
	if err := CombineStreams(result.TempIFrame, result.TempPBFrames, result.H264Path, opts.EncodeCfg); err != nil {
		return nil, fmt.Errorf("combine H.264 streams: %w", err)
	}
	if err := MuxMP4WithAudio(ctx, opts.EncodeCfg.FFmpegBins, result.H264Path, result.TempAudio, opts.OutPath, opts.EncodeCfg.FPS); err != nil {
		return nil, fmt.Errorf("mux MP4 with audio: %w", err)
	}
	return result, nil
}

func CombineStreams(iFramePath, pbFramesPath, outH264 string, cfg EncodeConfig) error {
	iData, err := os.ReadFile(iFramePath)
	if err != nil {
		return fmt.Errorf("read I-frame H.264: %w", err)
	}
	pbData, err := os.ReadFile(pbFramesPath)
	if err != nil {
		return fmt.Errorf("read P/B H.264: %w", err)
	}
	iNALs := ParseNALUnits(iData)
	pbNALs := ParseNALUnits(pbData)
	if len(iNALs) == 0 {
		return fmt.Errorf("no NAL units found in I-frame stream")
	}
	if len(pbNALs) == 0 {
		return fmt.Errorf("no NAL units found in P/B stream")
	}
	spsNAL := FindNALByType(iNALs, NALTypeSPS)
	if spsNAL == nil {
		spsNAL = FindNALByType(pbNALs, NALTypeSPS)
	}
	ppsNAL := FindNALByType(iNALs, NALTypePPS)
	if ppsNAL == nil {
		ppsNAL = FindNALByType(pbNALs, NALTypePPS)
	}
	if spsNAL == nil || ppsNAL == nil {
		return fmt.Errorf("missing SPS or PPS in either stream")
	}
	sps, err := ParseSPS(spsNAL)
	if err != nil {
		return fmt.Errorf("parse SPS: %w", err)
	}
	var iFrameNAL *NALUnit
	for i := range iNALs {
		if iNALs[i].Type == NALTypeIDR {
			iFrameNAL = &iNALs[i]
			break
		}
	}
	if iFrameNAL == nil {
		return fmt.Errorf("no IDR NAL (I-frame) found in A stream")
	}
	iSH, err := ParseSliceHeader(iFrameNAL, sps)
	if err != nil {
		return fmt.Errorf("parse I-frame slice header: %w", err)
	}
	maxFrameNum := uint32(1) << uint(sps.Log2MaxFrameNumMinus4+4)
	maxPOCLsb := uint32(1) << uint(sps.Log2MaxPicOrderCntLsbMinus4+4)
	_ = maxFrameNum
	pbVCLNALs := make([]NALUnit, 0, len(pbNALs))
	for _, n := range pbNALs {
		if n.IsVCL() {
			pbVCLNALs = append(pbVCLNALs, n)
		}
	}
	if len(pbVCLNALs) == 0 {
		return fmt.Errorf("no VCL NAL units (P/B frames) found in B stream")
	}
	baseFrameNum := iSH.FrameNum
	basePOC := iSH.PicOrderCntLsb
	pocIncrement := uint32(1)
	frameNumIncrement := uint32(1)
	rewrittenVCLs := make([]NALUnit, 0, len(pbVCLNALs))
	pocIdx := uint32(0)
	fnIdx := uint32(0)
	for _, vcl := range pbVCLNALs {
		pocIdx++
		fnIdx++
		newFN := baseFrameNum + fnIdx*frameNumIncrement
		newPOC := (basePOC + pocIdx*pocIncrement) % maxPOCLsb
		if newFN == iSH.FrameNum {
			newFN = (newFN + 1) % maxFrameNum
		}
		rewritten, err := RewriteSliceHeaderForRef(&vcl, sps, newFN, newPOC)
		if err != nil {
			return fmt.Errorf("rewrite PB slice header: %w", err)
		}
		rewrittenVCLs = append(rewrittenVCLs, *rewritten)
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
	writeNAL(*iFrameNAL)
	for _, rv := range rewrittenVCLs {
		writeNAL(rv)
	}
	eoSeq := NALUnit{Type: NALTypeEOSeq, HeaderByte: byte(NALTypeEOSeq) | 0x00, Payload: []byte{}}
	writeNAL(eoSeq)
	if err := os.WriteFile(outH264, outBuf.Bytes(), 0o644); err != nil {
		return fmt.Errorf("write combined H.264: %w", err)
	}
	return nil
}

func MuxMP4WithAudio(ctx context.Context, bins media.FFmpegBins, h264Path, audioPath, outPath string, fps int) error {
	if bins.FFmpeg == "" {
		return fmt.Errorf("ffmpeg binary required")
	}
	args := []string{
		"-y",
		"-r", fmt.Sprintf("%d", fps),
		"-i", h264Path,
		"-i", audioPath,
		"-c:v", "copy",
		"-c:a", "aac",
		"-ar", "44100",
		"-ac", "2",
		"-shortest",
		"-movflags", "+faststart",
		outPath,
	}
	out, err := media.Run(ctx, bins.FFmpeg, args...)
	if err != nil {
		return fmt.Errorf("mux MP4 failed: %w, output: %s", err, out)
	}
	return nil
}

func cleanupTempFiles(r *ABSpliceResult) {
	paths := []string{r.TempIFrame, r.TempPBFrames, r.TempAudio, r.H264Path}
	for _, p := range paths {
		if p != "" {
			_ = os.Remove(p)
		}
	}
}
