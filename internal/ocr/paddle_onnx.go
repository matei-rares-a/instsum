//go:build onnx

package ocr

import (
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	ort "github.com/yalue/onnxruntime_go"
	"instsum/internal/config"
	"instsum/internal/core"
)

type paddle struct {
	config config.Config
}

func clampOCR(value float64) float64 {
	return math.Max(0, math.Min(1, value))
}

func newPaddle(c config.Config) core.OCREngine {
	return &paddle{config: c}
}

var ortState struct {
	sync.Mutex
	path  string
	ready bool
}

func initializeORT(runtimePath string) error {
	if runtimePath == "" {
		return errors.New("OCR runtime_path is required for paddle-onnx")
	}
	ortState.Lock()
	defer ortState.Unlock()
	if ortState.ready {
		if ortState.path != runtimePath {
			return fmt.Errorf("ONNX Runtime already initialized from %q, cannot switch to %q", ortState.path, runtimePath)
		}
		return nil
	}
	ort.SetSharedLibraryPath(runtimePath)
	if err := ort.InitializeEnvironment(); err != nil {
		return fmt.Errorf("initialize ONNX Runtime: %w", err)
	}
	ortState.path = runtimePath
	ortState.ready = true
	return nil
}

func (p *paddle) Recognize(parent context.Context, frames []core.Frame) ([]core.Segment, error) {
	if err := validatePaddleDevice(p.config.OCR.Device, p.config.GPU.Device); err != nil {
		return nil, err
	}
	if p.config.OCR.DetectionModel == "" || p.config.OCR.RecognitionModel == "" || p.config.OCR.Dictionary == "" {
		return nil, errors.New("paddle-onnx requires detection_model, recognition_model and dictionary")
	}
	if err := initializeORT(p.config.OCR.RuntimePath); err != nil {
		return nil, err
	}
	timeout := p.config.OCR.Timeout
	if timeout <= 0 {
		timeout = 120
	}
	ctx, cancel := context.WithTimeout(parent, time.Duration(timeout)*time.Second)
	defer cancel()

	dictionary, err := loadDictionary(p.config.OCR.Dictionary)
	if err != nil {
		return nil, err
	}
	detector, err := newPaddleSession(p.config, p.config.OCR.DetectionModel, "sigmoid_0.tmp_0")
	if err != nil {
		return nil, fmt.Errorf("load Paddle detection ONNX model: %w", err)
	}
	defer detector.Destroy()
	recognizer, err := newPaddleSession(p.config, p.config.OCR.RecognitionModel, "softmax_2.tmp_0")
	if err != nil {
		return nil, fmt.Errorf("load Paddle recognition ONNX model: %w", err)
	}
	defer recognizer.Destroy()

	result := make([]core.Segment, 0)
	for _, frame := range frames {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		img, err := decodeFrame(frame.Path)
		if err != nil {
			return nil, fmt.Errorf("decode OCR frame %q: %w", frame.Path, err)
		}
		geometry, tensor := preprocessDetection(img)
		output, outputShape, err := runORT(ctx, detector, tensor, ort.NewShape(1, 3, int64(geometry.height), int64(geometry.width)))
		if err != nil {
			return nil, fmt.Errorf("run Paddle detection model: %w", err)
		}
		probabilities, shape, err := probabilityPlane(output, outputShape)
		if err != nil {
			return nil, err
		}
		boxes, err := detectTextBoxes(probabilities, shape.width, shape.height, geometry)
		if err != nil {
			return nil, err
		}
		for _, box := range boxes {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			crop := cropBox(img, box, geometry)
			input, inputShape := preprocessRecognition(crop)
			output, _, err := runORT(ctx, recognizer, input, inputShape)
			if err != nil {
				return nil, fmt.Errorf("run Paddle recognition model: %w", err)
			}
			text, confidence, err := ctcDecode(output, dictionary)
			if err != nil {
				return nil, err
			}
			confidence *= box.confidence
			if text == "" || confidence < p.config.OCR.MinConfidence {
				continue
			}
			result = append(result, core.Segment{
				Start: frame.Time, End: frame.Time, Text: text, Source: "paddle-onnx",
				MediaID: frame.MediaID, Slide: frame.Slide, Confidence: clampOCR(confidence),
			})
		}
	}
	return result, nil
}

func validatePaddleDevice(device string, gpuID int) error {
	switch strings.ToLower(strings.TrimSpace(device)) {
	case "", "cpu":
		return nil
	case "gpu":
		if gpuID < 0 {
			return fmt.Errorf("paddle-onnx GPU device index must be non-negative, got %d", gpuID)
		}
		return nil
	default:
		return fmt.Errorf("paddle-onnx OCR device %q is unsupported; use cpu or gpu", device)
	}
}

func newPaddleSession(c config.Config, model, output string) (*ort.DynamicAdvancedSession, error) {
	options, err := ort.NewSessionOptions()
	if err != nil {
		return nil, fmt.Errorf("create ONNX session options: %w", err)
	}
	defer options.Destroy()
	if strings.EqualFold(strings.TrimSpace(c.OCR.Device), "gpu") {
		cuda, err := ort.NewCUDAProviderOptions()
		if err != nil {
			return nil, fmt.Errorf("initialize CUDA provider options: %w", err)
		}
		defer cuda.Destroy()
		if err = cuda.Update(map[string]string{"device_id": strconv.Itoa(c.GPU.Device)}); err != nil {
			return nil, fmt.Errorf("configure CUDA device %d: %w", c.GPU.Device, err)
		}
		if err = options.AppendExecutionProviderCUDA(cuda); err != nil {
			return nil, fmt.Errorf("CUDA execution provider unavailable for device %d: %w", c.GPU.Device, err)
		}
	}
	session, err := ort.NewDynamicAdvancedSession(model, []string{"x"}, []string{output}, options)
	if err != nil {
		if strings.EqualFold(strings.TrimSpace(c.OCR.Device), "gpu") {
			return nil, fmt.Errorf("create CUDA-backed Paddle session on device %d: %w", c.GPU.Device, err)
		}
		return nil, err
	}
	return session, nil
}

func runORT(ctx context.Context, session *ort.DynamicAdvancedSession, data []float32, shape ort.Shape) ([]float32, ort.Shape, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	input, err := ort.NewTensor(shape, data)
	if err != nil {
		return nil, nil, err
	}
	defer input.Destroy()
	outputs := []ort.Value{nil}
	options, err := ort.NewRunOptions()
	if err != nil {
		return nil, nil, err
	}
	defer options.Destroy()
	finished := make(chan struct{})
	watcherDone := make(chan struct{})
	go func() {
		defer close(watcherDone)
		select {
		case <-ctx.Done():
			_ = options.Terminate()
		case <-finished:
		}
	}()
	runErr := session.RunWithOptions([]ort.Value{input}, outputs, options)
	close(finished)
	<-watcherDone
	if runErr != nil {
		if ctx.Err() != nil {
			if outputs[0] != nil {
				outputs[0].Destroy()
			}
			return nil, nil, ctx.Err()
		}
		if outputs[0] != nil {
			outputs[0].Destroy()
		}
		return nil, nil, runErr
	}
	if outputs[0] == nil {
		return nil, nil, errors.New("ONNX Runtime returned no output tensor")
	}
	defer outputs[0].Destroy()
	tensor, ok := outputs[0].(*ort.Tensor[float32])
	if !ok {
		return nil, nil, fmt.Errorf("Paddle model returned %T, expected float32 tensor", outputs[0])
	}
	values := append([]float32(nil), tensor.GetData()...)
	return values, tensor.GetShape().Clone(), nil
}

func decodeFrame(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	img, _, err := image.Decode(file)
	return img, err
}

type imageGeometry struct {
	width, height  int
	scaleX, scaleY float64
}

func preprocessDetection(img image.Image) (imageGeometry, []float32) {
	bounds := img.Bounds()
	originalWidth, originalHeight := bounds.Dx(), bounds.Dy()
	maxDimension := 960
	scale := math.Min(1, float64(maxDimension)/float64(max(originalWidth, originalHeight)))
	width := max(32, int(math.Ceil(float64(originalWidth)*scale/32))*32)
	height := max(32, int(math.Ceil(float64(originalHeight)*scale/32))*32)
	geometry := imageGeometry{
		width: width, height: height,
		scaleX: float64(width) / float64(originalWidth), scaleY: float64(height) / float64(originalHeight),
	}
	data := make([]float32, 3*height*width)
	fillNormalizedImage(data, width, height, img, bounds.Min.X, bounds.Min.Y)
	return geometry, data
}

func fillNormalizedImage(dst []float32, width, height int, img image.Image, offsetX, offsetY int) {
	means := [3]float64{0.485, 0.456, 0.406}
	deviations := [3]float64{0.229, 0.224, 0.225}
	bounds := img.Bounds()
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + min(bounds.Dy()-1, int(float64(y)*float64(bounds.Dy())/float64(height)))
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + min(bounds.Dx()-1, int(float64(x)*float64(bounds.Dx())/float64(width)))
			r, g, b, _ := img.At(sourceX, sourceY).RGBA()
			pixels := [3]float64{float64(b) / 65535, float64(g) / 65535, float64(r) / 65535}
			for channel := 0; channel < 3; channel++ {
				dst[channel*width*height+y*width+x] = float32((pixels[channel] - means[channel]) / deviations[channel])
			}
		}
	}
}

type textBox struct {
	left, top, right, bottom int
	confidence               float64
}

type planeShape struct {
	width, height int
}

func probabilityPlane(data []float32, shape ort.Shape) ([]float32, planeShape, error) {
	if len(data) < 4 || len(shape) < 2 {
		return nil, planeShape{}, errors.New("Paddle detection output is empty")
	}
	// Paddle DBNet exports produce [1,1,H,W]; the final two dimensions are the map.
	height := int(shape[len(shape)-2])
	width := int(shape[len(shape)-1])
	leading := 1
	for _, dimension := range shape[:len(shape)-2] {
		leading *= int(dimension)
	}
	if width < 2 || height < 2 || leading != 1 || height*width != len(data) {
		return nil, planeShape{}, errors.New("Paddle detection output has invalid dimensions")
	}
	return data, planeShape{width: width, height: height}, nil
}

func detectTextBoxes(probabilities []float32, width, height int, geometry imageGeometry) ([]textBox, error) {
	if width < 1 || height < 1 || len(probabilities) != width*height {
		return nil, errors.New("Paddle detection output dimensions do not match tensor data")
	}
	visited := make([]bool, len(probabilities))
	boxes := make([]textBox, 0)
	const threshold = 0.3
	for start, probability := range probabilities {
		if visited[start] || math.IsNaN(float64(probability)) || math.IsInf(float64(probability), 0) || probability < threshold {
			continue
		}
		queue := []int{start}
		visited[start] = true
		left, right, top, bottom := start%width, start%width, start/width, start/width
		sum, count := 0.0, 0
		for head := 0; head < len(queue); head++ {
			index := queue[head]
			x, y := index%width, index/width
			sum += float64(probabilities[index])
			count++
			left, right = min(left, x), max(right, x)
			top, bottom = min(top, y), max(bottom, y)
			for _, neighbor := range [4]int{index - 1, index + 1, index - width, index + width} {
				if neighbor < 0 || neighbor >= len(probabilities) || visited[neighbor] || probabilities[neighbor] < threshold {
					continue
				}
				nx, ny := neighbor%width, neighbor/width
				if (neighbor == index-1 && x == 0) || (neighbor == index+1 && x == width-1) ||
					(neighbor == index-width && y == 0) || (neighbor == index+width && y == height-1) {
					continue
				}
				if nx < 0 || nx >= width || ny < 0 || ny >= height {
					continue
				}
				visited[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
		if count < 3 || right-left < 1 || bottom-top < 1 {
			continue
		}
		scaleX := float64(geometry.width) / float64(width) / geometry.scaleX
		scaleY := float64(geometry.height) / float64(height) / geometry.scaleY
		boxes = append(boxes, textBox{
			left:       max(0, int(float64(left)*scaleX)-2),
			top:        max(0, int(float64(top)*scaleY)-2),
			right:      int(float64(right+1)*scaleX) + 2,
			bottom:     int(float64(bottom+1)*scaleY) + 2,
			confidence: sum / float64(count),
		})
	}
	sort.SliceStable(boxes, func(i, j int) bool {
		yi := (boxes[i].top + boxes[i].bottom) / 2
		yj := (boxes[j].top + boxes[j].bottom) / 2
		if abs(yi-yj) > 10 {
			return yi < yj
		}
		return boxes[i].left < boxes[j].left
	})
	return boxes, nil
}

func cropBox(img image.Image, box textBox, _ imageGeometry) image.Image {
	bounds := img.Bounds()
	left := clampInt(box.left, 0, bounds.Dx()-1)
	top := clampInt(box.top, 0, bounds.Dy()-1)
	right := clampInt(box.right, left+1, bounds.Dx())
	bottom := clampInt(box.bottom, top+1, bounds.Dy())
	crop := image.NewRGBA(image.Rect(0, 0, right-left, bottom-top))
	for y := top; y < bottom; y++ {
		for x := left; x < right; x++ {
			crop.Set(x-left, y-top, img.At(bounds.Min.X+x, bounds.Min.Y+y))
		}
	}
	return crop
}

func preprocessRecognition(img image.Image) ([]float32, ort.Shape) {
	bounds := img.Bounds()
	const height, maxWidth = 48, 320
	width := min(maxWidth, max(1, int(math.Ceil(float64(bounds.Dx())*height/float64(max(1, bounds.Dy()))))))
	data := make([]float32, 3*height*maxWidth)
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + min(bounds.Dy()-1, int(float64(y)*float64(bounds.Dy())/height))
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + min(bounds.Dx()-1, int(float64(x)*float64(bounds.Dx())/float64(width)))
			r, g, b, _ := img.At(sourceX, sourceY).RGBA()
			pixels := [3]float64{float64(b) / 65535, float64(g) / 65535, float64(r) / 65535}
			for channel := 0; channel < 3; channel++ {
				data[channel*height*maxWidth+y*maxWidth+x] = float32((pixels[channel] - 0.5) / 0.5)
			}
		}
	}
	return data, ort.NewShape(1, 3, height, maxWidth)
}

func loadDictionary(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Paddle recognition dictionary: %w", err)
	}
	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	dictionary := make([]string, 0, len(lines))
	for _, line := range lines {
		if line != "" {
			dictionary = append(dictionary, line)
		}
	}
	if len(dictionary) == 0 {
		return nil, errors.New("Paddle recognition dictionary is empty")
	}
	return dictionary, nil
}

func clampInt(value, low, high int) int {
	return max(low, min(value, high))
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}
