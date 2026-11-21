package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"instsum/internal/config"
	"instsum/internal/core"
)

type testFetch struct{ media []core.Media }

func (f testFetch) Fetch(ctx context.Context, input, dir string) (core.Metadata, []string, error) {
	var paths []string
	for _, m := range f.media {
		paths = append(paths, m.Path)
	}
	return core.Metadata{Caption: "recipe ingredients", Media: f.media}, paths, nil
}

type testMedia struct{}

func (testMedia) Probe(ctx context.Context, m core.Media, dir string) (core.Media, error) {
	m.AudioPath = "test.wav"
	return m, nil
}
func (testMedia) Extract(ctx context.Context, path, dir string) ([]core.Frame, float64, error) {
	var frames []core.Frame
	for i := 0; i < 3; i++ {
		im := image.NewRGBA(image.Rect(0, 0, 32, 32))
		for y := 0; y < 32; y++ {
			for x := 0; x < 32; x++ {
				if (x+y+i)%3 == 0 {
					im.Set(x, y, color.White)
				}
			}
		}
		p := filepath.Join(dir, fmt.Sprintf("frame-%d.jpg", i))
		f, e := os.Create(p)
		if e != nil {
			return nil, 0, e
		}
		e = jpeg.Encode(f, im, nil)
		f.Close()
		if e != nil {
			return nil, 0, e
		}
		frames = append(frames, core.Frame{Path: p, Time: float64(i), Sharpness: float64(i)})
	}
	return frames, 3, nil
}

type testASR struct{}

func (testASR) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{Start: 0, End: 2, Text: "Mix the flour"}}, nil
}

type testOCR struct{ fail bool }

func (o testOCR) Recognize(ctx context.Context, frames []core.Frame) ([]core.Segment, error) {
	if o.fail {
		return nil, fmt.Errorf("OCR model unavailable")
	}
	return []core.Segment{{Start: 0, End: 2, Text: "500 g flour"}}, nil
}

type testEngine struct{}

type retryWindowEngine struct {
	calls         int
	alwaysInvalid bool
}

func (e *retryWindowEngine) Ready(context.Context) error { return nil }
func (e *retryWindowEngine) Complete(ctx context.Context, r core.Request) (json.RawMessage, error) {
	if r.Kind == "window" {
		e.calls++
		if e.alwaysInvalid || e.calls == 1 {
			return json.Marshal(core.Window{Window: []float64{r.Window[0] + 1, r.Window[1]}, Events: []core.Event{{Time: r.Window[0], Action: "Mix", Objects: []string{}, Quantities: []string{}}}})
		}
		if !strings.Contains(r.Prompt, "model window does not match") {
			return nil, fmt.Errorf("missing repair information")
		}
	}
	return (testEngine{}).Complete(ctx, r)
}

func (testEngine) Ready(context.Context) error { return nil }
func (testEngine) Complete(ctx context.Context, r core.Request) (json.RawMessage, error) {
	if r.Kind == "window" {
		return json.Marshal(core.Window{Window: []float64{r.Window[0], r.Window[1]}, Events: []core.Event{{Time: r.Window[0], Action: "Mix", Objects: []string{"bowl"}, Quantities: []string{}, Confidence: 1}}})
	}
	var steps []any
	var ingredients []any
	for _, ev := range r.Evidence {
		if ev.Source == "ocr" {
			var stamp any = ev.Start
			if !ev.TimeKnown {
				stamp = nil
			}
			ingredients = append(ingredients, map[string]any{"name": "flour", "quantity": "500", "unit": "g", "source": "ocr", "estimated": false, "timestamp": stamp, "evidence_ids": []string{ev.ID}})
		}
		if ev.Source == "visual" {
			var start, end any = ev.Start, ev.End
			if !ev.TimeKnown {
				start = nil
				end = nil
			}
			steps = append(steps, map[string]any{"n": len(steps) + 1, "text": ev.Text, "source": ev.Source, "estimated": false, "t_start": start, "t_end": end, "evidence_ids": []string{ev.ID}, "media_id": ev.MediaID, "reference_frame": ""})
		}
	}
	if ingredients == nil {
		ingredients = []any{}
	}
	if steps == nil {
		steps = []any{}
	}
	return json.Marshal(map[string]any{"title": "Test recipe", "servings": "", "total_time": "", "ingredients": ingredients, "equipment": []string{}, "steps": steps, "tips": []string{}})
}
func journeyConfig(t *testing.T) config.Config {
	t.Helper()
	c, e := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if e != nil {
		t.Fatal(e)
	}

	c.Output.Dir = filepath.Join(t.TempDir(), "out")
	c.ProfilesDir = filepath.Join("..", "..", "profiles")
	c.PromptsDir = filepath.Join("..", "..", "prompts")
	c.Structure.Profile = "recipe"
	return c
}

func TestWindowSemanticRepairAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			c := journeyConfig(t)
			c.VLM.Retries = 1
			engine := &retryWindowEngine{alwaysInvalid: fail}
			items := []core.Media{{ID: "media-1", Path: "test.mp4", Kind: "video", Duration: 3}}
			d := Dependencies{Fetcher: testFetch{items}, Media: testMedia{}, ASR: testASR{}, OCR: testOCR{}, Engine: func(config.Config, string) Engine { return engine }}
			dir, err := RunWith(context.Background(), c, "input", d)
			if fail && !IsPartial(err) || !fail && err != nil {
				t.Fatalf("unexpected result: %v", err)
			}
			data, err := os.ReadFile(filepath.Join(dir, "result.json"))
			if err != nil {
				t.Fatal(err)
			}
			var result Result
			if err := json.Unmarshal(data, &result); err != nil {
				t.Fatal(err)
			}
			if engine.calls != 2 || fail && (len(result.FailedWindows) != 1 || len(result.Visual) != 0) || !fail && len(result.Visual) != 1 {
				t.Fatalf("calls=%d, visual=%d, failures=%d", engine.calls, len(result.Visual), len(result.FailedWindows))
			}
		})
	}
}
func TestRecipeJourneysAndPartialStages(t *testing.T) {
	for _, scenario := range []struct {
		name                     string
		carousel, silent, broken bool
	}{{"narrated", false, false, false}, {"silent-overlay", false, true, false}, {"carousel", true, true, false}, {"missing-ocr", false, false, true}} {
		t.Run(scenario.name, func(t *testing.T) {
			c := journeyConfig(t)
			items := []core.Media{{ID: "media-1", Path: "test.mp4", Kind: "video", HasAudio: !scenario.silent, Duration: 3}}
			if scenario.carousel {
				items = nil
				for i := 1; i <= 2; i++ {
					p := filepath.Join(t.TempDir(), fmt.Sprintf("slide-%d.jpg", i))
					if e := os.WriteFile(p, []byte("original image bytes"), 0600); e != nil {
						t.Fatal(e)
					}
					items = append(items, core.Media{ID: fmt.Sprintf("media-%d", i), Path: p, Kind: "image", Slide: i})
				}
			}
			d := Dependencies{Fetcher: testFetch{items}, Media: testMedia{}, ASR: testASR{}, OCR: testOCR{scenario.broken}, Engine: func(config.Config, string) Engine { return testEngine{} }}
			dir, e := RunWith(context.Background(), c, "input", d)
			if scenario.broken {
				if !IsPartial(e) {
					t.Fatalf("wanted partial: %v", e)
				}
			} else if e != nil {
				t.Fatal(e)
			}
			b, e := os.ReadFile(filepath.Join(dir, "result.json"))
			if e != nil {
				t.Fatal(e)
			}
			var r Result
			if e = json.Unmarshal(b, &r); e != nil {
				t.Fatal(e)
			}
			if r.Status == "complete" && scenario.broken {
				t.Fatal("missing OCR presented as success")
			}
			if (scenario.silent || scenario.carousel) && len(r.Transcript) > 0 {
				t.Fatal("invented transcript")
			}
			if !scenario.silent && !scenario.carousel && len(r.Transcript) == 0 {
				t.Fatal("speech lost")
			}
			if len(r.Visual) != len(items) || len(r.Notes) == 0 {
				t.Fatalf("missing media or notes %+v", r)
			}
			md, e := os.ReadFile(filepath.Join(dir, "result.md"))
			if e != nil {
				t.Fatal(e)
			}
			for i := range items {
				name := fmt.Sprintf("step_%02d.jpg", i+1)
				if _, e = os.Stat(filepath.Join(dir, name)); e != nil || !strings.Contains(string(md), name) {
					t.Fatalf("broken step reference %s: %v", name, e)
				}
			}
		})
	}
}
