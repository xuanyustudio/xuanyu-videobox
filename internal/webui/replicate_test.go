package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/narratoai/narrato-go/internal/config"
)

func TestReplicateRunRequiresAPIKey(t *testing.T) {
	s := &server{cfg: config.Default()}
	req := httptest.NewRequest(http.MethodPost, "/api/replicate/run", strings.NewReader(`{"video":"a.mp4"}`))
	rec := httptest.NewRecorder()
	s.handleReplicateRun(rec, req)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "API Key") {
		t.Fatalf("code=%d body=%s", rec.Code, rec.Body.String())
	}
	if s.replicating {
		t.Fatal("replicating flag should be reset")
	}
}

func TestReplicateFrameWithoutResult(t *testing.T) {
	s := &server{cfg: config.Default()}
	rec := httptest.NewRecorder()
	s.handleReplicateFrame(rec, httptest.NewRequest(http.MethodGet, "/api/replicate/frame?i=0", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("code=%d", rec.Code)
	}
}

func TestIndexHasReplicateTab(t *testing.T) {
	if !strings.Contains(indexHTML, `data-tab="replicate">爆款复刻`) || !strings.Contains(indexHTML, `id="repApiKey"`) {
		t.Fatal("index.html missing 爆款复刻 tab or settings")
	}
}
