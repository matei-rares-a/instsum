package structure

import (
	"encoding/json"
	"instsum/internal/core"
	"testing"
)

func TestProvenanceQuantitiesAndTemporalValidation(t *testing.T) {
	ev := []core.Segment{{ID: "ocr-1", Source: "ocr", Text: "500 g flour", Start: 1, End: 3, MediaID: "m", TimeKnown: true}}
	valid := json.RawMessage(`{"ingredients":[{"quantity":"500","source":"ocr","estimated":false,"timestamp":2,"evidence_ids":["ocr-1"]}]}`)
	if e := ValidateNotes(valid, ev, nil); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{
		`{"ingredients":[{"quantity":"50","source":"ocr","evidence_ids":["missing"]}]}`,
		`{"ingredients":[{"quantity":"600","source":"ocr","evidence_ids":["ocr-1"]}]}`,
		`{"ingredients":[{"quantity":"50","source":"ocr","evidence_ids":["ocr-1"]}]}`,
		`{"steps":[{"n":1,"source":"speech","evidence_ids":["ocr-1"]}]}`,
		`{"steps":[{"n":1,"source":"ocr","evidence_ids":["ocr-1"],"t_start":0,"t_end":4}]}`,
	} {
		if e := ValidateNotes([]byte(bad), ev, nil); e == nil {
			t.Fatalf("unsupported notes accepted: %s", bad)
		}
	}
}
func TestWindowAndOverlapDedup(t *testing.T) {
	w := core.Window{Window: []float64{9, 19}, MediaID: "m", Events: []core.Event{{Time: 10, Action: "mix"}}}
	if e := ValidateWindow(w, 9, 19); e != nil {
		t.Fatal(e)
	}

}

func TestCaptionFactsRequireSupportedQuantityAndUnit(t *testing.T) {
	ev := []core.Segment{{ID: "caption", Source: "caption", Text: "o lingurita mustar\n2 lingurite miere"}}
	aliases := map[string]string{"teaspoon": "linguriță"}
	for _, tt := range []struct {
		name  string
		note  string
		valid bool
	}{
		{"null time and explicit value", `{"source":"caption","quantity":"2","unit":"lingurițe","timestamp":null,"evidence_ids":["caption"]}`, true},
		{"caption has no timestamp", `{"source":"caption","quantity":"2","unit":"","timestamp":0,"evidence_ids":["caption"]}`, false},
		{"invented amount", `{"source":"caption","quantity":"5","unit":"","timestamp":null,"evidence_ids":["caption"]}`, false},
		{"wrong tablespoon", `{"source":"caption","quantity":"","unit":"lingură","timestamp":null,"evidence_ids":["caption"]}`, false},
		{"unit substring", `{"source":"caption","quantity":"","unit":"linguri","timestamp":null,"evidence_ids":["caption"]}`, false},
		{"wrong ingredient association", `{"name":"mustar","source":"caption","quantity":"2","unit":"lingurițe","timestamp":null,"evidence_ids":["caption"]}`, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateNotes([]byte(tt.note), ev, aliases)
			if (err == nil) != tt.valid {
				t.Fatalf("ValidateNotes: %v, valid=%v", err, tt.valid)
			}
		})
	}
}

func TestCaptionQuantityDoesNotMatchPartOfDecimal(t *testing.T) {
	ev := []core.Segment{{ID: "caption", Source: "caption", Text: "2.5 g flour"}}
	note := []byte(`{"name":"flour","source":"caption","quantity":"2","unit":"g","evidence_ids":["caption"]}`)
	if err := ValidateNotes(note, ev, nil); err == nil {
		t.Fatal("partial decimal accepted")
	}
	note = []byte(`{"name":"flour","source":"caption","quantity":"2.5","unit":"g","evidence_ids":["caption"]}`)
	if err := ValidateNotes(note, ev, nil); err != nil {
		t.Fatalf("exact decimal rejected: %v", err)
	}
	for _, tt := range []struct {
		text, quantity string
		valid          bool
	}{
		{"2, eggs", "2", true},
		{"2. eggs", "2", true},
		{"2,5 eggs", "2", false},
		{"2/3 eggs", "3", false},
	} {
		if got := evidenceContains(tt.text, tt.quantity); got != tt.valid {
			t.Errorf("evidenceContains(%q, %q) = %t", tt.text, tt.quantity, got)
		}
	}
}

func TestWindowDedupAndOutOfRange(t *testing.T) {
	w := core.Window{Window: []float64{9, 19}, MediaID: "m", Events: []core.Event{{Time: 10, Action: "mix"}}}
	got := Timeline(core.Metadata{}, nil, nil, []core.Window{w, w})
	if len(got) != 1 || got[0].MediaID != "m" {
		t.Fatalf("duplicate evidence %+v", got)
	}
	w.Events[0].Time = 20
	if e := ValidateWindow(w, 9, 19); e == nil {
		t.Fatal("out-of-window event accepted")
	}
}
