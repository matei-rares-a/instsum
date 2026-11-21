package gpu

import (
	"context"
	"testing"
)

type sampleRunner string

func (r sampleRunner) Run(context.Context, string, ...string) ([]byte, error) { return []byte(r), nil }
func TestUnitsAndBudget(t *testing.T) {
	m := Monitor{Runner: sampleRunner("8192, 10240"), BudgetMB: 9000}
	s, e := m.Sample(context.Background())
	if e != nil || s.UsedMB < 8589 || s.UsedMB > 8590 {
		t.Fatalf("units %+v %v", s, e)
	}
	m.Runner = sampleRunner("9000, 10240")
	if _, e = m.Sample(context.Background()); e == nil {
		t.Fatal("over-budget GPU admitted")
	}
	m.Runner = sampleRunner("N/A")
	if _, e = m.Sample(context.Background()); e == nil {
		t.Fatal("unreadable telemetry accepted")
	}
}
