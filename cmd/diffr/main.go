package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"diffr/internal/astgraph"
	"diffr/internal/cache"
	"diffr/internal/diffengine"
	"diffr/internal/resolver"
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

	fmt.Printf("Running impacted tests for %s..%s in repo '%s'...\n", ref1, ref2, *repoPath)
}

func handleStats(args []string) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	_ = fs.Parse(args)

	fmt.Println("Querying test impact metrics...")
}

func handleServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", ":8080", "address to listen on")
	_ = fs.Parse(args)

	fmt.Printf("Starting dashboard server on %s...\n", *addr)
}
