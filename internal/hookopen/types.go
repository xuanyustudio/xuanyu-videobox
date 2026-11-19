package hookopen

import "github.com/narratoai/narrato-go/internal/config"

const (
	MinHookSec      = 3.0
	MaxHookSec      = 8.0
	PrefHookSec     = 5.5
	DefaultXFadeSec = 0.5
)

// Cue is one timed line from speech recognition.
type Cue struct {
	Start float64
	End   float64
	Text  string
}

// Candidate is one hook opening choice.
type Candidate struct {
	Rank        int     `json:"rank"`
	HookType    string  `json:"hook_type"`
	Reason      string  `json:"reason"`
	StartSec    float64 `json:"start_sec"`
	EndSec      float64 `json:"end_sec"`
	Quote       string  `json:"quote"`
	Recommended bool    `json:"is_recommended"`
	PreviewPath string  `json:"-"`
	ThumbPath   string  `json:"-"`
}

func (c Candidate) Duration() float64 {
	if c.EndSec <= c.StartSec {
		return 0
	}
	return c.EndSec - c.StartSec
}

// Result is the analysis output shown in the UI.
type Result struct {
	VideoPath  string
	WorkDir    string
	Duration   float64
	Transcript string
	Candidates []Candidate
	UsedModel  bool
}

// Progress reports status while analyzing or rendering.
type Progress func(pct int, msg string)

// Settings used by Analyze and Render.
type Settings struct {
	Cfg *config.Config
}
