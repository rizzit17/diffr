package runner

import (
	"bytes"
	"os"
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
	RepoDir      string
	Graph        *astgraph.Graph
	TestPackages map[string]string
	TotalTests   int
	LastCommand  string
}

// New creates a new test runner.
func New(repoDir string, graph *astgraph.Graph) *Runner {
	total := 0
	if graph != nil {
		total = len(graph.AllTests)
	}
	return &Runner{
		RepoDir:    repoDir,
		Graph:      graph,
		TotalTests: total,
	}
}

// NewFromCache creates a runner using cached package paths without re-parsing the AST.
func NewFromCache(repoDir string, testPackages map[string]string, totalTests int) *Runner {
	return &Runner{
		RepoDir:      repoDir,
		TestPackages: testPackages,
		TotalTests:   totalTests,
	}
}

// Run executes only the impacted tests, grouped by package, capturing wall-clock duration.
func (r *Runner) Run(impactedTests []string) (*RunSummary, error) {
	totalTests := r.TotalTests
	if r.Graph != nil && len(r.Graph.AllTests) > 0 {
		totalTests = len(r.Graph.AllTests)
	}

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
		pkgDir := "."
		testName := testID

		if r.TestPackages != nil && r.TestPackages[testID] != "" {
			pkgDir = r.TestPackages[testID]
			parts := strings.Split(testID, ".")
			testName = parts[len(parts)-1]
		} else if r.Graph != nil {
			if info, exists := r.Graph.Functions[testID]; exists {
				pkgDir = info.PkgDir
				testName = info.Name
			}
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
	r.LastCommand = "go " + strings.Join(args, " ")

	goBin := findGoBinary()
	startTotal := time.Now()
	cmd := exec.Command(goBin, args...)
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
	goBin := findGoBinary()
	cmd := exec.Command(goBin, "test", "./...")
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

func findGoBinary() string {
	if p, err := exec.LookPath("go"); err == nil {
		return p
	}
	if goroot := os.Getenv("GOROOT"); goroot != "" {
		candidate := filepath.Join(goroot, "bin", "go.exe")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	commonPaths := []string{
		`C:\Program Files\Go\bin\go.exe`,
		`C:\Go\bin\go.exe`,
		`/usr/local/go/bin/go`,
		`/usr/bin/go`,
	}
	for _, cp := range commonPaths {
		if _, err := os.Stat(cp); err == nil {
			return cp
		}
	}
	return "go"
}
