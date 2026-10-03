package cache

import (
	"strings"
	"testing"
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
