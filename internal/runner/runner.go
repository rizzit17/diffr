package runner

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"diffr/internal/astgraph"
)

// RunSummary contains execution results and timing metrics.
type RunSummary struct {
	ImpactedTests       []string `json:"impacted_tests"`
	TotalTestsInRepo    int      `json:"total_tests_in_repo"`
	TestsSkipped        int      `json:"tests_skipped"`
	BaselineFullSuiteMs int64    `json:"baseline_full_suite_ms"`
	ActualRunMs         int64    `json:"actual_run_ms"`
	PctTimeSaved        float64  `json:"pct_time_saved"`
	Success             bool     `json:"success"`
	Output              string   `json:"output"`
}

// Runner handles scoped test execution via go test.
type Runner struct {
	RepoDir string
	Graph   *astgraph.Graph
}

// New creates a new test runner.
func New(repoDir string, graph *astgraph.Graph) *Runner {
	return &Runner{
		RepoDir: repoDir,
		Graph:   graph,
	}
}

// Run executes only the impacted tests, grouped by package, capturing wall-clock duration.
func (r *Runner) Run(impactedTests []string) (*RunSummary, error) {
	totalTests := len(r.Graph.AllTests)
	skipped := totalTests - len(impactedTests)
	if skipped < 0 {
		skipped = 0
	}

	if len(impactedTests) == 0 {
		return &RunSummary{
			ImpactedTests:    nil,
			TotalTestsInRepo: totalTests,
			TestsSkipped:     skipped,
			ActualRunMs:      0,
			Success:          true,
			Output:           "No impacted tests to execute.",
		}, nil
	}

	var allTestNames []string
	var pkgTargets []string
	seenPkg := make(map[string]bool)

	for _, testID := range impactedTests {
		info, exists := r.Graph.Functions[testID]
		pkgDir := "."
		testName := testID
		if exists {
			pkgDir = info.PkgDir
			testName = info.Name
		} else {
			parts := strings.Split(testID, ".")
			testName = parts[len(parts)-1]
		}
		allTestNames = append(allTestNames, testName)

		pkgTarget := "./" + filepath.ToSlash(pkgDir)
		if pkgDir == "." {
			pkgTarget = "."
		}
		if !seenPkg[pkgTarget] {
			seenPkg[pkgTarget] = true
			pkgTargets = append(pkgTargets, pkgTarget)
		}
	}

	regexPattern := "^(" + strings.Join(allTestNames, "|") + ")$"
	args := append([]string{"test", "-v", "-run", regexPattern}, pkgTargets...)

	startTotal := time.Now()
	cmd := exec.Command("go", args...)
	if r.RepoDir != "" {
		cmd.Dir = r.RepoDir
	}

	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	err := cmd.Run()
	actualDuration := time.Since(startTotal).Milliseconds()

	return &RunSummary{
		ImpactedTests:    impactedTests,
		TotalTestsInRepo: totalTests,
		TestsSkipped:     skipped,
		ActualRunMs:      actualDuration,
		Success:          err == nil,
		Output:           outBuf.String(),
	}, nil
}

// RunBaseline runs the full test suite (go test ./...) to compute the baseline execution time in ms.
func (r *Runner) RunBaseline() (int64, error) {
	cmd := exec.Command("go", "test", "./...")
	if r.RepoDir != "" {
		cmd.Dir = r.RepoDir
	}

	start := time.Now()
	var outBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &outBuf

	_ = cmd.Run() // Capture timing even if tests have failures
	durationMs := time.Since(start).Milliseconds()
	if durationMs < 1 {
		durationMs = 1
	}
	return durationMs, nil
}
