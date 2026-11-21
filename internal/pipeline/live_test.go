package pipeline

import (
	"context"
	"instsum/internal/config"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLiveRecipeOptIn(t *testing.T) {
	if os.Getenv("IGX_RUN_LIVE") != "1" {
		t.Skip("real models/hardware intentionally deferred; set IGX_RUN_LIVE=1 and IGX_LIVE_INPUT")
	}
	input := os.Getenv("IGX_LIVE_INPUT")
	if input == "" {
		t.Fatal("IGX_LIVE_INPUT must point to local media or manifest")
	}
	c, e := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if e != nil {
		t.Fatal(e)
	}
	if c.Ollama.AutoPull {
		t.Fatal("live test must not implicitly download models")
	}
	if c.VLM.Engine == "ollama" && c.Ollama.Mode != "live" {
		t.Fatal("live test requires ollama.mode live")
	}
	c.ProfilesDir = filepath.Join("..", "..", "profiles")
	c.PromptsDir = filepath.Join("..", "..", "prompts")
	c.Output.Dir = t.TempDir()
	c.GPU.Monitor = true
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	dir, e := Run(ctx, c, input)
	if e != nil {
		t.Fatalf("real recipe failed or incomplete (%s): %v", dir, e)
	}
}
