package astgraph

import (
	"path/filepath"
	"slices"
	"testing"

	"diffr/internal/diffengine"
)

func TestBuildCallGraph(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatalf("failed to resolve fixture directory: %v", err)
	}

	g, err := Build(fixtureDir)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	tests := []struct {
		name         string
		funcID       string
		wantCallees  []string
		wantCallers  []string
		expectIsTest bool
	}{
		{
			name:         "top-level function Add",
			funcID:       "calc.Add",
			wantCallees:  nil,
			wantCallers:  []string{"calc.TestAdd", "calc.(*Calculator).Compute"},
			expectIsTest: false,
		},
		{
			name:         "method Compute calling Add and Multiply",
			funcID:       "calc.(*Calculator).Compute",
			wantCallees:  []string{"calc.Add", "calc.Multiply"},
			wantCallers:  []string{"calc.TestCompute"},
			expectIsTest: false,
		},
		{
			name:         "test function TestAdd calling Add",
			funcID:       "calc.TestAdd",
			wantCallees:  []string{"calc.Add"},
			wantCallers:  nil,
			expectIsTest: true,
		},
		{
			name:         "test function TestCompute calling method Compute",
			funcID:       "calc.TestCompute",
			wantCallees:  []string{"calc.(*Calculator).Compute"},
			wantCallers:  nil,
			expectIsTest: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, exists := g.Functions[tt.funcID]
			if !exists {
				t.Fatalf("function %s was not discovered in graph", tt.funcID)
			}
			if info.IsTest != tt.expectIsTest {
				t.Errorf("function %s IsTest = %v, want %v", tt.funcID, info.IsTest, tt.expectIsTest)
			}

			callees := g.CallGraph[tt.funcID]
			for _, wantCallee := range tt.wantCallees {
				if !slices.Contains(callees, wantCallee) {
					t.Errorf("%s callees = %v, want to contain %s", tt.funcID, callees, wantCallee)
				}
			}

			callers := g.ReverseGraph[tt.funcID]
			for _, wantCaller := range tt.wantCallers {
				if !slices.Contains(callers, wantCaller) {
					t.Errorf("%s callers (reverse graph) = %v, want to contain %s", tt.funcID, callers, wantCaller)
				}
			}
		})
	}
}

func TestMapChangedFunctions(t *testing.T) {
	fixtureDir, err := filepath.Abs("../../testdata/fixture")
	if err != nil {
		t.Fatalf("failed to resolve fixture directory: %v", err)
	}

	g, err := Build(fixtureDir)
	if err != nil {
		t.Fatalf("Build() failed: %v", err)
	}

	addInfo, ok := g.Functions["calc.Add"]
	if !ok {
		t.Fatal("calc.Add not found in graph")
	}

	tests := []struct {
		name         string
		changedFiles []diffengine.ChangedFile
		wantFuncs    []string
	}{
		{
			name: "line range inside Add function",
			changedFiles: []diffengine.ChangedFile{
				{
					Path: addInfo.File,
					Lines: []diffengine.LineRange{
						{Start: addInfo.StartLine, End: addInfo.StartLine + 1},
					},
				},
			},
			wantFuncs: []string{"calc.Add"},
		},
		{
			name: "line range outside all functions (e.g. package comment)",
			changedFiles: []diffengine.ChangedFile{
				{
					Path: addInfo.File,
					Lines: []diffengine.LineRange{
						{Start: 1, End: 2},
					},
				},
			},
			wantFuncs: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := g.MapChangedFunctions(tt.changedFiles)
			if len(got) != len(tt.wantFuncs) {
				t.Fatalf("MapChangedFunctions() = %v, want %v", got, tt.wantFuncs)
			}
			for i := range got {
				if got[i] != tt.wantFuncs[i] {
					t.Errorf("MapChangedFunctions()[%d] = %s, want %s", i, got[i], tt.wantFuncs[i])
				}
			}
		})
	}
}
