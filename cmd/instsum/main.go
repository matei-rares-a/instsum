package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"instsum/internal/config"
	"instsum/internal/pipeline"
)

func main() {
	os.Exit(run())
}
func run() int {
	configPath := flag.String("config", "configs/config.yaml", "configuration file")
	engine := flag.String("engine", "", "override vlm.engine")
	profile := flag.String("profile", "", "override structure.profile")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: instsum [--config FILE] [--engine llamacpp|ollama] [--profile NAME] <instagram-url|local-video>")
		return 64
	}
	c, err := config.Load(*configPath)
	if err != nil {
		slog.Error("configuration", "error", err)
		return 1
	}
	if *engine != "" {
		c.VLM.Engine = *engine
	}
	if *profile != "" {
		c.Structure.Profile = *profile
	}
	if err = c.Validate(); err != nil {
		slog.Error("configuration", "error", err)
		return 1
	}
	var handler slog.Handler
	level := slog.LevelInfo
	if err = level.UnmarshalText([]byte(c.Logging.Level)); err != nil {
		fmt.Fprintln(os.Stderr, "invalid logging.level:", err)
		return 1
	}
	options := &slog.HandlerOptions{Level: level}
	if c.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stderr, options)
	} else if c.Logging.Format == "text" {
		handler = slog.NewTextHandler(os.Stderr, options)
	} else {
		fmt.Fprintln(os.Stderr, "logging.format must be text or json")
		return 1
	}
	slog.SetDefault(slog.New(handler))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	dir, err := pipeline.Run(ctx, c, flag.Arg(0))
	if err != nil {
		if pipeline.IsPartial(err) {
			fmt.Println(dir)
			slog.Warn("incomplete result", "output", dir)
			return 2
		}
		slog.Error("pipeline failed", "error", err)
		return 1
	}
	fmt.Println(dir)
	return 0
}
