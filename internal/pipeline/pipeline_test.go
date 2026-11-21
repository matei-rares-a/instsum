package pipeline

import (
	"context"
	"encoding/json"
	"image"
	"image/jpeg"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"instsum/internal/config"
	"instsum/internal/core"
)

func TestWindowsAndFrameSelection(t *testing.T) {
	got := Windows(22, 10, 1)
	want := [][2]float64{{0, 10}, {9, 19}, {18, 22}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("windows = %v", got)
	}
	frames := []core.Frame{{Time: 0}, {Time: 1}, {Time: 2}, {Time: 3}}
	selected := choose(frames, 0, 4, 2)
	if len(selected) != 2 || selected[0].Time != 0 || selected[1].Time != 2 {
		t.Fatalf("selection: %#v", selected)
	}
}
func TestStubImageEndToEnd(t *testing.T) {
	root, err := os.MkdirTemp(".", "test-run-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	path := filepath.Join(root, "input.jpg")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = jpeg.Encode(file, image.NewRGBA(image.Rect(0, 0, 4, 4)), nil); err != nil {
		t.Fatal(err)
	}
	file.Close()
	c, err := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c.Output.Dir = filepath.Join(root, "out")
	c.ProfilesDir = filepath.Join("..", "..", "profiles")
	c.PromptsDir = filepath.Join("..", "..", "prompts")
	c.Ollama.Fixtures = filepath.Join("..", "..", "testdata", "ollama_fixtures")
	c.VLM.Engine = "ollama"
	c.Ollama.Mode = "stub"
	c.Structure.Profile = "recipe"
	dir, err := Run(context.Background(), c, path)
	if err != nil && !IsPartial(err) {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "result.json"))
	if err != nil {
		t.Fatal(err)
	}
	var r Result
	if err = json.Unmarshal(b, &r); err != nil {
		t.Fatal(err)
	}
	if r.EngineMode != "stub" || len(r.Frames) != 1 || len(r.Transcript) != 0 {
		t.Fatalf("result: %#v", r)
	}
	if len(r.Notes) == 0 || string(r.Notes) == "null" {
		t.Fatalf("stub structuring failed: %+v", r.Warnings)
	}
	md, err := os.ReadFile(filepath.Join(dir, "result.md"))
	if err != nil || !strings.Contains(string(md), "STUB OUTPUT") {
		t.Fatalf("missing stub warning: %v", err)
	}
}
