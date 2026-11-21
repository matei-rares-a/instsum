package fakeserver

import (
	"encoding/json"
	"net/http"
	"time"
)

// Server is an offline protocol fixture, not an Ollama implementation.
type Server struct {
	Model             string
	Response          string
	Mode              string
	Capabilities      []string
	ContextLength     int
	Models            []string
	ModelCapabilities map[string][]string
	ModelContexts     map[string]int
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if s.Mode == "timeout" {
		select {
		case <-time.After(5 * time.Second):
		case <-r.Context().Done():
		}
		return
	}
	if s.Mode == "server-down" || s.Mode == "down" {
		http.Error(w, `{"error":"fake server unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	if s.Mode == "invalid-json" {
		w.Write([]byte("{"))
		return
	}
	if s.Mode == "error" {
		http.Error(w, `{"error":"fake protocol error"}`, http.StatusInternalServerError)
		return
	}
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/version":
		json.NewEncoder(w).Encode(map[string]any{"version": "fake"})
	case r.Method == "GET" && r.URL.Path == "/api/tags":
		models := []any{}
		if s.Mode != "missing" && s.Mode != "missing-pull-error" {
			names := append([]string(nil), s.Models...)
			if len(names) == 0 {
				names = []string{s.Model}
			}
			for _, name := range names {
				models = append(models, map[string]any{"name": name, "model": name})
			}
		}
		json.NewEncoder(w).Encode(map[string]any{"models": models})
	case r.Method == "POST" && r.URL.Path == "/api/show":
		if s.Mode == "show-error" {
			http.Error(w, `{"error":"model details unavailable"}`, http.StatusInternalServerError)
			return
		}
		var showRequest struct {
			Model string `json:"model"`
		}
		_ = json.NewDecoder(r.Body).Decode(&showRequest)
		capabilities := s.ModelCapabilities[showRequest.Model]
		if capabilities == nil {
			capabilities = s.Capabilities
		}
		if capabilities == nil {
			capabilities = []string{"completion", "vision"}
		}
		contextLength := s.ModelContexts[showRequest.Model]
		if contextLength == 0 {
			contextLength = s.ContextLength
		}
		if contextLength == 0 {
			contextLength = 8192
		}
		json.NewEncoder(w).Encode(map[string]any{"capabilities": capabilities, "model_info": map[string]any{"fake.context_length": contextLength}})
	case r.Method == "POST" && r.URL.Path == "/api/pull":
		if s.Mode == "pull-error" || s.Mode == "missing-pull-error" {
			json.NewEncoder(w).Encode(map[string]any{"error": "registry unavailable"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"status": "success"})
	case r.Method == "POST" && r.URL.Path == "/api/generate":
		if s.Mode == "unload-error" {
			json.NewEncoder(w).Encode(map[string]any{"error": "unload failed"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"done": true, "done_reason": "unload"})
	case r.Method == "POST" && r.URL.Path == "/api/chat":
		if s.Mode == "chat-error" {
			json.NewEncoder(w).Encode(map[string]any{"error": "inference failed"})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"message": map[string]any{"role": "assistant", "content": s.Response}, "done": true})
	default:
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
	}
}
