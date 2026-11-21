# Enabling and validating real Ollama

The live adapter has only been exercised against a fake server, not a real
Ollama instance or model. Do not treat this checklist as proof of parity.

1. Install Ollama locally using its official instructions and start it.
2. Choose a vision-capable model supported by your installed Ollama version,
   for example `ollama pull qwen2.5vl:7b`. Verify the actual tag via
   `ollama list`; update `models.<logical-name>.ollama_tag` if necessary.
3. Configure `vlm.engine: ollama`, `ollama.mode: live`, a loopback
   `ollama.base_url`, and an appropriate `ollama.keep_alive` and
   `vlm.context_size`. `ollama.auto_pull` is false by default.
   Select `structure.text_model: same_as_vlm`, or configure and explicitly
   install the separate text model too. Readiness checks distinguish vision
   from text capabilities. Set `vlm.fallback_engine: ""` during acceptance
   so a fallback cannot hide an Ollama failure.
4. Run `go test ./...` (these are **mock/fake** contracts), then manually
   exercise `/api/version`, `/api/tags`, `/api/show` and a multimodal
   schema-constrained `/api/chat` against the real server. Check responses and
   capability/context support for the exact installed version and model.
5. Install/configure ffmpeg, whisper.cpp and OCR as described in
   [README.md](./README.md); real Ollama does not replace these stages.
   Then run the opt-in recipe test from the repository root, for example
   in PowerShell:

   ```powershell
   $env:IGX_RUN_LIVE = "1"
   $env:IGX_LIVE_INPUT = (Resolve-Path .\samples\recipe.mp4).Path
   $env:IGX_VLM_ENGINE = "ollama"
   $env:IGX_OLLAMA_MODE = "live"
   go test -count=1 -timeout 20m ./internal/pipeline -run '^TestLiveRecipeOptIn$' -v
   Remove-Item Env:IGX_RUN_LIVE
   ```

   The test requires a complete result and enables the GPU monitor. Its
   temporary export is cleaned up by Go. For persistent inspection run
   `go run ./cmd/instsum --engine ollama --profile recipe sample.mp4`.
   A partial export exits with code 2; it is not acceptance.
6. Run representative local videos, inspect source attribution, quantities and
   timestamps, and compare with llama.cpp. Measure stage latency and peak VRAM
   under the configured budget before relying on it. Scoring and benchmark
   runners are implemented, but no real-model results are claimed:

   ```text
   go run ./cmd/igx-eval --expected expected.json --actual out/run-.../result.json
   go run ./cmd/igx-bench --allow-live --engine ollama --models qwen2.5vl-7b,gemma3-4b --windows 5,10,15 sample.mp4
   go run ./cmd/igx-bench --allow-live --engine llamacpp --models qwen2.5vl-7b,gemma3-4b --windows 5,10,15 sample.mp4
   ```

   Install every selected model explicitly. For a llama.cpp model matrix use
   `llamacpp.manage_server: true`; an external server must instead be switched
   manually to the matching model. Supply 5-10 annotated real posts separately;
   the bundled synthetic scorer examples are not quality evidence.

For missing models the adapter gives the tag and a pull command. A configured
`vlm.fallback_engine: llamacpp` permits fallback on a failed readiness check;
it does not recover errors during inference. Neither installation nor startup
of Ollama is automatic.

`ollama.unload_after_use` defaults to false. Enable it only when this application
may release its configured models after use: unloading can affect other clients
of a shared service. `keep_alive` otherwise controls residency; externally
resident models can invalidate sequential VRAM estimates. Monitoring samples the
whole GPU and does not guarantee a hard cap between samples. Never stop another
user's server to obtain a passing benchmark.
