# PRD.md — Diffr: Go Test Impact Analyzer

## 1. Problem
CI pipelines re-run entire test suites on every commit, even when a change touches one
function in one package. This wastes compute and slows feedback loops. Test Impact
Analysis (TIA) solves this at scale; Diffr implements a clean, focused version of
the same core concept for a single Go repository.

## 2. Goal
Given two git refs (e.g. `HEAD~1` and `HEAD`), determine the minimal set of Go test
functions that could be affected by the diff, run only those, and report time saved versus
a full suite run — with results cached and logged for repeat analysis.

## 3. Users
- A developer running `diffr run` locally before pushing.
- A CI job running `diffr run` as a pre-test gate step.

## 4. In-scope functional requirements

| ID | Requirement |
|---|---|
| FR1 | CLI command `diffr diff <ref1> <ref2>` lists changed Go files + changed top-level funcs/methods |
| FR2 | CLI builds a function-level call graph for the repo using `go/parser` + `go/ast` |
| FR3 | Diffr maps changed functions → test functions that transitively call them (including same-package test functions by naming convention `Test*`) |
| FR4 | CLI command `diffr run` executes only the impacted tests via `go test -run <regex>` |
| FR5 | Before running analysis, check Redis cache keyed by `repo+commitHashPair`; if hit, reuse cached impacted-test list |
| FR6 | After each run, write a record to MongoDB: timestamp, commit pair, changed files, impacted tests, tests skipped, full-suite baseline time vs actual time, % time saved |
| FR7 | CLI command `diffr stats` prints aggregate time-saved metrics from MongoDB |
| FR8 | `docker-compose.yml` spins up Redis + MongoDB + the Diffr CLI container together |
| FR9 | Minimal read-only dashboard (static HTML/JS, no framework) that queries a tiny Go HTTP endpoint and renders run history + cumulative time saved |

## 5. Explicitly out of scope (v1)
- Multi-language support (Python/JS/Java repos)
- Interface-based/reflection call resolution
- Authentication, multi-user, multi-repo dashboards
- Live Kubernetes/GCP deployment (design only — see architecture.md)
- Historical flaky-test detection

## 6. Success criteria (for the 2–3 hr build)
- [ ] `docker-compose up` brings up Redis + Mongo + CLI container cleanly
- [ ] Running `diffr diff` against two real commits in a demo repo returns a correct,
      sensible changed-function list
- [ ] `diffr run` actually executes a narrowed `go test` command and prints real timing
- [ ] A second run on the same commit pair hits Redis cache (demonstrable via a log line
      "cache hit" and near-zero analysis latency)
- [ ] MongoDB has at least 2 real run documents after a demo
- [ ] Dashboard loads and shows real numbers, not mocked data
- [ ] README has a 30-second "what this is and why" section explaining the
      architecture and motivation

## 7. Demo script (for interview / video)
1. Make a trivial change to one function in the demo repo.
2. Run `diffr run` — show it selecting ~2 of 40 tests, not all 40.
3. Run it again unchanged — show Redis cache hit, near-instant.
4. Open dashboard — show cumulative time saved trending up.
5. One sentence: "This implements the core Test Impact Analysis algorithm to eliminate
   redundant CI runs using syntactic AST call graphs."

## 8. Metrics to report on resume/interview
- % test suite reduction on typical single-function changes (target: demonstrate ≥60%)
- Cache hit latency vs cold analysis latency (order-of-magnitude callout)
