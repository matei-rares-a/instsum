# Approved implementation plan

## Scope and acceptance levels

Complete the recipe pipeline for video/Reels, images and ordered carousels.
Implement real adapters now and verify contracts and orchestration offline.
Real-model accuracy, runtime compatibility and hardware measurements remain a
separate gate, not evidence that can be supplied by fake servers.

Recoverable failures produce explicit partial results. A simulated response,
missing stage and observed absence of speech/text must remain distinguishable.
Batch, stage cache/resume and advanced optimization follow this prototype.

## Ollama vs llama.cpp

llama.cpp remains the default working target. Ollama is the preferred longer-term
engine, optional at build/test/runtime, with disabled/stub/live modes and shared
requests, prompts and schemas. Neither real backend is validated merely because
its fake-server contract passes. Vision and text models are independently selected.

## Architecture

Go orchestrates yt-dlp/gallery-dl/instaloader, ffmpeg/ffprobe, whisper.cpp and OCR.
The pipeline uses injected stage interfaces, ordered media manifests, traceable
evidence, per-stage status/timing and separate output rendering. Profile folders
remain data rather than hardcoded domains.

## Milestones and tasks

1. Foundation: T001 synchronize context/decisions/docs; T002 execution contracts
   and dependency injection; T003 configuration precedence, logging and CLI status.
2. Engines: T004 independent llama.cpp/Ollama adapters and text/vision selection;
   T005 shared fake-server contracts and failure scenarios.
3. Media: T006 fetch adapters and ordered local manifests; T007 probing, audio
   normalization, scene/interval sampling and real timestamps.
4. Recognition: T008 whisper.cpp CLI/server with no-speech distinction; T009
   optional Paddle ONNX and explicit Tesseract alternative with temporal OCR merge.
5. Evidence: T010 dedup/sharpness; T011 aligned windows/timeline and shared prompts;
   T012 profile structuring, provenance, exact quantities and semantic validation.
6. Output: T013 portable JSON/Markdown, original slides, step images and warnings.
7. Runtime: T014 owned-process lifecycle, sequential scheduling and GPU telemetry;
   T015 CPU/CUDA packaging, local Compose endpoints and verified model downloads.
8. Offline handoff: T016 complete offline journeys; T017 evaluation/benchmark
   runners; T018 documented implemented/offline/live matrix and opt-in live tests.
9. Deferred live gate: T019 confirm runtime/GPU/models and execute real media,
   evaluation and hardware benchmarks. Fix actual compatibility/quality failures.
10. Later: T020 stage cache/resume; T021 bounded batch; T022 measured optimization.

## Resource policy

Target RTX 3080 10 GB, configured budget 9000 MB. Estimated VLM weights ~4.7 GB,
projector ~1.3 GB and whisper ~1.5-2 GB exclude cache/runtime/image overhead.
Default sequential ASR, release, OCR, release, VLM and text. Do not terminate
externally owned processes. Polling does not prove a hard bound between samples.
Actual peak VRAM/RSS and CUDA compatibility must be measured on the target system.

## Verification

Each task runs focused tests. The canonical offline command remains
`go test ./...`, followed by `go vet ./...` and CLI builds at integration points.
Use local HTTP servers, command helpers and isolated temporary output folders.
Actual media tools, native ONNX builds and real inference are distinct test axes;
skipped tests are not passes. No network/model downloads during test execution.

Critical flows: narrated recipe, silent overlay recipe, still image, carousel,
engine switching, stage failures, invalid JSON, cancellation and output integrity.
Compare exact quantities, evidence IDs, order, times and image links, not just
file existence. Build the evaluation runner for 5-10 annotated recipes and
30-60-second benchmarks across models, engines and 5/10/15-second windows.

## Risks

Romanian recognition, scraping changes, native ABI/CUDA compatibility, JSON
semantics, model context limits and shared GPU residency require explicit evidence.
Current external-tool availability is checked during implementation; no global
installation, model download or external server startup is implicit.

## Delivery state

Milestones 1-8 have prototype implementations and offline tests. Recognition
and inference adapters are real integrations, not implemented by fixtures;
tests separately inject simulated dependencies. ffmpeg/ffprobe are exercised
on generated local videos in Linux. The optional native OCR code compiles with
the `onnx` tag without loading model weights.

CPU packaging is built. CUDA and `cuda-onnx` targets are candidate build recipes,
not tested runtime deliverables. The latter requires an explicitly supplied
compatible ONNX Runtime GPU distribution. Milestone 9 still gates actual
whisper/Paddle/VLM inference, target VRAM, quality evaluation and CUDA image
acceptance. Milestone 10 remains intentionally deferred. These gates must not
be described as completed by the offline suite.

## Approved Windows launcher and diagnostics

This addition does not change the pipeline, engine choice, local-first policy,
sequential GPU scheduling or RTX 3080 memory budget above.

1. Add a root `start.bat` for double-click use. Prompt for an Instagram URL,
   run from the project directory regardless of the initial working directory,
   and pass the URL as one safely handled argument without interpreting shell
   metacharacters. Use the existing configuration and default engine unchanged.
   Detect an available executable or Go toolchain; explain missing prerequisites
   rather than installing or downloading anything.
2. Record a separate, uniquely named local diagnostic log per invocation with
   startup failures, application stdout/stderr, timestamps and the actual exit
   code. Preserve complete/partial/fatal distinctions and display the log path
   and output folder when available. Clearly report a log-creation failure;
   a killed process or inaccessible filesystem cannot be guaranteed to be logged.
   Do not include cookie contents, environment dumps or credentials deliberately;
   warn that tool diagnostics may contain private paths, URLs or other sensitive
   data and should be reviewed before sharing. Ignore generated logs in Git.
3. Document prerequisites, where to enter the URL, log locations, privacy and
   how to share a log in `README.md`. Keep `DECISIONS.md` current if any durable
   implementation decision needs explanation.
4. Verify on Windows with isolated fake executables and local inputs: a URL with
   query-string metacharacters is passed intact; success, partial, missing
   dependencies, launch failure and stderr are visible and logged with correct
   status; missing/unwritable log directory is surfaced. Test both direct and
   double-click-style invocation without accessing Instagram or a real model.

Status: implemented and verified with Windows CLI/launcher tests, including the
real Go build fallback and isolated startup, output and logging failures. Real
Instagram, real inference and GPU acceptance remain separate live gates.
