package voiceclone

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

var voiceIDRe = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{6,254}[A-Za-z0-9]$`)

// NewVoiceID returns a unique voice_id that satisfies MiniMax rules.
func NewVoiceID() string {
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("Xuanyu%X", 0xA1B2C3D4)
	}
	return "Xuanyu" + strings.ToUpper(hex.EncodeToString(b[:]))
}

// NormalizeVoiceID trims space and, if empty, generates a new id.
func NormalizeVoiceID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return NewVoiceID()
	}
	return id
}

// ValidateVoiceID checks MiniMax custom voice_id rules.
func ValidateVoiceID(id string) error {
	id = strings.TrimSpace(id)
	n := utf8.RuneCountInString(id)
	if n < 8 || n > 256 {
		return fmt.Errorf("音色 ID 长度需在 8 到 256 个字符之间")
	}
	if !voiceIDRe.MatchString(id) {
		return fmt.Errorf("音色 ID 需以英文字母开头，只能含字母、数字、连字符或下划线，且不能以连字符或下划线结尾")
	}
	return nil
}
