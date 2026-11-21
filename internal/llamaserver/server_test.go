package llamaserver

import (
	"context"
	"instsum/internal/config"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	if os.Getenv("INSTSUM_TEST_LLAMA_HELPER") == "1" {
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
	t.Setenv("INSTSUM_TEST_LLAMA_HELPER", "1")
	s, e := Start(context.Background(), c, "test", dir)
	if e != nil {
		t.Fatal(e)
	}
	if other, e := Start(context.Background(), c, "test", dir); e == nil {
		other.Close()
		t.Fatal("occupied port reused")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
}
