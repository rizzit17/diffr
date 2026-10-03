# system-design.md — Diffr

## 1. Data models

### 1.1 MongoDB — `runs` collection
```json
{
  "_id": "ObjectId",
  "repo_path": "string",
  "ref1": "string (git sha)",
  "ref2": "string (git sha)",
  "timestamp": "ISODate",
  "changed_files": ["string"],
  "changed_functions": ["pkg.FuncName"],
  "impacted_tests": ["pkg.TestName"],
  "total_tests_in_repo": "int",
  "tests_skipped": "int",
  "baseline_full_suite_ms": "int",
  "actual_run_ms": "int",
  "pct_time_saved": "float",
  "cache_hit": "bool"
}
```
Index: compound index on `{repo_path: 1, ref1: 1, ref2: 1}` for fast cache-miss lookups
and dashboard filtering; single index on `timestamp` (descending) for the dashboard's
"recent runs" query.

### 1.2 Redis — key schema
- `diffr:{repoHash}:{ref1}:{ref2}` → JSON `{impacted_tests: [...], changed_functions: [...]}`, TTL 24h
- `diffr:baseline:{repoHash}` → `int` (ms), TTL 7d (full-suite baseline is stable unless
  the test suite itself changes significantly)

`repoHash` = SHA-256 of the absolute repo path (or git remote URL if available), truncated
to 12 hex chars — short enough to be readable in logs, long enough to avoid collisions for
a single-user tool.

## 2. Algorithm design

### 2.1 Call graph construction (core algorithm)
```
for each .go file in repo (excluding vendor/, _test.go handled separately):
    parse AST
    for each FuncDecl:
        node_id = package + "." + funcName (or "(Receiver)." + methodName)
        walk body with ast.Inspect:
            on CallExpr where Fun is an Ident or SelectorExpr:
                resolve callee name (best-effort, same-package or imported-package qualified)
                add edge node_id -> callee_id
build reverse_graph: callee_id -> [caller_id, ...]
```
This is **O(n)** in AST nodes, single pass, no type-checking — intentionally shallow for a
2–3hr build. Document this limitation explicitly in README ("static, syntactic call graph;
does not resolve interface dispatch or reflection — a known, acceptable approximation").

### 2.2 Impact resolution (BFS)
```
impacted_tests = {}
for fn in changed_functions:
    queue = [fn]; visited = {fn}
    while queue:
        current = queue.pop()
        if current is Test* function: impacted_tests.add(current); continue
        for caller in reverse_graph[current]:
            if caller not in visited:
                visited.add(caller); queue.append(caller)
return impacted_tests
```

### 2.3 Cache-first flow
```
key = cacheKey(repoHash, ref1, ref2)
if redis.exists(key): return redis.get(key)   # cache hit, skip 2.1/2.2 entirely
else:
    result = runFullAnalysis()
    redis.set(key, result, ttl=24h)
    return result
```

## 3. API surface

### CLI
```
diffr diff <ref1> <ref2>      # prints changed files + functions, no test run
diffr run [<ref1>] [<ref2>]   # default refs: HEAD~1 HEAD; runs impacted tests
diffr stats                   # prints aggregate savings from Mongo
diffr serve                   # starts the dashboard API on :8080
```

### HTTP (dashboard API)
```
GET /api/runs?limit=20
→ 200 OK
{
  "runs": [ ...run documents, newest first... ],
  "aggregate": {
    "total_runs": int,
    "avg_pct_time_saved": float,
    "total_ms_saved": int
  }
}
```
No auth in v1 (explicitly out of scope per PRD — localhost/demo tool only).

## 4. Scalability notes (for interview framing, not for this build)
- Call graph construction is the only O(n)-over-whole-repo step; everything else is O(1)
  cache lookups or O(impacted set) test execution — this follows the same design principle
  enterprise test impact platforms use at much larger scale (precomputed dependency graphs, incremental
  updates rather than full rebuilds per commit).
- A real production version would persist the call graph itself (not just results) and
  update it incrementally per commit rather than rebuilding from scratch — worth mentioning
  as "the obvious next step I'd take at scale" in interviews.
- Redis TTL-based caching is a deliberate simplification; a production version would
  invalidate based on actual graph-affecting changes, not just time.

## 5. Why Redis + Mongo specifically (interview talking point)
- Redis: sub-millisecond repeat-commit lookups during active development (same branch,
  incremental commits) — this is the hot path.
- MongoDB: run history is naturally document-shaped (variable-length arrays of files/tests
  per run), and the dashboard's query pattern (recent runs, aggregate stats) doesn't need
  relational joins — a deliberate, explainable choice rather than "Mongo because I know it."

## 6. Known limitations (be upfront about these — they show maturity, not weakness)
- No interface/reflection call resolution (documented in 2.1)
- Function-level, not statement-level, granularity
- Single-repo, single-language (Go) in v1
- No flaky-test awareness — a skipped-then-later-fails scenario is a known risk class,
  explicitly out of scope
