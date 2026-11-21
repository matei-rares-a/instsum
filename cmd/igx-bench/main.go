package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/pipeline"
	"instsum/internal/telemetry"
)

type measurement struct {
	Engine   string       `json:"engine"`
	Model    string       `json:"model"`
	Window   float64      `json:"window_s"`
	Input    string       `json:"input"`
	Result   string       `json:"result_directory"`
	Wall     float64      `json:"wall_s"`
	Error    string       `json:"error,omitempty"`
	PeakVRAM *float64     `json:"peak_vram_mb"`
	Stages   []core.Stage `json:"stages"`
	PeakRSS  *uint64      `json:"orchestrator_process_peak_rss_bytes"`
	Runtime  string       `json:"go_runtime"`
	Scope    string       `json:"measurement_scope"`
}

func main() {
	configFile := flag.String("config", "configs/config.yaml", "configuration")
	engine := flag.String("engine", "llamacpp", "llamacpp or ollama")
	models := flag.String("models", "", "comma-separated logical model keys; default configured model")
	windows := flag.String("windows", "5,10,15", "comma-separated window sizes")
	out := flag.String("output", "bench/results", "benchmark report directory")
	live := flag.Bool("allow-live", false, "explicitly permit real inference; no automatic model download")
	flag.Parse()
	if !*live || flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: igx-bench --allow-live --engine llamacpp|ollama [--models keys] [--windows 5,10,15] <local-media>")
		os.Exit(1)
	}
	if e := run(*configFile, *engine, *models, *windows, *out, flag.Arg(0)); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run(file, engine, models, windows, out, input string) error {
	c, e := config.Load(file)
	if e != nil {
		return e
	}
	c.VLM.Engine = engine
	if engine == "ollama" {
		c.Ollama.Mode = "live"
	}
	c.GPU.Monitor = true
	if c.Ollama.AutoPull {
		return fmt.Errorf("benchmark requires ollama.auto_pull:false; install models explicitly first")
	}
	if models == "" {
		models = c.VLM.Model
	}
	var sizes []float64
	for _, s := range strings.Split(windows, ",") {
		n, e := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if e != nil || n <= c.VLM.Overlap {
			return fmt.Errorf("invalid window %q", s)
		}
		sizes = append(sizes, n)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	var results []measurement
	for _, model := range strings.Split(models, ",") {
		for _, window := range sizes {
			c.VLM.Model = strings.TrimSpace(model)
			c.VLM.Window = window
			if e = c.Validate(); e != nil {
				return e
			}
			start := time.Now()
			dir, err := pipeline.Run(ctx, c, input)
			m := measurement{Engine: engine, Model: c.VLM.Model, Window: window, Input: input, Result: dir, Wall: time.Since(start).Seconds(), Runtime: runtime.Version() + " " + runtime.GOOS + "/" + runtime.GOARCH, Scope: "VRAM: sampled whole GPU including other users; RSS: cumulative orchestrator process high-water mark, excludes inference subprocesses"}
			if err != nil {
				m.Error = err.Error()
			}
			if dir != "" {
				b, e := os.ReadFile(filepath.Join(dir, "result.json"))
				if e == nil {
					var result struct {
						Peak   *float64     `json:"peak_vram_mb"`
						Stages []core.Stage `json:"stages"`
					}
					e = json.Unmarshal(b, &result)
					m.PeakVRAM = result.Peak
					m.Stages = result.Stages
				}
				if e != nil {
					m.Error += "; benchmark could not read stage telemetry: " + e.Error()
				}
			}
			rss, e := telemetry.PeakRSS()
			if e != nil {
				m.Error += "; RSS telemetry: " + e.Error()
			} else {
				m.PeakRSS = &rss
			}
			results = append(results, m)
			if ctx.Err() != nil {
				break
			}
		}
	}
	if e = os.MkdirAll(out, 0755); e != nil {
		return e
	}
	name := filepath.Join(out, "benchmark-"+time.Now().UTC().Format("20060102T150405.000000000")+".json")
	b, e := json.MarshalIndent(results, "", "  ")
	if e != nil {
		return e
	}
	if e = os.WriteFile(name, b, 0644); e != nil {
		return e
	}
	fmt.Println(name)
	for _, r := range results {
		if r.Error != "" {
			return errors.New("benchmark contains failed or partial runs; inspect report")
		}
	}
	return nil
}
