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

## Approved Windows first-run setup

Goal: a user with a supported Windows x64 machine and no Go, media tools or
models installed can run one setup launcher, then use `start.bat` without manual
PATH editing. Setup is an explicit user action; no downloads or installations
occur merely by launching `start.bat`. A working Internet connection, sufficient
free disk, an NVIDIA driver compatible with the selected CUDA runtime and access
to the upstream artifacts are external prerequisites. Instagram login/access
and model accuracy cannot be guaranteed by installation.

### Architecture and engines

- Add a double-click `setup.bat` and a PowerShell setup implementation. Install
  into project-owned directories and set paths for the launched process rather
  than changing system-wide PATH or other users' installations. Detect Windows
  architecture, disk space, network/artifact errors and GPU/driver prerequisites.
  Never replace user files, models or settings silently; make reruns safe.
- Bootstrap a verified Go toolchain only if required to build the Go CLI.
  Provision versioned, compatible ffmpeg/ffprobe, yt-dlp, gallery-dl and
  instaloader, whisper.cpp plus its model, and CPU Tesseract with both `ron` and
  `eng` language data. Use existing tools only when their versions and required
  functionality are checked. Preserve the supported download-source/mirror
  policy; no credentials or cookie contents in logs.
- Use managed llama.cpp as the working default: provision a compatible CUDA
  `llama-server`, a verified vision GGUF and its matching projector, with a
  real model rather than stub output. Configure executable/model paths and
  managed lifecycle in a temporary setup-generated configuration derived from
  the main configuration each launch; preserve custom paths and user settings.
  `start.bat` selects it and receives project-local tool paths.
- **Ollama vs llama.cpp:** keep llama.cpp as the default because Ollama is not
  available for live validation and must not be a build/runtime dependency.
  Do not automatically install or start Ollama. Once available, the existing
  explicit live configuration remains an optional alternative; switching must
  not force downloads or destroy another user's server/model.
- Preserve `allow_cloud: false`, sequential scheduling, 9000 MB target VRAM
  budget and model unloading policy. OCR runs on CPU to conserve VRAM. The
  32 GB RAM machine may have only 10-12 GB available; preflight reports actual
  available resources and gives actionable remediation rather than assuming
  total RAM or promising that a specific model fits. Download sizes, model
  licenses, CPU/GPU trade-offs and VRAM polling limitations are explained.

### Safety, milestones and acceptance

1. **Preflight and manifest:** report GPU, RAM, estimated disk requirement and
   model inventory, with confirmation before downloads. Pin binary hashes;
   obtain model hashes and repository revision from official source metadata.
   The latter is not independent author verification. Real compatibility and
   licensing require target-machine review.
2. **Provisioning:** create resumable, idempotent project-local setup with a
   unique diagnostic log, integrity checks, safe extraction, no silent
   overwrites, safe cleanup and clear errors. Never modify global environment,
   drivers, third-party services, or existing user configuration without consent.
3. **Runtime integration:** build the CLI; arrange local executables, language
   files and models; make the existing launcher use the generated configuration
   and process-local paths. Ensure managed inference is ready and closed only
   when owned. Preserve exit codes and partial-result diagnostics.
4. **Verification and handoff:** use isolated fake artifacts/command fixtures
   for fresh install, rerun, checksum failure, low disk, missing driver, failed
   download, interruption, existing settings and paths containing spaces.
   Run focused Windows tests and a clean-machine test when possible. A genuine
   full result requires a real local media/model smoke test, then a real
   accessible reel and RTX 3080 measurements; report these as unverified when
   hardware, models or access are unavailable, never as a stub success. Update
   README, DECISIONS and approved requirements with setup, privacy, logging,
   sizes, troubleshooting and remaining live gates.

Speed/accuracy comparison remains the existing 5/10/15-second benchmark and
annotated recipe evaluation; setup itself must not pretend dependency checks
establish model accuracy or guarantee VRAM/RAM compliance.

Status: user approved an experimental implementation to be tried on another
machine. Hugging Face remains inaccessible here; exact model/projector filenames,
model hashes, licenses, compatibility, clean installation and inference remain
unverified. The setup fails with a diagnostic if required metadata cannot be
selected; it does not invent checksums. Binary archive layouts and hashes for
llama.cpp and CUDA were checked locally. `ready` indicates that installation
steps completed, not that real inference, RTX 3080 VRAM or reel accuracy passed.
Offline fixtures cover launcher selection, configuration and selected error
paths; a complete clean-machine installation and live pipeline are still needed.

## Reusable Instagram URL for Windows launcher

### Problem and approach

Repeated `start.bat` runs currently wait for an interactive URL even when the
user is testing the same reel. Add a local, editable `start-url.txt` in the
repository root as an optional input source. If it contains a non-whitespace
URL, `scripts/start.ps1` will use that value without prompting; if the file is
absent or blank, preserve today's prompt. Keep the file out of Git so a user's
reel URL is not accidentally committed. Do not change Go CLI parsing, setup
behavior, inference configuration or cloud/local policy.

### Data flow and behavior

`start.bat` -> `scripts/start.ps1` -> read and trim root `start-url.txt` -> use
the selected URL as the existing single CLI argument -> same logging and exit
code behavior. A blank/missing file follows the existing console prompt. The
normal Go fetcher remains responsible for validating that the input is an
Instagram URL; diagnostics continue to be recorded in the per-run log. The URL
will naturally appear in local process arguments and may appear in diagnostic
logs, so document reviewing the log before sharing. Add a brief README
instruction showing where to paste the URL and how to return to prompting.

### Trade-offs, risks and constraints

- A separate ignored text file is safer and easier to edit than embedding a
  URL containing `&`, `%` or other special characters into a batch command.
- Missing/empty file means backward-compatible prompting; a nonempty file
  skips the prompt and consistently targets the same reel.
- A stale or malformed stored URL will be rejected by the normal application
  path; the launcher must not silently fall back to another URL.
- Preserve the local-only inference default, llama.cpp engine, 9000 MB VRAM
  budget, setup marker checks, logging, and current setup/no-setup launch paths.
- No engine or model is changed, so the existing sequential GPU schedule and
  RTX 3080/available RAM assessment are unaffected.

### Runnable milestones and acceptance

1. Extend Windows launcher selection and tests: a populated file is passed
   byte-for-byte after trimming surrounding whitespace, the prompt is skipped,
   an empty/missing file still prompts, and an invalid stored URL reaches the
   normal CLI validation without being replaced. Exercise special URL
   characters, local logs, and both setup-ready and legacy paths as appropriate.
2. Document creating/editing `start-url.txt`, clearing it to restore the prompt,
   and reviewing logs for privacy. Verify the file is ignored and not altered
   by setup or launcher runs.

Validation uses the existing focused Windows launcher tests and
`git diff --check`; no network, Instagram access, model inference, or GPU
benchmark is required for URL-source behavior. Speed/accuracy benchmarks and
recipe evaluation do not change because this only removes repeated input.

Status: implemented with a gitignored root URL file, interactive fallback,
README instructions and Windows launcher fixtures for legacy and setup-ready
paths. Focused test and diff-check results are recorded in the task response.

## Multi-GPU selection correction

The target setup log reports a GTX 1650 with 4096 MiB followed by an RTX 3080
with 10240 MiB. Setup incorrectly checks only the first GPU and rejects the
machine; the runtime also defaults to `gpu.device: 0`. Fix both paths rather
than merely bypassing the preflight or assuming matching CUDA and NVIDIA-SMI
device ordinals.

1. Enumerate and log physical GPU identity, index and VRAM during setup;
   recognize a suitable 9000 MB-budget device even when it is not first.
   Clearly identify the selected device and fail explicitly if no appropriate
   device exists or selection is ambiguous. Preserve the configured budget,
   do not stop other processes, and do not silently overwrite user settings.
2. Make selection reproducible at runtime using the existing central
   `gpu.device` configuration and `IGX_GPU_DEVICE` override. Document the
   target-machine selection and how to identify the RTX 3080; retain existing
   explicit settings and ensure setup does not imply runtime selection was
   changed when it has not been.
3. Ensure managed llama.cpp offloads only to the intended physical GPU; map
   the configured NVIDIA-SMI device to a stable physical identity for the
   process, rather than assuming CUDA and NVIDIA-SMI ordinals match. Ensure
   monitoring continues to query that same physical device, and account for
   pre-existing visibility restrictions with an explicit error instead of
   silently selecting a different GPU. CPU Whisper and Tesseract remain
   unchanged. Ollama remains optional and externally managed.
4. Add isolated tests for the reported two-GPU case, no suitable device,
   malformed/missing devices, explicit selection and existing environment
   overrides. Test the managed subprocess arguments/environment and monitor
   target without requiring NVIDIA hardware; run focused Go and PowerShell
   tests and update README and DECISIONS. No fake test can prove actual GPU
   placement, VRAM peak, CUDA/model compatibility or reel accuracy.

The existing architecture, local-first policy, sequential scheduling, 9000 MB
budget and Ollama-versus-llama.cpp decision remain unchanged. Actual RAM/VRAM,
5/10/15-second benchmarks and annotated recipe accuracy remain target-machine
validation gates. Do not equate reported total VRAM with currently free VRAM.

Implementation: setup reports the unique suitable GPU and instructs the user
to set its NVIDIA-SMI index in the canonical config; multiple eligible devices
require an explicit `IGX_GPU_DEVICE`. Managed llama.cpp resolves the configured
index to a UUID, validates physical capacity and restricts its subprocess to
that UUID with GPU splitting disabled. Runtime monitoring uses the same
physical index. These checks do not prove actual target-machine placement.

## Approved correction after the real Windows reel run

Status: implemented and verified offline with Go tests, vet, CLI build, setup
helper tests and PowerShell syntax checks. The target-machine live retest remains
outstanding; the actual ASR intermediate and model output are not verified.
This section supplements the existing
architecture, resource policy, risk assessment, verification and dedicated
Ollama vs llama.cpp assessment above.

### Observations and scope

- The saved input WAV is mono 16-bit PCM at 16 kHz, but contains a legitimate
  `LIST` chunk before `data`. The ASR adapter checks fixed header offsets;
  its second ffmpeg-produced temporary WAV still needs an end-to-end regression
  test because that file was deleted during the original run.
- Tesseract reports `Can't open tsv`, exits zero and returns empty stdout when
  the config file is missing. The direct `tessedit_create_tsv=1` diagnostic
  returns a TSV header and rows, but did not establish recognition accuracy.
- The CUDA server selected the RTX 3080 and loaded the model, yet five of six
  VLM responses used window boundaries different from the requested interval.
  Structure output used unsupported timestamps and invented recipe quantities
  and units. VRAM peak was not monitored.
- A PATH adjustment was necessary for NVIDIA-SMI even though it existed; the
  current setup-installed executable is built once, so updated source alone
  will not update the executable used by `start.bat`.

### Milestones and acceptance

1. **Recognition:** validate RIFF/WAVE chunks and PCM parameters without assuming
   the `data` chunk follows `fmt ` immediately; reject truncated, unsupported or
   malformed audio explicitly. Use Tesseract's directly configured TSV output
   rather than relying on a missing `tsv` config file; retain meaningful stderr
   when output is invalid and distinguish no observed text from a failed OCR
   invocation. Test representative FFmpeg WAV headers, malformed inputs, both
   Tesseract exit-zero/empty-output failure and successful TSV/blank-frame cases.
2. **Evidence and inference:** investigate the response path and implement
   bounded repair for invalid window output using the configured retry count.
   The requested interval is orchestration metadata; only retain events whose
   timestamps and evidence remain valid, never silently clamp timestamps or
   promote unvalidated facts. Preserve raw diagnostics on final failure. Improve
   prompts and semantic checks for unknown/null caption timestamps and
   unsupported quantities/units, without weakening provenance validation or
   inventing time, values or visual evidence. Test the observed incorrect window
   boundaries, out-of-range events, caption-only/null timestamps and the
   fabricated ingredient-unit cases, including exhaustion and partial results.
3. **Windows handoff:** make NVIDIA-SMI discovery reliable without changing
   global PATH or guessing among incompatible driver binaries; keep physical
   GPU UUID mapping and explicit failure when ambiguous or missing. Ensure the
   installed executable is rebuilt or explicitly refreshed from updated source
   without re-downloading models or overwriting user configuration; test
   cleanly with isolated fixtures and document the exact target-machine steps.
4. **Verification and delivery:** run focused ASR, OCR, structure, pipeline and
   Windows launcher tests, then `go test ./...`, `go vet ./...` and a build.
   Use local fixtures and mocked servers without network; preserve existing
   user files and do not include reel media or logs in a fix commit. Update
   README and DECISIONS as directly needed. After verified code fixes, commit
   and push per the user's log-fix workflow. Provide the user a target-machine
   retest with refreshed binary, reviewed logs, result JSON/Markdown and sampled
   VRAM; live inference and Romanian accuracy are not established by mocks.

The pipeline remains local-first and sequential: whisper.cpp ASR, CPU Tesseract
OCR and managed llama.cpp VLM/text on the configured RTX 3080; no real Ollama
server is required. The 9000 MB budget and safety margin remain unchanged.
Measure stage times and observed peak VRAM on target hardware; compare recipe
ingredients, exact quantities, unit correctness, sources and ordered steps
against the original evidence and the existing evaluation/benchmark plan.
Do not treat one successful reel or sampled monitoring as proof of all
acceptance criteria or a hard VRAM cap. System RAM availability and model
compatibility are measured on the target machine, not assumed.
