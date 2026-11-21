package pipeline

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"instsum/internal/config"
)

func TestRealTesseractWithStubInference(t *testing.T) {
	for _, name := range []string{"ffmpeg", "ffprobe", "tesseract"} {
		if _, err := exec.LookPath(name); err != nil {
			if os.Getenv("IGX_REQUIRE_MEDIA_TOOLS") == "1" {
				t.Fatal(name + " required for real OCR/media validation")
			}
			t.Skip(name + " unavailable; actual OCR boundary remains unverified")
		}
	}
	for _, kind := range []string{"image", "video"} {
		t.Run(kind, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			input := filepath.Join(t.TempDir(), "input.jpg")
			args := []string{"-v", "error", "-nostdin", "-f", "lavfi", "-i", "color=c=white:s=1280x720",
				"-vf", "drawtext=text=50g:fontcolor=black:fontsize=80:x=100:y=200", "-frames:v", "1", input}
			if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("generate overlay: %v %s", err, out)
			}
			if kind == "video" {
				video := filepath.Join(filepath.Dir(input), "silent.mp4")
				args = []string{"-v", "error", "-nostdin", "-loop", "1", "-i", input, "-t", "3", "-r", "4", "-c:v", "mpeg4", "-pix_fmt", "yuv420p", video}
				if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
					t.Fatalf("generate silent video: %v %s", err, out)
				}
				input = video
			}
			c := journeyConfig(t)
			c.VLM.Engine = "ollama"
			c.Ollama.Mode = "stub"
			c.Ollama.Fixtures = filepath.Join("..", "..", "testdata", "ollama_fixtures")
			c.OCR.Engine = "tesseract"
			c.OCR.Device = "cpu"
			dir, err := Run(ctx, c, input)
			if err != nil {
				t.Fatalf("actual OCR pipeline (%s): %v", dir, err)
			}
			b, err := os.ReadFile(filepath.Join(dir, "result.json"))
			if err != nil {
				t.Fatal(err)
			}
			var result Result
			if err := json.Unmarshal(b, &result); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, segment := range result.OCR {
				found = found || strings.Contains(strings.ReplaceAll(segment.Text, " ", ""), "50g")
			}
			if !found || len(result.Transcript) != 0 || result.EngineMode != "stub" || result.Status != "complete" {
				t.Fatalf("OCR boundary failed: status=%s, mode=%s, OCR=%+v, transcript=%+v", result.Status, result.EngineMode, result.OCR, result.Transcript)
			}
		})
	}
}

func TestRealMediaToolsWithSimulatedRecognitionAndInference(t *testing.T) {
	for _, name := range []string{"ffmpeg", "ffprobe"} {
		if _, err := exec.LookPath(name); err != nil {
			t.Skip(name + " unavailable; real media boundary remains unverified")
		}
	}
	for _, audio := range []bool{false, true} {
		name := "silent"
		if audio {
			name = "audio"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
			defer cancel()
			input := filepath.Join(t.TempDir(), "input.mp4")
			args := []string{"-v", "error", "-nostdin", "-f", "lavfi", "-i", "testsrc2=size=128x96:rate=10"}
			if audio {
				args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:sample_rate=16000")
			}
			args = append(args, "-t", "2", "-c:v", "mpeg4")
			if audio {
				args = append(args, "-c:a", "aac")
			}
			args = append(args, "-n", input)
			if out, err := exec.CommandContext(ctx, "ffmpeg", args...).CombinedOutput(); err != nil {
				t.Fatalf("generate input: %v %s", err, out)
			}
			c := journeyConfig(t)
			// Real fetch/probe/audio/frame extraction, deliberately fake recognition/model.
			deps := Dependencies{ASR: testASR{}, OCR: testOCR{}, Engine: func(config.Config, string) Engine { return testEngine{} }}
			dir, err := RunWith(ctx, c, input, deps)
			if err != nil {
				t.Fatalf("media pipeline (%s): %v", dir, err)
			}
		})
	}
}
