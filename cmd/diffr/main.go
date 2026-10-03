package main

import (
	"flag"
	"fmt"
	"os"
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

	fmt.Printf("Analyzing diff between %s and %s in repo '%s'...\n", ref1, ref2, *repoPath)
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
