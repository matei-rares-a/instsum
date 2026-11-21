package profiles

import (
	"path/filepath"
	"testing"
)

func TestDetectionAndTraversal(t *testing.T) {
	r := Registry{Dir: filepath.Join("..", "..", "profiles")}
	p, err := r.Detect("o rețetă bună")
	if err != nil || p.Name != "recipe" {
		t.Fatalf("detect: %v %v", p.Name, err)
	}
	p, err = r.Detect("unrelated content")
	if err != nil || p.Name != "notes" {
		t.Fatalf("fallback: %v %v", p.Name, err)
	}
	if _, err = r.Load("../recipe"); err == nil {
		t.Fatal("traversal accepted")
	}
}
