# Project context and requirements: Build `instsum`

This is the project's primary and canonical document for product context, requirements,
constraints, and approved decisions. Every AI assistant or contributor must read it in
full at the start of each task and use it throughout the work.

Approved decisions are reflected in the relevant sections below. Examples, defaults, and
alternatives that are not marked as mandatory remain proposals until reviewed and
approved. Once approved, record the current decision here; `DECISIONS.md` can preserve
the detailed rationale and history.

When project context or a durable decision changes, update this document after the change
is approved. `PLAN.md` describes the implementation plan, and `DECISIONS.md` may record
decision rationale; neither replaces or overrides the current requirements and approved
decisions recorded here. In case of conflict, follow this document and resolve the
conflict explicitly rather than silently deviating.

## 0. READ FIRST: project authority and workflow
### Approved delivery policy

The approved continuation prioritizes a complete recipe pipeline for videos,
images and carousels. Real adapters must be implemented now; offline/fake-server
validation and real-model validation are separate acceptance levels. Real-model
tests are deferred until the runtime and models are accessible. Batch, stage
cache/resume and advanced performance optimization follow the recipe prototype.

Recoverable stage failures continue automatically with explicit warnings and a
partial result. Missing ASR is not silence, missing OCR is not empty observed text,
and simulated model responses are not real evidence. Fatal input, configuration,
output and cancellation errors must still fail explicitly.

Use whisper.cpp through a managed local executable or configured server to avoid
mandatory native build dependencies. PaddleOCR ONNX remains the intended OCR
engine, with explicitly configured Tesseract as an alternative. Native integrations
must not prevent the core Go build or offline tests from running without CUDA.
Windows is the development context; the actual CUDA runtime platform, hardware
access and model versions require confirmation before live acceptance.

The portable prototype configuration explicitly selects CPU Tesseract; optional
Paddle CPU/CUDA execution requires the `onnx` build tag and compatible native
runtime/models. This does not replace the intended Paddle integration. Only
sequential GPU scheduling is supported before measured live optimization.
Ollama unloading is opt-in (`ollama.unload_after_use: false` by default) because
unloading shared models can disrupt other clients. Sampled GPU monitoring is
diagnostic/cancellation protection, not proof of a hard between-sample VRAM cap.
See the implementation/validation matrix in `README.md`; real-model accuracy,
CUDA runtime compatibility and real-post evaluation remain separate gates.
The approved Windows launcher accepts an Instagram URL interactively and writes
per-run local diagnostic logs without changing engine selection or downloading
dependencies. Logs can contain sensitive subprocess output and must be reviewed
before sharing. Logging failures must be visible to the user.

This is the primary project specification, not a step-by-step script. It is VERY
IMPORTANT that you:
1. **Analyze critically before coding.** Evaluate every requirement and technical suggestion below. If you find a faster or more accurate approach, or a requirement that conflicts with another one or with the hardware limits, say so, propose the better option, and justify it. Suggestions marked "e.g." or "default" are starting points, not mandates. The hard constraints in section 2 are mandatory.
2. **Write a plan first** in `PLAN.md`, before writing any implementation code:
   - architecture and data flow, with the chosen engine for every stage and the reason for it
   - **a dedicated section "Ollama vs llama.cpp"** (see section 2.1): your honest assessment, trade-offs, and recommendation, so we can discuss it before implementation
   - the VRAM/RAM budget and the scheduling of GPU stages on the target hardware (section 2)
   - the main risks (model quality, Romanian support, JSON reliability, scraping breakage, untested Ollama adapter) and how to mitigate each one
   - implementation milestones, each ending in something runnable and testable
   - how speed and accuracy will be measured (benchmarks + evaluation set)
   **Stop after writing `PLAN.md` and wait for review before implementing.**
3. **Then build the entire project** following the approved plan, milestone by milestone. Keep `PLAN.md` and `DECISIONS.md` up to date, and update this document whenever an approved requirement or durable decision changes.
4. **Optimize for both speed AND accuracy.** When the two conflict, prefer accuracy for extracted facts (quantities, ingredients, step order) and speed everywhere else. Every important trade-off must be configurable and documented.

## 1. Goal
Input: one Instagram URL (reel, video post, single-image post, or carousel).
Output: a folder containing:
- all information from the post as text: caption, speech, on-screen text, AND what is visually shown (e.g. recipe steps that appear only on screen and are never spoken)
- **structured notes** produced by a domain profile. **Phase 1: recipes.** The system must be designed from day one to be extended to notes in other domains (tutorials/DIY, workouts, tech tips, finance, travel, educational content, etc.)
- a small set of reference images (deduplicated keyframes, one frame per step, original carousel images)
- a machine-readable `result.json` and a human-readable `result.md`

## 2. Hard constraints (mandatory)

### 2.1 Ollama: preferred target engine, NOT available yet
- **Ollama is the preferred long-term engine** for running the vision and text models (free, simple model management, good local performance). It must be a first-class engine in the design.
- **During implementation there is NO access to Ollama** (no API, no model registry, no running instance). Nothing can be tested against a real Ollama server for now.
- Therefore:
  - The project must **build, run, and pass all tests without Ollama**.
  - The **default working engine for now is llama.cpp `llama-server`** (see 2.2).
  - Implement a **complete, ready-to-enable Ollama adapter** behind the same `VideoUnderstander` / `TextModel` interfaces, written against the official Ollama HTTP API documentation:
    - `GET /api/version` and `GET /api/tags` (health check, check that the configured model is installed)
    - `POST /api/show` (model capabilities, e.g. vision support, context length)
    - `POST /api/chat` with `stream: false`, multimodal messages (`images`: base64 frames), `format` set to the profile/window JSON Schema (structured outputs), `options` (`temperature`, `num_ctx`, `num_predict`), and `keep_alive`
    - optional `POST /api/pull` behind a config flag (`ollama.auto_pull`)
    - Ollama does not accept video files: always send ordered frames, exactly like the llama.cpp path
  - The adapter has three modes, set in the config (`ollama.mode`):
    - `disabled`: not used (current default)
    - `stub`: a **dummy implementation** that returns deterministic, schema-valid canned responses from fixture files (`testdata/ollama_fixtures/`), so the full pipeline can run end-to-end with `engine: ollama`. Every output produced in this mode must be clearly marked (`"engine_mode": "stub"` in `result.json` and a warning banner in `result.md`).
    - `live`: real calls to a running Ollama server
  - Provide a **fake Ollama HTTP server** (`internal/vlm/ollama/fakeserver`, also runnable as `cmd/fake-ollama`) that mimics the endpoints above, including error cases (model missing, timeout, invalid JSON, server down). Use it for contract tests of the adapter.
  - **Engine parity tests:** the same contract test suite must run against the llama.cpp engine (mocked server) and the Ollama engine (fake server), proving they are interchangeable.
  - In `live` mode, if Ollama is unreachable or the model is missing, fail with a clear message, or switch to `vlm.fallback_engine` (e.g. `llamacpp`) if configured.
  - A model mapping in the config links each logical model to both an Ollama tag and a GGUF path (e.g. `qwen2.5vl-7b` → `qwen2.5vl:7b` / `./models/Qwen2.5-VL-7B-Instruct-Q4_K_M.gguf`), so switching engines does not change prompts or profiles.
  - Write `ENABLE_OLLAMA.md`: the exact checklist for when access becomes available (install, pull models, set `vlm.engine: ollama` and `ollama.mode: live`, run the contract tests against the real server, run the benchmark to compare with llama.cpp).
  - `docker-compose.yml` includes an `ollama` service under an optional compose profile (`--profile ollama`), disabled by default.

### 2.2 Other inference engines
- **llama.cpp `llama-server`** (default for now): OpenAI-compatible HTTP API, multimodal through GGUF model + `mmproj` file, built with CUDA. Run it as a managed subprocess or a separate container.
- **vLLM** (optional, Linux only, AWQ/GPTQ quantized models; check whether it fits in 10 GB)
- **whisper.cpp** for ASR, **ONNX Runtime** for OCR

### 2.3 Models, hardware, cloud, language
- **Model weights** come from Hugging Face (GGUF / ONNX), a local directory, an internal mirror, or (later) the Ollama registry. The download script must support `--source hf|local|mirror-url`, and every model path must be configurable, because some download sources may be blocked.
- **Target GPU: NVIDIA RTX 3080, 10 GB VRAM** (CUDA). Everything must fit within this budget, with a safety margin of about 1 GB. Approximate figures to verify:
  - Qwen2.5-VL-7B-Instruct Q4_K_M ≈ 4.7 GB + mmproj ≈ 1.3 GB + KV cache (depends on context and on how many images are sent)
  - whisper `large-v3-turbo` ≈ 1.5–2 GB
  - PaddleOCR ONNX: small (it can run on the CPU if VRAM is tight)
  Decide in `PLAN.md` whether models stay loaded at the same time or GPU stages run one after another (e.g. ASR first, then free it, then the VLM). Make this configurable (`gpu.schedule: sequential|concurrent`). The same budget applies when Ollama is the engine (use `keep_alive` and `num_ctx` to control memory).
- **No cloud APIs** unless explicitly enabled (`allow_cloud: true`). The default is fully local.
- **Go is the orchestrator.** Technology rule for each component, in order of preference:
  1. Native Go library
  2. Go bindings over C/C++ (cgo), e.g. whisper.cpp or ONNX Runtime
  3. A C/C++ binary or local HTTP server called from Go (ffmpeg, llama-server, whisper-server, Ollama)
  4. A Python subprocess or sidecar, only when no reasonable alternative exists
  Document every choice in `DECISIONS.md` and keep the current approved decision reflected in this document. Do not reimplement ML models or Instagram scraping in Go.

## 3. Central configuration: `configs/config.yaml`
All tunable parameters live in ONE main config file. Flags and environment variables (`IGX_...`) can override it. Validate it at startup with clear error messages. Ship a fully commented default, starting from this sketch (extend it as needed):

```yaml
output: { dir: ./out, language: auto, max_reference_frames: 8, image_format: jpg }
fetch:  { cookies_from_browser: "", cookies_file: "", rate_limit_per_min: 10, retries: 3 }
gpu:    { device: 0, vram_budget_mb: 9000, schedule: sequential }
asr:
  engine: whispercpp-bindings        # whispercpp-bindings | whisper-server
  model_path: ./models/ggml-large-v3-turbo.bin
  language: auto                     # or "ro"
  vad: true
  no_speech_threshold: 0.6
frames:
  analysis_interval_s: 1.0
  scene_threshold: 0.25
  vlm_frame_size: 768                # longest side, px
  dedup_hamming: 4
ocr:
  engine: paddle-onnx                # paddle-onnx | tesseract | sidecar
  device: gpu                        # gpu | cpu
  langs: [ro, en]
  min_confidence: 0.6
models:                              # logical model -> per-engine identifiers
  qwen2.5vl-7b:
    ollama_tag: qwen2.5vl:7b
    gguf_path: ./models/Qwen2.5-VL-7B-Instruct-Q4_K_M.gguf
    mmproj_path: ./models/mmproj-Qwen2.5-VL-7B-Instruct-f16.gguf
  gemma3-4b:
    ollama_tag: gemma3:4b
    gguf_path: ./models/gemma-3-4b-it-Q4_K_M.gguf
    mmproj_path: ./models/mmproj-gemma-3-4b-it-f16.gguf
vlm:
  engine: llamacpp                   # llamacpp | ollama | vllm | gemini (cloud, needs allow_cloud)
  fallback_engine: llamacpp          # used if the primary engine is unavailable; "" = fail
  model: qwen2.5vl-7b                # key from `models`
  context_size: 8192
  window_s: 10                       # analysis window length
  window_overlap_s: 1
  frames_per_window: 8
  temperature: 0.1
  concurrency: 1
  json_retries: 1
llamacpp:
  server_url: http://127.0.0.1:8080/v1
  manage_server: true                # instsum starts and stops llama-server itself
ollama:
  mode: disabled                     # disabled | stub | live
  base_url: http://127.0.0.1:11434
  keep_alive: 5m
  auto_pull: false
  request_timeout_s: 120
  stub_fixtures_dir: ./testdata/ollama_fixtures
structure:
  profile: auto                      # auto | recipe | notes | <any profile folder name>
  text_model: same_as_vlm            # or a key from `models`
profiles_dir: ./profiles
allow_cloud: false
cache: { dir: ./.cache, enabled: true }
logging: { level: info, format: text }
```

## 4. Pipeline (stages)
1. **Fetch:** `yt-dlp` (subprocess) for video and reels, with `--write-info-json --write-thumbnail`, merging video and audio into MP4. `gallery-dl` (fallback: `instaloader`) for image posts and carousels. Normalize the metadata into one struct: author, date, caption, hashtags, mentions, stats, music track, and media list.
2. **Normalize media:** use ffmpeg to extract 16 kHz mono WAV and `ffprobe` JSON for duration, resolution, and fps.
3. **ASR:** whisper.cpp with Go bindings, CUDA, VAD, and timestamped segments. Detect "music only / no speech" so that no hallucinated transcript is produced.
4. **Frame sampling:**
   - *Analysis frames*: scene detection + an interval floor, resized, with light phash dedup (`goimagehash`) that keeps temporal coverage.
   - *Reference frames*: diverse, deduplicated, plus one sharp frame per step (chosen in stage 7).
5. **OCR:** PaddleOCR (PP-OCR) detection + recognition models in ONNX format, run through `onnxruntime_go`, with Romanian diacritics and English. Fallbacks: gosseract (`ron+eng`) or a Python sidecar. Fuzzy-merge text across frames, keep `first_seen` / `last_seen`, and filter by confidence.
6. **Visual understanding (VLM):** the key stage for information that is shown but not spoken.
   - Split the video into windows of `vlm.window_s` (default **10 s**, aligned to scene boundaries where possible, with an overlap of `window_overlap_s`). For each window, send at most `frames_per_window` ordered frames labeled `[t=12.0s]`, together with the context for that window: caption, transcript segments, and OCR text.
   - The request is built once in an engine-neutral form and translated by each engine: llama.cpp (OpenAI-compatible multi-image chat with `json_schema` / grammar constrained decoding) or Ollama (`/api/chat` with `images` and `format` = JSON Schema).
   - Instructions for the model: describe only what is visible; prefer exact numbers from OCR over estimates; mark estimates with `"estimated": true`; never invent steps.
   - Window schema: `{"window":[s,e],"events":[{"t","action","objects":[],"quantities":[],"on_screen_text","estimated","confidence"}]}`
   - Validate against the schema; on invalid JSON retry `json_retries` times with a repair prompt, then fall back to free text.
   - Carousels: one request per slide, in order (each slide is treated as a step).
   - Candidate models (compared in benchmarks): **Qwen2.5-VL-7B** (default), Qwen2.5-VL-3B (fast), **Gemma 3 4B / 12B** (12B Q4 is tight in 10 GB, verify it), Qwen3-VL-8B (if supported by the engine version in use), MiniCPM-V. Switching models or engines must only require editing the config.
7. **Aggregation + profile structuring**
   - Merge transcript + OCR + visual events into one timeline sorted by timestamp, with duplicates collapsed.
   - Pick a profile (`auto` = classify from the caption, hashtags, and timeline; or set it explicitly in the config/CLI).
   - Run the profile's prompt with its JSON schema over the timeline, using constrained decoding on whichever engine is active.
   - Every extracted item keeps its `source` (`speech|ocr|visual|caption`), its timestamp, and its `estimated` flag.
   - For each step, pick the sharpest frame (Laplacian variance) in its time range and save it as `step_XX.jpg`.
8. **Output:** `result.json` (everything + stats per stage + engine, engine mode, and models used) and `result.md` (Metadata / Structured notes with step thumbnails / Visual timeline / Transcript / On-screen text), rendered from the profile's Markdown template.

## 5. Domain profiles (recipes first, extensible to any domain)
A profile is a **folder**, not code. Adding a new domain must require **zero Go code changes**:
```
profiles/
  recipe/   profile.yaml  schema.json  extract.tmpl  render.md.tmpl  examples/
  notes/    (generic fallback: summary, key points, steps, terms, links, action items)
```
- `profile.yaml`: name, description, detection hints (keywords, hashtags, optional classifier prompt), extra instructions for the VLM stage (e.g. for recipes: "focus on ingredients, quantities, utensils, temperatures, cooking times"), and post-processing options.
- `schema.json`: the output JSON Schema, used for constrained decoding (llama.cpp and Ollama) and validation.
- **recipe** schema (Phase 1, must be excellent): `title, servings, total_time, ingredients[{name, quantity, unit, source, estimated}], equipment[], steps[{n, text, t_start, t_end, source, reference_frame}], tips[]`. Include unit normalization (g, ml, linguriță, lingură, cană…) and Romanian/English terms.
- **notes** (generic) must give useful results for any unknown domain.
- Each profile can ship Ollama stub fixtures, so `ollama.mode: stub` produces plausible output for that profile.
- Document in the README "How to add a new profile", with a worked example (e.g. a `workout` profile).
- VLM prompts take profile-specific hints, so visual extraction is focused on what matters for that domain.

## 6. Architecture requirements
- Interfaces: `Fetcher`, `Transcriber`, `FrameExtractor`, `OCREngine`, `VideoUnderstander`, `TextModel`, `Structurer`, `ProfileRegistry`. Engines are selected by a registry/factory from the config; no engine-specific code outside its own package.
- Concurrency: use `errgroup` and bounded worker pools, pass `context.Context` with timeouts everywhere, and respect `gpu.schedule` and `vram_budget_mb`.
- Manage the lifecycle of `llama-server`: start it, health check, keep it warm in batch mode, shut it down cleanly. For Ollama (live): health check, model presence check, `keep_alive` handling; never start or install Ollama automatically.
- Cache per post ID and stage. Support `--force` and `--from-stage vlm|structure`, to iterate on prompts, models, and engines without re-running ASR/OCR.
- Keep prompt templates in files (`text/template`), editable without recompiling and shared by all engines.
- Use `log/slog`. When a model, tool, or engine is missing, the error message must give the exact fix (path, download command, config key).
- Batch mode with a URL list, concurrency limits, and rate limiting. Missing optional engines degrade gracefully (warning, fallback engine or stage skipped).

## 7. Project structure (adjust in the plan if you find a better layout)
cmd/instsum/   cmd/fake-ollama/
internal/{fetch,media,asr,ocr,vlm/{llamacpp,ollama,ollama/fakeserver,vllm,gemini},llamaserver,structure,profiles,output,config}/
configs/config.yaml   profiles/{recipe,notes}/   prompts/   scripts/ (model download, --source hf|local|mirror)
sidecar/ (optional)   testdata/ (incl. ollama_fixtures/)   bench/

## 8. Deliverables
- `PLAN.md` (written first, including the "Ollama vs llama.cpp" section), `DECISIONS.md`, `ENABLE_OLLAMA.md`, `README.md` (setup on RTX 3080 / CUDA, VRAM table per model, config reference, engine switching, how to add a profile)
- Working code, `Makefile` (CUDA and CPU builds), multi-stage `Dockerfile` (CUDA base, ffmpeg, yt-dlp, gallery-dl, llama.cpp server, whisper.cpp, ONNX Runtime GPU) + `docker-compose.yml` (instsum + llama-server; optional `ollama` service under a compose profile)
- Unit tests: config validation, metadata parsing, dedup, OCR merge, windowing, schema validation, timeline merge, profile loading, recipe structuring (with mocked VLM responses), Ollama request/response mapping
- Contract tests: the same suite against the mocked llama.cpp server and the fake Ollama server, including error cases
- Integration tests on local sample MP4s: a narrated recipe, a **silent recipe with text overlays only**, a carousel; run once with `llamacpp` and once with `ollama.mode: stub`. No network calls in CI.
- **Evaluation set:** 5–10 recipe posts with hand-written expected output, plus a script that scores ingredient recall/precision, quantity accuracy, and step order
- **Benchmark** on 30–60 s reels: time per stage and peak VRAM, compared across Qwen2.5-VL-7B, Qwen2.5-VL-3B, and Gemma 3 4B, and across `window_s` values of 5/10/15. The benchmark must support `--engine ollama` so the llama.cpp vs Ollama comparison can be run as soon as access is available.

## 9. Acceptance criteria
- `instsum <reel-url>` produces `result.json`, `result.md`, and 3–8 distinct reference frames, entirely on the RTX 3080, locally, using llama.cpp
- With `vlm.engine: ollama` and `ollama.mode: stub`, the full pipeline runs end-to-end and the output is clearly marked as stub
- Switching to real Ollama later requires only config changes (`vlm.engine: ollama`, `ollama.mode: live`) and following `ENABLE_OLLAMA.md`; no code changes
- A silent recipe reel produces correct ingredients and ordered steps, each with a timestamp, source, and image, and no invented transcript. On-screen quantities match the OCR text exactly. Estimated quantities are flagged as estimated.
- Changing `window_s`, the model, the engine, or the profile only requires editing `configs/config.yaml`
- A new domain profile can be added by creating a folder in `profiles/`, with no code changes
- Peak VRAM stays below `vram_budget_mb`

## Non-goals
- No reimplementation of Instagram's private API
- No hard dependency on Ollama at build time, test time, or in the default runtime configuration
- No cloud APIs unless `allow_cloud: true`
