package tts

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type wavFormat struct {
	AudioFormat   uint16
	Channels      uint16
	SampleRate    uint32
	ByteRate      uint32
	BlockAlign    uint16
	BitsPerSample uint16
}

type wavFile struct {
	Format wavFormat
	Data   []byte
}

// ConcatWAVFiles appends PCM WAV files of the same format into outPath.
// A single input is copied as-is.
func ConcatWAVFiles(inputs []string, outPath string) error {
	if len(inputs) == 0 {
		return fmt.Errorf("没有可拼接的音频")
	}
	if len(inputs) == 1 {
		data, err := os.ReadFile(inputs[0])
		if err != nil {
			return err
		}
		return os.WriteFile(outPath, data, 0o644)
	}
	var acc wavFile
	for i, p := range inputs {
		w, err := readWAV(p)
		if err != nil {
			return fmt.Errorf("读取 %s: %w", p, err)
		}
		if i == 0 {
			acc = w
			continue
		}
		if w.Format != acc.Format {
			return fmt.Errorf("音频格式不一致，无法拼接：%s", p)
		}
		acc.Data = append(acc.Data, w.Data...)
	}
	return writeWAV(outPath, acc)
}

// WAVDurationSec returns the duration of a PCM WAV file.
func WAVDurationSec(path string) (float64, error) {
	w, err := readWAV(path)
	if err != nil {
		return 0, err
	}
	if w.Format.ByteRate == 0 {
		return 0, fmt.Errorf("无效的 WAV 字节率")
	}
	return float64(len(w.Data)) / float64(w.Format.ByteRate), nil
}

func readWAV(path string) (wavFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return wavFile{}, err
	}
	r := bytes.NewReader(raw)
	var riff [4]byte
	var size uint32
	var wave [4]byte
	if err := binary.Read(r, binary.LittleEndian, &riff); err != nil {
		return wavFile{}, err
	}
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return wavFile{}, err
	}
	if err := binary.Read(r, binary.LittleEndian, &wave); err != nil {
		return wavFile{}, err
	}
	if string(riff[:]) != "RIFF" || string(wave[:]) != "WAVE" {
		return wavFile{}, fmt.Errorf("不是 WAV 文件")
	}
	var fmtChunk wavFormat
	var data []byte
	gotFmt := false
	for {
		var id [4]byte
		var n uint32
		if err := binary.Read(r, binary.LittleEndian, &id); err != nil {
			if err == io.EOF {
				break
			}
			return wavFile{}, err
		}
		if err := binary.Read(r, binary.LittleEndian, &n); err != nil {
			return wavFile{}, err
		}
		payload := make([]byte, n)
		if _, err := io.ReadFull(r, payload); err != nil {
			return wavFile{}, err
		}
		if n%2 == 1 {
			_, _ = r.ReadByte()
		}
		switch string(id[:]) {
		case "fmt ":
			if len(payload) < 16 {
				return wavFile{}, fmt.Errorf("fmt 块过短")
			}
			fmtChunk.AudioFormat = binary.LittleEndian.Uint16(payload[0:2])
			fmtChunk.Channels = binary.LittleEndian.Uint16(payload[2:4])
			fmtChunk.SampleRate = binary.LittleEndian.Uint32(payload[4:8])
			fmtChunk.ByteRate = binary.LittleEndian.Uint32(payload[8:12])
			fmtChunk.BlockAlign = binary.LittleEndian.Uint16(payload[12:14])
			fmtChunk.BitsPerSample = binary.LittleEndian.Uint16(payload[14:16])
			gotFmt = true
		case "data":
			data = payload
		}
	}
	if !gotFmt || data == nil {
		return wavFile{}, fmt.Errorf("WAV 缺少 fmt 或 data 块")
	}
	return wavFile{Format: fmtChunk, Data: data}, nil
}

func writeWAV(path string, w wavFile) error {
	var buf bytes.Buffer
	dataLen := uint32(len(w.Data))
	// 4 (WAVE) + 8 (fmt hdr) + 16 (fmt) + 8 (data hdr) + data
	riffSize := uint32(4 + 8 + 16 + 8 + dataLen)
	buf.WriteString("RIFF")
	_ = binary.Write(&buf, binary.LittleEndian, riffSize)
	buf.WriteString("WAVE")
	buf.WriteString("fmt ")
	_ = binary.Write(&buf, binary.LittleEndian, uint32(16))
	_ = binary.Write(&buf, binary.LittleEndian, w.Format.AudioFormat)
	_ = binary.Write(&buf, binary.LittleEndian, w.Format.Channels)
	_ = binary.Write(&buf, binary.LittleEndian, w.Format.SampleRate)
	_ = binary.Write(&buf, binary.LittleEndian, w.Format.ByteRate)
	_ = binary.Write(&buf, binary.LittleEndian, w.Format.BlockAlign)
	_ = binary.Write(&buf, binary.LittleEndian, w.Format.BitsPerSample)
	buf.WriteString("data")
	_ = binary.Write(&buf, binary.LittleEndian, dataLen)
	buf.Write(w.Data)
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// WritePCM16WAV is a test helper that writes a mono 16-bit PCM WAV.
func WritePCM16WAV(path string, sampleRate uint32, samples []int16) error {
	data := make([]byte, len(samples)*2)
	for i, s := range samples {
		binary.LittleEndian.PutUint16(data[i*2:], uint16(s))
	}
	w := wavFile{
		Format: wavFormat{
			AudioFormat:   1,
			Channels:      1,
			SampleRate:    sampleRate,
			ByteRate:      sampleRate * 2,
			BlockAlign:    2,
			BitsPerSample: 16,
		},
		Data: data,
	}
	return writeWAV(path, w)
}
