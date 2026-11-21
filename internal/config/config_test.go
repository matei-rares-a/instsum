package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	c, err := Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	c.VLM.Overlap = c.VLM.Window
	if err = c.Validate(); err == nil {
		t.Fatal("overlap must be rejected")
	}
	c.VLM.Overlap = 0
	c.Llama.URL = "https://example.com/v1"
	if err = c.Validate(); err == nil || !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("remote endpoint: %v", err)
	}
}
