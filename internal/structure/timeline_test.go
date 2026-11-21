package structure

import (
	"encoding/json"
	"instsum/internal/core"
	"testing"
)

func TestProvenanceQuantitiesAndTemporalValidation(t *testing.T) {
	ev := []core.Segment{{ID: "ocr-1", Source: "ocr", Text: "500 g flour", Start: 1, End: 3, MediaID: "m", TimeKnown: true}}
	valid := json.RawMessage(`{"ingredients":[{"quantity":"500","source":"ocr","estimated":false,"timestamp":2,"evidence_ids":["ocr-1"]}]}`)
	if e := ValidateNotes(valid, ev); e != nil {
		t.Fatal(e)
	}
	for _, bad := range []string{
		`{"ingredients":[{"quantity":"50","source":"ocr","evidence_ids":["missing"]}]}`,
		`{"ingredients":[{"quantity":"600","source":"ocr","evidence_ids":["ocr-1"]}]}`,
		`{"ingredients":[{"quantity":"50","source":"ocr","evidence_ids":["ocr-1"]}]}`,
		`{"steps":[{"n":1,"source":"speech","evidence_ids":["ocr-1"]}]}`,
		`{"steps":[{"n":1,"source":"ocr","evidence_ids":["ocr-1"],"t_start":0,"t_end":4}]}`,
	} {
		if e := ValidateNotes([]byte(bad), ev); e == nil {
			t.Fatalf("unsupported notes accepted: %s", bad)
		}
	}
}
func TestWindowAndOverlapDedup(t *testing.T) {
	w := core.Window{Window: []float64{9, 19}, MediaID: "m", Events: []core.Event{{Time: 10, Action: "mix"}}}
	if e := ValidateWindow(w, 9, 19); e != nil {
		t.Fatal(e)
	}
	got := Timeline(core.Metadata{}, nil, nil, []core.Window{w, w})
	if len(got) != 1 || got[0].MediaID != "m" {
		t.Fatalf("duplicate evidence %+v", got)
	}
	w.Events[0].Time = 20
	if e := ValidateWindow(w, 9, 19); e == nil {
		t.Fatal("out-of-window event accepted")
	}
}
