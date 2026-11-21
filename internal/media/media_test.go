package media

import (
	"context"
	"instsum/internal/config"
	"instsum/internal/core"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestLocalVideoNormalizationAndPTS(t *testing.T) {
	for _, binary := range []string{"ffmpeg", "ffprobe"} {
		if _, e := exec.LookPath(binary); e != nil {
			if os.Getenv("IGX_REQUIRE_MEDIA_TOOLS") == "1" {
				t.Fatal(binary + " required for real-media validation but unavailable")
			}
			t.Skip(binary + " unavailable; actual media-tool boundary unverified")
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, "silent.mp4")
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=128x96:rate=10", "-t", "2", "-c:v", "mpeg4", "-n", path)
	if b, e := cmd.CombinedOutput(); e != nil {
		t.Fatalf("make local video: %v %s", e, b)
	}
	c := config.Defaults()
	tools := Tools{Config: c}
	m, e := tools.Probe(ctx, core.Media{Kind: "video", Path: path}, dir)
	if e != nil {
		t.Fatal(e)
	}
	if m.HasAudio || m.Width != 128 || m.Height != 96 || m.Duration < 1.9 {
		t.Fatalf("probe %+v", m)
	}
	frames, duration, e := tools.Extract(ctx, path, filepath.Join(dir, "frames"))
	if e != nil {
		t.Fatal(e)
	}
	if len(frames) < 2 || frames[0].Time != 0 || frames[1].Time <= 0 || duration < 1.9 {
		t.Fatalf("PTS %+v duration %f", frames, duration)
	}
}
