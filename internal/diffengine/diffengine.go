package diffengine

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// LineRange represents a 1-based inclusive range of lines changed in a file.
type LineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// ChangedFile represents a repository file modified between two git references.
type ChangedFile struct {
	Path  string      `json:"path"`
	Lines []LineRange `json:"lines"`
}

var (
	// Matches unified diff hunk headers: @@ -oldStart[,oldLen] +newStart[,newLen] @@
	hunkHeaderRegex = regexp.MustCompile(`^@@\s+-\d+(?:,\d+)?\s+\+(\d+)(?:,(\d+))?\s+@@`)
)

// Engine executes git diff commands and parses file line changes.
type Engine struct {
	RepoDir string
}

// New creates a new diff engine for the given repository path.
func New(repoDir string) *Engine {
	return &Engine{RepoDir: repoDir}
}

// GetDiff runs git diff -U0 between ref1 and ref2 and extracts changed Go files and line ranges.
func (e *Engine) GetDiff(ref1, ref2 string) ([]ChangedFile, error) {
	args := []string{"diff", "-U0"}
	if ref1 != "" {
		args = append(args, ref1)
	}
	if ref2 != "" {
		args = append(args, ref2)
	}

	cmd := exec.Command("git", args...)
	if e.RepoDir != "" {
		cmd.Dir = e.RepoDir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git diff failed: %w (stderr: %s)", err, strings.TrimSpace(stderr.String()))
	}

	return ParseUnifiedDiff(stdout.Bytes())
}

// ParseUnifiedDiff parses unified diff output (-U0) into structured ChangedFile slices.
func ParseUnifiedDiff(diffOutput []byte) ([]ChangedFile, error) {
	scanner := bufio.NewScanner(bytes.NewReader(diffOutput))
	var files []ChangedFile
	var currentFile *ChangedFile

	for scanner.Scan() {
		line := scanner.Text()

		// New file diff header: diff --git a/path b/path
		if strings.HasPrefix(line, "diff --git ") {
			if currentFile != nil && len(currentFile.Lines) > 0 {
				files = append(files, *currentFile)
			}
			currentFile = nil
			continue
		}

		// Target file path line: +++ b/path or +++ /dev/null
		if strings.HasPrefix(line, "+++ ") {
			targetPath := strings.TrimPrefix(line, "+++ ")
			if targetPath == "/dev/null" {
				// File was deleted
				currentFile = nil
				continue
			}
			// Strip prefix "b/"
			if strings.HasPrefix(targetPath, "b/") {
				targetPath = targetPath[2:]
			}
			// Normalize path separators to forward slash
			targetPath = filepath.ToSlash(targetPath)

			// Only process Go files
			if strings.HasSuffix(targetPath, ".go") {
				currentFile = &ChangedFile{
					Path:  targetPath,
					Lines: []LineRange{},
				}
			} else {
				currentFile = nil
			}
			continue
		}

		// Hunk header @@ ... @@
		if strings.HasPrefix(line, "@@") && currentFile != nil {
			matches := hunkHeaderRegex.FindStringSubmatch(line)
			if len(matches) >= 2 {
				startLine, err := strconv.Atoi(matches[1])
				if err != nil {
					continue
				}
				lineCount := 1
				if len(matches) >= 3 && matches[2] != "" {
					lineCount, err = strconv.Atoi(matches[2])
					if err != nil {
						lineCount = 1
					}
				}

				var lr LineRange
				if lineCount == 0 {
					// Pure deletion: line in new file where deletion occurred
					lr = LineRange{Start: startLine, End: startLine}
				} else {
					lr = LineRange{Start: startLine, End: startLine + lineCount - 1}
				}
				currentFile.Lines = append(currentFile.Lines, lr)
			}
		}
	}

	if currentFile != nil && len(currentFile.Lines) > 0 {
		files = append(files, *currentFile)
	}

	return files, scanner.Err()
}
