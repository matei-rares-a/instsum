package asr

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"instsum/internal/config"
	"instsum/internal/core"
)

type transcriber struct {
	config config.Config
	client *http.Client
}

// New constructs the configured Whisper transcriber.
func New(c config.Config) core.Transcriber {
	return &transcriber{
		config: c,
		client: &http.Client{Timeout: time.Duration(c.ASR.Timeout) * time.Second},
	}
}

func (t *transcriber) Transcribe(ctx context.Context, input string) ([]core.Segment, error) {
	switch t.config.ASR.Engine {
	case "disabled":
		return nil, errors.New("ASR is disabled")
	case "whisper-server":
		return t.transcribeServer(ctx, input)
	case "whisper-cli":
		return t.transcribeCLI(ctx, input)
	default:
		return nil, fmt.Errorf("unsupported ASR engine %q", t.config.ASR.Engine)
	}
}

func (t *transcriber) deadlineContext(ctx context.Context) (context.Context, context.CancelFunc) {
	timeout := t.config.ASR.Timeout
	if timeout <= 0 {
		timeout = 300
	}
	return context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
}

func convertToWAV(ctx context.Context, input, output string) error {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		return fmt.Errorf("ffmpeg not found; required to normalize ASR input to 16 kHz mono WAV: %w", err)
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-nostdin", "-v", "error", "-y", "-i", input,
		"-vn", "-ac", "1", "-ar", "16000", "-c:a", "pcm_s16le", output)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		return fmt.Errorf("convert ASR input to 16 kHz WAV: %w: %s", runErr, strings.TrimSpace(string(output)))
	}
	if err := validateWAV(output); err != nil {
		return fmt.Errorf("ffmpeg output is not 16 kHz mono PCM WAV: %w", err)
	}
	return nil
}

func validateWAV(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	header := make([]byte, 12)
	if _, err := io.ReadFull(file, header); err != nil {
		return err
	}
	end := int64(binary.LittleEndian.Uint32(header[4:8])) + 8
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" || end < 12 || end > info.Size() {
		return errors.New("invalid or truncated RIFF/WAVE file")
	}
	var formatOK, dataOK bool
	for pos := int64(12); pos < end; {
		var chunk [8]byte
		if end-pos < 8 {
			return errors.New("truncated WAV chunk header")
		}
		if _, err := io.ReadFull(file, chunk[:]); err != nil {
			return err
		}
		size := int64(binary.LittleEndian.Uint32(chunk[4:]))
		padded := size + size%2
		if padded > end-pos-8 {
			return errors.New("truncated WAV chunk")
		}
		switch string(chunk[:4]) {
		case "fmt ":
			if size < 16 || formatOK {
				return errors.New("invalid WAV format chunk")
			}
			var format [16]byte
			if _, err := io.ReadFull(file, format[:]); err != nil {
				return err
			}
			if binary.LittleEndian.Uint16(format[0:2]) != 1 ||
				binary.LittleEndian.Uint16(format[2:4]) != 1 ||
				binary.LittleEndian.Uint32(format[4:8]) != 16000 ||
				binary.LittleEndian.Uint32(format[8:12]) != 32000 ||
				binary.LittleEndian.Uint16(format[12:14]) != 2 ||
				binary.LittleEndian.Uint16(format[14:16]) != 16 {
				return errors.New("expected 16 kHz, mono, signed 16-bit PCM WAV")
			}
			formatOK = true
		case "data":
			if !formatOK || size%2 != 0 || dataOK {
				return errors.New("invalid WAV data chunk")
			}
			dataOK = true
		}
		pos += 8 + padded
		if _, err := file.Seek(pos, io.SeekStart); err != nil {
			return err
		}
	}
	if !formatOK || !dataOK {
		return errors.New("WAV requires format and data chunks")
	}
	return nil
}

func (t *transcriber) transcribeCLI(parent context.Context, input string) ([]core.Segment, error) {
	if t.config.ASR.Executable == "" {
		return nil, errors.New("ASR executable is not configured")
	}
	if t.config.ASR.ModelPath == "" {
		return nil, errors.New("ASR model_path is not configured")
	}
	ctx, cancel := t.deadlineContext(parent)
	defer cancel()

	dir, err := os.MkdirTemp("", "instsum-asr-*")
	if err != nil {
		return nil, fmt.Errorf("create ASR temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)
	wavPath := filepath.Join(dir, "input.wav")
	if err = convertToWAV(ctx, input, wavPath); err != nil {
		return nil, err
	}

	outputPrefix := filepath.Join(dir, "result")
	args := []string{"-m", t.config.ASR.ModelPath, "-f", wavPath, "-oj", "-of", outputPrefix,
		"-l", languageArg(t.config.ASR.Language), "-nth", strconv.FormatFloat(t.config.ASR.NoSpeechThreshold, 'g', -1, 64)}
	if t.config.ASR.VAD {
		if t.config.ASR.VADModelPath == "" {
			return nil, errors.New("ASR VAD is enabled but vad_model_path is not configured")
		}
		args = append(args, "--vad", "-vm", t.config.ASR.VADModelPath)
	}
	cmd := exec.CommandContext(ctx, t.config.ASR.Executable, args...)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("whisper-cli: %w", ctx.Err())
		}
		return nil, fmt.Errorf("whisper-cli failed: %w: %s", runErr, strings.TrimSpace(string(output)))
	}
	result, err := os.ReadFile(outputPrefix + ".json")
	if err != nil {
		return nil, fmt.Errorf("whisper-cli succeeded but did not produce JSON output: %w", err)
	}
	segments, err := parseOutput(result, "whisper-cli")
	if err != nil {
		return nil, fmt.Errorf("parse whisper-cli output: %w", err)
	}
	return segments, nil
}

func languageArg(language string) string {
	if language == "" {
		return "auto"
	}
	return language
}

func (t *transcriber) transcribeServer(parent context.Context, input string) ([]core.Segment, error) {
	if t.config.ASR.URL == "" {
		return nil, errors.New("ASR server_url is not configured")
	}
	ctx, cancel := t.deadlineContext(parent)
	defer cancel()

	dir, err := os.MkdirTemp("", "instsum-asr-*")
	if err != nil {
		return nil, fmt.Errorf("create ASR temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)
	wavPath := filepath.Join(dir, "input.wav")
	if err = convertToWAV(ctx, input, wavPath); err != nil {
		return nil, err
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := os.Open(wavPath)
	if err != nil {
		return nil, err
	}
	part, err := writer.CreateFormFile("file", filepath.Base(wavPath))
	if err == nil {
		_, err = io.Copy(part, file)
	}
	closeErr := file.Close()
	if err != nil {
		return nil, fmt.Errorf("prepare whisper-server request: %w", err)
	}
	if closeErr != nil {
		return nil, closeErr
	}
	for key, value := range map[string]string{
		"language":        languageArg(t.config.ASR.Language),
		"no_speech_thold": strconv.FormatFloat(t.config.ASR.NoSpeechThreshold, 'g', -1, 64),
		"response_format": "verbose_json",
		"vad":             strconv.FormatBool(t.config.ASR.VAD),
		"vad_model":       t.config.ASR.VADModelPath,
	} {
		if err = writer.WriteField(key, value); err != nil {
			return nil, fmt.Errorf("prepare whisper-server request: %w", err)
		}
	}
	if err = writer.Close(); err != nil {
		return nil, err
	}
	endpoint := strings.TrimRight(t.config.ASR.URL, "/") + "/inference"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("create whisper-server request: %w", err)
	}
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response, err := t.client.Do(request)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("whisper-server: %w", ctx.Err())
		}
		return nil, fmt.Errorf("whisper-server request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("whisper-server returned %s: %s", response.Status, strings.TrimSpace(string(detail)))
	}
	result, err := io.ReadAll(io.LimitReader(response.Body, 16<<20))
	if err != nil {
		return nil, fmt.Errorf("read whisper-server response: %w", err)
	}
	if len(bytes.TrimSpace(result)) == 0 {
		return nil, errors.New("whisper-server returned an empty response")
	}
	return parseOutput(result, "whisper-server")
}

type whisperResponse struct {
	Segments      []whisperSegment `json:"segments"`
	Transcription []whisperSegment `json:"transcription"`
}

type whisperSegment struct {
	Start      json.RawMessage `json:"start"`
	End        json.RawMessage `json:"end"`
	Text       string          `json:"text"`
	Confidence *float64        `json:"confidence"`
	AvgLogprob *float64        `json:"avg_logprob"`
	Timestamps struct {
		From json.RawMessage `json:"from"`
		To   json.RawMessage `json:"to"`
	} `json:"timestamps"`
	Offsets struct {
		From *float64 `json:"from"`
		To   *float64 `json:"to"`
	} `json:"offsets"`
	Tokens json.RawMessage `json:"tokens"`
	Words  []struct {
		Probability *float64 `json:"probability"`
	} `json:"words"`
}

func parseOutput(data []byte, source string) ([]core.Segment, error) {
	var response whisperResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	input := response.Segments
	if input == nil {
		input = response.Transcription
	}
	if input == nil {
		return nil, errors.New("response has no segments or transcription array")
	}
	result := make([]core.Segment, 0, len(input))
	for i, item := range input {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		start, startOK := parseTime(item.Start)
		end, endOK := parseTime(item.End)
		if !startOK || !endOK {
			start, startOK = parseTime(item.Timestamps.From)
			end, endOK = parseTime(item.Timestamps.To)
		}
		if !startOK && item.Offsets.From != nil {
			start, startOK = *item.Offsets.From/1000, true
		}
		if !endOK && item.Offsets.To != nil {
			end, endOK = *item.Offsets.To/1000, true
		}
		if !startOK || !endOK || start < 0 || end < start || !finite(start) || !finite(end) {
			return nil, fmt.Errorf("segment %d has invalid timestamps", i)
		}
		result = append(result, core.Segment{
			Start: start, End: end, Text: text, Source: source,
			Confidence: segmentConfidence(item),
		})
	}
	return result, nil
}

func segmentConfidence(item whisperSegment) float64 {
	if item.Confidence != nil && finite(*item.Confidence) {
		return clamp(*item.Confidence)
	}
	if item.AvgLogprob != nil && !math.IsNaN(*item.AvgLogprob) {
		return clamp(math.Exp(*item.AvgLogprob))
	}
	total, count := 0.0, 0
	var tokens []struct {
		Probability *float64 `json:"p"`
		Confidence  *float64 `json:"probability"`
	}
	_ = json.Unmarshal(item.Tokens, &tokens)
	for _, token := range tokens {
		value := token.Probability
		if value == nil {
			value = token.Confidence
		}
		if value != nil && finite(*value) {
			total += *value
			count++
		}
	}
	if count == 0 {
		for _, word := range item.Words {
			if word.Probability != nil && finite(*word.Probability) {
				total += *word.Probability
				count++
			}
		}
	}
	if count > 0 {
		return clamp(total / float64(count))
	}
	return 0
}

func parseTime(raw json.RawMessage) (float64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err == nil && !math.IsNaN(number) && !math.IsInf(number, 0) {
		return number, true
	}
	var text string
	if err := json.Unmarshal(raw, &text); err != nil {
		return 0, false
	}
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	if value, err := strconv.ParseFloat(text, 64); err == nil && !math.IsNaN(value) && !math.IsInf(value, 0) {
		return value, true
	}
	parts := strings.Split(text, ":")
	if len(parts) != 3 {
		return 0, false
	}
	hours, errH := strconv.ParseFloat(parts[0], 64)
	minutes, errM := strconv.ParseFloat(parts[1], 64)
	seconds, errS := strconv.ParseFloat(parts[2], 64)
	if errH != nil || errM != nil || errS != nil {
		return 0, false
	}
	value := hours*3600 + minutes*60 + seconds
	return value, finite(hours) && finite(minutes) && finite(seconds) && finite(value)
}

func finite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func clamp(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}
