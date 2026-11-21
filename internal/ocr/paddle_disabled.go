//go:build !onnx

package ocr

import (
	"context"
	"errors"

	"instsum/internal/config"
	"instsum/internal/core"
)

type paddleUnavailable struct{}

func newPaddle(config.Config) core.OCREngine {
	return paddleUnavailable{}
}

func (paddleUnavailable) Recognize(context.Context, []core.Frame) ([]core.Segment, error) {
	return nil, errors.New("paddle-onnx OCR is unavailable; rebuild with -tags onnx")
}
