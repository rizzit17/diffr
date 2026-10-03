# Diffr — Go Test Impact Analysis (TIA)

**Diffr** is a high-performance Test Impact Analysis CLI and minimal developer dashboard for Go repositories. Given two git references (e.g. `HEAD~1` and `HEAD`), Diffr calculates the syntactic diff, walks the Go AST to build a function-level reverse call graph, resolves the minimal set of impacted tests transitively using BFS, executes only those tests, and records time-saved metrics into MongoDB with sub-millisecond Redis caching.

---

## Why Diffr (Harness Alignment)

Built specifically to explore the core mechanics behind **Test Intelligence**—the category pioneered by Harness to eliminate redundant test execution in CI pipelines. Running an entire test suite on every single-line commit wastes compute, throttles developer throughput, and slows deployment cycles. Diffr proves the underlying algorithm in Go: parse syntactic deltas, trace caller-callee hierarchies through reverse call graphs, prune non-impacted tests with high fidelity, and maintain low-overhead caching and telemetry across distributed pipeline runs.

---

## Architecture

```
                 ┌─────────────────────┐
                 │   Developer / CI    │
                 │     (diffr CLI)     │
                 └──────────┬──────────┘
                            │
            ┌───────────────┼────────────────┐
            ▼               ▼                ▼
     ┌─────────────┐ ┌──────────────┐  ┌──────────────┐
     │ Diff Engine │ │ AST / Call   │  │ Test Runner  │
     │ (git diff)  │ │ Graph Engine │  │ (go test)    │
     └──────┬──────┘ └──────┬───────┘  └──────┬───────┘
            │               │                 │
            └───────┬───────┘                 │
                    ▼                         │
           ┌──────────────────┐               │
           │  Impact Resolver │◄──────────────┘
           │ (changed → test) │
           └────────┬─────────┘
                    │
           ┌────────┴─────────┐
           ▼                  ▼
     ┌───────────┐      ┌──────────────┐
     │   Redis   │      │   MongoDB    │
     │  (cache)  │      │ (telemetry)  │
     └───────────┘      └──────┬───────┘
                               │
                               ▼
                        ┌──────────────┐
                        │Dashboard API │
                        │(net/http Go) │
                        └──────┬───────┘
                               ▼
                        ┌──────────────┐
                        │ Static Web UI│
                        │ (HTML/JS/CSS)│
                        └──────────────┘
```

### Module Breakdown
- **Diff Engine (`internal/diffengine`)**: Shells out to `git diff -U0`, parses hunk headers, and extracts modified line ranges per Go file.
- **AST / Call Graph Engine (`internal/astgraph`)**: Walks the repository with `go/parser`, maps line ranges to enclosing top-level `FuncDecl` nodes, and constructs an adjacency call graph and reverse graph (`callee -> callers`).
- **Impact Resolver (`internal/resolver`)**: Performs BFS traversal over the reverse call graph from changed functions to locate directly and transitively impacted `Test*` functions, handling `_test.go` edits directly.
- **Cache Layer (`internal/cache`)**: Keyed by `diffr:{repoHash}:{ref1}:{ref2}` in Redis with a 24-hour TTL; bypasses AST parsing on repeated runs.
- **Test Runner (`internal/runner`)**: Compiles target test functions into a targeted `-run '^(TestA|TestB)$'` regex and invokes scoped `go test` with wall-clock timing.
- **Storage Layer (`internal/store`)**: Logs run records to MongoDB collection `runs` with compound indexes on `{repo_path: 1, ref1: 1, ref2: 1}` and `{timestamp: -1}`, equipped with automatic resilient local file fallback (`.diffr/runs.json`).
- **Dashboard & API (`internal/api` & `web/`)**: Clean developer-tool dashboard (monochrome, monospace data, SVG/Canvas cumulative savings chart, zebra-striped run table) served via Go stdlib `net/http`.

---

## Quickstart & Demo Walkthrough

### 1. Build or Run via Docker Compose

To start Redis, MongoDB, and the Diffr container together:

```bash
docker-compose up -d
```

Or build and run natively with Go:

```bash
go build -o diffr ./cmd/diffr
```

### 2. Step-by-Step Demo Script (PRD §7)

#### Step 1: Analyze Diff Between Commits
```bash
./diffr diff HEAD~1 HEAD
```
*Output: Lists changed files, enclosing functions, and impacted tests.*

#### Step 2: Execute Impacted Tests & Measure Savings
```bash
./diffr run HEAD~1 HEAD
```
*Output: Calibrates baseline test suite duration, executes only the impacted tests, reports milliseconds and % time saved, and persists metrics to MongoDB.*

#### Step 3: Demonstrate Sub-Millisecond Cache Hit
Run the identical command again:
```bash
./diffr run HEAD~1 HEAD
```
*Output: Reports `CACHE HIT` with near-zero analysis latency.*

#### Step 4: Inspect Aggregated Telemetry
```bash
./diffr stats
```
*Output: Queries MongoDB and prints cumulative test time saved.*

#### Step 5: Launch Dashboard
```bash
./diffr serve -addr :8080
```
Open [http://localhost:8080](http://localhost:8080) to inspect run history, time-saved trends, and cache status.

---

## Kubernetes Deployment Target

Production Kubernetes manifests are included in `/deploy/k8s/`:
- [`api-deployment.yaml`](file:///deploy/k8s/api-deployment.yaml): Dual-replica Deployment and ClusterIP Service for the web dashboard and REST API.
- [`ci-job.yaml`](file:///deploy/k8s/ci-job.yaml): Parameterized Kubernetes `Job` template designed to execute within CI pipelines (GKE / Harness CI Step) utilizing external managed services (GCP Memorystore for Redis and MongoDB Atlas via VPC peering).

*(Note: Designed deployment target, not exercised in local environments).*

---

## Known Limitations

- **Syntactic Call Graph**: Constructs call graphs via AST inspection without full type-checked interface resolution or reflection dispatch.
- **Function-Level Granularity**: Tracks mutations at function/method boundaries rather than statement-level delta slices.
- **Single Language**: Focused on Go repositories in v1.
- **Flaky-Test Isolation**: Does not currently track historical test flakiness.

---

## Scaling to Production (What I'd Do Differently)

1. **Incremental Graph Persistence**: Rather than re-parsing the entire AST per run, maintain an incremental dependency graph stored in an in-memory graph database, updating only modified nodes per git commit.
2. **Semantic Cache Invalidation**: Invalidate cache keys using commit tree hashes and dependency closure hashes rather than simple time-to-live expiration.
3. **Failure-Risk Test Prioritization**: Combine static call graph distance with historical defect density to prioritize the execution order of impacted tests within large test suites.
