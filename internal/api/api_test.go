package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"diffr/internal/store"
)

func TestServer_HandleRuns(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "diffr_api_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	st := store.New("mongodb://non-existent-host:27017", tempDir)
	srv := NewServer(st, "")

	// Insert dummy run into store
	rec := &store.RunRecord{
		RepoPath:            tempDir,
		Ref1:                "abc1234",
		Ref2:                "def5678",
		Timestamp:           time.Now().UTC(),
		ChangedFiles:        []string{"pkg/math.go"},
		ChangedFunctions:    []string{"math.Add"},
		ImpactedTests:       []string{"math.TestAdd"},
		TotalTestsInRepo:    10,
		TestsSkipped:        9,
		BaselineFullSuiteMs: 500,
		ActualRunMs:         100,
		PctTimeSaved:        80.0,
		CacheHit:            false,
	}
	_ = st.SaveRun(t.Context(), rec)

	req := httptest.NewRequest(http.MethodGet, "/api/runs?limit=10", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var resp Response
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if len(resp.Runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(resp.Runs))
	}

	if resp.Runs[0].Ref1 != "abc1234" {
		t.Errorf("expected Ref1 'abc1234', got '%s'", resp.Runs[0].Ref1)
	}

	if resp.Aggregate.TotalRuns != 1 {
		t.Errorf("expected Aggregate.TotalRuns 1, got %d", resp.Aggregate.TotalRuns)
	}
}
