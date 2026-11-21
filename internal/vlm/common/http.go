package common

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
)

const maxResponseBytes = 16 << 20

func NewHTTPClient(c config.Config, timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := c.CheckEndpoint("redirect endpoint", req.URL.String()); err != nil {
				return err
			}
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		},
	}
}

func Do(ctx context.Context, c config.Config, client *http.Client, method, endpoint string, body any) ([]byte, error) {
	if err := c.CheckEndpoint("inference endpoint", endpoint); err != nil {
		return nil, err
	}
	if client == nil {
		client = NewHTTPClient(c, 0)
	}
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	request, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	guardedClient := &http.Client{
		Transport: client.Transport,
		Jar:       client.Jar,
		Timeout:   client.Timeout,
	}
	priorRedirectPolicy := client.CheckRedirect
	guardedClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if err := c.CheckEndpoint("redirect endpoint", req.URL.String()); err != nil {
			return err
		}
		if priorRedirectPolicy != nil {
			return priorRedirectPolicy(req, via)
		}
		if len(via) >= 10 {
			return fmt.Errorf("stopped after 10 redirects")
		}
		return nil
	}
	response, err := guardedClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", endpoint, err)
	}
	defer response.Body.Close()
	content, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", endpoint, err)
	}
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s returned %s: %s", endpoint, response.Status, strings.TrimSpace(string(content)))
	}
	return content, nil
}

func Decode(endpoint string, content []byte, target any) error {
	if err := json.Unmarshal(content, target); err != nil {
		return fmt.Errorf("%s returned invalid JSON %q: %w", endpoint, string(content), err)
	}
	return nil
}

func FrameLabel(frame core.Frame, mediaID string, slide int) string {
	if mediaID == "" {
		mediaID = frame.MediaID
	}
	if slide == 0 {
		slide = frame.Slide
	}
	if mediaID == "" && slide == 0 {
		return fmt.Sprintf("[t=%.2fs]", frame.Time)
	}
	if mediaID == "" {
		return fmt.Sprintf("[t=%.2fs slide=%d]", frame.Time, slide)
	}
	return fmt.Sprintf("[t=%.2fs media_id=%q slide=%d]", frame.Time, mediaID, slide)
}
