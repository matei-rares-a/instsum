package vlm

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
	"instsum/internal/vlm/common"
	"instsum/internal/vlm/llamacpp"
	"instsum/internal/vlm/ollama"
)

type backend interface {
	Ready(context.Context) error
	Complete(context.Context, core.Request, string, bool) (json.RawMessage, error)
}

type Client struct {
	Config  config.Config
	Engine  string
	HTTP    *http.Client
	adapter backend
}

func New(c config.Config, engine string) *Client {
	client := &Client{
		Config: c,
		Engine: engine,
		HTTP:   common.NewHTTPClient(c, time.Duration(c.Ollama.Timeout)*time.Second),
	}
	switch engine {
	case "ollama":
		client.adapter = ollama.New(c, client.HTTP)
	case "llamacpp":
		client.adapter = llamacpp.New(c, client.HTTP)
	}
	return client
}

func (c *Client) Ready(ctx context.Context) error {
	if c.adapter == nil {
		return fmt.Errorf("unsupported VLM engine %q", c.Engine)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.adapter.Ready(ctx)
}

// Unload requests release of one Ollama model; it never stops an external server.
// An empty model selects the configured primary vision model.
func (c *Client) Unload(ctx context.Context, logicalModel string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if logicalModel == "" || logicalModel == "same_as_vlm" {
		logicalModel = c.Config.VLM.Model
	}
	unloader, ok := c.adapter.(interface {
		Unload(context.Context, string) error
	})
	if !ok {
		return fmt.Errorf("engine %q does not support model unload", c.Engine)
	}
	return unloader.Unload(ctx, logicalModel)
}

func Validate(schema, data []byte) error {
	return common.Validate(schema, data)
}

func (c *Client) Complete(ctx context.Context, req core.Request) (json.RawMessage, error) {
	if c.adapter == nil {
		return nil, fmt.Errorf("unsupported VLM engine %q", c.Engine)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	logicalModel := req.Model
	if logicalModel == "" {
		if req.Kind == "structure" && c.Config.Structure.TextModel != "" && c.Config.Structure.TextModel != "same_as_vlm" {
			logicalModel = c.Config.Structure.TextModel
		} else {
			logicalModel = c.Config.VLM.Model
		}
	}
	if _, ok := c.Config.Models[logicalModel]; !ok {
		return nil, fmt.Errorf("logical model %q is not configured in models", logicalModel)
	}
	vision := req.Kind == "window" || len(req.Images) > 0
	var last error
	var lastOutput json.RawMessage
	for attempt := 0; attempt <= c.Config.VLM.Retries; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		request := req
		if attempt > 0 {
			request.Prompt += "\nReturn only valid JSON matching the provided schema. Previous response was invalid: " + last.Error()
		}
		result, err := c.adapter.Complete(ctx, request, logicalModel, vision)
		if err != nil {
			return lastOutput, err
		}
		lastOutput = result
		if err := Validate(req.Schema, result); err == nil {
			return result, nil
		} else {
			last = err
		}
	}
	return lastOutput, fmt.Errorf("model output failed schema validation after %d attempts: %w", c.Config.VLM.Retries+1, last)
}
