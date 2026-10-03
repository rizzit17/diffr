package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"diffr/internal/api"
	"diffr/internal/astgraph"
	"diffr/internal/cache"
	"diffr/internal/diffengine"
	"diffr/internal/resolver"
	"diffr/internal/runner"
	"diffr/internal/store"
)

const usageText = `diffr - Go Test Impact Analysis (TIA) CLI

Usage:
  diffr <command> [arguments]

Commands:
  diff [<ref1>] [<ref2>]   Analyze git diff and list changed functions & impacted tests
  run  [<ref1>] [<ref2>]   Execute only impacted tests and record time-saved metrics
  stats                    Print cumulative test execution savings from MongoDB
  serve                    Start the dashboard web server and API

Run 'diffr <command> -h' for details on a specific command.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usageText)
		os.Exit(1)
	}

	command := os.Args[1]
	args := os.Args[2:]

	switch command {
	case "diff":
		handleDiff(args)
	case "run":
		handleRun(args)
	case "stats":
		handleStats(args)
	case "serve":
		handleServe(args)
	case "help", "-h", "--help":
		fmt.Print(usageText)
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n%s", command, usageText)
		os.Exit(1)
	}
}

func handleDiff(args []string) {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "path to git repository")
	_ = fs.Parse(args)

	ref1 := "HEAD~1"
	ref2 := "HEAD"
	remaining := fs.Args()
	if len(remaining) >= 1 {
		ref1 = remaining[0]
	}
	if len(remaining) >= 2 {
		ref2 = remaining[1]
	}

	start := time.Now()
	repoHash := cache.ComputeRepoHash(*repoPath)
	cClient := cache.New("")
	defer cClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 1. Cache-first lookup
	if cached, hit := cClient.GetImpact(ctx, repoHash, ref1, ref2); hit {
		elapsed := time.Since(start)
		fmt.Printf("Diff Analysis: %s..%s (repo: %s)\n", ref1, ref2, *repoPath)
		fmt.Printf("Status: CACHE HIT (retrieval: %v)\n\n", elapsed)
		printImpactSummary(cached.ChangedFiles, cached.ChangedFunctions, cached.ImpactedTests, cached.TotalTestsInRepo, cached.TestsSkipped)
		return
	}

	// 2. Cache miss: Full analysis
	diffEng := diffengine.New(*repoPath)
	changedFiles, err := diffEng.GetDiff(ref1, ref2)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error computing git diff: %v\n", err)
		os.Exit(1)
	}

	graph, err := astgraph.Build(*repoPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error building AST call graph: %v\n", err)
		os.Exit(1)
	}

	changedFuncs := graph.MapChangedFunctions(changedFiles)
	res := resolver.New(graph).Resolve(changedFiles, changedFuncs)

	// 3. Populate cache
	cClient.SetImpact(ctx, repoHash, ref1, ref2, &cache.CachedImpact{
		ImpactedTests:    res.ImpactedTests,
		ChangedFunctions: res.ChangedFunctions,
		ChangedFiles:     res.ChangedFiles,
		TotalTestsInRepo: res.TotalTestsInRepo,
		TestsSkipped:     res.TestsSkipped,
	})

	elapsed := time.Since(start)
	fmt.Printf("Diff Analysis: %s..%s (repo: %s)\n", ref1, ref2, *repoPath)
	fmt.Printf("Status: CACHE MISS (analysis time: %v)\n\n", elapsed)
	printImpactSummary(res.ChangedFiles, res.ChangedFunctions, res.ImpactedTests, res.TotalTestsInRepo, res.TestsSkipped)
}

func printImpactSummary(files, funcs, tests []string, totalTests, skipped int) {
	fmt.Printf("Changed Files (%d):\n", len(files))
	for _, f := range files {
		fmt.Printf("  • %s\n", f)
	}
	if len(files) == 0 {
		fmt.Println("  (none)")
	}

	fmt.Printf("\nChanged Functions (%d):\n", len(funcs))
	for _, fn := range funcs {
		fmt.Printf("  • %s\n", fn)
	}
	if len(funcs) == 0 {
		fmt.Println("  (none)")
	}

	reductionPct := 0.0
	if totalTests > 0 {
		reductionPct = (float64(skipped) / float64(totalTests)) * 100.0
	}

	fmt.Printf("\nImpacted Tests (%d of %d total, %d skipped — %.1f%% reduction):\n",
		len(tests), totalTests, skipped, reductionPct)
	for _, t := range tests {
		fmt.Printf("  ✓ %s\n", t)
	}
	if len(tests) == 0 {
		fmt.Println("  (no impacted tests detected)")
	}
}

func handleRun(args []string) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "path to git repository")
	forceBaseline := fs.Bool("baseline", false, "force recomputing full-suite baseline time")
	dryRun := fs.Bool("dry-run", false, "simulate test impact without executing tests")
	_ = fs.Parse(args)

	ref1 := "HEAD~1"
	ref2 := "HEAD"
	remaining := fs.Args()
	if len(remaining) >= 1 {
		ref1 = remaining[0]
	}
	if len(remaining) >= 2 {
		ref2 = remaining[1]
	}

	startTotal := time.Now()
	repoHash := cache.ComputeRepoHash(*repoPath)
	cClient := cache.New("")
	defer cClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var (
		changedFiles     []string
		changedFuncs     []string
		impactedTests    []string
		totalTestsInRepo int
		testsSkipped     int
		cacheHit         bool
		graph            *astgraph.Graph
	)

	// 1. Check Redis cache for impact resolution
	if cached, hit := cClient.GetImpact(ctx, repoHash, ref1, ref2); hit {
		cacheHit = true
		changedFiles = cached.ChangedFiles
		changedFuncs = cached.ChangedFunctions
		impactedTests = cached.ImpactedTests
		totalTestsInRepo = cached.TotalTestsInRepo
		testsSkipped = cached.TestsSkipped

		// Load graph for package directory lookups during run
		var err error
		graph, err = astgraph.Build(*repoPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not rebuild AST graph for runner: %v\n", err)
		}
	} else {
		// Cache miss: full AST diff & reverse call graph traversal
		diffEng := diffengine.New(*repoPath)
		rawChangedFiles, err := diffEng.GetDiff(ref1, ref2)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error computing git diff: %v\n", err)
			os.Exit(1)
		}

		var errGraph error
		graph, errGraph = astgraph.Build(*repoPath)
		if errGraph != nil {
			fmt.Fprintf(os.Stderr, "error building AST graph: %v\n", errGraph)
			os.Exit(1)
		}

		rawChangedFuncs := graph.MapChangedFunctions(rawChangedFiles)
		res := resolver.New(graph).Resolve(rawChangedFiles, rawChangedFuncs)

		changedFiles = res.ChangedFiles
		changedFuncs = res.ChangedFunctions
		impactedTests = res.ImpactedTests
		totalTestsInRepo = res.TotalTestsInRepo
		testsSkipped = res.TestsSkipped

		// Save to Redis cache
		cClient.SetImpact(ctx, repoHash, ref1, ref2, &cache.CachedImpact{
			ImpactedTests:    impactedTests,
			ChangedFunctions: changedFuncs,
			ChangedFiles:     changedFiles,
			TotalTestsInRepo: totalTestsInRepo,
			TestsSkipped:     testsSkipped,
		})
	}

	analysisTime := time.Since(startTotal)

	// 2. Resolve baseline full-suite timing
	var baselineMs int64
	if !*forceBaseline {
		if cachedBaseline, hit := cClient.GetBaseline(ctx, repoHash); hit {
			baselineMs = cachedBaseline
		}
	}

	rn := runner.New(*repoPath, graph)

	if baselineMs <= 0 {
		fmt.Print("Computing full test suite baseline (one-time calibration)... ")
		bMs, err := rn.RunBaseline()
		if err != nil {
			fmt.Printf("warning: baseline calculation returned error: %v\n", err)
		}
		baselineMs = bMs
		cClient.SetBaseline(ctx, repoHash, baselineMs)
		fmt.Printf("%d ms\n", baselineMs)
	}

	// Print analysis header
	cacheLabel := "CACHE MISS"
	if cacheHit {
		cacheLabel = "CACHE HIT"
	}
	fmt.Printf("\n=== Diffr Test Impact Run ===\n")
	fmt.Printf("Commit Range: %s..%s (analysis: %v, %s)\n", ref1, ref2, analysisTime, cacheLabel)
	printImpactSummary(changedFiles, changedFuncs, impactedTests, totalTestsInRepo, testsSkipped)

	// 3. Execute scoped tests
	var actualRunMs int64
	var testOutput string
	var success bool = true

	if *dryRun {
		fmt.Println("\n[dry-run] Skipping test execution.")
	} else if len(impactedTests) == 0 {
		fmt.Println("\nNo impacted tests. Full suite skipped safely.")
		actualRunMs = 0
		success = true
	} else {
		fmt.Printf("\nExecuting %d selected test(s)...\n", len(impactedTests))
		runStart := time.Now()
		summary, err := rn.Run(impactedTests)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error executing tests: %v\n", err)
		}
		actualRunMs = time.Since(runStart).Milliseconds()
		if summary != nil {
			testOutput = summary.Output
			success = summary.Success
		}
		if success {
			fmt.Printf("✓ Tests completed successfully in %d ms.\n", actualRunMs)
		} else {
			fmt.Printf("✗ Tests completed with failures in %d ms.\n", actualRunMs)
		}
	}

	// 4. Calculate savings
	pctSaved := 0.0
	msSaved := baselineMs - actualRunMs
	if baselineMs > 0 {
		pctSaved = (float64(msSaved) / float64(baselineMs)) * 100.0
		if pctSaved < 0 {
			pctSaved = 0
		}
	}

	fmt.Printf("\n--- Performance Impact ---\n")
	fmt.Printf("Baseline Full Suite : %d ms\n", baselineMs)
	fmt.Printf("Scoped Test Run     : %d ms\n", actualRunMs)
	fmt.Printf("Time Saved          : %d ms (%.1f%% reduction)\n", msSaved, pctSaved)

	// 5. Store run metrics
	st := store.New("", *repoPath)
	defer st.Close(context.Background())

	rec := &store.RunRecord{
		RepoPath:            *repoPath,
		Ref1:                ref1,
		Ref2:                ref2,
		Timestamp:           time.Now().UTC(),
		ChangedFiles:        changedFiles,
		ChangedFunctions:    changedFuncs,
		ImpactedTests:       impactedTests,
		TotalTestsInRepo:    totalTestsInRepo,
		TestsSkipped:        testsSkipped,
		BaselineFullSuiteMs: baselineMs,
		ActualRunMs:         actualRunMs,
		PctTimeSaved:        pctSaved,
		CacheHit:            cacheHit,
	}

	if err := st.SaveRun(ctx, rec); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to persist run record: %v\n", err)
	} else {
		if st.IsConnected() {
			fmt.Println("Persistence         : MongoDB (synced)")
		} else {
			fmt.Println("Persistence         : Local store (.diffr/runs.json)")
		}
	}

	_ = testOutput
}

func handleStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	repoPath := fs.String("repo", ".", "path to git repository")
	_ = fs.Parse(args)

	st := store.New("", *repoPath)
	defer st.Close(context.Background())

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	stats, err := st.GetStats(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error retrieving stats: %v\n", err)
		os.Exit(1)
	}

	backend := "Local Store (.diffr/runs.json)"
	if st.IsConnected() {
		backend = "MongoDB (Connected)"
	}

	fmt.Println("Diffr Test Impact Intelligence — Cumulative Metrics")
	fmt.Println("──────────────────────────────────────────────────")
	fmt.Printf("Total Runs Recorded    : %d\n", stats.TotalRuns)
	fmt.Printf("Average Test Time Saved: %.1f%%\n", stats.AvgPctTimeSaved)
	fmt.Printf("Total Compute Time Saved: %d ms\n", stats.TotalMsSaved)
	fmt.Printf("Telemetry Backend      : %s\n", backend)
}

func handleServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	defaultAddr := ":8080"
	if envPort := os.Getenv("PORT"); envPort != "" {
		if !strings.HasPrefix(envPort, ":") {
			defaultAddr = ":" + envPort
		} else {
			defaultAddr = envPort
		}
	}
	addr := fs.String("addr", defaultAddr, "address to listen on (e.g. :8080)")
	repoPath := fs.String("repo", ".", "path to git repository")
	staticDir := fs.String("static", "web", "path to web dashboard static files")
	_ = fs.Parse(args)

	st := store.New("", *repoPath)
	defer st.Close(context.Background())

	srv := api.NewServer(st, *staticDir)

	storageDesc := "MongoDB (connected)"
	if !st.IsConnected() {
		storageDesc = "Local Store (.diffr/runs.json fallback)"
	}

	displayAddr := *addr
	if strings.HasPrefix(displayAddr, ":") {
		displayAddr = "http://localhost" + displayAddr
	}

	fmt.Printf("\n┌────────────────────────────────────────────────────────┐\n")
	fmt.Printf("│  Diffr — Test Impact Intelligence Dashboard            │\n")
	fmt.Printf("└────────────────────────────────────────────────────────┘\n")
	fmt.Printf("  • Server listening on  : %s\n", displayAddr)
	fmt.Printf("  • API endpoint         : %s/api/runs\n", displayAddr)
	fmt.Printf("  • Storage backend      : %s\n", storageDesc)
	fmt.Printf("  • Static assets dir    : %s\n\n", *staticDir)
	fmt.Println("Press Ctrl+C to stop.")

	if err := http.ListenAndServe(*addr, srv.Handler()); err != nil {
		fmt.Fprintf(os.Stderr, "server stopped: %v\n", err)
		os.Exit(1)
	}
}
