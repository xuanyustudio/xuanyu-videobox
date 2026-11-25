package script

import "testing"

func TestShiftSubtitleTimesOffsetsCues(t *testing.T) {
	in := "1\n00:00:00,290 --> 00:00:02,050\n你好\n"
	got := ShiftSubtitleTimes(in, 174720)
	if want := "1\n00:02:55,010 --> 00:02:56,770\n你好\n"; got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestShiftSubtitleTimesZeroIsNoop(t *testing.T) {
	in := "00:00:01,000 --> 00:00:02,000"
	if ShiftSubtitleTimes(in, 0) != in {
		t.Fatal("zero offset should not rewrite")
	}
}
