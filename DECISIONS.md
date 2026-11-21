# Decisions and implementation status

The canonical specification remains authoritative. The continuation plan is now
approved: implement the complete recipe path and verify it offline first; validate
actual models and hardware when access is available. Batch/cache/optimization are
subsequent work. Offline success does not waive live acceptance criteria.

## Approved continuation

Recoverable stage failures continue with an explicitly partial result. Distinguish
no audio, observed no speech, missing recognition and simulated inference. Prefer
whisper.cpp executable/server integration without mandatory cgo; implement optional
PaddleOCR ONNX and explicit Tesseract alternative. Preserve local-only defaults.
Only owned processes may be shut down. Tests using fake servers cannot certify
model accuracy, ONNX/CUDA ABI compatibility or peak VRAM.

## Implemented prototype

* Go is the orchestrator; the standard library handles HTTP, images, subprocesses
  and templates. YAML and JSON Schema use Go libraries rather than bespoke parsers.
* yt-dlp downloads Reels; gallery-dl with instaloader fallback handles post URLs,
  including mixed-media carousels. A video-only result cannot establish that a
  carousel is complete. Downloader protocols are tested with command doubles;
  actual Instagram authentication and scraping remain unverified.
* ffmpeg/ffprobe perform stream probing, 16 kHz audio normalization and
  scene/interval sampling with actual timestamps. Go handles still images,
  perceptual deduplication and sharpness-based reference/step frame selection.
* whisper.cpp CLI/server adapters avoid mandatory cgo. ONNX is explicitly
  build-tagged, allowing the default Go build and tests without native libraries.
  Paddle sessions are released per recognition call; CUDA provider selection
  is explicit and must not silently fall back to CPU. Tesseract is an explicit
  CPU alternative. No real whisper/Paddle model quality is claimed.
* Separate llama.cpp and Ollama adapters share prompts, schemas and contract
  tests, not wire protocols. Vision and text model selection are independent.
  Stub output is identified in both JSON and Markdown. Actual inference and
  version/model compatibility remain live gates.
* Timeline IDs and source/media/time validation connect structured facts to
  observed evidence. Exact OCR quantities use numeric boundaries; profile unit
  aliases preserve the original unit. These checks do not prove that a model
  correctly recognized the source image, speech or recipe.
* Recoverable stage failures retain diagnostics and export partial results.
  Invalid model output is preserved as diagnostic text, never promoted to
  schema-valid facts. CLI codes distinguish complete (0), partial (2), fatal
  (1) and usage (64).
* Owned llama-server processes are health-checked and stopped by owned PID;
  an occupied port is rejected. External servers are never killed. Ollama
  unloading is opt-in because it affects other clients of a shared service.
* Sequential scheduling is the supported prototype policy. The nvidia-smi
  monitor converts MiB to decimal MB, checks physical capacity and cancels
  on an observed budget overrun. It samples the whole GPU, not just this
  process, and is not a hard between-sample memory limit.
* Loopback inference is the default. Explicit `local_hosts` allows trusted
  Compose/local service names without enabling cloud. Other remote endpoints
  require `allow_cloud: true`; redirect targets are checked too.
* CPU packaging is separate from candidate CUDA/native packaging. The
  `cuda-onnx` target consumes an explicitly supplied ONNX Runtime GPU
  distribution rather than silently fetching an unverified native dependency.
* Scoring and opt-in benchmark runners exist. Their synthetic examples are
  not the required real-post evaluation set. Batch, stage cache/resume and
  advanced optimization remain deferred, not implemented.
* The Windows launcher prompts for a URL and records a separate local diagnostic
  log per run. It does not change engine configuration or install dependencies.
  Logs may contain sensitive subprocess diagnostics and require review before
  sharing; the launcher reports log failures rather than pretending to record them.

## Validation boundaries

The canonical offline command is `go test ./...`; Linux validation also requires
real ffmpeg tools (`IGX_REQUIRE_MEDIA_TOOLS=1`). Fake-server contracts run without
Ollama or llama-server. Native `onnx` compilation is a separate check and does
not load an ONNX Runtime shared library or model. Actual models, CUDA images,
the RTX 3080 10 GB memory budget and the 5-10 annotated-post evaluation still
require explicit live validation. See [ENABLE_OLLAMA.md](./ENABLE_OLLAMA.md).
