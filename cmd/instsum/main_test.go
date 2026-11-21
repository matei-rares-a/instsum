package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("INSTSUM_TEST_LAUNCHER_HELPER") == "1" {
		fmt.Fprintln(os.Stdout, "argument:", os.Args[1])
		fmt.Fprintln(os.Stderr, "diagnostic from stderr")
		if os.Getenv("INSTSUM_TEST_PARTIAL") == "1" {
			os.Exit(2)
		}
		os.Exit(0)
	}
	if os.Getenv("INSTSUM_TEST_CLI_HELPER") == "1" {
		flag.CommandLine = flag.NewFlagSet("instsum", flag.ExitOnError)
		main()
		return
	}
	os.Exit(m.Run())
}
func TestCLIOverrideAndPartialExitContract(t *testing.T) {
	root, e := filepath.Abs(filepath.Join("..", ".."))
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "input.jpg")
	f, e := os.Create(input)
	if e != nil {
		t.Fatal(e)
	}
	e = jpeg.Encode(f, image.NewRGBA(image.Rect(0, 0, 32, 32)), nil)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	cmd := exec.Command(exe, "--config", filepath.Join(root, "configs", "config.yaml"), "--engine", "ollama", "--profile", "recipe", input)
	cmd.Env = append(os.Environ(),
		"INSTSUM_TEST_CLI_HELPER=1",
		"IGX_VLM_ENGINE=invalid-before-cli",
		"IGX_OLLAMA_MODE=stub",
		"IGX_OCR_ENGINE=disabled",
		"IGX_PROFILES_DIR="+filepath.Join(root, "profiles"),
		"IGX_PROMPTS_DIR="+filepath.Join(root, "prompts"),
		"IGX_OLLAMA_STUB_FIXTURES_DIR="+filepath.Join(root, "testdata", "ollama_fixtures"),
		"IGX_OUTPUT_DIR="+filepath.Join(dir, "out"))
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	exit, ok := err.(*exec.ExitError)
	if !ok || exit.ExitCode() != 2 {
		t.Fatalf("expected partial exit 2, got %v; %s", err, stderr.String())
	}
	out := strings.TrimSpace(stdout.String())
	b, e := os.ReadFile(filepath.Join(out, "result.json"))
	if e != nil {
		t.Fatal(e)
	}
	var result struct {
		Status string          `json:"status"`
		Mode   string          `json:"engine_mode"`
		Notes  json.RawMessage `json:"structured_notes"`
	}
	if e = json.Unmarshal(b, &result); e != nil {
		t.Fatal(e)
	}
	if result.Status != "partial" || result.Mode != "stub" || len(result.Notes) == 0 || string(result.Notes) == "null" {
		t.Fatalf("invalid result %s", b)
	}
}
