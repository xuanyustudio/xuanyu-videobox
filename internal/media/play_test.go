package media

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveFFplayLooksBesideFFmpeg(t *testing.T) {
	dir := t.TempDir()
	ff := filepath.Join(dir, "ffmpeg.exe")
	fp := filepath.Join(dir, "ffplay.exe")
	if err := os.WriteFile(ff, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fp, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := ResolveFFplay(ff, "")
	if err != nil {
		t.Fatal(err)
	}
	if got != fp {
		t.Fatalf("got %s want %s", got, fp)
	}
}

func TestPlayAudioMissingFile(t *testing.T) {
	if err := PlayAudio(filepath.Join(t.TempDir(), "missing.wav")); err == nil {
		t.Fatal("expected error")
	}
}

func TestPlayAndStopAudio(t *testing.T) {
	if _, err := ResolveFFplay("", ""); err != nil {
		t.Skip("ffplay not available")
	}
	// 0.3s of silence, PCM16 mono 8kHz, 16-byte header + samples
	path := filepath.Join(t.TempDir(), "silent.wav")
	sr := 8000
	n := sr / 3
	data := make([]byte, 44+n*2)
	copy(data[0:4], "RIFF")
	le := func(off int, v uint32, n int) {
		for i := 0; i < n; i++ {
			data[off+i] = byte(v >> (8 * i))
		}
	}
	le(4, uint32(36+n*2), 4)
	copy(data[8:12], "WAVE")
	copy(data[12:16], "fmt ")
	le(16, 16, 4)
	le(20, 1, 2)
	le(22, 1, 2)
	le(24, uint32(sr), 4)
	le(28, uint32(sr*2), 4)
	le(32, 2, 2)
	le(34, 16, 2)
	copy(data[36:40], "data")
	le(40, uint32(n*2), 4)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := PlayAudio(path); err != nil {
		t.Fatal(err)
	}
	time.Sleep(80 * time.Millisecond)
	if !AudioPlaying() {
		t.Fatal("expected playing")
	}
	StopAudio()
	deadline := time.Now().Add(2 * time.Second)
	for AudioPlaying() && time.Now().Before(deadline) {
		time.Sleep(20 * time.Millisecond)
	}
	if AudioPlaying() {
		t.Fatal("still playing after stop")
	}
}
