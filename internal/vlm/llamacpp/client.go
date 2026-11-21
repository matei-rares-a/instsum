package llamacpp

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/vlm/common"
)

type Client struct {
	Config config.Config
	HTTP   *http.Client

	mu       sync.RWMutex
	modelIDs []string
	checked  map[string]bool
}

func New(c config.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = common.NewHTTPClient(c, time.Duration(c.Ollama.Timeout)*time.Second)
	}
	return &Client{Config: c, HTTP: httpClient, checked: make(map[string]bool)}
}

func (c *Client) Ready(ctx context.Context) error {
	return c.ensureModel(ctx, c.Config.VLM.Model, true)
}

func (c *Client) ensureModel(ctx context.Context, logicalModel string, requireVision bool) error {
	key := fmt.Sprintf("%s|%t", logicalModel, requireVision)
	c.mu.RLock()
	ready := c.checked[key]
	c.mu.RUnlock()
	if ready {
		return nil
	}
	if c.Config.VLM.Context < 1 {
		return fmt.Errorf("vlm.context_size must be positive")
	}
	configured, ok := c.Config.Models[logicalModel]
	if !ok || configured.GGUFPath == "" {
		return fmt.Errorf("llama.cpp logical model %q has no gguf_path in models configuration", logicalModel)
	}
	if requireVision && configured.MMProjPath == "" {
		return fmt.Errorf("llama.cpp model %q has no mmproj_path; configure the vision projector in models", logicalModel)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.checked[key] {
		return nil
	}
	c.checked = make(map[string]bool)
	{
		endpoint := strings.TrimRight(c.Config.Llama.URL, "/") + "/models"
		data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("llama-server unavailable; start a multimodal llama-server and set llamacpp.server_url: %w", err)
		}
		var response struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if err := common.Decode(endpoint, data, &response); err != nil {
			return err
		}
		var modelIDs []string
		for _, model := range response.Data {
			if id := strings.TrimSpace(model.ID); id != "" {
				modelIDs = append(modelIDs, id)
			}
		}
		if len(modelIDs) == 0 {
			return fmt.Errorf("llama-server /models returned no loaded model ID")
		}
		c.modelIDs = modelIDs
	}
	if _, err := c.selectModelID(logicalModel, configured.GGUFPath); err != nil {
		return err
	}
	if err := c.checkContext(ctx); err != nil {
		return err
	}
	c.checked[key] = true
	return nil
}

func (c *Client) checkContext(ctx context.Context) error {
	if c.Config.VLM.Context < 1 {
		return fmt.Errorf("vlm.context_size must be positive")
	}
	base := strings.TrimRight(c.Config.Llama.URL, "/")
	if strings.HasSuffix(base, "/v1") {
		base = strings.TrimSuffix(base, "/v1")
	}
	endpoint := base + "/props"
	data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodGet, endpoint, nil)
	if err != nil {
		if strings.Contains(err.Error(), "404") {
			return nil
		}
		return fmt.Errorf("read llama-server context settings: %w", err)
	}
	var response map[string]any
	if err := common.Decode(endpoint, data, &response); err != nil {
		return err
	}
	loaded := findContextLimit(response)
	if loaded > 0 && c.Config.VLM.Context > loaded {
		return fmt.Errorf("configured context_size %d exceeds llama-server context limit %d", c.Config.VLM.Context, loaded)
	}
	return nil
}

func findContextLimit(value any) int {
	switch typed := value.(type) {
	case map[string]any:
		for _, key := range []string{"n_ctx", "context_length", "n_ctx_train"} {
			if child, ok := typed[key]; ok {
				if number, ok := child.(float64); ok && number > 0 {
					return int(number)
				}
			}
		}
		for _, child := range typed {
			if limit := findContextLimit(child); limit > 0 {
				return limit
			}
		}
	case []any:
		for _, child := range typed {
			if limit := findContextLimit(child); limit > 0 {
				return limit
			}
		}
	}
	return 0
}

func (c *Client) Complete(ctx context.Context, req core.Request, logicalModel string, vision bool) (json.RawMessage, error) {
	c.mu.RLock()
	_, configured := c.Config.Models[logicalModel]
	model := c.Config.Models[logicalModel]
	c.mu.RUnlock()
	if !configured {
		return nil, fmt.Errorf("logical llama.cpp model %q is not configured", logicalModel)
	}
	if err := c.ensureModel(ctx, logicalModel, vision); err != nil {
		return nil, err
	}
	c.mu.RLock()
	modelID, err := c.selectModelID(logicalModel, model.GGUFPath)
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}
	content := []any{map[string]any{"type": "text", "text": req.Prompt}}
	for _, frame := range req.Images {
		data, err := os.ReadFile(frame.Path)
		if err != nil {
			return nil, fmt.Errorf("read frame %q: %w", frame.Path, err)
		}
		encoded := base64.StdEncoding.EncodeToString(data)
		content = append(content,
			map[string]any{"type": "text", "text": common.FrameLabel(frame, req.MediaID, req.Slide)},
			map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/jpeg;base64," + encoded}},
		)
	}
	var schema any
	if err := json.Unmarshal(req.Schema, &schema); err != nil {
		return nil, fmt.Errorf("decode JSON schema: %w", err)
	}
	body := map[string]any{
		"model": modelID,
		"messages": []any{map[string]any{
			"role": "user", "content": content,
		}},
		"temperature": c.Config.VLM.Temperature,
		"max_tokens":  c.Config.VLM.NumPredict,
		"response_format": map[string]any{"type": "json_schema", "json_schema": map[string]any{
			"name": "result", "schema": schema,
		}},
	}
	endpoint := strings.TrimRight(c.Config.Llama.URL, "/") + "/chat/completions"
	data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	var response struct {
		Error   any `json:"error"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := common.Decode(endpoint, data, &response); err != nil {
		return nil, err
	}
	if response.Error != nil {
		return nil, fmt.Errorf("llama-server: %v", response.Error)
	}
	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("llama-server returned no choices: %s", string(data))
	}
	if strings.TrimSpace(response.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("llama-server returned empty message content")
	}
	return json.RawMessage(response.Choices[0].Message.Content), nil
}

func (c *Client) selectModelID(logicalModel, ggufPath string) (string, error) {
	if len(c.modelIDs) == 1 {
		primary := c.Config.Models[c.Config.VLM.Model]
		if logicalModel == c.Config.VLM.Model || ggufPath == primary.GGUFPath {
			return c.modelIDs[0], nil
		}
	}
	for _, id := range c.modelIDs {
		if id == logicalModel || id == ggufPath || filepath.Base(id) == filepath.Base(ggufPath) {
			return id, nil
		}
	}
	return "", fmt.Errorf("llama-server has no model ID matching logical model %q; server IDs: %v", logicalModel, c.modelIDs)
}
