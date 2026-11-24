package tts

import "testing"

func TestRatingMarks(t *testing.T) {
	if got := WithRatingMark("云希（男）· 普通话", RecRecommended); got != "云希（男）· 普通话  ★★★ 推荐" {
		t.Fatalf("recommended mark: %q", got)
	}
	if got := WithRatingMark("晓伊（女）· 普通话", RecCommon); got != "晓伊（女）· 普通话  ★★ 常用" {
		t.Fatalf("common mark: %q", got)
	}
	if got := WithRatingMark("晓北（女）· 辽宁", RecNone); got != "晓北（女）· 辽宁" {
		t.Fatalf("unmarked should stay plain: %q", got)
	}
	in := "云健（男）· 普通话  ★★★ 推荐"
	if got := StripRatingMark(in); got != "云健（男）· 普通话" {
		t.Fatalf("strip: %q", got)
	}
}

func TestRankedEdgeVoicesOrder(t *testing.T) {
	vs := RankedEdgeVoices()
	if len(vs) != len(EdgeVoices()) {
		t.Fatalf("count %d != %d", len(vs), len(EdgeVoices()))
	}
	var last int = RecRecommended + 1
	var sawRec, sawCommon, sawPlain bool
	for i, v := range vs {
		lvl := EdgeRecommendation(v.ID)
		if lvl > last {
			t.Fatalf("not sorted at %d: %s level %d after %d", i, v.ID, lvl, last)
		}
		last = lvl
		mark := RatingMark(lvl)
		if mark != "" && !containsMark(v.Label, mark) {
			t.Fatalf("label %q missing %q", v.Label, mark)
		}
		if mark == "" && (containsMark(v.Label, "★★★") || containsMark(v.Label, "★★ 常用")) {
			t.Fatalf("unrated label has mark: %q", v.Label)
		}
		switch lvl {
		case RecRecommended:
			sawRec = true
		case RecCommon:
			sawCommon = true
		default:
			sawPlain = true
		}
	}
	if !sawRec || !sawCommon || !sawPlain {
		t.Fatalf("expected all three grades, rec=%v common=%v plain=%v", sawRec, sawCommon, sawPlain)
	}
	if !IsEdgeVoice(vs[0].ID) {
		t.Fatal("first ranked should be an Edge voice")
	}
	if vs[0].ID != "zh-CN-YunxiNeural" && EdgeRecommendation(vs[0].ID) != RecRecommended {
		t.Fatalf("first should be recommended, got %s", vs[0].ID)
	}
	if id := VoiceIDForLabel(vs[0].Label); id != vs[0].ID {
		t.Fatalf("VoiceIDForLabel(%q)=%s", vs[0].Label, id)
	}
	if LabelForVoiceID("zh-CN-YunjianNeural") != WithRatingMark("云健（男）· 普通话", RecRecommended) {
		t.Fatalf("LabelForVoiceID 云健: %q", LabelForVoiceID("zh-CN-YunjianNeural"))
	}
}

func TestMiniMaxRatingsAndSort(t *testing.T) {
	if MiniMaxRecommendation("Chinese (Mandarin)_Male_Announcer") != RecRecommended {
		t.Fatal("announcer should be recommended")
	}
	if MiniMaxRecommendation("male-qn-qingse") != RecCommon {
		t.Fatal("qingse should be common")
	}
	if MiniMaxRecommendation("audiobook_male_1") != RecRecommended {
		t.Fatal("audiobook_male_1 should be recommended")
	}
	if MiniMaxRecommendation("cartoon_pig") != RecNone {
		t.Fatal("cartoon should be unmarked")
	}
	if MiniMaxVoiceLevel("male-qn-qingse", "voice_cloning") != RecNone {
		t.Fatal("cloned voices must stay unmarked")
	}
	if !LessMiniMax("Chinese (Mandarin)_Male_Announcer", "system", "male-qn-qingse", "system") {
		t.Fatal("recommended should sort before common")
	}
	if !LessMiniMax("male-qn-qingse", "system", "cartoon_pig", "system") {
		t.Fatal("common should sort before unmarked system")
	}
	if !LessMiniMax("male-qn-qingse", "system", "XuanyuABC", "voice_cloning") {
		t.Fatal("common system should sort before clones")
	}
	if !LessMiniMax("XuanyuABC", "voice_cloning", "cartoon_pig", "system") {
		t.Fatal("clones should sort before unmarked system")
	}
	if IsEdgeVoice("male-qn-qingse") {
		t.Fatal("MiniMax id must not be treated as Edge")
	}
}

func containsMark(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) > 0 && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}

func TestIndexTTS2Recommendation(t *testing.T) {
	if IndexTTS2Recommendation("indextts2_voice_05") != RecRecommended {
		t.Fatal("voice_05 should be recommended")
	}
	if IndexTTS2Recommendation("官方示例 05") != RecRecommended {
		t.Fatal("label 05")
	}
	if IndexTTS2Recommendation("indextts2_voice_03") != RecCommon {
		t.Fatal("voice_03 should be common")
	}
	if IndexTTS2Recommendation("indextts2_voice_01") != RecNone {
		t.Fatal("English example should be unmarked")
	}
	if IndexTTS2Recommendation(IndexTTS2CloneID) != RecNone {
		t.Fatal("clone should be unmarked")
	}
	list := RankedIndexTTS2Voices(nil)
	if list[0].ID != "indextts2_voice_05" {
		t.Fatalf("first should be recommended 05, got %#v", list[0])
	}
	if !containsMark(list[0].Label, "★★★ 推荐") {
		t.Fatalf("missing recommended mark: %q", list[0].Label)
	}
	if list[len(list)-1].ID != IndexTTS2CloneID {
		t.Fatalf("clone should be last: %#v", list[len(list)-1])
	}
	if containsMark(list[len(list)-1].Label, "★") {
		t.Fatalf("clone should have no mark: %q", list[len(list)-1].Label)
	}
	if IndexTTS2IDForLabel(list[0].Label) != "indextts2_voice_05" {
		t.Fatalf("round-trip marked label: %q -> %s", list[0].Label, IndexTTS2IDForLabel(list[0].Label))
	}
}
