package llamaserver

import (
	"context"
	"fmt"
	"instsum/internal/config"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testGPUUUID = "GPU-bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"

type gpuRunner struct{}

func (gpuRunner) Run(_ context.Context, _ string, _ ...string) ([]byte, error) {
	return []byte(testGPUUUID + ", 10240"), nil
}

func TestMain(m *testing.M) {
	if os.Getenv("INSTSUM_TEST_LLAMA_HELPER") == "1" {
		fmt.Printf("CUDA_VISIBLE_DEVICES=%s args=%s\n", os.Getenv("CUDA_VISIBLE_DEVICES"), strings.Join(os.Args[1:], " "))
		port := ""
		for i, s := range os.Args {
			if s == "--port" && i+1 < len(os.Args) {
				port = os.Args[i+1]
			}
		}
		mux := http.NewServeMux()
		mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
		if e := http.ListenAndServe("127.0.0.1:"+port, mux); e != nil {
			os.Exit(1)
		}
		return
	}
	os.Exit(m.Run())
}
func TestOwnedProcessReadinessAndCleanup(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	address := listener.Addr().String()
	listener.Close()
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if e = os.WriteFile(model, []byte("test model, not loaded by helper"), 0600); e != nil {
		t.Fatal(e)
	}
	exe, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	c := config.Defaults()
	c.VLM.Model = "test"
	c.Models = map[string]config.Model{"test": {GGUFPath: model, MMProjPath: model}}
	c.Llama.URL = "http://" + address + "/v1"
	c.Llama.Manage = true
	c.Llama.Executable = exe
	c.Llama.StartupTimeout = 5
	c.GPU.Device = 1
	t.Setenv("INSTSUM_TEST_LLAMA_HELPER", "1")
	t.Setenv("CUDA_VISIBLE_DEVICES", testGPUUUID)
	s, e := start(context.Background(), c, "test", dir, gpuRunner{})
	if e != nil {
		t.Fatal(e)
	}
	if other, e := start(context.Background(), c, "test", dir, gpuRunner{}); e == nil {
		other.Close()
		t.Fatal("occupied port reused")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	log, e := os.ReadFile(filepath.Join(dir, "llama-server.log"))
	if e != nil {
		t.Fatal(e)
	}
	for _, text := range []string{"Selected NVIDIA GPU 1: " + testGPUUUID, "CUDA_VISIBLE_DEVICES=" + testGPUUUID, "--split-mode none --main-gpu 0"} {
		if !strings.Contains(string(log), text) {
			t.Fatalf("missing %q in %s", text, log)
		}
	}
	t.Setenv("CUDA_VISIBLE_DEVICES", "0")
	if _, e = start(context.Background(), c, "test", dir, gpuRunner{}); e == nil || !strings.Contains(e.Error(), "CUDA_VISIBLE_DEVICES conflicts") {
		t.Fatalf("expected conflicting visibility error, got %v", e)
	}
}
