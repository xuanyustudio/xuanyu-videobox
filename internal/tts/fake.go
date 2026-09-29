package tts

import (
	"context"
	"encoding/binary"
	"math"
	"os"
	"path/filepath"
)

// Fake writes a silent WAV whose length matches EstimateDurationSec (tests / offline).
type Fake struct{}

func (Fake) Synthesize(_ context.Context, text, _, outPath string) (float64, error) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return 0, err
	}
	dur := EstimateDurationSec(text)
	if err := writeSilentWAV(outPath, dur); err != nil {
		return 0, err
	}
	return dur, nil
}

func writeSilentWAV(path string, durationSec float64) error {
	const (
		sampleRate    = 8000
		bitsPerSample = 16
		numChannels   = 1
	)
	if durationSec < 1 {
		durationSec = 1
	}
	numSamples := int(math.Round(durationSec * sampleRate))
	if numSamples < sampleRate {
		numSamples = sampleRate
	}
	blockAlign := numChannels * bitsPerSample / 8
	byteRate := sampleRate * blockAlign
	dataSize := numSamples * blockAlign

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	write := func(v any) error {
		return binary.Write(f, binary.LittleEndian, v)
	}

	if _, err := f.Write([]byte("RIFF")); err != nil {
		return err
	}
	if err := write(uint32(36 + dataSize)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("WAVEfmt ")); err != nil {
		return err
	}
	if err := write(uint32(16)); err != nil {
		return err
	}
	if err := write(uint16(1)); err != nil {
		return err
	}
	if err := write(uint16(numChannels)); err != nil {
		return err
	}
	if err := write(uint32(sampleRate)); err != nil {
		return err
	}
	if err := write(uint32(byteRate)); err != nil {
		return err
	}
	if err := write(uint16(blockAlign)); err != nil {
		return err
	}
	if err := write(uint16(bitsPerSample)); err != nil {
		return err
	}
	if _, err := f.Write([]byte("data")); err != nil {
		return err
	}
	if err := write(uint32(dataSize)); err != nil {
		return err
	}
	samples := make([]int16, numSamples)
	return binary.Write(f, binary.LittleEndian, samples)
}
