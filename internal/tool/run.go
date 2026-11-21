package tool

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}
type Exec struct{}
type limitedBuffer struct {
	bytes.Buffer
	remaining int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > b.remaining {
		p = p[:b.remaining]
	}
	b.remaining -= len(p)
	_, err := b.Buffer.Write(p)
	return n, err
}
func (Exec) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if _, err := exec.LookPath(name); err != nil {
		return nil, fmt.Errorf("%s unavailable: install it and configure its executable/PATH: %w", name, err)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	b := &limitedBuffer{remaining: 8 << 20}
	cmd.Stdout = b
	cmd.Stderr = b
	err := cmd.Run()
	if err != nil {
		return b.Bytes(), fmt.Errorf("%s failed: %w: %s", name, err, b.String())
	}
	if b.remaining == 0 {
		return nil, fmt.Errorf("%s output exceeded 8 MiB diagnostic limit", name)
	}
	return b.Bytes(), nil
}
