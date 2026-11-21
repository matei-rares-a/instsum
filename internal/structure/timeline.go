package structure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"text/template"
	"unicode"

	"instsum/internal/core"
)

func Timeline(meta core.Metadata, speech, ocr []core.Segment, windows []core.Window) []core.Segment {
	result := make([]core.Segment, 0, len(speech)+len(ocr))
	if meta.Caption != "" {
		result = append(result, core.Segment{ID: "caption", Text: meta.Caption, Source: "caption"})
	}
	result = append(result, speech...)
	result = append(result, ocr...)
	for wi, w := range windows {
		for ei, e := range w.Events {
			text := e.Action
			if len(e.Quantities) > 0 {
				text += "; quantities: " + strings.Join(e.Quantities, ", ")
			}
			if e.OnScreenText != "" {
				text += "; visible text: " + e.OnScreenText
			}
			segment := core.Segment{ID: fmt.Sprintf("visual-%s-%d-%d", w.MediaID, wi, ei), Start: e.Time, End: e.Time, Source: "visual", Text: text, MediaID: w.MediaID, Slide: w.Slide, Estimated: e.Estimated, Confidence: e.Confidence, TimeKnown: w.TimeKnown}
			duplicate := false
			for _, prev := range result {
				if prev.Source == "visual" && prev.MediaID == segment.MediaID && prev.Slide == segment.Slide && prev.Text == segment.Text && abs(prev.Start-segment.Start) <= 1 {
					duplicate = true
					break
				}
			}
			if !duplicate {
				result = append(result, segment)
			}
		}
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].Slide != result[j].Slide {
			return result[i].Slide < result[j].Slide
		}
		return result[i].Start < result[j].Start
	})
	return result
}
func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
func ForWindow(in []core.Segment, mediaID string, start, end float64) []core.Segment {
	var out []core.Segment
	for _, s := range in {
		if s.Source == "caption" || (s.MediaID == mediaID && s.End >= start && s.Start <= end) {
			out = append(out, s)
		}
	}
	return out
}
func Render(name, text string, data any) (string, error) {
	t, e := template.New(name).Funcs(template.FuncMap{"json": func(x any) (string, error) { b, e := json.Marshal(x); return string(b), e }}).Option("missingkey=error").Parse(text)
	if e != nil {
		return "", e
	}
	var b bytes.Buffer
	if e = t.Execute(&b, data); e != nil {
		return "", e
	}
	return b.String(), nil
}
func ValidateWindow(w core.Window, start, end float64) error {
	if len(w.Window) != 2 || abs(w.Window[0]-start) > .01 || abs(w.Window[1]-end) > .01 {
		return fmt.Errorf("model window does not match requested interval %.3f-%.3f", start, end)
	}
	for _, ev := range w.Events {
		if ev.Time < start || ev.Time > end {
			return fmt.Errorf("event %.3f outside requested interval", ev.Time)
		}
	}
	return nil
}
func fold(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, s)
}

func evidenceContains(text, term string) bool {
	normalize := func(s string) string {
		return strings.Map(func(r rune) rune {
			switch unicode.ToLower(r) {
			case 'ă', 'â':
				return 'a'
			case 'î':
				return 'i'
			case 'ș', 'ş':
				return 's'
			case 'ț', 'ţ':
				return 't'
			default:
				return unicode.ToLower(r)
			}
		}, s)
	}
	haystack, needle := []rune(normalize(text)), []rune(normalize(strings.TrimSpace(term)))
	if len(needle) == 0 {
		return false
	}
	numeric := true
	for _, r := range needle {
		if !unicode.IsDigit(r) && r != '.' && r != ',' && r != '/' {
			numeric = false
			break
		}
	}
	word := func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	}
	separator := func(r rune) bool {
		return r == '.' || r == ',' || r == '/'
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		end := i + len(needle)
		if numeric && ((i >= 2 && separator(haystack[i-1]) && unicode.IsDigit(haystack[i-2])) ||
			(end+1 < len(haystack) && separator(haystack[end]) && unicode.IsDigit(haystack[end+1]))) {
			continue
		}
		if string(haystack[i:i+len(needle)]) == string(needle) &&
			(i == 0 || !word(haystack[i-1])) &&
			(end == len(haystack) || !word(haystack[end])) {
			return true
		}
	}
	return false
}

// ValidateNotes checks provenance against supplied evidence, not model assertions.
func ValidateNotes(notes json.RawMessage, evidence []core.Segment, aliases map[string]string) error {
	var value any
	if e := json.Unmarshal(notes, &value); e != nil {
		return e
	}
	index := map[string]core.Segment{}
	for _, s := range evidence {
		index[s.ID] = s
	}
	var walk func(any) error
	walk = func(v any) error {
		switch x := v.(type) {
		case []any:
			for _, item := range x {
				if e := walk(item); e != nil {
					return e
				}
			}
		case map[string]any:
			if source, ok := x["source"].(string); ok {
				ids, ok := x["evidence_ids"].([]any)
				if !ok || len(ids) == 0 {
					return fmt.Errorf("extracted %s item has no evidence_ids", source)
				}
				var matched []core.Segment
				for _, id := range ids {
					name, ok := id.(string)
					if !ok {
						return fmt.Errorf("invalid evidence identifier")
					}
					ev, exists := index[name]
					if !exists {
						return fmt.Errorf("unknown evidence %q", name)
					}
					if ev.Source != source {
						return fmt.Errorf("source %s does not match evidence %s (%s)", source, name, ev.Source)
					}
					matched = append(matched, ev)
				}
				factEvidence := matched
				if source == "caption" {
					name, hasName := x["name"].(string)
					_, hasQuantity := x["quantity"].(string)
					if hasName && hasQuantity && strings.TrimSpace(name) != "" {
						factEvidence = nil
						for _, ev := range matched {
							for _, line := range strings.Split(ev.Text, "\n") {
								if evidenceContains(line, name) {
									fragment := ev
									fragment.Text = line
									factEvidence = append(factEvidence, fragment)
								}
							}
						}
						if len(factEvidence) == 0 {
							return fmt.Errorf("ingredient %q is not present in cited caption", name)
						}
					}
				}
				if quantity, ok := x["quantity"].(string); ok && quantity != "" && source == "ocr" {
					pattern := regexp.MustCompile(`(^|[^0-9.,/])` + regexp.QuoteMeta(fold(quantity)) + `($|[^0-9.,/])`)
					found := false
					for _, ev := range factEvidence {
						if pattern.MatchString(fold(ev.Text)) {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("OCR quantity %q is not present verbatim in its evidence", quantity)
					}
				}
				if quantity, ok := x["quantity"].(string); ok && quantity != "" && source != "ocr" {
					estimated, _ := x["estimated"].(bool)
					if !estimated {
						found := false
						for _, ev := range factEvidence {
							found = found || evidenceContains(ev.Text, quantity)
						}
						if !found {
							return fmt.Errorf("quantity %q is not supported by cited evidence", quantity)
						}
					}
				}
				if unit, ok := x["unit"].(string); ok && strings.TrimSpace(unit) != "" {
					terms := []string{unit}
					for alias, canonical := range aliases {
						if strings.EqualFold(canonical, unit) {
							terms = append(terms, alias)
						}
					}
					found := false
					for _, ev := range factEvidence {
						for _, term := range terms {
							found = found || evidenceContains(ev.Text, term)
						}
					}
					if !found {
						return fmt.Errorf("unit %q is not supported by cited evidence", unit)
					}
				}
				if estimate, ok := x["estimated"].(bool); ok && !estimate {
					for _, ev := range matched {
						if ev.Estimated {
							return fmt.Errorf("estimated evidence cannot produce an unestimated item")
						}
					}
				}
				for _, key := range []string{"timestamp", "t_start", "t_end"} {
					t, hasTime := x[key].(float64)
					if !hasTime {
						continue
					}
					found := false
					for _, ev := range matched {
						if ev.TimeKnown && t >= ev.Start-.01 && t <= ev.End+.01 {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("%s %.3f is unsupported by evidence timestamps", key, t)
					}
				}
				if mediaID, ok := x["media_id"].(string); ok && mediaID != "" {
					found := false
					for _, ev := range matched {
						if ev.MediaID == mediaID {
							found = true
						}
					}
					if !found {
						return fmt.Errorf("media_id %s does not match cited evidence", mediaID)
					}
				}
			}
			for _, item := range x {
				if e := walk(item); e != nil {
					return e
				}
			}
		}
		return nil
	}
	if e := walk(value); e != nil {
		return e
	}
	if obj, ok := value.(map[string]any); ok {
		if steps, ok := obj["steps"].([]any); ok {
			lastTime := map[string]float64{}
			for i, s := range steps {
				step, ok := s.(map[string]any)
				if !ok {
					continue
				}
				if n, ok := step["n"].(float64); ok && n != float64(i+1) {
					return fmt.Errorf("steps must have sequential n")
				}
				a, aok := step["t_start"].(float64)
				b, bok := step["t_end"].(float64)
				if aok != bok || aok && (a < 0 || b < a) {
					return fmt.Errorf("step interval invalid")
				}
				media, _ := step["media_id"].(string)
				if aok {
					if prev, exists := lastTime[media]; exists && a < prev {
						return fmt.Errorf("steps are not temporally ordered")
					}
					lastTime[media] = a
				}
			}
		}
	}
	return nil
}
