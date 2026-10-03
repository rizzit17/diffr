package runner

import (
	"path/filepath"
	"strings"
	"testing"

	"diffr/internal/astgraph"
)

func TestRunner_RunScoped(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatalf("failed to resolve fixture directory: %v", err)
	}

	g, err := astgraph.Build(fixtureDir)
	if err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	rn := New(fixtureDir, g)

	// Execute only calc.TestAdd
	summary, err := rn.Run([]string{"calc.TestAdd"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if !summary.Success {
		t.Errorf("expected test execution to succeed, output:\n%s", summary.Output)
	}

	if !strings.Contains(summary.Output, "RUN   TestAdd") {
		t.Errorf("output should include TestAdd run, got:\n%s", summary.Output)
	}

	if strings.Contains(summary.Output, "RUN   TestCompute") {
		t.Errorf("output should NOT include TestCompute, got:\n%s", summary.Output)
	}

	if strings.Contains(summary.Output, "RUN   TestUnusedDirect") {
		t.Errorf("output should NOT include TestUnusedDirect, got:\n%s", summary.Output)
	}
}
