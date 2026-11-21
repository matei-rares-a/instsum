package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"instsum/internal/core"
	"instsum/internal/media"
)

type Result struct {
	Status        string          `json:"status"`
	Metadata      core.Metadata   `json:"metadata"`
	Engine        string          `json:"engine"`
	EngineMode    string          `json:"engine_mode"`
	Model         string          `json:"model"`
	TextModel     string          `json:"text_model"`
	Profile       string          `json:"profile"`
	Warnings      []string        `json:"warnings"`
	Stages        []core.Stage    `json:"stages"`
	Transcript    []core.Segment  `json:"transcript"`
	OCR           []core.Segment  `json:"ocr"`
	Visual        []core.Window   `json:"visual_timeline"`
	Timeline      []core.Segment  `json:"timeline"`
	Notes         json.RawMessage `json:"structured_notes"`
	RawOutput     string          `json:"unvalidated_model_output,omitempty"`
	FailedWindows []WindowFailure `json:"unvalidated_windows,omitempty"`
	Frames        []core.Frame    `json:"reference_frames"`
	Duration      float64         `json:"duration_s"`
	Elapsed       float64         `json:"elapsed_s"`
	PeakVRAM      *float64        `json:"peak_vram_mb"`
}
type WindowFailure struct {
	MediaID string     `json:"media_id"`
	Window  [2]float64 `json:"window"`
	Output  string     `json:"raw_output"`
	Error   string     `json:"error"`
}

func Copy(src, dst string) error {
	b, e := os.ReadFile(src)
	if e != nil {
		return e
	}
	return os.WriteFile(dst, b, 0644)
}
func StepImages(notes json.RawMessage, field string, frames []core.Frame, evidence []core.Segment, dir string) (json.RawMessage, []string, error) {
	var obj map[string]any
	if e := json.Unmarshal(notes, &obj); e != nil {
		return nil, nil, e
	}
	steps, _ := obj[field].([]any)
	index := map[string]core.Segment{}
	for _, s := range evidence {
		index[s.ID] = s
	}
	var warnings []string
	for i, item := range steps {
		step, ok := item.(map[string]any)
		if !ok {
			continue
		}
		ids, _ := step["evidence_ids"].([]any)
		var candidates []core.Frame
		for _, id := range ids {
			name, _ := id.(string)
			ev, ok := index[name]
			if !ok {
				continue
			}
			if ev.Source == "caption" {
				continue
			}
			start, end := ev.Start, ev.End
			if t, ok := step["t_start"].(float64); ok {
				start = t
			}
			if t, ok := step["t_end"].(float64); ok {
				end = t
			}
			if ev.Slide > 0 {
				start = 0
				end = 0
			}
			f, e := media.Best(frames, ev.MediaID, ev.Slide, start, end)
			if e == nil {
				candidates = append(candidates, f)
			}
		}
		if len(candidates) == 0 {
			step["reference_frame"] = ""
			warnings = append(warnings, fmt.Sprintf("step %d has no evidence-aligned reference frame", i+1))
			continue
		}
		best := candidates[0]
		for _, f := range candidates {
			if f.Sharpness > best.Sharpness {
				best = f
			}
		}
		name := fmt.Sprintf("step_%02d.jpg", i+1)
		if e := Copy(best.Path, filepath.Join(dir, name)); e != nil {
			return nil, nil, e
		}
		step["reference_frame"] = name
	}
	b, e := json.Marshal(obj)
	return b, warnings, e
}
func Write(dir string, r Result, p core.Profile) error {
	if len(r.Notes) == 0 {
		r.Notes = json.RawMessage("null")
	}
	var md bytes.Buffer
	if r.EngineMode == "stub" {
		md.WriteString("> **WARNING: STUB OUTPUT. Canned fixtures do not describe this media.**\n\n")
	}
	if r.Status != "complete" {
		fmt.Fprintf(&md, "> **INCOMPLETE RESULT (%s). Read stage warnings before using these notes.**\n\n", r.Status)
	}
	fmt.Fprintf(&md, "# %s\n\n## Metadata\n\n- URL: %s\n- Author: %s\n- Caption: %s\n- Engine: %s (%s)\n\n", p.Name, r.Metadata.URL, r.Metadata.Author, r.Metadata.Caption, r.Engine, r.EngineMode)
	if string(r.Notes) != "null" {
		var notes any
		if e := json.Unmarshal(r.Notes, &notes); e != nil {
			return e
		}
		t, e := template.New("render").Option("missingkey=error").Parse(p.RenderTemplate)
		if e != nil {
			return e
		}
		if e = t.Execute(&md, notes); e != nil {
			return e
		}
	} else {
		md.WriteString("## Structured notes\n\nUnavailable; inspect timeline and stage diagnostics.\n")
	}
	md.WriteString("\n## Reference frames\n\n")
	for _, f := range r.Frames {
		fmt.Fprintf(&md, "![%s at %.3fs](%s)\n\n", f.MediaID, f.Time, filepath.ToSlash(f.Path))
	}
	md.WriteString("## Visual timeline\n\n")
	for _, s := range r.Timeline {
		if s.Source == "visual" {
			fmt.Fprintf(&md, "- %s, slide %d, %.3fs: %s\n", s.MediaID, s.Slide, s.Start, s.Text)
		}
	}
	for _, section := range []struct {
		name     string
		segments []core.Segment
	}{{"Transcript", r.Transcript}, {"On-screen text", r.OCR}} {
		fmt.Fprintf(&md, "\n## %s\n\n", section.name)
		for _, s := range section.segments {
			fmt.Fprintf(&md, "- [%s] %s (%.3f-%.3fs)\n", s.ID, s.Text, s.Start, s.End)
		}
		if len(section.segments) == 0 {
			md.WriteString("No observations; inspect the corresponding stage status below.\n")
		}
	}
	md.WriteString("\n## Stages\n\n")
	for _, s := range r.Stages {
		fmt.Fprintf(&md, "- %s / %s: **%s** (%.3fs) %s\n", s.MediaID, s.Name, s.Status, s.Duration, strings.ReplaceAll(s.Reason, "\n", " "))
	}
	md.WriteString("\n## Warnings\n\n")
	for _, w := range r.Warnings {
		fmt.Fprintf(&md, "- %s\n", w)
	}
	b, e := json.MarshalIndent(r, "", "  ")
	if e != nil {
		return e
	}
	// Render and validate before committing either public output.
	if e = os.WriteFile(filepath.Join(dir, "result.json.tmp"), b, 0644); e != nil {
		return e
	}
	if e = os.WriteFile(filepath.Join(dir, "result.md.tmp"), md.Bytes(), 0644); e != nil {
		return e
	}
	for _, name := range []string{"result.json", "result.md"} {
		if e = os.Rename(filepath.Join(dir, name+".tmp"), filepath.Join(dir, name)); e != nil {
			return e
		}
	}
	return nil
}
