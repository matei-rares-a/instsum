package config

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestEnvironmentAndCLIValidationPrecedence(t *testing.T) {
	t.Setenv("IGX_VLM_ENGINE", "ollama")
	t.Setenv("IGX_OLLAMA_MODE", "disabled")
	t.Setenv("IGX_FRAMES_ANALYSIS_INTERVAL_S", "0.5")
	c, e := Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	if c.Frames.Interval != .5 {
		t.Fatal("numeric override not applied")
	}
	c.Ollama.Mode = "stub"
	if e = c.Validate(); e != nil {
		t.Fatal("CLI override should repair environment before validation:", e)
	}
}
func TestGenericOverridesAndInvalidNumbers(t *testing.T) {
	t.Setenv("IGX_ALLOW_CLOUD", "false")
	t.Setenv("IGX_LOCAL_HOSTS", "llama-server,ollama")
	c, e := Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	c.Llama.URL = "http://llama-server:8080/v1"
	if e = c.Validate(); e != nil {
		t.Fatal(e)
	}
	c.VLM.Window = math.NaN()
	if e = c.Validate(); e == nil {
		t.Fatal("invalid window accepted")
	}
}
func TestUnknownConfigKey(t *testing.T) {
	file := filepath.Join(t.TempDir(), "bad.yaml")
	if e := os.WriteFile(file, []byte("unknown_stage: true\n"), 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := Load(file); e == nil {
		t.Fatal("unknown key accepted")
	}
}
