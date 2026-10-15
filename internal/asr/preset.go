package asr

import "strings"

// EnsureFunASRURL returns the FunASR-Pack address for SenseVoiceSmall.
// An empty field gets the default; a manual URL is kept.
func EnsureFunASRURL(current string) string {
	if strings.TrimSpace(current) == "" {
		return DefaultAPIURL
	}
	return strings.TrimSpace(current)
}

// EnsureFireRedURL returns the FireRed address, using the default when empty.
func EnsureFireRedURL(current string) string {
	if strings.TrimSpace(current) == "" {
		return DefaultFireRedAPIURL
	}
	return strings.TrimSpace(current)
}
