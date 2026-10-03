package store

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestStore_LocalFallback(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "diffr_store_test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tempDir)

	s := New("mongodb://non-existent-host:27017", tempDir)
	if s.IsConnected() {
		t.Errorf("expected disconnected status for dummy host")
	}

	ctx := context.Background()

	// Insert run 1
	r1 := &RunRecord{
		RepoPath:            tempDir,
		Ref1:                "HEAD~1",
		Ref2:                "HEAD",
		Timestamp:           time.Now().UTC(),
		ChangedFiles:        []string{"pkg/calc.go"},
		ChangedFunctions:    []string{"calc.Add"},
		ImpactedTests:       []string{"calc.TestAdd"},
		TotalTestsInRepo:    10,
		TestsSkipped:        9,
		BaselineFullSuiteMs: 1000,
		ActualRunMs:         200,
		PctTimeSaved:        80.0,
		CacheHit:            false,
	}

	if err := s.SaveRun(ctx, r1); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	// Insert run 2
	r2 := &RunRecord{
		RepoPath:            tempDir,
		Ref1:                "HEAD~1",
		Ref2:                "HEAD",
		Timestamp:           time.Now().UTC(),
		ChangedFiles:        []string{"pkg/calc.go"},
		ChangedFunctions:    []string{"calc.Add"},
		ImpactedTests:       []string{"calc.TestAdd"},
		TotalTestsInRepo:    10,
		TestsSkipped:        9,
		BaselineFullSuiteMs: 1000,
		ActualRunMs:         10,
		PctTimeSaved:        99.0,
		CacheHit:            true,
	}

	if err := s.SaveRun(ctx, r2); err != nil {
		t.Fatalf("SaveRun() error = %v", err)
	}

	// Fetch recent runs
	runs, err := s.GetRecentRuns(ctx, 10)
	if err != nil {
		t.Fatalf("GetRecentRuns() error = %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}

	// Verify order: newest first
	if !runs[0].CacheHit {
		t.Errorf("expected first returned run to be the newest (cache_hit = true)")
	}

	// Verify stats
	stats, err := s.GetStats(ctx)
	if err != nil {
		t.Fatalf("GetStats() error = %v", err)
	}
	if stats.TotalRuns != 2 {
		t.Errorf("TotalRuns = %d, want 2", stats.TotalRuns)
	}
	expectedSaved := int64((1000 - 200) + (1000 - 10))
	if stats.TotalMsSaved != expectedSaved {
		t.Errorf("TotalMsSaved = %d, want %d", stats.TotalMsSaved, expectedSaved)
	}
	expectedAvgPct := (80.0 + 99.0) / 2.0
	if stats.AvgPctTimeSaved != expectedAvgPct {
		t.Errorf("AvgPctTimeSaved = %f, want %f", stats.AvgPctTimeSaved, expectedAvgPct)
	}
}

func TestLiveMongoIndexes(t *testing.T) {
	s := New("mongodb://127.0.0.1:27017", "")
	if !s.IsConnected() {
		t.Skip("MongoDB not running on localhost:27017, skipping live index verification")
	}
	defer s.Close(t.Context())

	cursor, err := s.collection.Indexes().List(t.Context())
	if err != nil {
		t.Fatalf("Indexes().List() error = %v", err)
	}

	var indexes []bson.M
	if err := cursor.All(t.Context(), &indexes); err != nil {
		t.Fatalf("cursor.All error = %v", err)
	}

	hasCompound := false
	hasTimestamp := false
	for _, idx := range indexes {
		name := idx["name"]
		key := idx["key"]
		t.Logf("Index discovered in MongoDB: name=%v, key=%v", name, key)
		if name == "idx_repo_refs" {
			hasCompound = true
		}
		if name == "idx_timestamp_desc" {
			hasTimestamp = true
		}
	}

	if !hasCompound {
		t.Errorf("expected compound index 'idx_repo_refs' {repo_path: 1, ref1: 1, ref2: 1} to be present")
	}
	if !hasTimestamp {
		t.Errorf("expected timestamp index 'idx_timestamp_desc' {timestamp: -1} to be present")
	}
}
