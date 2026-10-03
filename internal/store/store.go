package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

const (
	DefaultMongoURI = "mongodb://localhost:27017"
	DefaultDatabase = "diffr"
	RunsCollection  = "runs"
	LocalFallbackDir = ".diffr"
	LocalFallbackFile = ".diffr/runs.json"
)

// RunRecord represents a single Test Impact Analysis execution record.
type RunRecord struct {
	ID                  bson.ObjectID `bson:"_id,omitempty" json:"id"`
	RepoPath            string        `bson:"repo_path" json:"repo_path"`
	Ref1                string        `bson:"ref1" json:"ref1"`
	Ref2                string        `bson:"ref2" json:"ref2"`
	Timestamp           time.Time     `bson:"timestamp" json:"timestamp"`
	ChangedFiles        []string      `bson:"changed_files" json:"changed_files"`
	ChangedFunctions    []string      `bson:"changed_functions" json:"changed_functions"`
	ImpactedTests       []string      `bson:"impacted_tests" json:"impacted_tests"`
	TotalTestsInRepo    int           `bson:"total_tests_in_repo" json:"total_tests_in_repo"`
	TestsSkipped        int           `bson:"tests_skipped" json:"tests_skipped"`
	BaselineFullSuiteMs int64         `bson:"baseline_full_suite_ms" json:"baseline_full_suite_ms"`
	ActualRunMs         int64         `bson:"actual_run_ms" json:"actual_run_ms"`
	PctTimeSaved        float64       `bson:"pct_time_saved" json:"pct_time_saved"`
	CacheHit            bool          `bson:"cache_hit" json:"cache_hit"`
}

// AggregateStats holds summary metrics across runs.
type AggregateStats struct {
	TotalRuns       int     `json:"total_runs"`
	AvgPctTimeSaved float64 `json:"avg_pct_time_saved"`
	TotalMsSaved    int64   `json:"total_ms_saved"`
}

// Store handles persisting run metrics to MongoDB with local file fallback.
type Store struct {
	client     *mongo.Client
	collection *mongo.Collection
	connected  bool
	repoDir    string
	mu         sync.Mutex
}

// New connects to MongoDB and initializes compound indexes.
func New(uri string, repoDir string) *Store {
	if uri == "" {
		uri = os.Getenv("MONGO_URI")
		if uri == "" {
			uri = DefaultMongoURI
		}
	}

	s := &Store{
		repoDir: repoDir,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	clientOpts := options.Client().
		ApplyURI(uri).
		SetConnectTimeout(500 * time.Millisecond).
		SetServerSelectionTimeout(500 * time.Millisecond)

	client, err := mongo.Connect(clientOpts)
	if err == nil && client.Ping(ctx, nil) == nil {
		s.client = client
		s.collection = client.Database(DefaultDatabase).Collection(RunsCollection)
		s.connected = true

		// Create compound index: {repo_path: 1, ref1: 1, ref2: 1} and {timestamp: -1}
		_, _ = s.collection.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
			{
				Keys: bson.D{
					{Key: "repo_path", Value: 1},
					{Key: "ref1", Value: 1},
					{Key: "ref2", Value: 1},
				},
				Options: options.Index().SetName("idx_repo_refs"),
			},
			{
				Keys: bson.D{
					{Key: "timestamp", Value: -1},
				},
				Options: options.Index().SetName("idx_timestamp_desc"),
			},
		})
	}

	return s
}

// SaveRun stores a run record in MongoDB or falls back to local JSON store.
func (s *Store) SaveRun(ctx context.Context, record *RunRecord) error {
	if record.Timestamp.IsZero() {
		record.Timestamp = time.Now().UTC()
	}

	if s.connected && s.collection != nil {
		_, err := s.collection.InsertOne(ctx, record)
		if err == nil {
			return nil
		}
	}

	// Fallback: save to local json file
	return s.saveLocal(record)
}

func (s *Store) saveLocal(record *RunRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := filepath.Join(s.repoDir, LocalFallbackDir)
	_ = os.MkdirAll(dir, 0755)

	filePath := filepath.Join(s.repoDir, LocalFallbackFile)
	var runs []*RunRecord

	data, err := os.ReadFile(filePath)
	if err == nil {
		_ = json.Unmarshal(data, &runs)
	}

	runs = append(runs, record)
	out, err := json.MarshalIndent(runs, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filePath, out, 0644)
}

// GetRecentRuns fetches up to limit runs sorted newest first.
func (s *Store) GetRecentRuns(ctx context.Context, limit int64) ([]*RunRecord, error) {
	if s.connected && s.collection != nil {
		findOpts := options.Find().
			SetSort(bson.D{{Key: "timestamp", Value: -1}}).
			SetLimit(limit)

		cursor, err := s.collection.Find(ctx, bson.D{}, findOpts)
		if err == nil {
			var runs []*RunRecord
			if err := cursor.All(ctx, &runs); err == nil {
				return runs, nil
			}
		}
	}

	// Fallback: read from local file
	return s.getLocalRuns(limit)
}

func (s *Store) getLocalRuns(limit int64) ([]*RunRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	filePath := filepath.Join(s.repoDir, LocalFallbackFile)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return []*RunRecord{}, nil
	}

	var runs []*RunRecord
	if err := json.Unmarshal(data, &runs); err != nil {
		return []*RunRecord{}, nil
	}

	// Reverse to newest first
	n := len(runs)
	for i := 0; i < n/2; i++ {
		runs[i], runs[n-1-i] = runs[n-1-i], runs[i]
	}

	if limit > 0 && int64(len(runs)) > limit {
		runs = runs[:limit]
	}

	return runs, nil
}

// GetStats computes aggregate metrics across all runs.
func (s *Store) GetStats(ctx context.Context) (*AggregateStats, error) {
	if s.connected && s.collection != nil {
		pipeline := mongo.Pipeline{
			bson.D{{Key: "$group", Value: bson.D{
				{Key: "_id", Value: nil},
				{Key: "total_runs", Value: bson.D{{Key: "$sum", Value: 1}}},
				{Key: "avg_pct_time_saved", Value: bson.D{{Key: "$avg", Value: "$pct_time_saved"}}},
				{Key: "total_baseline_ms", Value: bson.D{{Key: "$sum", Value: "$baseline_full_suite_ms"}}},
				{Key: "total_actual_ms", Value: bson.D{{Key: "$sum", Value: "$actual_run_ms"}}},
			}}},
		}

		cursor, err := s.collection.Aggregate(ctx, pipeline)
		if err == nil {
			var results []struct {
				TotalRuns        int     `bson:"total_runs"`
				AvgPctTimeSaved  float64 `bson:"avg_pct_time_saved"`
				TotalBaselineMs  int64   `bson:"total_baseline_ms"`
				TotalActualMs    int64   `bson:"total_actual_ms"`
			}
			if err := cursor.All(ctx, &results); err == nil && len(results) > 0 {
				r := results[0]
				saved := r.TotalBaselineMs - r.TotalActualMs
				if saved < 0 {
					saved = 0
				}
				return &AggregateStats{
					TotalRuns:       r.TotalRuns,
					AvgPctTimeSaved: r.AvgPctTimeSaved,
					TotalMsSaved:    saved,
				}, nil
			}
		}
	}

	// Fallback calculation from local runs
	runs, err := s.getLocalRuns(0)
	if err != nil || len(runs) == 0 {
		return &AggregateStats{TotalRuns: 0, AvgPctTimeSaved: 0, TotalMsSaved: 0}, nil
	}

	totalRuns := len(runs)
	var sumPct float64
	var sumSaved int64

	for _, r := range runs {
		sumPct += r.PctTimeSaved
		saved := r.BaselineFullSuiteMs - r.ActualRunMs
		if saved > 0 {
			sumSaved += saved
		}
	}

	return &AggregateStats{
		TotalRuns:       totalRuns,
		AvgPctTimeSaved: sumPct / float64(totalRuns),
		TotalMsSaved:    sumSaved,
	}, nil
}

// Close disconnects the MongoDB client.
func (s *Store) Close(ctx context.Context) error {
	if s.client != nil {
		return s.client.Disconnect(ctx)
	}
	return nil
}

// IsConnected returns whether the MongoDB connection is alive.
func (s *Store) IsConnected() bool {
	return s.connected
}

// FormatStats prints a human-readable summary table.
func (a *AggregateStats) String() string {
	return fmt.Sprintf("Total Runs: %d | Avg Time Saved: %.1f%% | Total Time Saved: %d ms",
		a.TotalRuns, a.AvgPctTimeSaved, a.TotalMsSaved)
}
