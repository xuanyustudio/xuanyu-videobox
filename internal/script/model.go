package script

// Item is one row in a Narrato script JSON array.
type Item struct {
	ID              int      `json:"_id"`
	VideoID         int      `json:"video_id"`
	VideoName       string   `json:"video_name"`
	Timestamp       string   `json:"timestamp"`
	Picture         string   `json:"picture"`
	Narration       string   `json:"narration"`
	OST             int      `json:"OST"`
	HookSourceID    *int     `json:"hook_source_id"`
	Duration        *float64 `json:"duration"`
	EditedTimeRange string   `json:"editedTimeRange"`
}
