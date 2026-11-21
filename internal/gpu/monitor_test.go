package gpu

import (
	"context"
	"strings"
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

type uuidRunner struct {
	output string
	args   []string
}

func (r *uuidRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return []byte(r.output), nil
}

func TestDeviceUUIDUsesPhysicalIndexAndBudget(t *testing.T) {
	const uuid = "GPU-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	r := &uuidRunner{output: uuid + ", 10240\n"}
	got, err := DeviceUUID(context.Background(), r, 1, 9000)
	if err != nil || got != uuid || strings.Join(r.args, " ") != "nvidia-smi -i 1 --query-gpu=uuid,memory.total --format=csv,noheader,nounits" {
		t.Fatalf("got %q, args %v, error %v", got, r.args, err)
	}
	for _, invalid := range []string{uuid + ", 4096", uuid + ", NaN", "bad, 10240", uuid + ", N/A", ""} {
		r.output = invalid
		if _, err := DeviceUUID(context.Background(), r, 1, 9000); err == nil {
			t.Fatalf("accepted invalid device: %q", invalid)
		}
	}
}
