package asr

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBailianResultToSRT(t *testing.T) {
	raw := []byte(`{
	  "transcripts":[{"sentences":[
	    {"begin_time":0,"end_time":1200,"text":"你好","speaker_id":0},
	    {"begin_time":1200,"end_time":2500,"text":"世界","speaker_id":1}
	  ]}]
	}`)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	srt, err := bailianResultToSRT(m)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(srt, "说话人1: 你好") || !strings.Contains(srt, "说话人2: 世界") {
		t.Fatalf("srt=%q", srt)
	}
}

func TestNormalizeBackend(t *testing.T) {
	if NormalizeBackend("bailian") != BackendBailian {
		t.Fatal()
	}
	if NormalizeBackend("") != BackendLocal {
		t.Fatal()
	}
}
