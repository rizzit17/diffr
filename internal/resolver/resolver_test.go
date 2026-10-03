package resolver

import (
	"path/filepath"
	"slices"
	"testing"

	"diffr/internal/astgraph"
	"diffr/internal/diffengine"
)

func TestResolver_Resolve(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatalf("failed to resolve fixture directory: %v", err)
	}

	g, err := astgraph.Build(fixtureDir)
	if err != nil {
		t.Fatalf("failed to build graph: %v", err)
	}

	res := New(g)

	tests := []struct {
		name              string
		changedFiles      []diffengine.ChangedFile
		changedFuncs      []string
		wantImpactedTests []string
		wantSkippedTests  int
	}{
		{
			name:              "change in Add impacts TestAdd directly and TestCompute via Compute method",
			changedFiles:      []diffengine.ChangedFile{{Path: "calc/calc.go"}},
			changedFuncs:      []string{"calc.Add"},
			wantImpactedTests: []string{"calc.TestAdd", "calc.TestCompute"},
			wantSkippedTests:  1, // TestUnusedDirect is skipped
		},
		{
			name:              "change in Multiply impacts only TestCompute via Compute",
			changedFiles:      []diffengine.ChangedFile{{Path: "calc/calc.go"}},
			changedFuncs:      []string{"calc.Multiply"},
			wantImpactedTests: []string{"calc.TestCompute"},
			wantSkippedTests:  2, // TestAdd and TestUnusedDirect are skipped
		},
		{
			name:              "change in Unused impacts only TestUnusedDirect",
			changedFiles:      []diffengine.ChangedFile{{Path: "calc/calc.go"}},
			changedFuncs:      []string{"calc.Unused"},
			wantImpactedTests: []string{"calc.TestUnusedDirect"},
			wantSkippedTests:  2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := res.Resolve(tt.changedFiles, tt.changedFuncs)
			if result.TotalTestsInRepo != 3 {
				t.Errorf("TotalTestsInRepo = %d, want 3", result.TotalTestsInRepo)
			}
			if result.TestsSkipped != tt.wantSkippedTests {
				t.Errorf("TestsSkipped = %d, want %d", result.TestsSkipped, tt.wantSkippedTests)
			}
			if len(result.ImpactedTests) != len(tt.wantImpactedTests) {
				t.Fatalf("ImpactedTests = %v, want %v", result.ImpactedTests, tt.wantImpactedTests)
			}
			for _, wantTest := range tt.wantImpactedTests {
				if !slices.Contains(result.ImpactedTests, wantTest) {
					t.Errorf("ImpactedTests missing %s", wantTest)
				}
			}
		})
	}
}
