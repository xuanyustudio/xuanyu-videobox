package script_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/narratoai/narrato-go/internal/script"
)

func TestChatCompletionsParsesContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"[{\"_id\":1,\"OST\":0,\"narration\":\"hi\",\"timestamp\":\"00:00:00,000-00:00:02,000\"}]"}}]}`))
	}))
	defer srv.Close()
	client := script.NewLLM(srv.URL, "key", "model")
	items, err := client.GenerateScript(context.Background(), script.GenerateInput{
		SubtitleText:  "测试字幕",
		WordCount:     100,
		OriginalRatio: 30,
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %#v", err, items)
	}
}

func TestChatCompletionsParsesMarkdownJSONFence(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		payload, _ := json.Marshal(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{
					"content": "```json\n[{\"_id\":2,\"OST\":1,\"narration\":\"原片\",\"timestamp\":\"00:00:05,000-00:00:08,000\"}]\n```",
				}},
			},
		})
		_, _ = w.Write(payload)
	}))
	defer srv.Close()
	client := script.NewLLM(srv.URL, "key", "model")
	items, err := client.GenerateScript(context.Background(), script.GenerateInput{
		SubtitleText:  "字幕",
		WordCount:     50,
		OriginalRatio: 20,
	})
	if err != nil || len(items) != 1 {
		t.Fatalf("%v %#v", err, items)
	}
	if items[0].ID != 2 || items[0].OST != 1 {
		t.Fatalf("%#v", items[0])
	}
}
