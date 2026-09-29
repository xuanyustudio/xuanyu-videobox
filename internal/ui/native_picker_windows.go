//go:build fyne_gui && windows

package ui

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"github.com/TheTitanrain/w32"
	"github.com/sqweek/dialog"
)

func pickVideoFiles() ([]string, error) {
	return pickFilesMulti(
		"选择视频",
		[]fileFilter{
			{Desc: "视频文件", Exts: []string{"mp4", "mov", "mkv", "avi", "m4v", "webm", "ts"}},
			{Desc: "所有文件", Exts: []string{"*"}},
		},
	)
}

func pickSubtitleFiles() ([]string, error) {
	return pickFilesMulti(
		"选择字幕",
		[]fileFilter{
			{Desc: "字幕文件", Exts: []string{"srt", "ass", "ssa", "vtt"}},
			{Desc: "所有文件", Exts: []string{"*"}},
		},
	)
}

func pickAudioFiles() ([]string, error) {
	return pickFilesMulti(
		"选择音频",
		[]fileFilter{
			{Desc: "音频文件", Exts: []string{"mp3", "wav", "m4a", "aac", "flac", "ogg"}},
			{Desc: "所有文件", Exts: []string{"*"}},
		},
	)
}

func pickDirectory(title string) (string, error) {
	if title == "" {
		title = "选择文件夹"
	}
	dir, err := dialog.Directory().Title(title).Browse()
	if err != nil {
		if err == dialog.ErrCancelled || err == dialog.Cancelled {
			return "", nil
		}
		return "", err
	}
	return dir, nil
}

type fileFilter struct {
	Desc string
	Exts []string
}

// pickFilesMulti uses the Win32 common file dialog directly (no PowerShell / no console).
func pickFilesMulti(title string, filters []fileFilter) ([]string, error) {
	const bufChars = 64 * 1024
	buf := make([]uint16, bufChars)

	filterBuf := make([]uint16, 0, 256)
	for _, f := range filters {
		filterBuf = append(filterBuf, utf16.Encode([]rune(f.Desc))...)
		filterBuf = append(filterBuf, 0)
		for i, ext := range f.Exts {
			if i > 0 {
				filterBuf = append(filterBuf, ';')
			}
			pat := "*." + ext
			filterBuf = append(filterBuf, utf16.Encode([]rune(pat))...)
		}
		filterBuf = append(filterBuf, 0)
	}
	filterBuf = append(filterBuf, 0)

	ofn := &w32.OPENFILENAME{
		StructSize: uint32(unsafe.Sizeof(w32.OPENFILENAME{})),
		File:       &buf[0],
		MaxFile:    uint32(len(buf)),
		Filter:     &filterBuf[0],
		Flags: w32.OFN_FILEMUSTEXIST |
			w32.OFN_PATHMUSTEXIST |
			w32.OFN_EXPLORER |
			w32.OFN_ALLOWMULTISELECT |
			w32.OFN_NOCHANGEDIR,
	}
	if title != "" {
		ofn.Title, _ = syscall.UTF16PtrFromString(title)
	}

	if !w32.GetOpenFileName(ofn) {
		if w32.CommDlgExtendedError() == 0 {
			return nil, nil // cancelled
		}
		return nil, fmt.Errorf("打开文件对话框失败 (0x%x)", w32.CommDlgExtendedError())
	}
	return parseMultiFileBuf(buf), nil
}

func parseMultiFileBuf(buf []uint16) []string {
	parts := splitUTF16Z(buf)
	if len(parts) == 0 {
		return nil
	}
	if len(parts) == 1 {
		return []string{parts[0]}
	}
	dir := parts[0]
	out := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

func splitUTF16Z(buf []uint16) []string {
	var parts []string
	start := 0
	for i := 0; i < len(buf); i++ {
		if buf[i] != 0 {
			continue
		}
		if i == start {
			break
		}
		parts = append(parts, string(utf16.Decode(buf[start:i])))
		start = i + 1
	}
	return parts
}
