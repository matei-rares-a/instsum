# Always-on project instructions

At the beginning of every task, read the complete `context/requirements.md`. It is the
primary source for project context, requirements, constraints, and approved decisions;
this instruction file is only a reminder and does not replace it. Also follow the root
`AGENTS.md` and keep both documents' requirements in context through the task.

Do not silently deviate from hard constraints. If the task or a technical suggestion
conflicts with the specification, explain the trade-off and propose an alternative.
When a requirement or durable decision is approved to change, update
`context/requirements.md`. `PLAN.md` and `DECISIONS.md` are supporting documents and must
not contradict it.

Before writing implementation code, create/update `PLAN.md` with all items required by
`context/requirements.md`, including the dedicated “Ollama vs llama.cpp” assessment.
Then stop and wait for user review and approval. After approval, implement milestone by
milestone and keep `PLAN.md` and `DECISIONS.md` current.

Preserve the project's core invariants: Go orchestrator; recipes first with code-free
domain-profile extensibility; local inference by default and no cloud without
`allow_cloud: true`; llama.cpp as the working default until Ollama is available; optional
Ollama support that does not make build/test/runtime depend on a real server; and the
configured RTX 3080 10 GB VRAM budget. Preserve evidence source/timestamps, do not invent
content, prefer accuracy for extracted facts, and verify changes against the exact
acceptance criteria in `context/requirements.md`.

