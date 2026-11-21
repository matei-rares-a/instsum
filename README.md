# instsum
Take a instagram reel/post and summarize recipes or content

## AI assistant context

The primary project context, requirements, constraints, and approved decisions are
defined in [`context/requirements.md`](./context/requirements.md). Read it in full before
any project task and update it when an approved requirement or durable decision changes.

Always-on assistant guidance is provided for common coding tools:

- [`AGENTS.md`](./AGENTS.md) for assistants that support repository agent instructions
- [`.github/copilot-instructions.md`](./.github/copilot-instructions.md) for GitHub Copilot
- [`.kiro/steering/project-context.md`](./.kiro/steering/project-context.md) as Kiro steering
  with `inclusion: always`

These files direct assistants to the primary document; they do not replace it. Other
chat/LLM tools may not automatically load repository instructions, so attach or provide
`context/requirements.md` when using a tool without repository-context support.

## Status

The complete recipe path is implemented as a prototype following [PLAN.md](./PLAN.md).
Offline validation, real-tool validation and real-model/hardware validation are
separate gates. The implementation includes adapters for video/images/carousels,
recognition, engine-neutral inference, evidence timelines, step images and explicit
partial results. Actual model accuracy and the RTX 3080 memory budget are **not
certified by fake-server tests**. Stage cache/resume, batch and advanced concurrent
GPU optimization remain subsequent work.

| Component | Implemented and verified | Still unverified |
|---|---|---|
| Fetch/ordered manifests | Local inputs, downloader command contracts, mixed-media order | Actual Instagram/authentication |
| ffmpeg/ffprobe | Actual generated videos, streams, audio, frame timestamps in Linux | Representative real-post corpus |
| Whisper CLI/server | Subprocess/HTTP contracts, no-speech and error handling | Actual whisper models/CUDA/RO quality |
| Tesseract | Actual image and silent-video overlay OCR in CPU container | Representative RO/EN recipe corpus |
| Paddle ONNX | Tagged Linux tests, native CLI and Docker build-stage compilation | Runtime/model inference, CUDA and model ABI |
| llama.cpp/Ollama | Shared fake-server contracts, stub, repair/fallback/cancellation | Actual model/version parity and quality |
| Evidence/output | Offline recipe/carousel/partial journeys, provenance and images | End-to-end factual accuracy on real posts |
| Packaging/resources | CPU runtime smoke, Compose validation, owned-process/monitor tests | CUDA images, target RTX 3080 peak VRAM |
| Evaluation | Scorer tests, five synthetic examples, live benchmark runner | 5-10 annotated real posts and measurements |

Final offline suites pass on Windows and Linux; Linux also exercises actual
ffmpeg/ffprobe and Tesseract. These checks do not certify live models.

## Run

### Windows launcher

#### Experimental first-time setup

1. On Windows x64, install a compatible NVIDIA driver first and ensure
   `nvidia-smi` works. An RTX 3080 with 10 GB VRAM is the target; setup checks
   the first GPU against the 9000 MB budget. Have Internet access to GitHub,
   conda-forge, PyPI, Hugging Face and Go's module services.
2. Double-click `setup.bat`. It reports GPU, available RAM, model sizes and
   estimated disk space. Review the model licenses and type `DA` to approve the
   multi-GB download. Setup installs Go, ffmpeg, downloaders, Tesseract,
   whisper.cpp, CUDA llama-server and models under `runtime/setup-v1`; it does
   not change system PATH, install drivers or install/start Ollama. Restart
   `setup.bat` after an error; if a conflicting or incomplete file is reported,
   inspect it before removing it. Existing configuration is not overwritten.
3. If setup succeeds, double-click `start.bat`; paste the Instagram URL only
   when no saved URL is configured below. The window stays open. Instagram
   access or login may still require separate configuration.

To retest the same reel without entering it every time, create `start-url.txt`
in the repository root and paste the URL on one line. `start.bat` will use it
automatically; the file is ignored by Git so the private URL is not committed.
Delete the file or leave it empty to return to the interactive prompt. A saved
URL is still validated by the application, and may appear in local diagnostic
logs, so review logs before sharing them.

Setup writes `logs/setup-*.log`; each run writes `logs/instsum-*.log`. On
failure, review the newest relevant log before sharing it. Managed llama-server
may also write `llama-server.log` in the result directory. Logs may contain
personal paths, URLs or subprocess output: remove sensitive data and never
share passwords, cookies or tokens.

This installer is **experimental**: it has not been run end-to-end on a clean
target machine. Hugging Face access was unavailable during development. The
exact selected model files, licenses, CUDA/driver compatibility, actual VRAM
use, Instagram fetch and inference quality require validation on that machine.
Model downloads are verified against SHA-256 values obtained from the official
metadata for a recorded repository revision, not independently pinned hashes.
Conda dependencies are resolved by conda-forge rather than fully lockfile-pinned.
The existing `scripts/download-model.ps1` supports local/mirror sources when
the setup's official Hugging Face source is unavailable.

After setup, `start.bat` reads `configs/config.yaml` each run and creates a
temporary effective configuration without modifying it. Default model and
executable paths point to the local installation; non-default paths are retained.
When the executable remains the default `llama-server`, setup's launcher enables
managed-server mode. To use an externally managed server, set a custom executable
value or `IGX_LLAMACPP_MANAGE_SERVER=false`. Other `IGX_...` overrides remain
available. Temporary configuration is deleted after the run. Without a completed
setup, the original launcher behavior remains: use root `instsum.exe`, or build
temporarily using Go 1.23+ on PATH. `start.bat` never downloads dependencies.

Each invocation writes a separate `logs/instsum-*.log` containing startup
diagnostics, application output, warnings and exit code; the window prints its
path. Results still go to the configured output directory. Code 2 means a partial
result, not a complete run. If the log cannot be created or written, the error
is shown in the window; abrupt termination or filesystem failure can prevent
recording diagnostics.

Logs can contain private paths, URLs or other sensitive information emitted by
external tools. Review and remove sensitive information before sharing a log;
never share cookies, passwords or tokens. Logs are not excluded from Git, so
review them before staging or committing.

Install Go 1.23+, then `go test ./...` and `go build -o instsum ./cmd/instsum`.
Run from the repository root. For a completely offline demonstration using a
local image, set `IGX_VLM_ENGINE=ollama` and `IGX_OLLAMA_MODE=stub`, then run
`instsum --profile recipe image.jpg`. On PowerShell use
`$env:IGX_VLM_ENGINE="ollama"; $env:IGX_OLLAMA_MODE="stub"`. Fixture output
is **not** derived from the image and is prominently marked in both outputs.
Each run creates a new folder under `output.dir`; no previous run is overwritten.
Recognition stages are not secretly stubbed by `ollama.mode: stub`. Install/configure
OCR or expect a partial result. To test all stages without model installations,
use `go test ./...`, which injects explicitly simulated dependencies.

Exit codes: `0` complete execution, `2` partial result exported, `1` fatal error,
`64` invalid usage. Stub execution can finish but remains explicitly simulated.
`result.json` includes run/stage statuses, warnings, evidence IDs, engine mode,
per-stage durations and sampled GPU memory when enabled. Markdown shows the same
partial/stub warnings. A partial result is not acceptance of the full pipeline.

For video, install ffmpeg and ffprobe on PATH. Instagram video URLs also need
yt-dlp and may require `fetch.cookies_file` or `fetch.cookies_from_browser`;
post/carousel URLs use gallery-dl with instaloader fallback, preserving mixed
image/video posts rather than stopping at the first video. Downloaders
may break when Instagram changes and require real-service verification.
The default engine needs a separately started multimodal llama-server, or
`llamacpp.manage_server: true` with configured GGUF/mmproj/executable. Managed
servers use only their own process and refuse an occupied port.

Inference is loopback-only unless a trusted local service hostname is explicitly
listed in `local_hosts`, or cloud is explicitly enabled. Do not enable cloud merely
to reach Compose services. See [ENABLE_OLLAMA.md](./ENABLE_OLLAMA.md) for live activation.

### Local carousel

Pass a JSON manifest instead of a URL; file paths are relative to the manifest.
Array order is slide order, not filename sorting:

```json
{"caption":"Recipe caption","media":[{"path":"slide-1.jpg"},{"path":"slide-2.png"}]}
```

Video entries are also accepted. Images preserve slide identity rather than
inventing elapsed-video timestamps. Original images and selected step/reference
images are copied into the result folder.

### Recognition

Install whisper.cpp and configure `asr.engine: whisper-cli`, its executable and
model path, or select `whisper-server` with an explicitly configured local server.
Enable VAD only with a compatible whisper.cpp build and configured VAD model.
Missing ASR is `unavailable`, absent audio is `no_audio`, and a successfully run
recognizer with no speech is `no_speech`; these are never interchangeable.

The portable config explicitly selects Tesseract (`ron+eng` language data).
`paddle-onnx` is an optional native integration, not a requirement for `go test`
or a basic Go build. See recognition package documentation for native prerequisites.
Missing OCR produces a warning and partial output, not proof that no text exists.
Native build/runtime prerequisites and model input/output requirements are in
[internal/ocr/README.md](./internal/ocr/README.md).

## Configuration and profiles

Edit [configs/config.yaml](./configs/config.yaml). Precedence is defaults, YAML,
`IGX_<SECTION>_<KEY>`, then CLI `--engine`/`--profile`. Validation occurs after
the overrides. Examples: `IGX_VLM_WINDOW_S=5`, `IGX_ASR_MODEL_PATH=...`,
`IGX_GPU_MONITOR=true`. Lists use comma-separated environment values. Model keys
are uppercased with punctuation replaced by underscores, e.g.
`IGX_MODELS_QWEN2_5VL_7B_OLLAMA_TAG`. Unknown YAML settings are rejected, not ignored.

To add a domain without Go changes, copy `profiles/notes` to
`profiles/workout`, set its name and detection hints in `profile.yaml`, edit
`schema.json`, `extract.tmpl` and `render.md.tmpl`, then select
`--profile workout`. Stub mode additionally needs
`testdata/ollama_fixtures/workout.json` matching the schema. Templates are Go
`text/template`; the extraction template receives Metadata, Visual, Transcript
and OCR plus a JSON-serializable evidence Timeline with IDs. Structured facts
must cite existing IDs with matching sources; OCR quantities are checked against
their evidence. Profile `unit_aliases` normalize units while retaining `unit_raw`.
Schema/provenance checks cannot establish whether a model correctly saw the image.

## RTX 3080 sizing

The specification targets 10 GB VRAM with a 9 GB budget. Approximate,
**unverified** figures: Qwen2.5-VL-7B Q4 weights 4.7 GB, projector 1.3 GB,
plus context/image cache and overhead; whisper large-v3-turbo 1.5–2 GB. Run
ASR, OCR and VLM sequentially. `gpu.monitor: true` enables preflight and runtime
sampling through `nvidia-smi`; over-budget observations cancel the active stage.
MB is decimal; NVIDIA's MiB figures are converted explicitly. This samples the
whole GPU, including other users, and cannot guarantee a cap between samples.
Externally managed models may stay resident; no external process is killed.
Reduce context/frame count or release memory before retrying.
`ollama.unload_after_use` defaults to false; enable it only when releasing the
configured models will not disrupt other service clients. Actual target hardware
and model compatibility are still live acceptance prerequisites.

### Packaging and validation

- `docker build --target cpu -t instsum:cpu .`: portable orchestrator, ffmpeg,
  downloaders and Tesseract. Install/provide native models separately.
- `docker build --target cuda -t instsum:cuda .`: candidate CUDA runtime with
  compiled llama.cpp and whisper.cpp. CUDA container build/runtime compatibility
  remains unverified until tested on the chosen environment.
- `docker build --target cuda-onnx -t instsum:cuda-onnx .`: optional candidate
  CUDA runtime with the native-tagged orchestrator. First provide a compatible
  ONNX Runtime GPU distribution under `runtime/onnxruntime/lib` (or set build
  argument `ONNX_RUNTIME_DIR` to another build-context directory). This target
  includes those libraries and selects Paddle GPU OCR; configure the detection,
  recognition and dictionary paths separately. Native artifacts are ignored by
  Git, not downloaded automatically, and their ABI/CUDA compatibility is unverified.
- Compose profiles: `cli`, `inference` (llama-server), `ollama` (optional).
  Configure model files/mounts explicitly. Ollama is never auto-installed/started
  by the Go application.
- `go test ./...`, `go vet ./...`, `go build ./cmd/...`: offline verification.
  Real ffmpeg tests skip explicitly if the executable is missing.
- `docker build --target test -t instsum-offline-test:local .` verifies Linux
  tests with ffmpeg/ffprobe installed and missing tools treated as failures.
- Live tests require `IGX_RUN_LIVE=1`, `IGX_LIVE_INPUT` and correctly configured
  actual engines. They are intentionally not part of offline success claims.
- `go run ./cmd/igx-eval --expected expected.json --actual out/run-.../result.json`
  scores ingredient precision/recall, exact quantities and step-text LCS order.
- `go run ./cmd/igx-bench --allow-live --engine ollama --windows 5,10,15 sample.mp4`
  requires real models, captures stage timings and sampled GPU usage. RSS is the
  orchestrator's process high-water mark, not total inference process RAM.

The five hand-written cases in `testdata/evaluation/synthetic-cases.json` exercise
the scoring format; they are **not** the required 5-10 real-post evaluation set.
Real sample acquisition/annotation and quality acceptance remain a live-stage gate.

Model files are configured under `models`. `scripts/download-model.ps1`
accepts `-Source hf`, `local`, or `mirror-url`, `-From`, and `-Destination`;
remote sources additionally require a verified `-Sha256`. It refuses to
overwrite an existing destination. Models are downloaded only by an explicit
download command or an approved `setup.bat` installation.
