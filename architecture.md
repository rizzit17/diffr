# architecture.md — Diffr

## 1. High-level component diagram (describe, don't need literal image)

```
                 ┌─────────────────────┐
                 │   Developer / CI     │
                 │  (diffr CLI)      │
                 └──────────┬───────────┘
                             │
            ┌────────────────┼─────────────────┐
            ▼                ▼                  ▼
     ┌─────────────┐  ┌──────────────┐   ┌──────────────┐
     │ Diff Engine  │  │ AST / Call   │   │ Test Runner  │
     │ (git diff)   │  │ Graph Engine │   │ (go test)    │
     └──────┬───────┘  └──────┬───────┘   └──────┬───────┘
            │                 │                   │
            └────────┬────────┘                   │
                      ▼                            │
             ┌──────────────────┐                  │
             │  Impact Resolver  │◄────────────────┘
             │ (changed fn → test fn)
             └────────┬──────────┘
                       │
           ┌───────────┼────────────┐
           ▼                        ▼
     ┌───────────┐           ┌──────────────┐
     │   Redis    │           │   MongoDB    │
     │ (cache:    │           │ (run history,│
     │ commit→    │           │  metrics)    │
     │ impacted   │           └──────┬───────┘
     │  tests)    │                  │
     └───────────┘                   ▼
                              ┌───────────────┐
                              │ Dashboard API  │
                              │ (Go net/http)  │
                              └───────┬───────┘
                                      ▼
                              ┌───────────────┐
                              │ Static HTML/JS │
                              │   Dashboard    │
                              └───────────────┘
```

## 2. Module breakdown

### 2.1 Diff Engine (`internal/diffengine`)
- Shells out to `git diff --name-only <ref1> <ref2>` and `git diff -U0 <ref1> <ref2>`.
- Parses hunks to get changed line ranges per file.
- Output: `[]ChangedFile{Path string, Lines []LineRange}`

### 2.2 AST / Call Graph Engine (`internal/astgraph`)
- Walks the repo with `go/parser` to build an AST per `.go` file.
- For each file, maps line ranges → enclosing top-level func/method declarations
  (`FuncDecl`) to get `ChangedFunctions []string` (qualified as `pkg.FuncName` or
  `pkg.(Receiver).Method`).
- Separately builds a **static call graph**: for every function, which functions it calls
  (via `ast.Inspect` walking `CallExpr` nodes, best-effort — no interface resolution in v1).
- Call graph is an adjacency map: `caller -> []callee`. Build reverse map once:
  `callee -> []caller`.

### 2.3 Impact Resolver (`internal/resolver`)
- BFS/DFS over the reverse call graph starting from each changed function, up to a
  configurable depth (default: unbounded within package, depth-2 across packages — keeps
  v1 fast and avoids near-whole-repo blowup).
- Any `Test*` function reached (by Go test naming convention, in a `_test.go` file) is
  marked impacted.
- If a changed file *is* a `_test.go` file itself, its own test functions are impacted
  directly (no graph walk needed).

### 2.4 Cache Layer (`internal/cache`)
- Redis key: `diffr:{repoHash}:{ref1}:{ref2}` → JSON list of impacted test names + TTL
  (e.g. 24h, configurable).
- On `diffr run`: compute `repoHash` (hash of repo remote URL or local path), check
  cache before running AST analysis. Cache hit skips steps 2.2/2.3 entirely.

### 2.5 Test Runner (`internal/runner`)
- Builds a `-run` regex from the impacted test function names, grouped by package.
- Executes `go test -run '^(TestA|TestB)$' ./pkg/...` per affected package, capturing
  wall-clock time.
- Also computes (once, not every run) a baseline full-suite time for comparison —
  cache this baseline separately with a longer TTL.

### 2.6 Storage Layer (`internal/store`)
- MongoDB collection `runs`: one document per `diffr run` invocation.
- Schema detailed in system-design.md.

### 2.7 Dashboard API (`internal/api`)
- Minimal `net/http` server, one route: `GET /api/runs` returns last N run documents +
  aggregate stats as JSON.
- No framework needed — stdlib is enough and is itself a small JD-relevant signal
  (shows comfort without leaning on heavy deps).

### 2.8 Dashboard UI (`web/`)
- Single static `index.html` + `app.js` + `style.css`. Fetches `/api/runs`, renders a
  simple table + one cumulative-savings line chart (plain canvas or a tiny dependency-free
  chart, see design.md).

## 3. Deployment architecture (designed, not required to run live for the demo)

- **Docker**: each component (CLI/API binary, Redis, MongoDB) has its own container;
  `docker-compose.yml` wires them together for local dev with named volumes for Mongo data.
- **Kubernetes** (design-only artifact, included as `/deploy/k8s/*.yaml`):
  - `diffr-api` Deployment + Service (ClusterIP) for the dashboard API.
  - `diffr-ci-job` as a Kubernetes `Job` template, intended to be invoked per-CI-run
    (parameterized via env vars `REF1`, `REF2`, `REPO_PATH`).
  - Redis and MongoDB referenced as external managed services (e.g. GCP Memorystore for
    Redis, MongoDB Atlas) rather than in-cluster — documented as the production-realistic
    choice, with a one-paragraph rationale in system-design.md.
- **GCP note**: architecture assumes the CI Job runs inside a GKE cluster, with Memorystore
  (Redis) and MongoDB Atlas (or self-hosted) reachable via VPC peering. This is a documented
  design decision, not a built/running deployment — be explicit about that in interviews.

## 4. Failure modes & how they're handled (design-level, keep v1 pragmatic)
- **Cache unavailable**: fall back to full AST analysis, log a warning, don't fail the run.
- **Mongo unavailable**: log run results to stdout/local file as fallback, don't block the
  test run itself (observability should never gate CI).
- **Call graph miss (e.g. reflection, interface dispatch)**: documented known limitation;
  fallback policy is "if in doubt, run the whole package's tests" rather than silently
  skip tests that might actually be impacted — correctness over speed when uncertain.
