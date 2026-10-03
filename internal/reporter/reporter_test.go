package reporter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPRComment_ColdRun_WithCalibration(t *testing.T) {
	data := CommentData{
		Ref1:             "origin/main",
		Ref2:             "HEAD",
		CacheHit:         false,
		AnalysisDuration: "58.4ms",
		TotalTests:       14,
		ImpactedTests:    []string{"calc.TestAdd", "service.TestExecuteOperation"},
		TestsSkipped:     12,
		BaselineMs:       1800,
		ActualRunMs:      420,
		CalibrationMs:    1800,
		PctSaved:         76.7,
		ChangedFiles:     []string{"calc/calc.go"},
		ChangedFunctions: []string{"calc.Add"},
		Command:          "go test -v -run ^(TestAdd|TestExecuteOperation)$ ./pkg/calc ./service",
		Success:          true,
	}

	rendered := RenderPRComment(data)

	// Check key markdown elements
	expectedPhrases := []string{
		"### ⚡ Diffr — Test Impact Analysis",
		"| **Commit Range** | `origin/main..HEAD` |",
		"| **Tests Executed** | **2** of **14** (12 skipped — **85.7%** test count reduction) |",
		"| **Scoped Test Run Time** | **420 ms** |",
		"| **One-Time Calibration Overhead** | **1800 ms** (full-suite baseline measurement) |",
		"| **Total Pipeline Time** | **2220 ms** (calibration + scoped run) |",
		"🟡 **Cold Run (Calibration Phase)** (58.4ms analysis)",
		"✅ **Passed**",
		"First-Run Calibration Notice",
		"<summary><b>Impact Details (1 changed / 2 tests)</b></summary>",
		"`calc/calc.go`",
		"`calc.Add`",
		"`calc.TestAdd`",
		"`service.TestExecuteOperation`",
		"*Scoped Test Command:* `go test -v -run ^(TestAdd|TestExecuteOperation)$ ./pkg/calc ./service`",
	}

	for _, phrase := range expectedPhrases {
		if !strings.Contains(rendered, phrase) {
			t.Errorf("rendered comment missing expected phrase: %q", phrase)
		}
	}
}

func TestRenderPRComment_WarmRun(t *testing.T) {
	data := CommentData{
		Ref1:             "origin/main",
		Ref2:             "HEAD",
		CacheHit:         true,
		AnalysisDuration: "512µs",
		TotalTests:       14,
		ImpactedTests:    []string{"calc.TestAdd"},
		TestsSkipped:     13,
		BaselineMs:       1800,
		ActualRunMs:      210,
		CalibrationMs:    0,
		PctSaved:         88.3,
		ChangedFiles:     []string{"calc/calc.go"},
		ChangedFunctions: []string{"calc.Add"},
		Command:          "go test -v -run ^(TestAdd)$ ./pkg/calc",
		Success:          true,
	}

	rendered := RenderPRComment(data)

	if !strings.Contains(rendered, "🟢 **Warm Cache Hit** (512µs retrieval)") {
		t.Errorf("expected warm cache badge, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Fast-path cache hit") {
		t.Errorf("expected fast-path cache notice, got: %s", rendered)
	}
	if !strings.Contains(rendered, "| **Execution Time** | **210 ms** (full-suite baseline: 1800 ms) |") {
		t.Errorf("expected execution time row, got: %s", rendered)
	}
	if !strings.Contains(rendered, "| **Compute Time Saved** | **1590 ms** (**88.3%** reduction) |") {
		t.Errorf("expected compute time saved row, got: %s", rendered)
	}
}

func TestRenderPRComment_ZeroImpact(t *testing.T) {
	data := CommentData{
		Ref1:             "origin/main",
		Ref2:             "HEAD",
		CacheHit:         true,
		AnalysisDuration: "490µs",
		TotalTests:       14,
		ImpactedTests:    nil,
		TestsSkipped:     14,
		BaselineMs:       1800,
		ActualRunMs:      0,
		CalibrationMs:    0,
		PctSaved:         100.0,
		ChangedFiles:     []string{"README.md"},
		ChangedFunctions: nil,
		Command:          "",
		Success:          true,
	}

	rendered := RenderPRComment(data)

	if !strings.Contains(rendered, "⚪ **Skipped (No Impact)**") {
		t.Errorf("expected skipped status, got: %s", rendered)
	}
	if !strings.Contains(rendered, "Zero tests impacted") {
		t.Errorf("expected zero tests notice, got: %s", rendered)
	}
}

func TestRenderPRComment_TestFailure(t *testing.T) {
	data := CommentData{
		Ref1:          "origin/main",
		Ref2:          "HEAD",
		ImpactedTests: []string{"calc.TestAdd"},
		TotalTests:    10,
		Success:       false,
	}

	rendered := RenderPRComment(data)

	if !strings.Contains(rendered, "❌ **Failed**") {
		t.Errorf("expected failed status, got: %s", rendered)
	}
}

func TestWriteCommentFile(t *testing.T) {
	tempDir := t.TempDir()
	commentPath := filepath.Join(tempDir, "comment.md")
	summaryPath := filepath.Join(tempDir, "step_summary.md")

	t.Setenv("GITHUB_STEP_SUMMARY", summaryPath)

	data := CommentData{
		Ref1:          "main",
		Ref2:          "HEAD",
		TotalTests:    5,
		ImpactedTests: []string{"TestA"},
		Success:       true,
	}

	if err := WriteCommentFile(commentPath, data); err != nil {
		t.Fatalf("WriteCommentFile() failed: %v", err)
	}

	// Verify file was written
	content, err := os.ReadFile(commentPath)
	if err != nil {
		t.Fatalf("failed to read comment file: %v", err)
	}
	if !strings.Contains(string(content), "Diffr — Test Impact Analysis") {
		t.Errorf("comment file content invalid: %s", string(content))
	}

	// Verify step summary was written
	summaryContent, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatalf("failed to read step summary file: %v", err)
	}
	if !strings.Contains(string(summaryContent), "Diffr — Test Impact Analysis") {
		t.Errorf("summary file content invalid: %s", string(summaryContent))
	}
}
