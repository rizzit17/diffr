package cache

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
)

func TestComputeRepoHash(t *testing.T) {
	h1 := ComputeRepoHash(".")
	if len(h1) != 12 {
		t.Fatalf("expected hash length 12, got %d (%s)", len(h1), h1)
	}

	// Repeated computation on same directory should be deterministic
	h2 := ComputeRepoHash(".")
	if h1 != h2 {
		t.Errorf("expected deterministic hash: %s != %s", h1, h2)
	}

	// Test fallback path with a non-git directory
	tmp := t.TempDir()
	h3 := ComputeRepoHash(tmp)
	if len(h3) != 12 {
		t.Fatalf("expected hash length 12 for temp dir, got %d (%s)", len(h3), h3)
	}
}

func TestImpactKey(t *testing.T) {
	key := impactKey("abc123def456", "HEAD~1", "HEAD")
	expected := "diffr:abc123def456:HEAD~1:HEAD"
	if key != expected {
		t.Errorf("impactKey() = %s, want %s", key, expected)
	}

	bKey := baselineKey("abc123def456")
	if !strings.HasPrefix(bKey, "diffr:baseline:") {
		t.Errorf("baselineKey() = %s, expected diffr:baseline: prefix", bKey)
	}
}

func TestDiskFallback_WhenRedisUnreachable(t *testing.T) {
	// Isolate all file operations to a clean temp directory
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ctx := context.Background()

	// 1. Initialize client against an unreachable port to guarantee Redis is offline
	client := New("127.0.0.1:54321")
	defer client.Close()

	if client.available {
		t.Fatal("expected client to be unavailable when Redis cannot be reached")
	}

	repoHash := "testrepo1234"
	ref1 := "feature/branch-auth"
	ref2 := "release/v1.0.0"

	sampleImpact := &CachedImpact{
		ImpactedTests:    []string{"auth.TestLogin", "auth.TestToken"},
		TestPackages:     map[string]string{"auth.TestLogin": "pkg/auth"},
		ChangedFunctions: []string{"auth.ValidateToken"},
		ChangedFiles:     []string{"pkg/auth/token.go"},
		TotalTestsInRepo: 10,
		TestsSkipped:     8,
	}

	// 2. Cache miss before writing
	cached, hit := client.GetImpact(ctx, repoHash, ref1, ref2)
	if hit || cached != nil {
		t.Errorf("expected cache miss, got hit=%v cached=%+v", hit, cached)
	}

	baseline, hitB := client.GetBaseline(ctx, repoHash)
	if hitB || baseline != 0 {
		t.Errorf("expected baseline miss, got hit=%v baseline=%d", hitB, baseline)
	}

	// 3. Write impact to disk fallback
	client.SetImpact(ctx, repoHash, ref1, ref2, sampleImpact)

	// Verify physical file was written with slashes converted to underscores
	expectedImpactPath := filepath.Join(".diffr", "cache_testrepo1234_feature_branch-auth_release_v1.0.0.json")
	if _, err := os.Stat(expectedImpactPath); os.IsNotExist(err) {
		t.Fatalf("expected fallback file %s to exist on disk", expectedImpactPath)
	}

	// 4. Read impact back via disk fallback
	gotImpact, hit := client.GetImpact(ctx, repoHash, ref1, ref2)
	if !hit {
		t.Fatal("expected cache hit from disk fallback")
	}
	if !reflect.DeepEqual(gotImpact, sampleImpact) {
		t.Errorf("GetImpact() = %+v, want %+v", gotImpact, sampleImpact)
	}

	// 5. Write and read baseline via disk fallback
	const expectedBaselineMs int64 = 3450
	client.SetBaseline(ctx, repoHash, expectedBaselineMs)

	expectedBaselinePath := filepath.Join(".diffr", "baseline_testrepo1234.txt")
	if _, err := os.Stat(expectedBaselinePath); os.IsNotExist(err) {
		t.Fatalf("expected fallback file %s to exist on disk", expectedBaselinePath)
	}

	gotBaseline, hitB := client.GetBaseline(ctx, repoHash)
	if !hitB {
		t.Fatal("expected baseline hit from disk fallback")
	}
	if gotBaseline != expectedBaselineMs {
		t.Errorf("GetBaseline() = %d, want %d", gotBaseline, expectedBaselineMs)
	}

	// 6. Test handling of corrupt/malformed disk files
	corruptRef1 := "corrupt"
	corruptRef2 := "corrupt"
	corruptPath := filepath.Join(".diffr", "cache_testrepo1234_corrupt_corrupt.json")
	_ = os.WriteFile(corruptPath, []byte("NOT_VALID_JSON{"), 0644)

	corruptImpact, corruptHit := client.GetImpact(ctx, repoHash, corruptRef1, corruptRef2)
	if corruptHit || corruptImpact != nil {
		t.Errorf("expected failure on corrupt JSON file, got hit=%v", corruptHit)
	}

	corruptBaselinePath := filepath.Join(".diffr", "baseline_corruptrepo.txt")
	_ = os.WriteFile(corruptBaselinePath, []byte("not-a-number"), 0644)

	corruptBase, corruptBaseHit := client.GetBaseline(ctx, "corruptrepo")
	if corruptBaseHit || corruptBase != 0 {
		t.Errorf("expected failure on corrupt baseline file, got hit=%v", corruptBaseHit)
	}
}

func TestRedis_WithMiniredis(t *testing.T) {
	tempDir := t.TempDir()
	t.Chdir(tempDir)

	ctx := context.Background()

	// 1. Start an in-memory Redis server using miniredis
	mr := miniredis.RunT(t)

	client := New(mr.Addr())
	defer client.Close()

	if !client.available {
		t.Fatal("expected client to be available with running miniredis")
	}

	repoHash := "minirepo456"
	ref1 := "HEAD~1"
	ref2 := "HEAD"

	impact := &CachedImpact{
		ImpactedTests:    []string{"calc.TestAdd"},
		ChangedFunctions: []string{"calc.Add"},
		ChangedFiles:     []string{"calc/calc.go"},
		TotalTestsInRepo: 5,
		TestsSkipped:     4,
	}

	// 2. SetImpact writes to miniredis (and disk fallback)
	client.SetImpact(ctx, repoHash, ref1, ref2, impact)

	key := impactKey(repoHash, ref1, ref2)
	if !mr.Exists(key) {
		t.Fatalf("expected key %s to exist in miniredis", key)
	}

	// 3. GetImpact reads from miniredis
	gotImpact, hit := client.GetImpact(ctx, repoHash, ref1, ref2)
	if !hit {
		t.Fatal("expected cache hit from miniredis")
	}
	if !reflect.DeepEqual(gotImpact, impact) {
		t.Errorf("GetImpact() = %+v, want %+v", gotImpact, impact)
	}

	// 4. SetBaseline and GetBaseline with miniredis
	client.SetBaseline(ctx, repoHash, 1850)
	bKey := baselineKey(repoHash)
	val, err := mr.Get(bKey)
	if err != nil || val != "1850" {
		t.Fatalf("miniredis baseline key %s = %s, err = %v", bKey, val, err)
	}

	gotB, hitB := client.GetBaseline(ctx, repoHash)
	if !hitB || gotB != 1850 {
		t.Errorf("GetBaseline() = %d, hit=%v, want 1850, true", gotB, hitB)
	}

	// 5. Test degradation to disk when miniredis is shut down
	mr.Close()

	// Wait brief moment for socket close
	time.Sleep(10 * time.Millisecond)

	// GetImpact should degrade cleanly to reading the disk copy written during SetImpact
	degradedImpact, hitDegraded := client.GetImpact(ctx, repoHash, ref1, ref2)
	if !hitDegraded {
		t.Fatal("expected cache hit via disk fallback after miniredis shutdown")
	}
	if !reflect.DeepEqual(degradedImpact, impact) {
		t.Errorf("degraded GetImpact() = %+v, want %+v", degradedImpact, impact)
	}

	degradedB, hitDegradedB := client.GetBaseline(ctx, repoHash)
	if !hitDegradedB || degradedB != 1850 {
		t.Errorf("degraded GetBaseline() = %d, hit=%v, want 1850, true", degradedB, hitDegradedB)
	}
}

func TestNew_WithEnvAddr(t *testing.T) {
	mr := miniredis.RunT(t)

	t.Setenv("REDIS_ADDR", mr.Addr())
	client := New("")
	defer client.Close()

	if !client.available {
		t.Errorf("expected client using REDIS_ADDR to be available")
	}
}

func TestNoopLogger(t *testing.T) {
	nl := &noopLogger{}
	nl.Printf(context.Background(), "test %s", "message")
}
