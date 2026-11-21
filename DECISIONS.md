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
  log per run. It does not install dependencies; without the optional setup it
  retains the existing engine configuration.
  Logs may contain sensitive subprocess diagnostics and require review before
  sharing; the launcher reports log failures rather than pretending to record them.
* For faster repeated reel testing, `start.bat` reads an optional root
  `start-url.txt` before prompting. The local file is gitignored to avoid
  committing a personal URL; missing/blank content preserves the prompt.
* The experimental Windows setup uses project-local micromamba/conda-forge for
  media tools, Python and CPU Tesseract, a verified gallery-dl wheel, and official
  prebuilt Go, CUDA llama.cpp and CPU whisper.cpp binaries. A small Python/PyYAML
  helper derives a temporary effective YAML from the canonical config at launch;
  explicit custom paths and environment overrides remain usable. This is setup
  glue, not an inference sidecar. The launcher uses managed llama.cpp for the
  default executable; no Ollama service is installed or started.
* Hugging Face model filenames, revision and SHA-256 are selected from official
  API metadata at setup time. This checks downloads against the same source, not
  independent pre-pinned model hashes; compatibility and licenses remain to be
  checked on the target machine. Conda packages are not fully lockfile-pinned.
* On multi-GPU machines setup reports eligible NVIDIA-SMI indices without
  rewriting `gpu.device`. Managed llama.cpp resolves the selected physical
  index to a UUID, limits the owned CUDA process to that GPU, and rejects
  conflicting CUDA visibility restrictions; monitoring retains the physical
  NVIDIA-SMI index. This mapping does not configure externally managed engines.
* Windows launch/setup resolve NVIDIA-SMI via explicit `IGX_NVIDIA_SMI_PATH`,
  existing PATH or standard NVIDIA locations; they do not guess between
  DriverStore packages or modify global PATH. The installed Go executable needs
  an explicit rebuild after updating source; see the Windows handoff instructions.
* FFmpeg WAV validation accepts bounded RIFF metadata chunks and verifies PCM
  parameters. CPU Tesseract requests TSV directly without a separate config file.
  Semantic retries retain rejected window output if repair fails. Caption
  ingredient quantities and units require textual support on a matching line;
  this conservative check can reject valid paraphrases and is not proof of
  model accuracy or of association when multiple facts share a line.

## Validation boundaries

The canonical offline command is `go test ./...`; Linux validation also requires
real ffmpeg tools (`IGX_REQUIRE_MEDIA_TOOLS=1`). Fake-server contracts run without
Ollama or llama-server. Native `onnx` compilation is a separate check and does
not load an ONNX Runtime shared library or model. Actual models, CUDA images,
the RTX 3080 10 GB memory budget and the 5-10 annotated-post evaluation still
require explicit live validation. See [ENABLE_OLLAMA.md](./ENABLE_OLLAMA.md).
