package asr

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestResultToSRTFromSegments(t *testing.T) {
	raw := []byte(`{"text":"你好世界","segments":[{"start":0.0,"end":1.2,"text":"你好"},{"start":1.2,"end":2.0,"text":"世界"}]}`)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	srt, err := resultToSRT(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(srt, "你好") || !strings.Contains(srt, "-->") {
		t.Fatalf("bad srt: %q", srt)
	}
}

func TestBaseURLNormalize(t *testing.T) {
	c := &Client{APIURL: "127.0.0.1:7860/v1"}
	if got := c.baseURL(); got != "http://127.0.0.1:7860" {
		t.Fatalf("got %q", got)
	}
}
