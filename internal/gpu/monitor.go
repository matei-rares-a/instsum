package gpu

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"instsum/internal/tool"
)

type Sample struct {
	UsedMB  float64
	TotalMB float64
}
type Monitor struct {
	Runner   tool.Runner
	Device   int
	BudgetMB float64
	mu       sync.Mutex
	peak     float64
}

func (m *Monitor) Sample(ctx context.Context) (Sample, error) {
	r := m.Runner
	if r == nil {
		r = tool.Exec{}
	}
	query, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	b, e := r.Run(query, "nvidia-smi", "-i", strconv.Itoa(m.Device), "--query-gpu=memory.used,memory.total", "--format=csv,noheader,nounits")
	if e != nil {
		return Sample{}, e
	}
	fields := strings.Split(strings.TrimSpace(string(b)), ",")
	if len(fields) != 2 {
		return Sample{}, fmt.Errorf("nvidia-smi returned unexpected memory data")
	}
	used, e := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
	if e != nil {
		return Sample{}, e
	}
	total, e := strconv.ParseFloat(strings.TrimSpace(fields[1]), 64)
	if e != nil {
		return Sample{}, e
	}
	s := Sample{UsedMB: used * 1048576 / 1e6, TotalMB: total * 1048576 / 1e6}
	if m.BudgetMB > s.TotalMB {
		return s, fmt.Errorf("configured GPU budget %.0f MB exceeds physical GPU capacity %.0f MB; set gpu.vram_budget_mb for the actual runtime hardware", m.BudgetMB, s.TotalMB)
	}
	m.mu.Lock()
	if s.UsedMB > m.peak {
		m.peak = s.UsedMB
	}
	m.mu.Unlock()
	if s.UsedMB > m.BudgetMB {
		return s, fmt.Errorf("GPU %d uses %.0f MB, exceeding gpu.vram_budget_mb %.0f; release GPU memory or lower model/context/frames", m.Device, s.UsedMB, m.BudgetMB)
	}
	return s, nil
}
func (m *Monitor) Peak() float64 { m.mu.Lock(); defer m.mu.Unlock(); return m.peak }
func (m *Monitor) Run(ctx context.Context, fn func(context.Context) error) error {
	if _, e := m.Sample(ctx); e != nil {
		return e
	}
	child, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- fn(child) }()
	timer := time.NewTicker(250 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case e := <-done:
			return e
		case <-ctx.Done():
			cancel()
			<-done
			return ctx.Err()
		case <-timer.C:
			if _, e := m.Sample(child); e != nil {
				cancel()
				<-done
				return e
			}
		}
	}
}
