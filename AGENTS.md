# Project guidance for AI assistants

## Required project context

At the start of every task in this repository, read `context/requirements.md` in full.
It is the primary source for project context, requirements, constraints, and approved
decisions; keep it in view throughout analysis, planning, implementation, and verification.

`PLAN.md` describes implementation, and `DECISIONS.md` may contain decision rationale;
neither supersedes the approved decisions in `context/requirements.md`. If a request
conflicts with this document or a hard constraint, explain the conflict and propose a
justified alternative before proceeding. Do not silently discard or weaken requirements.

## Workflow

- Analyze the request and relevant repository state before changing code.
- Before any implementation, create or update `PLAN.md` with the architecture, data flow,
  engine choices, Ollama-versus-llama.cpp assessment, hardware/memory budget, risks,
  testable milestones, and speed/accuracy evaluation described in
  `context/requirements.md`.
- Stop after writing the plan and wait for the user's review and approval. Do not implement
  until it is approved.
- After approval, implement incrementally against the approved plan. Keep `PLAN.md` and
  `DECISIONS.md` current, and verify each milestone with appropriate tests.
- Prefer accuracy for extracted facts (especially quantities, ingredients, and step
  order); optimize speed elsewhere. Document and configure meaningful trade-offs.

## Product and engineering invariants

- Build `instsum`, a Go CLI that turns Instagram posts/reels into structured notes and
  reference images. Phase one is recipes; domain profiles must remain extensible without
  Go code changes.
- Preserve information from captions, speech, on-screen text, and visual content.
  Distinguish evidence sources and timestamps; do not invent facts or speech. Prefer exact
  OCR values and mark estimates.
- Keep inference local by default. Never use cloud APIs unless `allow_cloud: true`.
- llama.cpp is the working default while Ollama is unavailable. Ollama must remain an
  optional, first-class, engine-neutral adapter; the project must build, run, and test
  without a real Ollama service.
- Respect the target RTX 3080 10 GB VRAM budget and configured safety margin. Make GPU
  scheduling explicit and avoid requiring all models to stay resident concurrently.
- Centralize tunable settings in `configs/config.yaml`; support documented flags and
  `IGX_...` environment overrides, with clear startup validation errors.
- Keep model/engine-specific code inside its own package and use shared interfaces,
  context-aware timeouts, bounded concurrency, and graceful handling of optional engines.
- Do not reimplement machine-learning models or Instagram scraping in Go. Follow the
  technology preference and document component choices in `DECISIONS.md`.
- Treat `context/requirements.md` as the complete source for the required pipeline, schemas,
  profiles, tests, deliverables, acceptance criteria, and non-goals; this summary does
  not replace it.

