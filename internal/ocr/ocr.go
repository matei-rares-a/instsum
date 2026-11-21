package ocr

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
)

type engine struct {
	config config.Config
}

// New constructs the configured OCR engine.
func New(c config.Config) core.OCREngine {
	return &engine{config: c}
}

func (e *engine) Recognize(ctx context.Context, frames []core.Frame) ([]core.Segment, error) {
	switch e.config.OCR.Engine {
	case "disabled":
		return nil, errors.New("OCR is disabled")
	case "tesseract":
		if err := validateTesseractDevice(e.config.OCR.Device); err != nil {
			return nil, err
		}
		return e.recognizeTesseract(ctx, frames)
	case "paddle-onnx":
		return newPaddle(e.config).Recognize(ctx, frames)
	default:
		return nil, fmt.Errorf("unsupported OCR engine %q", e.config.OCR.Engine)
	}
}

func validateTesseractDevice(device string) error {
	switch strings.ToLower(strings.TrimSpace(device)) {
	case "", "cpu":
		return nil
	default:
		return fmt.Errorf("tesseract OCR supports only cpu, not %q; use paddle-onnx for CUDA", device)
	}
}

func (e *engine) recognizeTesseract(parent context.Context, frames []core.Frame) ([]core.Segment, error) {
	if e.config.OCR.Executable == "" {
		return nil, errors.New("OCR executable is not configured")
	}
	timeout := e.config.OCR.Timeout
	if timeout <= 0 {
		timeout = 120
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()

	langs := tesseractLanguages(e.config.OCR.Langs)
	segments := make([]core.Segment, 0)
	for _, frame := range frames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		cmd := exec.CommandContext(ctx, e.config.OCR.Executable, frame.Path, "stdout", "-l", langs, "-c", "tessedit_create_tsv=1")
		stderr := boundedDiagnostic{}
		cmd.Stderr = &stderr
		output, err := cmd.Output()
		if err != nil {
			if ctx.Err() != nil {
				return nil, fmt.Errorf("tesseract: %w", ctx.Err())
			}
			return nil, fmt.Errorf("tesseract failed for %q: %w: %s", frame.Path, err, strings.TrimSpace(stderr.String()))
		}
		lines, err := parseTSV(output, e.config.OCR.MinConfidence)
		if err != nil {
			return nil, fmt.Errorf("parse tesseract TSV for %q: %w; stderr: %s", frame.Path, err, strings.TrimSpace(stderr.String()))
		}
		for _, line := range lines {
			segments = append(segments, core.Segment{
				Start: frame.Time, End: frame.Time, Text: line.Text, Source: "tesseract",
				MediaID: frame.MediaID, Slide: frame.Slide, Confidence: line.Confidence,
			})
		}
	}
	return segments, nil
}

type boundedDiagnostic struct {
	buffer bytes.Buffer
}

func (b *boundedDiagnostic) String() string { return b.buffer.String() }

func (b *boundedDiagnostic) Write(data []byte) (int, error) {
	const limit = 4096
	count := len(data)
	if remaining := limit - b.buffer.Len(); remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		_, _ = b.buffer.Write(data)
	}
	return count, nil
}

func tesseractLanguages(languages []string) string {
	if len(languages) == 0 {
		return "ron+eng"
	}
	mapped := make([]string, 0, len(languages))
	seen := make(map[string]bool)
	for _, language := range languages {
		value := strings.TrimSpace(language)
		switch strings.ToLower(value) {
		case "ro":
			value = "ron"
		case "en":
			value = "eng"
		}
		if value != "" && !seen[value] {
			seen[value] = true
			mapped = append(mapped, value)
		}
	}
	if len(mapped) == 0 {
		return "ron+eng"
	}
	return strings.Join(mapped, "+")
}

type recognizedLine struct {
	Text       string
	Confidence float64
}

type lineKey struct {
	page, block, paragraph, line string
}

type lineBuilder struct {
	words      []string
	weighted   float64
	wordLength int
}

func parseTSV(data []byte, minimum float64) ([]recognizedLine, error) {
	reader := csv.NewReader(strings.NewReader(string(data)))
	reader.Comma = '\t'
	reader.FieldsPerRecord = -1
	reader.LazyQuotes = true
	rows, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errors.New("empty TSV output")
	}
	if len(rows[0]) < 12 || rows[0][0] != "level" {
		return nil, errors.New("TSV header is missing required columns")
	}
	builders := make(map[lineKey]*lineBuilder)
	order := make([]lineKey, 0)
	for rowIndex, row := range rows[1:] {
		if len(row) < 12 {
			return nil, fmt.Errorf("row %d has %d columns", rowIndex+2, len(row))
		}
		if row[0] != "5" {
			continue
		}
		confidence, err := strconv.ParseFloat(strings.TrimSpace(row[10]), 64)
		if err != nil || math.IsNaN(confidence) || math.IsInf(confidence, 0) || confidence < 0 {
			continue
		}
		word := strings.TrimSpace(row[11])
		if word == "" {
			continue
		}
		key := lineKey{page: row[1], block: row[2], paragraph: row[3], line: row[4]}
		builder := builders[key]
		if builder == nil {
			builder = &lineBuilder{}
			builders[key] = builder
			order = append(order, key)
		}
		builder.words = append(builder.words, word)
		weight := len([]rune(word))
		if weight == 0 {
			weight = 1
		}
		builder.weighted += confidence * float64(weight)
		builder.wordLength += weight
	}
	result := make([]recognizedLine, 0, len(order))
	for _, key := range order {
		builder := builders[key]
		confidence := builder.weighted / float64(builder.wordLength) / 100
		if confidence < minimum {
			continue
		}
		result = append(result, recognizedLine{
			Text: strings.Join(builder.words, " "), Confidence: math.Max(0, math.Min(1, confidence)),
		})
	}
	return result, nil
}
