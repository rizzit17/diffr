package resolver

import (
	"sort"
	"strings"

	"diffr/internal/astgraph"
	"diffr/internal/diffengine"
)

// Result holds the resolved impact analysis data.
type Result struct {
	ChangedFiles      []string `json:"changed_files"`
	ChangedFunctions  []string `json:"changed_functions"`
	ImpactedTests     []string `json:"impacted_tests"`
	TotalTestsInRepo  int      `json:"total_tests_in_repo"`
	TestsSkipped      int      `json:"tests_skipped"`
}

// Resolver resolves impacted tests from changed functions and files.
type Resolver struct {
	Graph *astgraph.Graph
}

// New creates an Impact Resolver backed by the parsed repository AST graph.
func New(graph *astgraph.Graph) *Resolver {
	return &Resolver{Graph: graph}
}

// Resolve identifies all test functions affected directly or transitively.
func (r *Resolver) Resolve(changedFiles []diffengine.ChangedFile, changedFuncs []string) *Result {
	impactedSet := make(map[string]bool)

	// 1. Direct changes to _test.go files: any changed test function in a test file is directly impacted
	for _, cf := range changedFiles {
		if strings.HasSuffix(cf.Path, "_test.go") {
			for _, fn := range r.Graph.Functions {
				if fn.File == cf.Path && fn.IsTest {
					// Check if changed line ranges intersect with the test function
					for _, lr := range cf.Lines {
						if !(lr.End < fn.StartLine || lr.Start > fn.EndLine) {
							impactedSet[fn.ID] = true
							break
						}
					}
				}
			}
		}
	}

	// 2. BFS over reverse call graph for all changed non-test functions
	for _, changedFn := range changedFuncs {
		queue := []string{changedFn}
		visited := map[string]bool{changedFn: true}

		for len(queue) > 0 {
			current := queue[0]
			queue = queue[1:]

			// If current node is a Test* function, mark it as impacted
			if info, exists := r.Graph.Functions[current]; exists && info.IsTest {
				impactedSet[current] = true
				continue
			}

			// Traverse reverse call graph to callers
			for _, caller := range r.Graph.ReverseGraph[current] {
				if !visited[caller] {
					visited[caller] = true
					queue = append(queue, caller)
				}
			}
		}
	}

	var impactedTests []string
	for testID := range impactedSet {
		impactedTests = append(impactedTests, testID)
	}
	sort.Strings(impactedTests)

	var changedFilePaths []string
	for _, cf := range changedFiles {
		changedFilePaths = append(changedFilePaths, cf.Path)
	}
	sort.Strings(changedFilePaths)

	totalTests := len(r.Graph.AllTests)
	testsSkipped := totalTests - len(impactedTests)
	if testsSkipped < 0 {
		testsSkipped = 0
	}

	return &Result{
		ChangedFiles:     changedFilePaths,
		ChangedFunctions: changedFuncs,
		ImpactedTests:    impactedTests,
		TotalTestsInRepo: totalTests,
		TestsSkipped:     testsSkipped,
	}
}
