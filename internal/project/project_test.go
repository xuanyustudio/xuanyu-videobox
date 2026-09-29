package project_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/narratoai/narrato-go/internal/project"
	"github.com/narratoai/narrato-go/internal/script"
)

func TestCreateLoadList(t *testing.T) {
	root := t.TempDir()
	p, err := project.Create(root, "测试工程")
	if err != nil {
		t.Fatal(err)
	}
	if p.Slug == "" || p.Name == "" {
		t.Fatal(p)
	}
	loaded, err := project.Load(root, p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	loaded.VideoPaths = []string{`E:\a.mp4`}
	if err := project.Save(root, loaded); err != nil {
		t.Fatal(err)
	}
	list, err := project.List(root)
	if err != nil || len(list) != 1 {
		t.Fatalf("%v %v", list, err)
	}
}

func TestAddExport(t *testing.T) {
	root := t.TempDir()
	p, err := project.Create(root, "export-test")
	if err != nil {
		t.Fatal(err)
	}
	items := []script.Item{{ID: 1, Narration: "hello"}}
	if err := project.AddExport(root, p, "task-1", items, ""); err != nil {
		t.Fatal(err)
	}
	if len(p.Exports) != 1 || p.Exports[0].ID == "" {
		t.Fatalf("%+v", p.Exports)
	}
	snap := filepath.Join(root, p.Slug, filepath.FromSlash(p.Exports[0].ScriptSnapshotPath))
	if _, err := os.Stat(snap); err != nil {
		t.Fatal(err)
	}
	loaded, err := project.Load(root, p.Slug)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Exports) != 1 {
		t.Fatalf("%+v", loaded.Exports)
	}
}

func TestSlugifyStripsIllegalChars(t *testing.T) {
	got := project.Slugify(`bad<>:"/\|?*name`)
	if got != "bad_name" {
		t.Fatalf("got %q", got)
	}
}

func TestSavePreservesUnknownJSONKeys(t *testing.T) {
	root := t.TempDir()
	slug := "fake"
	dir := filepath.Join(root, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fake := `{
  "version": 1,
  "name": "fake",
  "slug": "fake",
  "created_at": "2020-01-01T00:00:00+08:00",
  "updated_at": "2020-01-01T00:00:00+08:00",
  "mode": "xuanyu",
  "ui_state": {"x": 1},
  "video_paths": [],
  "subtitle_paths": [],
  "script_items": [],
  "exports": []
}`
	path := filepath.Join(dir, "project.json")
	if err := os.WriteFile(path, []byte(fake), 0o644); err != nil {
		t.Fatal(err)
	}

	p, err := project.Load(root, slug)
	if err != nil {
		t.Fatal(err)
	}
	p.VideoPaths = []string{`E:\a.mp4`}
	if err := project.Save(root, p); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["mode"]; !ok {
		t.Fatalf("mode missing after save: %s", raw)
	}
	if string(doc["mode"]) != `"xuanyu"` {
		t.Fatalf("mode=%s", doc["mode"])
	}
	uiState, ok := doc["ui_state"]
	if !ok {
		t.Fatalf("ui_state missing after save: %s", raw)
	}
	var ui map[string]int
	if err := json.Unmarshal(uiState, &ui); err != nil || ui["x"] != 1 {
		t.Fatalf("ui_state=%s err=%v", uiState, err)
	}
	var paths []string
	if err := json.Unmarshal(doc["video_paths"], &paths); err != nil || len(paths) != 1 || paths[0] != `E:\a.mp4` {
		t.Fatalf("video_paths=%s err=%v", doc["video_paths"], err)
	}
}
