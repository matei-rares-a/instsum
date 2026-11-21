# OCR engines

`ocr.New(config)` selects the configured OCR adapter. The default engine is
Tesseract and remains CPU-only; it invokes Tesseract with TSV output and maps
the configured `ro` and `en` languages to `ron` and `eng`. Selecting a GPU
device for Tesseract returns an explicit unsupported-device error.

## Paddle ONNX

Paddle OCR is optional and is compiled only with the `onnx` build tag:

```sh
go test -tags onnx ./internal/ocr
```

The adapter uses `github.com/yalue/onnxruntime_go` and requires an ONNX Runtime
shared library path, detection and recognition ONNX models, and the matching
recognition dictionary. Build with cgo enabled and a C compiler installed. The
v1.38.0 Go wrapper uses the ONNX Runtime 1.31.0 C API; provide a compatible
shared library at `runtime_path`. The Paddle models must expose the conventional
Paddle OCR inputs/outputs: detection `x` → `sigmoid_0.tmp_0` and recognition
`x` → `softmax_2.tmp_0`. The implementation performs image
normalization/resizing, detection-map box extraction, image crops, and CTC
decoding.

Example configuration:

```yaml
ocr:
  engine: paddle-onnx
  device: cpu
  runtime_path: ./runtime/libonnxruntime.so
  detection_model: ./models/det.onnx
  recognition_model: ./models/rec.onnx
  dictionary: ./models/character_dict.txt
```

CPU execution is the default for this engine. To request CUDA, set
`ocr.device: gpu` and `gpu.device` to the CUDA device index. This requires an
ONNX Runtime build with its CUDA execution provider and compatible CUDA
dependencies available to the process. Provider setup or session creation
errors are returned; the adapter does not silently fall back to CPU. Other
device values are rejected.

Detection and recognition sessions, provider options, and input/output tensors
are released at the end of each `Recognize` call so sequential GPU workloads do
not retain model-session memory. The ONNX Runtime environment remains
initialized process-wide. The build-tagged tests cover device validation and
pure CTC decoding; they do not load models or verify CPU/CUDA inference.
