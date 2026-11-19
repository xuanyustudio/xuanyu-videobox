package hookopen

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

type energyPeak struct {
	Time  float64
	Level float64
}

// rmsSeries returns RMS loudness for successive hopSec windows of a 16-bit PCM WAV (mono preferred).
func rmsSeries(path string, winSec, hopSec float64) ([]energyPeak, error) {
	raw, rate, ch, err := readPCM16(path)
	if err != nil {
		return nil, err
	}
	if rate <= 0 || len(raw) == 0 {
		return nil, fmt.Errorf("空音轨")
	}
	if winSec <= 0 {
		winSec = 0.5
	}
	if hopSec <= 0 {
		hopSec = 0.25
	}
	win := int(winSec * float64(rate))
	hop := int(hopSec * float64(rate))
	if win < 16 {
		win = 16
	}
	if hop < 8 {
		hop = 8
	}
	step := ch
	if step < 1 {
		step = 1
	}
	nFrames := len(raw) / step
	var out []energyPeak
	for start := 0; start+win <= nFrames; start += hop {
		var sum float64
		count := 0
		for i := 0; i < win; i++ {
			idx := (start + i) * step
			if idx >= len(raw) {
				break
			}
			v := float64(raw[idx]) / 32768.0
			sum += v * v
			count++
		}
		if count == 0 {
			continue
		}
		rms := math.Sqrt(sum / float64(count))
		t := (float64(start) + float64(win)/2) / float64(rate)
		out = append(out, energyPeak{Time: t, Level: rms})
	}
	return out, nil
}

func peakTimes(series []energyPeak, maxN int) []float64 {
	if len(series) == 0 {
		return nil
	}
	// local maxima, then take the loudest
	type scored struct {
		T, L float64
	}
	var cands []scored
	for i := 1; i < len(series)-1; i++ {
		if series[i].Level >= series[i-1].Level && series[i].Level >= series[i+1].Level && series[i].Level > 0.02 {
			cands = append(cands, scored{T: series[i].Time, L: series[i].Level})
		}
	}
	if len(cands) == 0 {
		// fallback: globally loudest windows
		for _, p := range series {
			cands = append(cands, scored{T: p.Time, L: p.Level})
		}
	}
	for i := 0; i < len(cands); i++ {
		for j := i + 1; j < len(cands); j++ {
			if cands[j].L > cands[i].L {
				cands[i], cands[j] = cands[j], cands[i]
			}
		}
	}
	var times []float64
	for _, c := range cands {
		tooClose := false
		for _, t := range times {
			if abs(t-c.T) < 3 {
				tooClose = true
				break
			}
		}
		if tooClose {
			continue
		}
		times = append(times, c.T)
		if maxN > 0 && len(times) >= maxN {
			break
		}
	}
	return times
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

func readPCM16(path string) ([]int16, int, int, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, 0, err
	}
	defer f.Close()
	hdr := make([]byte, 12)
	if _, err := io.ReadFull(f, hdr); err != nil {
		return nil, 0, 0, err
	}
	if string(hdr[0:4]) != "RIFF" || string(hdr[8:12]) != "WAVE" {
		return nil, 0, 0, fmt.Errorf("不是 WAV 文件")
	}
	var (
		rate, ch, bits int
		data           []byte
	)
	for {
		chunkHdr := make([]byte, 8)
		if _, err := io.ReadFull(f, chunkHdr); err != nil {
			break
		}
		id := string(chunkHdr[0:4])
		size := int(binary.LittleEndian.Uint32(chunkHdr[4:8]))
		if size < 0 {
			return nil, 0, 0, fmt.Errorf("损坏的 WAV")
		}
		body := make([]byte, size)
		if _, err := io.ReadFull(f, body); err != nil {
			return nil, 0, 0, err
		}
		if size%2 == 1 {
			io.CopyN(io.Discard, f, 1)
		}
		switch id {
		case "fmt ":
			if len(body) >= 16 {
				ch = int(binary.LittleEndian.Uint16(body[2:4]))
				rate = int(binary.LittleEndian.Uint32(body[4:8]))
				bits = int(binary.LittleEndian.Uint16(body[14:16]))
			}
		case "data":
			data = body
		}
	}
	if bits != 16 || rate == 0 || ch == 0 || len(data) < 2 {
		return nil, 0, 0, fmt.Errorf("仅支持 16-bit PCM WAV")
	}
	n := len(data) / 2
	samples := make([]int16, n)
	for i := 0; i < n; i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2 : i*2+2]))
	}
	return samples, rate, ch, nil
}
