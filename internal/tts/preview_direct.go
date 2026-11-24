package tts

import (
	"fmt"
	"os"
	"strings"
)

// DirectPreviewPath returns a local audio file that can be played without
// synthesis. ok is true when the caller should play path directly. An error
// means preview cannot proceed (for example 克隆音色 with no reference).
// ok is false and err is nil when the engine still needs to synthesize.
func DirectPreviewPath(engine, voice, indexTTS2Reference string) (path string, ok bool, err error) {
	if NormalizeEngine(engine) != EngineIndexTTS2 {
		return "", false, nil
	}
	id := IndexTTS2IDForLabel(voice)
	if id == IndexTTS2CloneID {
		ref := strings.TrimSpace(indexTTS2Reference)
		if ref == "" {
			return "", false, fmt.Errorf("克隆音色需要先选择一段参考音频，才能直接预听")
		}
		st, err := os.Stat(ref)
		if err != nil {
			return "", false, fmt.Errorf("参考音频不存在，请重新选择 wav/mp3")
		}
		if st.IsDir() || st.Size() == 0 {
			return "", false, fmt.Errorf("参考音频无效或为空，请重新选择 wav/mp3")
		}
		return ref, true, nil
	}
	p := IndexTTS2ExamplePath(id)
	if p == "" {
		return "", false, fmt.Errorf("找不到官方示例音频（%s）。请确认 E:\\indextts2\\examples 下有对应的 wav", IndexTTS2LabelForID(id))
	}
	return p, true, nil
}
