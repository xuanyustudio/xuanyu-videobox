package script

// Item is one row in a Narrato script JSON array.
type Item struct {
	ID              int      `json:"_id"`
	VideoID         int      `json:"video_id"`
	VideoIndex      int      `json:"video_index,omitempty"` // alias used by some scripts when video_id is empty
	VideoName       string   `json:"video_name"`
	SourceVideo     string   `json:"source_video,omitempty"` // file name or path naming the episode
	Timestamp       string   `json:"timestamp"`
	Picture         string   `json:"picture"`
	Narration       string   `json:"narration"`
	OST             int      `json:"OST"`
	HookSourceID    *int     `json:"hook_source_id"`
	Duration        *float64 `json:"duration"`
	EditedTimeRange string   `json:"editedTimeRange"`
}
