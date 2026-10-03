package reporter

import (
	"fmt"
	"os"
	"strings"
)

// CommentData contains the metrics and metadata required to render a PR comment.
type CommentData struct {
	Ref1             string
	Ref2             string
	CacheHit         bool
	AnalysisDuration string
	TotalTests       int
	ImpactedTests    []string
	TestsSkipped     int
	BaselineMs       int64
	ActualRunMs      int64
	CalibrationMs    int64 // > 0 if baseline calibration ran inline on this execution
	PctSaved         float64
	ChangedFiles     []string
	ChangedFunctions []string
	Command          string
	Success          bool
}

// RenderPRComment formats the test impact metrics into GitHub Flavored Markdown.
func RenderPRComment(data CommentData) string {
	var sb strings.Builder

	reductionPct := 0.0
	if data.TotalTests > 0 {
		reductionPct = (float64(data.TestsSkipped) / float64(data.TotalTests)) * 100.0
	}

	testStatus := "✅ **Passed**"
	if len(data.ImpactedTests) == 0 {
		testStatus = "⚪ **Skipped (No Impact)**"
	} else if !data.Success {
		testStatus = "❌ **Failed**"
	}

	sb.WriteString("### ⚡ Diffr — Test Impact Analysis\n\n")

	sb.WriteString("| Metric | Result |\n")
	sb.WriteString("| :--- | :--- |\n")
	sb.WriteString(fmt.Sprintf("| **Commit Range** | `%s..%s` |\n", data.Ref1, data.Ref2))
	sb.WriteString(fmt.Sprintf("| **Tests Executed** | **%d** of **%d** (%d skipped — **%.1f%%** test count reduction) |\n",
		len(data.ImpactedTests), data.TotalTests, data.TestsSkipped, reductionPct))

	if data.CalibrationMs > 0 {
		// First-run / cold-run where baseline calibration ran inline
		totalPipelineMs := data.CalibrationMs + data.ActualRunMs
		sb.WriteString(fmt.Sprintf("| **Scoped Test Run Time** | **%d ms** |\n", data.ActualRunMs))
		sb.WriteString(fmt.Sprintf("| **One-Time Calibration Overhead** | **%d ms** (full-suite baseline measurement) |\n", data.CalibrationMs))
		sb.WriteString(fmt.Sprintf("| **Total Pipeline Time** | **%d ms** (calibration + scoped run) |\n", totalPipelineMs))
		sb.WriteString(fmt.Sprintf("| **Cache Status** | 🟡 **Cold Run (Calibration Phase)** (%s analysis) |\n", data.AnalysisDuration))
	} else {
		// Warm steady-state run where baseline was cached
		msSaved := data.BaselineMs - data.ActualRunMs
		if msSaved < 0 {
			msSaved = 0
		}
		cacheStatus := fmt.Sprintf("🟢 **Warm Cache Hit** (%s retrieval)", data.AnalysisDuration)
		if !data.CacheHit {
			cacheStatus = fmt.Sprintf("🟡 **Cold Cache Miss** (%s analysis)", data.AnalysisDuration)
		}
		sb.WriteString(fmt.Sprintf("| **Execution Time** | **%d ms** (full-suite baseline: %d ms) |\n", data.ActualRunMs, data.BaselineMs))
		sb.WriteString(fmt.Sprintf("| **Compute Time Saved** | **%d ms** (**%.1f%%** reduction) |\n", msSaved, data.PctSaved))
		sb.WriteString(fmt.Sprintf("| **Cache Status** | %s |\n", cacheStatus))
	}

	sb.WriteString(fmt.Sprintf("| **Test Status** | %s |\n\n", testStatus))

	// Contextual Callout
	if data.CalibrationMs > 0 {
		sb.WriteString(fmt.Sprintf("> ℹ️ **First-Run Calibration Notice**: Because this was the initial execution on this repository, Diffr ran an inline full-suite calibration (**%d ms**) to establish the baseline and cached it for 7 days.\n", data.CalibrationMs))
		sb.WriteString(fmt.Sprintf("> - **Scoped tests ran**: %d of %d tests were executed in %d ms.\n", len(data.ImpactedTests), data.TotalTests, data.ActualRunMs))
		sb.WriteString("> - **Subsequent PR runs**: Calibration overhead will be **0 ms**, so only the scoped test execution time applies.\n\n")
	} else if data.CacheHit {
		sb.WriteString(fmt.Sprintf("> ⚡ **Fast-path cache hit**: Retrieved scoped tests and package mappings in %s, skipping AST re-parsing.\n\n", data.AnalysisDuration))
	} else if !data.CacheHit {
		sb.WriteString(fmt.Sprintf("> 🔍 **AST analysis completed**: Analyzed changed files and reverse call graph in %s.\n\n", data.AnalysisDuration))
	}

	if len(data.ImpactedTests) == 0 {
		sb.WriteString("> ✨ **Zero tests impacted**: Changed files do not affect existing test paths. Full test suite safely bypassed.\n\n")
	}

	// Collapsible details for files, functions, and tests
	sb.WriteString(fmt.Sprintf("<details>\n<summary><b>Impact Details (%d changed / %d tests)</b></summary>\n\n",
		len(data.ChangedFunctions), len(data.ImpactedTests)))

	if len(data.ChangedFiles) > 0 {
		sb.WriteString(fmt.Sprintf("**Changed Files (%d):**\n", len(data.ChangedFiles)))
		for _, f := range data.ChangedFiles {
			sb.WriteString(fmt.Sprintf("- `%s`\n", f))
		}
		sb.WriteString("\n")
	}

	if len(data.ChangedFunctions) > 0 {
		sb.WriteString(fmt.Sprintf("**Changed Functions (%d):**\n", len(data.ChangedFunctions)))
		for _, fn := range data.ChangedFunctions {
			sb.WriteString(fmt.Sprintf("- `%s`\n", fn))
		}
		sb.WriteString("\n")
	}

	if len(data.ImpactedTests) > 0 {
		sb.WriteString(fmt.Sprintf("**Executed Tests (%d):**\n", len(data.ImpactedTests)))
		for _, t := range data.ImpactedTests {
			sb.WriteString(fmt.Sprintf("- `%s`\n", t))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("</details>\n\n")

	if data.Command != "" {
		sb.WriteString(fmt.Sprintf("*Scoped Test Command:* `%s`\n", data.Command))
	}

	return sb.String()
}

// WriteCommentFile writes the rendered markdown to targetPath and optional GITHUB_STEP_SUMMARY.
func WriteCommentFile(targetPath string, data CommentData) error {
	content := RenderPRComment(data)
	if targetPath != "" {
		if err := os.WriteFile(targetPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to write comment file: %w", err)
		}
	}

	if summaryPath := os.Getenv("GITHUB_STEP_SUMMARY"); summaryPath != "" {
		f, err := os.OpenFile(summaryPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = f.WriteString("\n" + content + "\n")
			_ = f.Close()
		}
	}

	return nil
}
