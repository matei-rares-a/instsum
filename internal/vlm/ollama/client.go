package ollama

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
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

	mu       sync.Mutex
	verified map[string]modelInfo
}

type modelInfo struct {
	capabilities map[string]bool
	contextSize  int
}

func New(c config.Config, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = common.NewHTTPClient(c, time.Duration(c.Ollama.Timeout)*time.Second)
	}
	return &Client{Config: c, HTTP: httpClient, verified: make(map[string]modelInfo)}
}

func (c *Client) Ready(ctx context.Context) error {
	if c.Config.Ollama.Mode == "disabled" {
		return fmt.Errorf("Ollama is disabled; set ollama.mode to stub or live")
	}
	if c.Config.Ollama.Mode == "stub" {
		return nil
	}
	return c.ensureModel(ctx, c.Config.VLM.Model, true)
}

// Unload explicitly releases a configured Ollama model without stopping the server.
func (c *Client) Unload(ctx context.Context, logicalModel string) error {
	if c.Config.Ollama.Mode == "stub" {
		return nil
	}
	if c.Config.Ollama.Mode != "live" {
		return fmt.Errorf("Ollama is disabled; set ollama.mode to stub or live")
	}
	model, ok := c.Config.Models[logicalModel]
	if !ok || model.OllamaTag == "" {
		return fmt.Errorf("logical Ollama model %q has no ollama_tag in models configuration", logicalModel)
	}
	endpoint := strings.TrimRight(c.Config.Ollama.URL, "/") + "/api/generate"
	body := map[string]any{"model": model.OllamaTag, "prompt": "", "stream": false, "keep_alive": 0}
	data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodPost, endpoint, body)
	if err != nil {
		return fmt.Errorf("unload Ollama model %q: %w", model.OllamaTag, err)
	}
	var response struct {
		Error string `json:"error"`
	}
	if err := common.Decode(endpoint, data, &response); err != nil {
		return err
	}
	if response.Error != "" {
		return fmt.Errorf("unload Ollama model %q: %s", model.OllamaTag, response.Error)
	}
	return nil
}

func (c *Client) Complete(ctx context.Context, req core.Request, logicalModel string, vision bool) (json.RawMessage, error) {
	if c.Config.Ollama.Mode == "stub" {
		return c.completeFixture(req)
	}
	if c.Config.Ollama.Mode != "live" {
		return nil, fmt.Errorf("Ollama is disabled; set ollama.mode to stub or live")
	}
	if err := c.ensureModel(ctx, logicalModel, vision); err != nil {
		return nil, err
	}
	model := c.Config.Models[logicalModel]
	prompt, images, err := encodeImages(req)
	if err != nil {
		return nil, err
	}
	var schema any
	if err = json.Unmarshal(req.Schema, &schema); err != nil {
		return nil, fmt.Errorf("decode JSON schema: %w", err)
	}
	body := map[string]any{
		"model":  model.OllamaTag,
		"stream": false,
		"messages": []any{map[string]any{
			"role": "user", "content": prompt, "images": images,
		}},
		"format": schema,
		"options": map[string]any{
			"temperature": c.Config.VLM.Temperature,
			"num_ctx":     c.Config.VLM.Context,
			"num_predict": c.Config.VLM.NumPredict,
		},
		"keep_alive": c.Config.Ollama.KeepAlive,
	}
	endpoint := strings.TrimRight(c.Config.Ollama.URL, "/") + "/api/chat"
	data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}
	var response struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := common.Decode(endpoint, data, &response); err != nil {
		return nil, err
	}
	if response.Error != "" {
		return nil, fmt.Errorf("Ollama /api/chat: %s", response.Error)
	}
	if strings.TrimSpace(response.Message.Content) == "" {
		return nil, fmt.Errorf("Ollama /api/chat returned an empty message")
	}
	return json.RawMessage(response.Message.Content), nil
}

func (c *Client) ensureModel(ctx context.Context, logicalModel string, requireVision bool) error {
	model, ok := c.Config.Models[logicalModel]
	if !ok || model.OllamaTag == "" {
		return fmt.Errorf("logical Ollama model %q has no ollama_tag in models configuration", logicalModel)
	}
	c.mu.Lock()
	if cached, ok := c.verified[model.OllamaTag]; ok {
		c.mu.Unlock()
		return checkCapabilities(logicalModel, model.OllamaTag, cached, c.Config.VLM.Context, requireVision)
	}
	defer c.mu.Unlock()
	base := strings.TrimRight(c.Config.Ollama.URL, "/")
	for _, endpoint := range []string{base + "/api/version", base + "/api/tags"} {
		data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("Ollama unavailable; start Ollama and set ollama.base_url: %w", err)
		}
		if strings.HasSuffix(endpoint, "/api/version") {
			var version struct {
				Version string `json:"version"`
			}
			if err := common.Decode(endpoint, data, &version); err != nil {
				return err
			}
			if version.Version == "" {
				return fmt.Errorf("Ollama /api/version response is missing version")
			}
			continue
		}
		var tags struct {
			Models []struct {
				Name  string `json:"name"`
				Model string `json:"model"`
			} `json:"models"`
		}
		if err := common.Decode(endpoint, data, &tags); err != nil {
			return err
		}
		found := false
		for _, installed := range tags.Models {
			if installed.Name == model.OllamaTag || installed.Model == model.OllamaTag {
				found = true
				break
			}
		}
		if !found && c.Config.Ollama.AutoPull {
			pullURL := base + "/api/pull"
			pull, err := common.Do(ctx, c.Config, c.HTTP, http.MethodPost, pullURL, map[string]any{"model": model.OllamaTag, "stream": false})
			if err != nil {
				return fmt.Errorf("pull Ollama model %q: %w", model.OllamaTag, err)
			}
			var pullResponse struct {
				Error string `json:"error"`
			}
			if err := common.Decode(pullURL, pull, &pullResponse); err != nil {
				return err
			}
			if pullResponse.Error != "" {
				return fmt.Errorf("pull Ollama model %q: %s", model.OllamaTag, pullResponse.Error)
			}
			found = true
		}
		if !found {
			return fmt.Errorf("Ollama model %q missing; run ollama pull %s or set ollama.auto_pull", model.OllamaTag, model.OllamaTag)
		}
	}
	showURL := base + "/api/show"
	data, err := common.Do(ctx, c.Config, c.HTTP, http.MethodPost, showURL, map[string]any{"model": model.OllamaTag})
	if err != nil {
		return fmt.Errorf("Ollama model capabilities: %w", err)
	}
	var show struct {
		Capabilities []string       `json:"capabilities"`
		ModelInfo    map[string]any `json:"model_info"`
	}
	if err := common.Decode(showURL, data, &show); err != nil {
		return err
	}
	info := modelInfo{capabilities: make(map[string]bool)}
	for _, capability := range show.Capabilities {
		info.capabilities[capability] = true
	}
	info.contextSize = readContextSize(show.ModelInfo)
	if info.contextSize == 0 {
		info.contextSize = readContextSizeFromJSON(data)
	}
	if err := checkCapabilities(logicalModel, model.OllamaTag, info, c.Config.VLM.Context, requireVision); err != nil {
		return err
	}
	c.verified[model.OllamaTag] = info
	return nil
}

func checkCapabilities(logicalModel, tag string, info modelInfo, contextSize int, vision bool) error {
	if !info.capabilities["completion"] {
		return fmt.Errorf("Ollama model %q (%s) does not report completion capability", logicalModel, tag)
	}
	if vision && !info.capabilities["vision"] {
		return fmt.Errorf("Ollama model %q does not report vision capability; select a vision model", tag)
	}
	if info.contextSize <= 0 {
		return fmt.Errorf("Ollama model %q does not report a context length; cannot verify configured context_size %d", tag, contextSize)
	}
	if contextSize > info.contextSize {
		return fmt.Errorf("configured context_size %d exceeds Ollama model %q context length %d", contextSize, tag, info.contextSize)
	}
	return nil
}

func readContextSize(info map[string]any) int {
	for key, value := range info {
		if strings.HasSuffix(key, ".context_length") || key == "context_length" {
			if number, ok := value.(float64); ok && number > 0 {
				return int(number)
			}
		}
	}
	return 0
}

func readContextSizeFromJSON(data []byte) int {
	var value map[string]any
	if json.Unmarshal(data, &value) != nil {
		return 0
	}
	info, _ := value["model_info"].(map[string]any)
	return readContextSize(info)
}

func encodeImages(req core.Request) (string, []string, error) {
	images := make([]string, 0, len(req.Images))
	var labels []string
	for _, frame := range req.Images {
		data, err := os.ReadFile(frame.Path)
		if err != nil {
			return "", nil, fmt.Errorf("read frame %q: %w", frame.Path, err)
		}
		images = append(images, base64.StdEncoding.EncodeToString(data))
		labels = append(labels, common.FrameLabel(frame, req.MediaID, req.Slide))
	}
	prompt := req.Prompt
	if len(labels) != 0 {
		prompt = strings.Join(labels, ", ") + "\n" + prompt
	}
	return prompt, images, nil
}

var windowRange = regexp.MustCompile(`(?i)window\s+(-?[0-9]+(?:\.[0-9]+)?)\s+to\s+(-?[0-9]+(?:\.[0-9]+)?)\s+seconds`)

func (c *Client) completeFixture(req core.Request) (json.RawMessage, error) {
	name := "window.json"
	if req.Kind == "structure" {
		name = req.Profile + ".json"
	}
	if filepath.Base(name) != name || name == "." {
		return nil, fmt.Errorf("invalid fixture name")
	}
	data, err := os.ReadFile(filepath.Join(c.Config.Ollama.Fixtures, name))
	if err != nil {
		return nil, fmt.Errorf("stub fixture %s: %w", name, err)
	}
	if req.Kind == "structure" {
		return fitStructureFixture(data, req)
	}
	return fitWindowFixture(data, req)
}

func fitWindowFixture(data []byte, req core.Request) (json.RawMessage, error) {
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("decode window fixture: %w", err)
	}
	start, end, found := 0.0, 0.0, false
	if req.Kind == "window" || req.Window[1] > req.Window[0] || req.Window[0] != 0 || req.Window[1] != 0 {
		start, end, found = req.Window[0], req.Window[1], true
	}
	if !found {
		if match := windowRange.FindStringSubmatch(req.Prompt); len(match) == 3 {
			var err error
			start, err = strconv.ParseFloat(match[1], 64)
			if err != nil {
				return nil, fmt.Errorf("parse fixture window start: %w", err)
			}
			end, err = strconv.ParseFloat(match[2], 64)
			if err != nil {
				return nil, fmt.Errorf("parse fixture window end: %w", err)
			}
			found = true
		}
	}
	if !found && len(req.Images) > 0 {
		start, end = req.Images[0].Time, req.Images[0].Time
		for _, frame := range req.Images[1:] {
			if frame.Time < start {
				start = frame.Time
			}
			if frame.Time > end {
				end = frame.Time
			}
		}
		found = true
	}
	if !found {
		if window, ok := output["window"].([]any); ok && len(window) == 2 {
			start, _ = window[0].(float64)
			end, _ = window[1].(float64)
		}
	}
	if end < start {
		return nil, fmt.Errorf("invalid requested window %.3f to %.3f", start, end)
	}
	output["window"] = []float64{start, end}
	mediaID, slide := req.MediaID, req.Slide
	if len(req.Images) > 0 {
		if mediaID == "" {
			mediaID = req.Images[0].MediaID
		}
		if slide == 0 {
			slide = req.Images[0].Slide
		}
	}
	if mediaID != "" {
		output["media_id"] = mediaID
	}
	if slide != 0 {
		output["slide"] = slide
	}
	if events, ok := output["events"].([]any); ok {
		for _, value := range events {
			event, ok := value.(map[string]any)
			if !ok {
				continue
			}
			eventTime := start
			if len(req.Images) > 0 {
				eventTime = req.Images[0].Time
				if eventTime < start {
					eventTime = start
				}
				if eventTime > end {
					eventTime = end
				}
			}
			event["t"] = eventTime
			if mediaID != "" {
				event["media_id"] = mediaID
			}
			if slide != 0 {
				event["slide"] = slide
			}
		}
	}
	result, err := json.Marshal(output)
	return result, err
}

func fitStructureFixture(data []byte, req core.Request) (json.RawMessage, error) {
	var output map[string]any
	if err := json.Unmarshal(data, &output); err != nil {
		return nil, fmt.Errorf("decode structure fixture: %w", err)
	}
	if req.Profile == "recipe" {
		output["equipment"] = []any{}
		output["ingredients"] = []any{}
		steps := make([]any, 0, len(req.Evidence))
		for _, evidence := range req.Evidence {
			if evidence.ID == "" || strings.TrimSpace(evidence.Text) == "" || evidence.Source != "visual" {
				continue
			}
			end := evidence.End
			if end < evidence.Start {
				end = evidence.Start
			}
			var startTime, endTime any = evidence.Start, end
			if !evidence.TimeKnown {
				startTime, endTime = nil, nil
			}
			step := map[string]any{
				"n": len(steps) + 1, "text": evidence.Text,
				"t_start": startTime, "t_end": endTime,
				"source": evidence.Source, "reference_frame": "",
				"estimated": evidence.Estimated, "evidence_ids": []string{evidence.ID},
			}
			if evidence.MediaID != "" {
				step["media_id"] = evidence.MediaID
			}
			if evidence.Slide != 0 {
				step["slide"] = evidence.Slide
			}
			steps = append(steps, step)
		}
		output["steps"] = steps
	} else if req.Profile == "notes" {
		points := make([]any, 0, len(req.Evidence))
		steps := make([]any, 0, len(req.Evidence))
		for _, evidence := range req.Evidence {
			if evidence.ID == "" || strings.TrimSpace(evidence.Text) == "" {
				continue
			}
			points = append(points, evidence.Text)
			steps = append(steps, evidence.Text)
		}
		if len(points) > 0 {
			output["summary"] = points[0]
		} else {
			output["summary"] = "No supplied evidence."
		}
		output["key_points"] = points
		output["steps"] = steps
	}
	result, err := json.Marshal(output)
	return result, err
}
