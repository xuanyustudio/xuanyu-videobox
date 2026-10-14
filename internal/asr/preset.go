package asr

import "strings"

// LocalFunASRPreset is the one-click SenseVoiceSmall / FunASR-Pack setup.
// It always selects the local backend and the default pack address.
// Callers can still edit the URL afterwards and save a manual override.
func LocalFunASRPreset() (backend, apiURL string) {
	return BackendLocal, DefaultAPIURL
}

// FormatLocalProbeStatus turns Client.Probe into a short Chinese settings line.
// A nil error means the pack responded. Dial failures keep the existing
// FunASR-Pack download hint that Probe already puts in the error text.
func FormatLocalProbeStatus(err error) string {
	if err == nil {
		return "已连接 SenseVoiceSmall（FunASR-Pack）"
	}
	msg := err.Error()
	if strings.Contains(msg, "无法连接") ||
		strings.Contains(msg, "connection refused") ||
		strings.Contains(msg, "actively refused") ||
		strings.Contains(msg, "connectex") ||
		strings.Contains(msg, "No connection could be made") {
		return "未启动。" + msg
	}
	return "未连接。" + msg
}
