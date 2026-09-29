package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/narratoai/narrato-go/internal/script"
)

const projectVersion = 1

var illegalSlugRe = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]+`)

// Export is one render/export snapshot recorded in project.json.
type Export struct {
	ID                 string `json:"id"`
	CreatedAt          string `json:"created_at"`
	TaskID             string `json:"task_id"`
	TaskDir            string `json:"task_dir"`
	CombinedMP4        string `json:"combined_mp4"`
	ScriptSnapshotPath string `json:"script_snapshot_path"`
	ScriptPathAtExport string `json:"script_path_at_export"`
	ClipCount          int    `json:"clip_count"`
	Status             string `json:"status"`
}

// projectKnownJSONKeys are top-level keys owned by Project; any other keys are preserved across Save.
var projectKnownJSONKeys = map[string]struct{}{
	"version": {}, "name": {}, "slug": {}, "created_at": {}, "updated_at": {},
	"video_paths": {}, "subtitle_paths": {}, "script_items": {}, "exports": {},
}

// Project is the on-disk project.json payload (compatible with the Python workspace).
type Project struct {
	Version       int           `json:"version"`
	Name          string        `json:"name"`
	Slug          string        `json:"slug"`
	CreatedAt     string        `json:"created_at"`
	UpdatedAt     string        `json:"updated_at"`
	VideoPaths    []string      `json:"video_paths"`
	SubtitlePaths []string      `json:"subtitle_paths"`
	ScriptItems   []script.Item `json:"script_items"`
	Exports       []Export      `json:"exports"`

	extra map[string]json.RawMessage `json:"-"`
}

// Slugify turns a display name into a safe directory slug (Windows filename rules).
func Slugify(name string) string {
	text := strings.TrimSpace(name)
	text = illegalSlugRe.ReplaceAllString(text, "_")
	text = strings.Trim(text, " ._")
	if text == "" {
		return "untitled"
	}
	return text
}

// Create makes a new project directory under root and writes project.json.
func Create(root, name string) (*Project, error) {
	slug, err := uniqueSlug(root, name)
	if err != nil {
		return nil, err
	}
	now := nowISO()
	p := &Project{
		Version:       projectVersion,
		Name:          strings.TrimSpace(name),
		Slug:          slug,
		CreatedAt:     now,
		UpdatedAt:     now,
		VideoPaths:    []string{},
		SubtitlePaths: []string{},
		ScriptItems:   []script.Item{},
		Exports:       []Export{},
	}
	if p.Name == "" {
		p.Name = slug
	}
	dir := projectDir(root, slug)
	if err := os.MkdirAll(filepath.Join(dir, "exports"), 0o755); err != nil {
		return nil, err
	}
	if err := Save(root, p); err != nil {
		return nil, err
	}
	return p, nil
}

// Load reads project.json for slug from root.
func Load(root, slug string) (*Project, error) {
	path := projectFile(root, slug)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("project not found: %s", Slugify(slug))
		}
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid project.json: %w", err)
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid project.json: %w", err)
	}
	p.extra = make(map[string]json.RawMessage)
	for k, v := range raw {
		if _, known := projectKnownJSONKeys[k]; !known {
			p.extra[k] = append(json.RawMessage(nil), v...)
		}
	}
	if p.Slug == "" {
		p.Slug = Slugify(slug)
	}
	if p.VideoPaths == nil {
		p.VideoPaths = []string{}
	}
	if p.SubtitlePaths == nil {
		p.SubtitlePaths = []string{}
	}
	if p.ScriptItems == nil {
		p.ScriptItems = []script.Item{}
	}
	if p.Exports == nil {
		p.Exports = []Export{}
	}
	return &p, nil
}

// Save writes project.json under root/slug/.
func Save(root string, p *Project) error {
	if p == nil {
		return fmt.Errorf("project is nil")
	}
	p.Slug = Slugify(p.Slug)
	if p.Slug == "" {
		p.Slug = Slugify(p.Name)
	}
	if p.Slug == "" {
		return fmt.Errorf("project slug is empty")
	}
	p.UpdatedAt = nowISO()
	if p.VideoPaths == nil {
		p.VideoPaths = []string{}
	}
	if p.SubtitlePaths == nil {
		p.SubtitlePaths = []string{}
	}
	if p.ScriptItems == nil {
		p.ScriptItems = []script.Item{}
	}
	if p.Exports == nil {
		p.Exports = []Export{}
	}
	path := projectFile(root, p.Slug)
	data, err := marshalProjectJSON(p)
	if err != nil {
		return err
	}
	return atomicWriteBytes(path, data)
}

// Delete removes the project directory under root.
func Delete(root, slug string) error {
	slug = Slugify(slug)
	if slug == "" {
		return fmt.Errorf("project slug is empty")
	}
	dir := projectDir(root, slug)
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("project not found: %s", slug)
		}
		return err
	}
	return os.RemoveAll(dir)
}

// ImportFromDir copies (or opens) a folder that contains project.json into root.
func ImportFromDir(root, srcDir string) (*Project, error) {
	srcDir = filepath.Clean(srcDir)
	if _, err := os.Stat(filepath.Join(srcDir, "project.json")); err != nil {
		return nil, fmt.Errorf("folder is not a project (missing project.json)")
	}
	rel, err := filepath.Rel(filepath.Clean(root), srcDir)
	if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
		return Load(root, filepath.Base(srcDir))
	}
	data, err := os.ReadFile(filepath.Join(srcDir, "project.json"))
	if err != nil {
		return nil, err
	}
	var p Project
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("invalid project.json: %w", err)
	}
	name := p.Name
	if name == "" {
		name = filepath.Base(srcDir)
	}
	slug, err := uniqueSlug(root, name)
	if err != nil {
		return nil, err
	}
	dst := projectDir(root, slug)
	if err := copyDir(srcDir, dst); err != nil {
		return nil, err
	}
	p2, err := Load(root, slug)
	if err != nil {
		return nil, err
	}
	p2.Name = name
	p2.Slug = slug
	if err := Save(root, p2); err != nil {
		return nil, err
	}
	return p2, nil
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

// List returns all projects under root that contain project.json, newest first.
func List(root string) ([]Project, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []Project{}, nil
		}
		return nil, err
	}
	var out []Project
	for _, ent := range entries {
		if !ent.IsDir() {
			continue
		}
		path := filepath.Join(root, ent.Name(), "project.json")
		if _, err := os.Stat(path); err != nil {
			continue
		}
		p, err := Load(root, ent.Name())
		if err != nil {
			continue
		}
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt > out[j].UpdatedAt
	})
	return out, nil
}

// AddExport appends an export snapshot, writes script.snapshot.json, and saves the project.
func AddExport(root string, p *Project, taskID string, scriptItems []script.Item, scriptPathAtExport string) error {
	if p == nil {
		return fmt.Errorf("project is nil")
	}
	slug := Slugify(p.Slug)
	if slug == "" {
		slug = Slugify(p.Name)
	}
	exportsRoot := filepath.Join(projectDir(root, slug), "exports")
	if err := os.MkdirAll(exportsRoot, 0o755); err != nil {
		return err
	}
	exportID := uniqueExportID(exportsRoot)
	exportDir := filepath.Join(exportsRoot, exportID)
	if err := os.MkdirAll(exportDir, 0o755); err != nil {
		return err
	}
	snapshotRel := filepath.ToSlash(filepath.Join("exports", exportID, "script.snapshot.json"))
	snapshotAbs := filepath.Join(projectDir(root, slug), filepath.FromSlash(snapshotRel))
	if err := atomicWriteJSON(snapshotAbs, scriptItems); err != nil {
		return err
	}
	taskDir := filepath.Join(root, slug, "tasks", taskID)
	combined := filepath.Join(taskDir, "combined.mp4")
	items := append([]script.Item(nil), scriptItems...)
	exp := Export{
		ID:                 exportID,
		CreatedAt:          nowISO(),
		TaskID:             taskID,
		TaskDir:            taskDir,
		CombinedMP4:        combined,
		ScriptSnapshotPath: snapshotRel,
		ScriptPathAtExport: scriptPathAtExport,
		ClipCount:          len(items),
		Status:             "running",
	}
	p.Exports = append(p.Exports, exp)
	p.ScriptItems = items
	idRaw, err := json.Marshal(exportID)
	if err != nil {
		return err
	}
	if p.extra == nil {
		p.extra = make(map[string]json.RawMessage)
	}
	p.extra["active_export_id"] = idRaw
	return Save(root, p)
}

func projectDir(root, slug string) string {
	return filepath.Join(root, Slugify(slug))
}

// Dir returns the on-disk project directory under root.
func Dir(root, slug string) string {
	return projectDir(root, slug)
}

func projectFile(root, slug string) string {
	return filepath.Join(projectDir(root, slug), "project.json")
}

func uniqueSlug(root, name string) (string, error) {
	base := Slugify(name)
	candidate := base
	index := 2
	for {
		dir := projectDir(root, candidate)
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			return candidate, nil
		} else if err != nil {
			return "", err
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func uniqueExportID(exportsRoot string) string {
	base := time.Now().Format("20060102-150405")
	candidate := base
	index := 2
	for {
		path := filepath.Join(exportsRoot, candidate)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return candidate
		}
		candidate = fmt.Sprintf("%s-%d", base, index)
		index++
	}
}

func nowISO() string {
	return time.Now().Format("2006-01-02T15:04:05-07:00")
}

func marshalProjectJSON(p *Project) ([]byte, error) {
	merged := make(map[string]json.RawMessage)
	for k, v := range p.extra {
		merged[k] = append(json.RawMessage(nil), v...)
	}
	known, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	var knownMap map[string]json.RawMessage
	if err := json.Unmarshal(known, &knownMap); err != nil {
		return nil, err
	}
	for k, v := range knownMap {
		merged[k] = v
	}
	return json.MarshalIndent(merged, "", "  ")
}

func atomicWriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicWriteBytes(path, data)
}

func atomicWriteBytes(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(temp, path)
}
