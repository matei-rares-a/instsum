package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"
	"instsum/internal/asr"
	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/fetch"
	"instsum/internal/gpu"
	"instsum/internal/llamaserver"
	"instsum/internal/media"
	"instsum/internal/ocr"
	"instsum/internal/output"
	"instsum/internal/profiles"
	"instsum/internal/structure"
	"instsum/internal/vlm"
)

var windowSchema = json.RawMessage(`{"type":"object","required":["window","events"],"properties":{"window":{"type":"array","minItems":2,"maxItems":2,"items":{"type":"number"}},"events":{"type":"array","items":{"type":"object","required":["t","action","objects","quantities","on_screen_text","estimated","confidence"],"properties":{"t":{"type":"number","minimum":0},"action":{"type":"string"},"objects":{"type":"array","items":{"type":"string"}},"quantities":{"type":"array","items":{"type":"string"}},"on_screen_text":{"type":"string"},"estimated":{"type":"boolean"},"confidence":{"type":"number","minimum":0,"maximum":1}}}}}}`)

type Result = output.Result
type PartialError struct{ Dir string }

func (e *PartialError) Error() string { return "incomplete result exported to " + e.Dir }

type Engine interface {
	Ready(context.Context) error
	Complete(context.Context, core.Request) (json.RawMessage, error)
}
type Processor interface {
	Probe(context.Context, core.Media, string) (core.Media, error)
	Extract(context.Context, string, string) ([]core.Frame, float64, error)
}
type Dependencies struct {
	Fetcher core.Fetcher
	Media   Processor
	ASR     core.Transcriber
	OCR     core.OCREngine
	Engine  func(config.Config, string) Engine
}

func defaults(c config.Config, d Dependencies) Dependencies {
	if d.Fetcher == nil {
		d.Fetcher = fetch.New(c)
	}
	if d.Media == nil {
		d.Media = media.Tools{Config: c}
	}
	if d.ASR == nil {
		d.ASR = asr.New(c)
	}
	if d.OCR == nil {
		d.OCR = ocr.New(c)
	}
	if d.Engine == nil {
		d.Engine = func(c config.Config, e string) Engine { return vlm.New(c, e) }
	}
	return d
}
func Windows(duration, size, overlap float64) [][2]float64 {
	if duration <= 0 || size <= 0 || overlap < 0 || overlap >= size {
		return nil
	}
	var result [][2]float64
	for start := 0.0; start < duration; start += size - overlap {
		end := start + size
		if end > duration {
			end = duration
		}
		result = append(result, [2]float64{start, end})
		if end == duration {
			break
		}
	}
	return result
}
func choose(frames []core.Frame, start, end float64, limit int) []core.Frame {
	return media.Select(frames, start, end, limit)
}
func Run(ctx context.Context, c config.Config, input string) (string, error) {
	return RunWith(ctx, c, input, Dependencies{})
}
func RunWith(ctx context.Context, c config.Config, input string, d Dependencies) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	d = defaults(c, d)
	started := time.Now()
	if err := os.MkdirAll(c.Output.Dir, 0755); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(c.Output.Dir, "run-")
	if err != nil {
		return "", err
	}
	r := Result{Status: "complete", Engine: c.VLM.Engine, EngineMode: "live", Model: c.VLM.Model, TextModel: c.Structure.TextModel,
		Warnings: []string{}, Stages: []core.Stage{}, Transcript: []core.Segment{}, OCR: []core.Segment{}, Visual: []core.Window{}, Timeline: []core.Segment{}, Frames: []core.Frame{}}
	if r.TextModel == "" || r.TextModel == "same_as_vlm" {
		r.TextModel = c.VLM.Model
	}
	if c.VLM.Engine == "ollama" && c.Ollama.Mode == "stub" {
		r.EngineMode = "stub"
		r.Warnings = append(r.Warnings, "STUB: model output is simulated and must not be used as factual extraction.")
	}
	record := func(name, id, engine, status string, at time.Time, e error) {
		s := core.Stage{Name: name, MediaID: id, Engine: engine, Status: status, Duration: time.Since(at).Seconds()}
		if status == "simulated" {
			s.Mode = "stub"
		}
		if e != nil {
			s.Reason = e.Error()
		}
		if status == "failed" || status == "unavailable" || status == "skipped" {
			r.Status = "partial"
			if s.Reason == "" {
				s.Reason = "stage not executed"
			}
			r.Warnings = append(r.Warnings, fmt.Sprintf("%s/%s: %s", id, name, s.Reason))
			slog.Warn("partial pipeline stage", "stage", name, "media", id, "reason", s.Reason)
		}
		r.Stages = append(r.Stages, s)
	}
	monitor := &gpu.Monitor{Device: c.GPU.Device, BudgetMB: float64(c.GPU.Budget)}
	guard := func(fn func(context.Context) error) error {
		if c.GPU.Monitor {
			return monitor.Run(ctx, fn)
		}
		return fn(ctx)
	}
	at := time.Now()
	meta, paths, err := d.Fetcher.Fetch(ctx, input, dir)
	if err != nil {
		return dir, fmt.Errorf("fetch: %w", err)
	}
	if len(paths) == 0 {
		return dir, fmt.Errorf("fetch returned no media")
	}
	if len(meta.Media) != len(paths) {
		meta.Media = nil
		for i, path := range paths {
			slide := 0
			if len(paths) > 1 {
				slide = i + 1
			}
			meta.Media = append(meta.Media, core.Media{ID: fmt.Sprintf("media-%d", i+1), Path: path, Kind: fetch.Kind(path), Slide: slide})
		}
	}
	record("fetch", "", "external", "complete", at, nil)
	r.Metadata = meta
	reg := profiles.Registry{Dir: c.ProfilesDir}
	p, err := reg.Load(c.Structure.Profile)
	if c.Structure.Profile == "auto" {
		p, err = reg.Detect(meta.Caption + " " + meta.Title + " " + strings.Join(meta.Hashtags, " "))
	}
	if err != nil {
		return dir, err
	}
	r.Profile = p.Name
	var allFrames []core.Frame
	type work struct {
		media  core.Media
		frames []core.Frame
	}
	var works []work
	normalizedCount := 0
	for i, m := range meta.Media {
		if err := ctx.Err(); err != nil {
			return dir, err
		}
		mediaDir := filepath.Join(dir, m.ID)
		if err = os.MkdirAll(mediaDir, 0755); err != nil {
			return dir, err
		}
		at = time.Now()
		normalized, e := d.Media.Probe(ctx, m, mediaDir)
		if e != nil {
			record("normalize", m.ID, "ffmpeg", "failed", at, e)
			continue
		}
		m = normalized
		normalizedCount++
		meta.Media[i] = m
		r.Duration += m.Duration
		record("normalize", m.ID, "ffmpeg", "complete", at, nil)
		if m.Kind == "image" {
			name := "original_" + m.ID + strings.ToLower(filepath.Ext(m.Path))
			if e = output.Copy(m.Path, filepath.Join(dir, name)); e != nil {
				return dir, e
			}
			meta.Media[i].Path = name
		}
		at = time.Now()
		if m.Kind == "image" {
			record("asr", m.ID, c.ASR.Engine, "not_applicable", at, nil)
		} else if !m.HasAudio {
			record("asr", m.ID, c.ASR.Engine, "no_audio", at, nil)
		} else {
			var segments []core.Segment
			e = guard(func(stageCtx context.Context) error {
				var e error
				segments, e = d.ASR.Transcribe(stageCtx, m.AudioPath)
				return e
			})
			if e != nil {
				record("asr", m.ID, c.ASR.Engine, "unavailable", at, e)
			} else {
				assign(segments, "speech", m)
				r.Transcript = append(r.Transcript, segments...)
				status := "complete"
				if len(segments) == 0 {
					status = "no_speech"
				}
				record("asr", m.ID, c.ASR.Engine, status, at, nil)
			}
		}
		at = time.Now()
		frames, _, e := d.Media.Extract(ctx, m.Path, mediaDir)
		if e != nil {
			record("frames", m.ID, "ffmpeg", "failed", at, e)
			continue
		}
		for i := range frames {
			frames[i].MediaID = m.ID
			frames[i].Slide = m.Slide
		}
		frames, e = media.Dedup(frames, c.Frames.DedupHamming, 2*c.Frames.Interval)
		if e != nil {
			record("frames", m.ID, "goimagehash", "failed", at, e)
			continue
		}
		record("frames", m.ID, "ffmpeg+goimagehash", "complete", at, nil)
		at = time.Now()
		var segments []core.Segment
		e = guard(func(stageCtx context.Context) error {
			var e error
			segments, e = d.OCR.Recognize(stageCtx, frames)
			return e
		})
		if e != nil {
			record("ocr", m.ID, c.OCR.Engine, "unavailable", at, e)
		} else {
			assign(segments, "ocr", m)
			r.OCR = append(r.OCR, segments...)
			record("ocr", m.ID, c.OCR.Engine, "complete", at, nil)
		}
		allFrames = append(allFrames, frames...)
		works = append(works, work{m, frames})
	}
	if err = ctx.Err(); err != nil {
		return dir, err
	}
	r.Metadata = meta
	windowPrompt, err := os.ReadFile(filepath.Join(c.PromptsDir, "window.tmpl"))
	if err != nil {
		return dir, fmt.Errorf("load prompts/window.tmpl: %w", err)
	}
	var managed *llamaserver.Server
	defer func() {
		if managed != nil {
			if e := managed.Close(); e != nil {
				slog.Error("owned llama-server cleanup", "error", e)
			}
		}
	}()
	engine := d.Engine(c, r.Engine)
	at = time.Now()
	ready := func(stageCtx context.Context) error {
		if r.Engine == "llamacpp" && c.Llama.Manage {
			var e error
			managed, e = llamaserver.Start(stageCtx, c, c.VLM.Model, dir)
			if e != nil {
				return e
			}
		}
		return engine.Ready(stageCtx)
	}
	err = guard(ready)
	if err != nil && c.VLM.Fallback != "" && c.VLM.Fallback != r.Engine {
		record("primary_readiness", "", r.Engine, "failed", at, err)
		r.Engine = c.VLM.Fallback
		r.EngineMode = "live"
		engine = d.Engine(c, r.Engine)
		at = time.Now()
		err = guard(ready)
	}
	canInfer := err == nil
	if !canInfer {
		record("inference_readiness", "", r.Engine, "unavailable", at, err)
	} else {
		record("inference_readiness", "", r.Engine, "complete", at, nil)
	}
	if canInfer {
		type job struct {
			w      [2]float64
			media  core.Media
			frames []core.Frame
		}
		var jobs []job
		for _, work := range works {
			windows := Windows(work.media.Duration, c.VLM.Window, c.VLM.Overlap)
			if work.media.Kind == "image" {
				windows = [][2]float64{{0, 0}}
			}
			for _, w := range windows {
				frames := choose(work.frames, w[0], w[1], c.VLM.Frames)
				if len(frames) == 0 {
					record("vlm", work.media.ID, r.Engine, "skipped", time.Now(), fmt.Errorf("no frames in window %.3f-%.3f", w[0], w[1]))
					continue
				}
				jobs = append(jobs, job{w, work.media, frames})
			}
		}
		// Native/GPU inference stays sequential; bounded jobs still propagate cancellation.
		g, gctx := errgroup.WithContext(ctx)
		g.SetLimit(1)
		for _, j := range jobs {
			j := j
			g.Go(func() error {
				at := time.Now()
				data := struct {
					MediaID         string
					Slide           int
					Window          [2]float64
					Caption         string
					Instructions    string
					Transcript, OCR []core.Segment
				}{j.media.ID, j.media.Slide, j.w, meta.Caption, p.VisualInstructions, structure.ForWindow(r.Transcript, j.media.ID, j.w[0], j.w[1]), structure.ForWindow(r.OCR, j.media.ID, j.w[0], j.w[1])}
				prompt, e := structure.Render("window", string(windowPrompt), data)
				if e != nil {
					return e
				}
				var b json.RawMessage
				var w core.Window
				for attempt := 0; attempt <= c.VLM.Retries; attempt++ {
					e = guard(func(stageCtx context.Context) error {
						var e error
						b, e = engine.Complete(stageCtx, core.Request{Prompt: prompt, Images: j.frames, Schema: windowSchema, Kind: "window", Profile: p.Name, Window: j.w, MediaID: j.media.ID, Slide: j.media.Slide})
						return e
					})
					if e != nil {
						break
					}
					w = core.Window{}
					e = json.Unmarshal(b, &w)
					if e == nil {
						e = structure.ValidateWindow(w, j.w[0], j.w[1])
					}
					if e == nil {
						break
					}
					prompt += "\nCorrect the JSON without inventing evidence: " + e.Error()
				}
				if e != nil {
					r.FailedWindows = append(r.FailedWindows, output.WindowFailure{MediaID: j.media.ID, Window: j.w, Output: string(b), Error: e.Error()})
					record("vlm", j.media.ID, r.Engine, "failed", at, e)
					return gctx.Err()
				}
				w.MediaID = j.media.ID
				w.Slide = j.media.Slide
				w.TimeKnown = j.media.Kind == "video"
				r.Visual = append(r.Visual, w)
				status := "complete"
				if r.EngineMode == "stub" {
					status = "simulated"
				}
				record("vlm", j.media.ID, r.Engine, status, at, nil)
				return nil
			})
		}
		if e := g.Wait(); e != nil {
			return dir, e
		}
		if r.Engine == "ollama" && c.Ollama.UnloadAfterUse && r.TextModel != c.VLM.Model {
			if unloader, ok := engine.(interface {
				Unload(context.Context, string) error
			}); ok {
				at = time.Now()
				e := unloader.Unload(ctx, c.VLM.Model)
				status := "complete"
				if e != nil {
					status = "failed"
				}
				record("vision_unload", "", r.Engine, status, at, e)
			}
		}
	}
	r.Timeline = structure.Timeline(meta, r.Transcript, r.OCR, r.Visual)
	if c.Structure.Profile == "auto" {
		var text strings.Builder
		text.WriteString(meta.Caption)
		text.WriteString(" " + strings.Join(meta.Hashtags, " "))
		for _, ev := range r.Timeline {
			text.WriteString(" " + ev.Text)
		}
		p, err = reg.Detect(text.String())
		if err != nil {
			return dir, err
		}
		r.Profile = p.Name
	}
	if canInfer && len(r.Timeline) > 0 {
		at = time.Now()
		if managed != nil && r.TextModel != c.VLM.Model {
			e := managed.Close()
			managed = nil
			if e == nil {
				managed, e = llamaserver.Start(ctx, c, r.TextModel, dir)
			}
			if e != nil {
				record("text_model_readiness", "", r.Engine, "failed", at, e)
				canInfer = false
			}
		}
		data := struct {
			Metadata                  core.Metadata
			Visual                    []core.Window
			Transcript, OCR, Timeline []core.Segment
		}{meta, r.Visual, r.Transcript, r.OCR, r.Timeline}
		prompt, e := structure.Render("extract", p.ExtractTemplate, data)
		if e != nil {
			return dir, e
		}
		for attempt := 0; canInfer && attempt <= c.VLM.Retries; attempt++ {
			e = guard(func(stageCtx context.Context) error {
				var e error
				r.Notes, e = engine.Complete(stageCtx, core.Request{Prompt: prompt, Schema: p.Schema, Kind: "structure", Profile: p.Name, Model: r.TextModel, Evidence: r.Timeline})
				return e
			})
			if e == nil {
				e = structure.ValidateNotes(r.Notes, r.Timeline, p.UnitAliases)
			}
			if e == nil {
				break
			}
			r.RawOutput = string(r.Notes)
			r.Notes = nil
			prompt += "\nRepair semantic evidence errors without inventing evidence: " + e.Error()
		}
		if !canInfer {
			record("structure", "", r.Engine, "skipped", at, fmt.Errorf("text model unavailable"))
			r.Notes = nil
		} else if e != nil {
			record("structure", "", r.Engine, "failed", at, e)
			r.Notes = nil
		} else {
			status := "complete"
			if r.EngineMode == "stub" {
				status = "simulated"
			}
			record("structure", "", r.Engine, status, at, nil)
		}
	} else {
		record("structure", "", r.Engine, "skipped", time.Now(), fmt.Errorf("no available model or supporting evidence"))
	}
	if err = ctx.Err(); err != nil {
		return dir, err
	}
	if managed != nil {
		at = time.Now()
		e := managed.Close()
		managed = nil
		status := "complete"
		if e != nil {
			status = "failed"
		}
		record("server_shutdown", "", r.Engine, status, at, e)
	}
	if r.Engine == "ollama" && c.Ollama.UnloadAfterUse && canInfer {
		if unloader, ok := engine.(interface {
			Unload(context.Context, string) error
		}); ok {
			at = time.Now()
			e := unloader.Unload(ctx, r.TextModel)
			status := "complete"
			if e != nil {
				status = "failed"
			}
			record("text_unload", "", r.Engine, status, at, e)
		}
	}
	if len(r.Notes) > 0 {
		r.Notes, err = structure.NormalizeUnits(r.Notes, p.UnitAliases)
		if err != nil {
			return dir, err
		}
		var warnings []string
		r.Notes, warnings, err = output.StepImages(r.Notes, p.StepField, allFrames, r.Timeline, dir)
		if err != nil {
			return dir, err
		}
		if len(warnings) > 0 {
			r.Status = "partial"
			r.Warnings = append(r.Warnings, warnings...)
		}
	}
	selected, err := media.Diverse(allFrames, c.Output.MaxReferenceFrames, c.Frames.DedupHamming)
	if err != nil {
		return dir, err
	}
	for i, f := range selected {
		name := fmt.Sprintf("reference_%02d.jpg", i+1)
		if err = output.Copy(f.Path, filepath.Join(dir, name)); err != nil {
			return dir, err
		}
		f.Path = name
		r.Frames = append(r.Frames, f)
	}
	if normalizedCount == 0 {
		r.Status = "failed"
		r.Warnings = append(r.Warnings, "No media could be normalized and sampled.")
	}
	if !c.GPU.Monitor {
		r.Warnings = append(r.Warnings, "GPU memory was not monitored; hardware budget is unverified.")
	}
	if c.GPU.Monitor {
		peak := monitor.Peak()
		r.PeakVRAM = &peak
	}
	r.Elapsed = time.Since(started).Seconds()
	if err = output.Write(dir, r, p); err != nil {
		return dir, err
	}
	if r.Status == "failed" {
		return dir, fmt.Errorf("no media could be processed; diagnostics exported to %s", dir)
	}
	if r.Status != "complete" {
		return dir, &PartialError{Dir: dir}
	}
	return dir, nil
}
func assign(segments []core.Segment, source string, m core.Media) {
	for i := range segments {
		segments[i].ID = fmt.Sprintf("%s-%s-%d", source, m.ID, i)
		segments[i].Source = source
		segments[i].MediaID = m.ID
		segments[i].Slide = m.Slide
		segments[i].TimeKnown = m.Kind == "video"
	}
}
func referenceFrames(frames []core.Frame, limit int) []core.Frame {
	if len(frames) <= limit {
		return frames
	}
	var out []core.Frame
	for i := 0; i < limit; i++ {
		out = append(out, frames[i*len(frames)/limit])
	}
	return out
}
func IsPartial(err error) bool { var p *PartialError; return errors.As(err, &p) }
