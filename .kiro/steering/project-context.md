---
inclusion: always
---

# Project context and required workflow

At the start of every task, read the complete `context/requirements.md`. It is the
primary source for project context, requirements, constraints, and approved decisions,
and must remain in context through analysis, planning, implementation, and verification.
Also follow the root `AGENTS.md`; these steering notes are reminders, not substitutes
for the full specification.

Do not silently weaken a hard requirement. If a request or technical suggestion conflicts
with the specification, state the conflict and propose a justified alternative. Update
`context/requirements.md` when an approved requirement or durable decision changes;
`PLAN.md` and `DECISIONS.md` are supporting documents and must stay consistent with it.

## Before implementation

Create or update `PLAN.md` first, covering the architecture and data flow, engine choice
for every stage, a dedicated “Ollama vs llama.cpp” assessment, hardware and VRAM/RAM
budget, risks and mitigations, runnable/testable milestones, and speed/accuracy
measurement, as required by `context/requirements.md`. Stop after writing the plan and
wait for user review and approval; do not write implementation code before approval.

After approval, implement milestone by milestone, keep `PLAN.md` and `DECISIONS.md`
current, and verify each milestone with appropriate tests.

## Invariants

- The product is `instsum`, a Go CLI that extracts structured notes and reference
  images from Instagram posts/reels. Recipes are phase one; new domains must be addable
  as profiles without Go code changes.
- Preserve caption, speech, on-screen text, and visual evidence. Keep source and time
  information, do not invent facts or speech, prefer exact OCR values, and flag estimates.
- Local inference is the default. Cloud APIs require `allow_cloud: true`.
- llama.cpp is the working default while Ollama is unavailable. Ollama remains an
  optional, first-class adapter, but a real Ollama server must not be required to build,
  run, or test the project.
- Respect the configured target RTX 3080 10 GB VRAM budget and GPU schedule.
- Keep tunable settings in `configs/config.yaml`, validate them clearly, isolate
  engine-specific code, and follow the technology preferences and interfaces in
  `context/requirements.md`.
- Prefer accuracy for extracted facts (quantities, ingredients, step order); document
  and configure important trade-offs.

The full `context/requirements.md` defines the pipeline, interfaces, schemas, deliverables,
testing requirements, acceptance criteria, and non-goals. Consult it rather than relying
on this summary.

