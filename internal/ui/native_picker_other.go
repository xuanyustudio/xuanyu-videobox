//go:build fyne_gui && !windows

package ui

import (
	"errors"
	"strings"

	"github.com/sqweek/dialog"
)

func pickVideoFiles() ([]string, error) {
	path, err := dialog.File().
		Title("选择视频").
		Filter("视频文件", "mp4", "mov", "mkv", "avi", "m4v", "webm", "ts").
		Filter("所有文件", "*").
		Load()
	if isPickerCanceled(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	return []string{path}, nil
}

func pickSubtitleFiles() ([]string, error) {
	path, err := dialog.File().
		Title("选择字幕").
		Filter("字幕文件", "srt", "ass", "ssa", "vtt").
		Filter("所有文件", "*").
		Load()
	if isPickerCanceled(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	return []string{path}, nil
}

func pickAudioFiles() ([]string, error) {
	path, err := dialog.File().
		Title("选择音频").
		Filter("音频文件", "mp3", "wav", "m4a", "aac", "flac", "ogg").
		Filter("所有文件", "*").
		Load()
	if isPickerCanceled(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	return []string{path}, nil
}

func pickDirectory(title string) (string, error) {
	if title == "" {
		title = "选择文件夹"
	}
	dir, err := dialog.Directory().Title(title).Browse()
	if isPickerCanceled(err) {
		return "", nil
	}
	return strings.TrimSpace(dir), err
}

func isPickerCanceled(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "cancelled") || strings.Contains(msg, "canceled") || errors.Is(err, dialog.Cancelled)
}
