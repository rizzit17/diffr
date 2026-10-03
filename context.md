# context.md — Project Context for Antigravity

## Who this is for
Abhi, an Information Technology undergrad (VIT Vellore), building this project as a systems
engineering showcase around **Test Impact Analysis (TIA) and CI/CD developer tooling** —
skipping redundant tests in CI by understanding which tests are actually impacted by a code
change. This project is a focused, working implementation of that core idea.

## Why this stack
The stack utilizes: **Golang, Docker, Kubernetes, MongoDB, Redis**, focusing on
distributed resilient software patterns and low-latency developer infrastructure.
This project is built to demonstrate real-world systems engineering and developer tooling skill.

## Build constraint
Must be buildable **end-to-end in 2–3 hours** using Antigravity (agentic AI coding tool).
This means:
- No over-engineering. Function-level call graph, not full type-resolved static analysis.
- MVP = CLI + Go AST engine + Redis cache + MongoDB logging, all runnable locally via
  `docker-compose up`.
- Kubernetes manifest and GCP notes are **designed, documented, and included in the repo**
  but do not need a live cluster to demo — this is explicitly fine and should be described
  honestly in interviews as "designed for k8s deployment" rather than "running in production."

## Target repo this tool analyzes
Diffr analyzes **Go repositories** (its own repo is a fine default demo target — a tool
that analyzes itself is a strong, slightly funny interview story). It should also be pointed
at a second, slightly larger open-source-style Go repo for the demo video/screenshot, to
prove it's not hardcoded to its own structure.

## User's existing skill context (for the agent's calibration, not for the UI)
Primarily MERN-stack experience to date, picking up Go for this project. Prefers clean,
surgical, well-commented code over cleverness. Prefers minimal, working MVP over broad but
shallow feature coverage. Do not add speculative features (auth, multi-language support,
cloud deployment automation) beyond what's specified in PRD.md.

## Non-goals (explicitly out of scope)
- Supporting languages other than Go for the analyzed repo (Python/JS parsing is a stretch
  goal only, not required).
- A real GCP deployment. Only an architecture note.
- User accounts, auth, multi-tenant dashboard.
- Perfect call-graph precision (interface satisfaction, reflection-based calls). Document
  known limitations instead of trying to solve them.

## Source of truth order
If anything conflicts: PRD.md > architecture.md > system-design.md > design.md.
context.md (this file) only explains *why*, never *what* — defer to the other docs for specs.
