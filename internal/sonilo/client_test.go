package sonilo_test

import (
	"bytes"
	"encoding/base64"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/sonilo"
)

func TestConsumeNDJSONAssemblesFirstStream(t *testing.T) {
	chunkA := base64.StdEncoding.EncodeToString([]byte("AAA"))
	chunkB := base64.StdEncoding.EncodeToString([]byte("BBB"))
	chunkC := base64.StdEncoding.EncodeToString([]byte("CCC"))
	body := strings.Join([]string{
		`{"type":"title","data":"x"}`,
		`{"type":"audio_chunk","stream_index":1,"data":"` + chunkC + `"}`,
		`{"type":"audio_chunk","stream_index":0,"data":"` + chunkA + `"}`,
		`{"type":"audio_chunk","stream_index":0,"data":"` + chunkB + `"}`,
		`{"type":"complete"}`,
	}, "\n")
	got, err := sonilo.ConsumeNDJSON(bytes.NewReader([]byte(body)))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "AAABBB" {
		t.Fatalf("got %q", got)
	}
}

func TestConsumeNDJSONErrorAndIncomplete(t *testing.T) {
	_, err := sonilo.ConsumeNDJSON(bytes.NewReader([]byte(`{"type":"error","message":"nope"}`)))
	if err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("error event: %v", err)
	}
	_, err = sonilo.ConsumeNDJSON(bytes.NewReader([]byte(`{"type":"audio_chunk","stream_index":0,"data":"QQ=="}`)))
	if err == nil || !strings.Contains(err.Error(), "complete") {
		t.Fatalf("incomplete: %v", err)
	}
}

func TestDefaultBaseURL(t *testing.T) {
	if sonilo.DefaultBaseURL != "https://api.sonilo.com" {
		t.Fatalf("%s", sonilo.DefaultBaseURL)
	}
}
