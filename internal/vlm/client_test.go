package vlm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/vlm/ollama/fakeserver"
)

var contractSchema = json.RawMessage(`{"type":"object","required":["ok"],"properties":{"ok":{"type":"boolean"}}}`)

func testConfig(t *testing.T) config.Config {
	t.Helper()
	c, err := config.Load(filepath.Join("..", "..", "configs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEngineContract(t *testing.T) {
	c := testConfig(t)
	dir, err := os.MkdirTemp(".", "vlm-images-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	first, second := filepath.Join(dir, "first.jpg"), filepath.Join(dir, "second.jpg")
	if err := os.WriteFile(first, []byte("first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("second"), 0600); err != nil {
		t.Fatal(err)
	}

	for _, engine := range []string{"llamacpp", "ollama"} {
		t.Run(engine, func(t *testing.T) {
			var chatCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case engine == "ollama" && r.Method == http.MethodGet && r.URL.Path == "/api/version":
					_, _ = w.Write([]byte(`{"version":"fake"}`))
				case engine == "ollama" && r.Method == http.MethodGet && r.URL.Path == "/api/tags":
					_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5vl:7b","model":"qwen2.5vl:7b"}]}`))
				case engine == "ollama" && r.Method == http.MethodPost && r.URL.Path == "/api/show":
					_, _ = w.Write([]byte(`{"capabilities":["completion","vision"],"model_info":{"qwen.context_length":8192}}`))
				case engine == "ollama" && r.Method == http.MethodPost && r.URL.Path == "/api/chat":
					chatCalls.Add(1)
					var body struct {
						Model    string `json:"model"`
						Messages []struct {
							Content string   `json:"content"`
							Images  []string `json:"images"`
						} `json:"messages"`
						Format    json.RawMessage `json:"format"`
						Stream    bool            `json:"stream"`
						Options   map[string]any  `json:"options"`
						KeepAlive string          `json:"keep_alive"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode Ollama request: %v", err)
					} else if body.Model != "qwen2.5vl:7b" || body.Stream || body.KeepAlive != c.Ollama.KeepAlive ||
						len(body.Messages) != 1 || len(body.Messages[0].Images) != 2 ||
						body.Messages[0].Images[0] != "Zmlyc3Q=" || body.Messages[0].Images[1] != "c2Vjb25k" ||
						!strings.Contains(body.Messages[0].Content, "[t=1.00s]") ||
						!strings.Contains(body.Messages[0].Content, "[t=2.00s]") ||
						len(body.Format) == 0 || body.Options["num_ctx"] != float64(c.VLM.Context) ||
						body.Options["num_predict"] != float64(c.VLM.NumPredict) {
						t.Errorf("incorrect Ollama request mapping: %#v", body)
					}
					_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"{\"ok\":true}"},"done":true}`))
				case engine == "llamacpp" && r.Method == http.MethodGet && r.URL.Path == "/v1/models":
					_, _ = w.Write([]byte(`{"data":[{"id":"server-model-id"}]}`))
				case engine == "llamacpp" && r.Method == http.MethodGet && r.URL.Path == "/props":
					_, _ = w.Write([]byte(`{"default_generation_settings":{"n_ctx":8192}}`))
				case engine == "llamacpp" && r.Method == http.MethodPost && r.URL.Path == "/v1/chat/completions":
					chatCalls.Add(1)
					var body struct {
						Model    string `json:"model"`
						Messages []struct {
							Content []struct {
								Type string `json:"type"`
								Text string `json:"text"`
								URL  struct {
									URL string `json:"url"`
								} `json:"image_url"`
							} `json:"content"`
						} `json:"messages"`
						ResponseFormat map[string]any `json:"response_format"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
						t.Errorf("decode llama.cpp request: %v", err)
					} else {
						if body.Model != "server-model-id" || body.Model == c.Models[c.VLM.Model].GGUFPath {
							t.Errorf("llama model must be a server-reported ID, got %q", body.Model)
						}
						if body.ResponseFormat["json_schema"] == nil || len(body.Messages) != 1 || len(body.Messages[0].Content) != 5 {
							t.Errorf("incorrect llama.cpp schema or message mapping: %#v", body)
						} else {
							content := body.Messages[0].Content
							if content[1].Text != "[t=1.00s]" || content[2].URL.URL != "data:image/jpeg;base64,Zmlyc3Q=" ||
								content[3].Text != "[t=2.00s]" || content[4].URL.URL != "data:image/jpeg;base64,c2Vjb25k" {
								t.Errorf("llama.cpp frame order/timestamps were not preserved: %#v", content)
							}
						}
					}
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			if engine == "ollama" {
				c.Ollama.URL = srv.URL
				c.Ollama.Mode = "live"
			} else {
				c.Llama.URL = srv.URL + "/v1"
			}
			client := New(c, engine)
			if err := client.Ready(context.Background()); err != nil {
				t.Fatal(err)
			}
			result, err := client.Complete(context.Background(), core.Request{
				Prompt: "test", Schema: contractSchema,
				Images: []core.Frame{{Path: first, Time: 1}, {Path: second, Time: 2}},
			})
			if err != nil || string(result) != `{"ok":true}` {
				t.Fatalf("complete %s: %v", result, err)
			}
			if got := chatCalls.Load(); got != 1 {
				t.Fatalf("chat calls = %d, want one", got)
			}
		})
	}
}

func TestCanceledContextContractForBothEngines(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	for _, engine := range []string{"llamacpp", "ollama"} {
		t.Run(engine, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			client := New(c, engine)
			if err := client.Ready(ctx); err == nil || !strings.Contains(err.Error(), "canceled") {
				t.Fatalf("Ready cancellation = %v", err)
			}
			if _, err := client.Complete(ctx, core.Request{Schema: contractSchema}); err == nil ||
				!strings.Contains(err.Error(), "canceled") {
				t.Fatalf("Complete cancellation = %v", err)
			}
		})
	}
}

func TestEngineRepairAndFailureContract(t *testing.T) {
	for _, engine := range []string{"llamacpp", "ollama"} {
		for _, scenario := range []string{"repair", "invalid-envelope", "http-error", "cancellation"} {
			t.Run(engine+"/"+scenario, func(t *testing.T) {
				c := testConfig(t)
				c.Ollama.Mode = "live"
				var calls atomic.Int32
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch r.URL.Path {
					case "/api/version":
						_, _ = w.Write([]byte(`{"version":"fake"}`))
					case "/api/tags":
						_, _ = w.Write([]byte(`{"models":[{"name":"qwen2.5vl:7b"}]}`))
					case "/api/show":
						_, _ = w.Write([]byte(`{"capabilities":["completion","vision"],"model_info":{"context_length":8192}}`))
					case "/v1/models":
						_, _ = w.Write([]byte(`{"data":[{"id":"server-model"}]}`))
					case "/props":
						_, _ = w.Write([]byte(`{"n_ctx":8192}`))
					case "/api/chat", "/v1/chat/completions":
						attempt := calls.Add(1)
						if scenario == "cancellation" {
							var body any
							_ = json.NewDecoder(r.Body).Decode(&body)
							select {
							case <-r.Context().Done():
							case <-time.After(time.Second):
							}
							return
						}
						if scenario == "http-error" {
							http.Error(w, "raw backend failure", http.StatusBadGateway)
							return
						}
						if scenario == "invalid-envelope" {
							_, _ = w.Write([]byte(`{"invalid"`))
							return
						}
						var body struct {
							Messages []struct {
								Content json.RawMessage `json:"content"`
							} `json:"messages"`
						}
						if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
							t.Errorf("decode repair request: %v", err)
						}
						if attempt == 2 && (len(body.Messages) != 1 || !strings.Contains(string(body.Messages[0].Content), "Previous response was invalid")) {
							t.Errorf("missing repair instruction: %v", body.Messages)
						}
						content := `{"ok":true}`
						if attempt == 1 {
							content = `{"ok":"not a boolean"}`
						}
						if engine == "ollama" {
							_ = json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"content": content}})
						} else {
							_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": content}}}})
						}
					default:
						http.NotFound(w, r)
					}
				}))
				defer server.Close()
				c.Ollama.URL = server.URL
				c.Llama.URL = server.URL + "/v1"
				client := New(c, engine)
				if err := client.Ready(context.Background()); err != nil {
					t.Fatal(err)
				}
				ctx := context.Background()
				if scenario == "cancellation" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
					defer cancel()
				}
				output, err := client.Complete(ctx, core.Request{Kind: "structure", Schema: contractSchema, Prompt: "produce output"})
				switch scenario {
				case "repair":
					if err != nil || string(output) != `{"ok":true}` || calls.Load() != 2 {
						t.Fatalf("repair result=%q err=%v attempts=%d", output, err, calls.Load())
					}
				case "invalid-envelope":
					if err == nil || !strings.Contains(err.Error(), "invalid JSON") || !strings.Contains(err.Error(), `{\"invalid\"`) {
						t.Fatalf("invalid envelope error = %v", err)
					}
				case "http-error":
					if err == nil || !strings.Contains(err.Error(), "raw backend failure") {
						t.Fatalf("HTTP error = %v", err)
					}
				case "cancellation":
					if err == nil || !strings.Contains(err.Error(), "deadline") {
						t.Fatalf("cancellation error = %v", err)
					}
				}
			})
		}
	}
}

func TestOllamaModelReadinessAndOverride(t *testing.T) {
	c := testConfig(t)
	c.Models[c.VLM.Model] = config.Model{OllamaTag: "vision:latest"}
	c.Models["text-model"] = config.Model{OllamaTag: "text:latest"}
	c.Structure.TextModel = "text-model"
	var requestedChatModel atomic.Value
	server := fakeserver.Server{
		Model:    "vision:latest",
		Models:   []string{"vision:latest", "text:latest"},
		Response: `{"ok":true}`,
		ModelCapabilities: map[string][]string{
			"vision:latest": {"completion", "vision"},
			"text:latest":   {"completion"},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chat" {
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			requestedChatModel.Store(body.Model)
		}
		server.ServeHTTP(w, r)
	}))
	defer srv.Close()
	c.Ollama.URL = srv.URL
	c.Ollama.Mode = "live"
	client := New(c, "ollama")
	if err := client.Ready(context.Background()); err != nil {
		t.Fatalf("primary vision readiness: %v", err)
	}
	result, err := client.Complete(context.Background(), core.Request{Kind: "structure", Profile: "recipe", Schema: contractSchema})
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("text completion: %s %v", result, err)
	}
	if got := requestedChatModel.Load(); got != "text:latest" {
		t.Fatalf("structure model = %v, want text:latest", got)
	}
	result, err = client.Complete(context.Background(), core.Request{
		Kind: "structure", Model: c.VLM.Model, Profile: "recipe", Schema: contractSchema,
	})
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("logical model override: %s %v", result, err)
	}
	if got := requestedChatModel.Load(); got != "vision:latest" {
		t.Fatalf("explicit logical model override = %v, want vision:latest", got)
	}
}

func TestOllamaTextCapabilityAndContextChecks(t *testing.T) {
	c := testConfig(t)
	c.Models["text-model"] = config.Model{OllamaTag: "text:latest"}
	c.Structure.TextModel = "text-model"
	c.Ollama.Mode = "live"
	c.Ollama.URL = "http://127.0.0.1:11434"
	for _, test := range []struct {
		name         string
		capabilities []string
		context      int
		want         string
	}{
		{name: "vision is not required for text", capabilities: []string{"completion"}, context: 16384},
		{name: "completion capability required", capabilities: []string{"vision"}, context: 16384, want: "completion capability"},
		{name: "context limit enforced", capabilities: []string{"completion"}, context: 1024, want: "exceeds"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := fakeserver.Server{
				Model: "text:latest", Response: `{"ok":true}`,
				Capabilities: test.capabilities, ContextLength: test.context,
			}
			srv := httptest.NewServer(server)
			defer srv.Close()
			c.Ollama.URL = srv.URL
			client := New(c, "ollama")
			result, err := client.Complete(context.Background(), core.Request{Kind: "structure", Schema: contractSchema})
			if test.want == "" {
				if err != nil || string(result) != `{"ok":true}` {
					t.Fatalf("text-only request: %s %v", result, err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestOllamaReadyRequiresPrimaryVisionCapability(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	srv := httptest.NewServer(fakeserver.Server{
		Model: "qwen2.5vl:7b", Capabilities: []string{"completion"},
	})
	defer srv.Close()
	c.Ollama.URL = srv.URL
	err := New(c, "ollama").Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), "vision capability") {
		t.Fatalf("Ready error = %v, want vision capability failure", err)
	}
}

func TestLlamaTextRequestDoesNotRequireVisionProjector(t *testing.T) {
	c := testConfig(t)
	c.Models[c.VLM.Model] = config.Model{GGUFPath: "local-model.gguf"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"server-text-model"}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"default_generation_settings":{"n_ctx":8192}}`))
		case "/v1/chat/completions":
			var body struct {
				Model string `json:"model"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "server-text-model" {
				t.Errorf("unexpected llama.cpp server model ID: %+v (%v)", body, err)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c.Llama.URL = srv.URL + "/v1"
	client := New(c, "llamacpp")
	result, err := client.Complete(context.Background(), core.Request{
		Kind: "structure", Schema: contractSchema,
	})
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("text completion: %s %v", result, err)
	}
	if err := client.Ready(context.Background()); err == nil || !strings.Contains(err.Error(), "mmproj_path") {
		t.Fatalf("vision Ready error = %v, want missing projector", err)
	}
}

func TestLlamaRejectsInsufficientContext(t *testing.T) {
	c := testConfig(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"server-model"}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"default_generation_settings":{"n_ctx":4096}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c.Llama.URL = srv.URL + "/v1"
	err := New(c, "llamacpp").Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("context error = %v, want insufficient context failure", err)
	}
}

func TestLlamaModelOverrideMustMatchServerID(t *testing.T) {
	c := testConfig(t)
	c.Models["other"] = config.Model{GGUFPath: "other.gguf"}
	var requested atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			_, _ = w.Write([]byte(`{"data":[{"id":"server-vision"}]}`))
		case "/props":
			_, _ = w.Write([]byte(`{"n_ctx":8192}`))
		case "/v1/chat/completions":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			requested.Store(body.Model)
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c.Llama.URL = server.URL + "/v1"
	client := New(c, "llamacpp")
	if err := client.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	_, err := client.Complete(context.Background(), core.Request{Kind: "structure", Model: "other", Schema: contractSchema})
	if err == nil || !strings.Contains(err.Error(), "no model ID matching") {
		t.Fatalf("unmatched override error = %v", err)
	}
	if requested.Load() != nil {
		t.Fatal("sent a request to the wrong server model")
	}
}

func TestLlamaRefreshesModelsAfterManagedServerSwitch(t *testing.T) {
	c := testConfig(t)
	c.Models["other"] = config.Model{GGUFPath: "other.gguf"}
	var switched atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			if switched.Load() {
				_, _ = w.Write([]byte(`{"data":[{"id":"other.gguf"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"data":[{"id":"server-vision"}]}`))
			}
		case "/props":
			_, _ = w.Write([]byte(`{"n_ctx":8192}`))
		case "/v1/chat/completions":
			var body struct {
				Model string `json:"model"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Model != "other.gguf" {
				t.Errorf("sent stale server model ID %q", body.Model)
			}
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"{\"ok\":true}"}}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c.Llama.URL = server.URL + "/v1"
	client := New(c, "llamacpp")
	if err := client.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	switched.Store(true)
	result, err := client.Complete(context.Background(), core.Request{Kind: "structure", Model: "other", Schema: contractSchema})
	if err != nil || string(result) != `{"ok":true}` {
		t.Fatalf("completion after model switch = %q, %v", result, err)
	}
}

func TestOllamaFailureScenarios(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	for _, test := range []struct {
		mode string
		want string
	}{
		{mode: "missing", want: "missing"},
		{mode: "invalid-json", want: "invalid JSON"},
		{mode: "server-down", want: "503"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			srv := httptest.NewServer(fakeserver.Server{Model: "qwen2.5vl:7b", Mode: test.mode})
			defer srv.Close()
			c.Ollama.URL = srv.URL
			err := New(c, "ollama").Ready(context.Background())
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Ready error = %v, want %q", err, test.want)
			}
		})
	}

	t.Run("timeout", func(t *testing.T) {
		srv := httptest.NewServer(fakeserver.Server{Mode: "timeout"})
		defer srv.Close()
		c.Ollama.URL = srv.URL
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := New(c, "ollama").Ready(ctx); err == nil || !strings.Contains(err.Error(), "deadline") {
			t.Fatalf("timeout error = %v", err)
		}
	})

	t.Run("unreachable server", func(t *testing.T) {
		srv := httptest.NewServer(fakeserver.Server{})
		url := srv.URL
		srv.Close()
		c.Ollama.URL = url
		err := New(c, "ollama").Ready(context.Background())
		if err == nil || !strings.Contains(err.Error(), "connect") {
			t.Fatalf("unreachable server error = %v", err)
		}
	})

	t.Run("raw chat error", func(t *testing.T) {
		srv := httptest.NewServer(fakeserver.Server{Model: "qwen2.5vl:7b", Mode: "chat-error"})
		defer srv.Close()
		c.Ollama.URL = srv.URL
		client := New(c, "ollama")
		if err := client.Ready(context.Background()); err != nil {
			t.Fatal(err)
		}
		_, err := client.Complete(context.Background(), core.Request{Schema: contractSchema})
		if err == nil || !strings.Contains(err.Error(), "inference failed") {
			t.Fatalf("chat error = %v", err)
		}
	})

	t.Run("cancel", func(t *testing.T) {
		srv := httptest.NewServer(fakeserver.Server{Mode: "timeout"})
		defer srv.Close()
		c.Ollama.URL = srv.URL
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := New(c, "ollama").Ready(ctx); err == nil || !strings.Contains(err.Error(), "canceled") {
			t.Fatalf("cancellation error = %v", err)
		}
	})
}

func TestOllamaAutoPull(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	c.Ollama.AutoPull = true
	for _, mode := range []string{"missing", "missing-pull-error"} {
		t.Run(mode, func(t *testing.T) {
			var pullCalls atomic.Int32
			server := fakeserver.Server{Model: "qwen2.5vl:7b", Mode: mode}
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/pull" {
					pullCalls.Add(1)
					var body struct {
						Model  string `json:"model"`
						Stream bool   `json:"stream"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
						body.Model != "qwen2.5vl:7b" || body.Stream {
						t.Errorf("incorrect /api/pull request: %+v (%v)", body, err)
					}
				}
				server.ServeHTTP(w, r)
			}))
			defer srv.Close()
			c.Ollama.URL = srv.URL
			err := New(c, "ollama").Ready(context.Background())
			if mode == "missing" {
				if err != nil {
					t.Fatalf("auto-pull Ready: %v", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "registry unavailable") {
				t.Fatalf("auto-pull error = %v", err)
			}
			if pullCalls.Load() != 1 {
				t.Fatalf("pull calls = %d, want one", pullCalls.Load())
			}
		})
	}
}

func TestOllamaExplicitUnload(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	c.Models["text"] = config.Model{OllamaTag: "text:latest"}
	for _, mode := range []string{"", "unload-error"} {
		t.Run(mode, func(t *testing.T) {
			fake := fakeserver.Server{Mode: mode}
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/generate" {
					calls.Add(1)
					var body struct {
						Model     string `json:"model"`
						Prompt    string `json:"prompt"`
						KeepAlive int    `json:"keep_alive"`
						Stream    bool   `json:"stream"`
					}
					if err := json.NewDecoder(r.Body).Decode(&body); err != nil ||
						body.Model != "text:latest" || body.Prompt != "" || body.KeepAlive != 0 || body.Stream {
						t.Errorf("incorrect unload request: %+v (%v)", body, err)
					}
				}
				fake.ServeHTTP(w, r)
			}))
			defer server.Close()
			c.Ollama.URL = server.URL
			err := New(c, "ollama").Unload(context.Background(), "text")
			if mode == "" && err != nil {
				t.Fatal(err)
			}
			if mode != "" && (err == nil || !strings.Contains(err.Error(), "unload failed")) {
				t.Fatalf("unload error = %v", err)
			}
			if calls.Load() != 1 {
				t.Fatalf("unload calls = %d", calls.Load())
			}
		})
	}
}

func TestEndpointRedirectGuard(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://example.com/api/version", http.StatusTemporaryRedirect)
	}))
	defer srv.Close()
	c.Ollama.URL = srv.URL
	err := New(c, "ollama").Ready(context.Background())
	if err == nil || !strings.Contains(err.Error(), "allow_cloud") {
		t.Fatalf("redirect endpoint was not blocked by CheckEndpoint: %v", err)
	}
}

func TestStubWindowUsesRequestedTimesAndSlide(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "stub"
	c.Ollama.Fixtures = filepath.Join("..", "..", "testdata", "ollama_fixtures")
	client := New(c, "ollama")
	result, err := client.Complete(context.Background(), core.Request{
		Kind: "window", Profile: "recipe", Schema: json.RawMessage(`{"type":"object"}`),
		Window: [2]float64{10, 20}, MediaID: "carousel-1", Slide: 2,
		Images: []core.Frame{{Time: 1, MediaID: "carousel-1", Slide: 2}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Window  []float64 `json:"window"`
		MediaID string    `json:"media_id"`
		Slide   int       `json:"slide"`
		Events  []struct {
			Time    float64 `json:"t"`
			MediaID string  `json:"media_id"`
			Slide   int     `json:"slide"`
		} `json:"events"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Window) != 2 || response.Window[0] != 10 || response.Window[1] != 20 ||
		response.MediaID != "carousel-1" || response.Slide != 2 || len(response.Events) != 1 ||
		response.Events[0].Time < 10 || response.Events[0].Time > 20 ||
		response.Events[0].MediaID != "carousel-1" || response.Events[0].Slide != 2 {
		t.Fatalf("fixture did not adapt to requested window/slide: %s", result)
	}
}

func TestStubRecipeUsesSuppliedEvidence(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "stub"
	c.Ollama.Fixtures = filepath.Join("..", "..", "testdata", "ollama_fixtures")
	schema, err := os.ReadFile(filepath.Join("..", "..", "profiles", "recipe", "schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	evidence := core.Segment{
		ID: "visual-12.5", Source: "visual", Text: "Mix the ingredients",
		Start: 12.5, End: 13, TimeKnown: true, Estimated: false,
	}
	result, err := New(c, "ollama").Complete(context.Background(), core.Request{
		Kind: "structure", Profile: "recipe", Schema: schema, Evidence: []core.Segment{evidence},
	})
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Ingredients []json.RawMessage `json:"ingredients"`
		Steps       []struct {
			Text        string   `json:"text"`
			Start       *float64 `json:"t_start"`
			End         *float64 `json:"t_end"`
			EvidenceIDs []string `json:"evidence_ids"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(result, &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Ingredients) != 0 || len(response.Steps) != 1 ||
		response.Steps[0].Text != evidence.Text || response.Steps[0].Start == nil || *response.Steps[0].Start != evidence.Start ||
		response.Steps[0].End == nil || *response.Steps[0].End != evidence.End || len(response.Steps[0].EvidenceIDs) != 1 ||
		response.Steps[0].EvidenceIDs[0] != evidence.ID {
		t.Fatalf("stub output was not grounded in supplied evidence: %s", result)
	}

	still := core.Segment{
		ID: "still-event", Source: "visual", Text: "A still image shows a bowl.",
		MediaID: "image-1",
	}
	result, err = New(c, "ollama").Complete(context.Background(), core.Request{
		Kind: "structure", Profile: "recipe", Schema: schema, Evidence: []core.Segment{still},
	})
	if err != nil {
		t.Fatal(err)
	}
	var stillResponse struct {
		Steps []struct {
			Start       *float64 `json:"t_start"`
			End         *float64 `json:"t_end"`
			EvidenceIDs []string `json:"evidence_ids"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(result, &stillResponse); err != nil {
		t.Fatal(err)
	}
	if len(stillResponse.Steps) != 1 || stillResponse.Steps[0].Start != nil ||
		stillResponse.Steps[0].End != nil || len(stillResponse.Steps[0].EvidenceIDs) != 1 ||
		stillResponse.Steps[0].EvidenceIDs[0] != still.ID {
		t.Fatalf("still-image timestamps should remain unknown: %s", result)
	}
}

func TestSchemaValidation(t *testing.T) {
	if err := Validate(contractSchema, []byte(`{"ok":true}`)); err != nil {
		t.Fatal(err)
	}
	if err := Validate(contractSchema, []byte(`{}`)); err == nil {
		t.Fatal("schema violation accepted")
	}
	if err := Validate([]byte(`{"$ref":"https://example.com/schema.json"}`), []byte(`{}`)); err == nil {
		t.Fatal("external schema reference accepted")
	}
}

func TestInvalidModelJSONIsRepairedThenReported(t *testing.T) {
	c := testConfig(t)
	c.Ollama.Mode = "live"
	var chatCalls atomic.Int32
	fake := fakeserver.Server{Model: "qwen2.5vl:7b", Response: "not-json"}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/chat" {
			chatCalls.Add(1)
		}
		fake.ServeHTTP(w, r)
	}))
	defer srv.Close()
	c.Ollama.URL = srv.URL
	client := New(c, "ollama")
	if err := client.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := client.Complete(context.Background(), core.Request{Schema: contractSchema})
	if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
		t.Fatalf("validation error = %v", err)
	}
	if string(raw) != "not-json" {
		t.Fatalf("invalid raw output lost: %q", raw)
	}
	if got := chatCalls.Load(); got != int32(c.VLM.Retries+1) {
		t.Fatalf("chat attempts = %d, want %d", got, c.VLM.Retries+1)
	}
}
