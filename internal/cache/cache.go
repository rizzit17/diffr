package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	DefaultImpactTTL   = 24 * time.Hour
	DefaultBaselineTTL = 7 * 24 * time.Hour
	DefaultRedisAddr   = "127.0.0.1:6379"
)

type noopLogger struct{}

func (n *noopLogger) Printf(ctx context.Context, format string, v ...interface{}) {}

func init() {
	redis.SetLogger(&noopLogger{})
}

// CachedImpact holds the cached analysis results for a commit pair.
type CachedImpact struct {
	ImpactedTests    []string          `json:"impacted_tests"`
	TestPackages     map[string]string `json:"test_packages,omitempty"`
	ChangedFunctions []string          `json:"changed_functions"`
	ChangedFiles     []string          `json:"changed_files"`
	TotalTestsInRepo int               `json:"total_tests_in_repo"`
	TestsSkipped     int               `json:"tests_skipped"`
}

// Client wraps a Redis client with resilient cache operations.
type Client struct {
	rdb       *redis.Client
	available bool
}

// New creates a new Redis cache client. If Redis is unreachable, it logs a warning and degrades gracefully.
func New(addr string) *Client {
	if addr == "" {
		addr = os.Getenv("REDIS_ADDR")
		if addr == "" {
			addr = DefaultRedisAddr
		}
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  30 * time.Millisecond,
		ReadTimeout:  30 * time.Millisecond,
		WriteTimeout: 30 * time.Millisecond,
		MaxRetries:   0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Millisecond)
	defer cancel()

	available := true
	if err := rdb.Ping(ctx).Err(); err != nil {
		available = false
	}

	return &Client{
		rdb:       rdb,
		available: available,
	}
}

// ComputeRepoHash calculates a 12-char SHA-256 hash from the repo remote URL or absolute path.
func ComputeRepoHash(repoPath string) string {
	absPath, err := filepath.Abs(repoPath)
	if err != nil {
		absPath = repoPath
	}

	// Try reading .git/config directly first for zero-overhead hash calculation
	gitConfigPath := filepath.Join(absPath, ".git", "config")
	if data, err := os.ReadFile(gitConfigPath); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "url =") {
				url := strings.TrimSpace(strings.TrimPrefix(line, "url ="))
				if url != "" {
					hash := sha256.Sum256([]byte(url))
					return hex.EncodeToString(hash[:])[:12]
				}
			}
		}
	}

	// Fallback to git remote get-url origin
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = absPath
	out, err := cmd.Output()
	source := strings.TrimSpace(string(out))
	if err != nil || source == "" {
		source = filepath.ToSlash(absPath)
	}

	hash := sha256.Sum256([]byte(source))
	return hex.EncodeToString(hash[:])[:12]
}

func impactKey(repoHash, ref1, ref2 string) string {
	return fmt.Sprintf("diffr:%s:%s:%s", repoHash, ref1, ref2)
}

func baselineKey(repoHash string) string {
	return fmt.Sprintf("diffr:baseline:%s", repoHash)
}

// GetImpact attempts to retrieve cached impact analysis for the commit pair.
func (c *Client) GetImpact(ctx context.Context, repoHash, ref1, ref2 string) (*CachedImpact, bool) {
	if c.available && c.rdb != nil {
		key := impactKey(repoHash, ref1, ref2)
		val, err := c.rdb.Get(ctx, key).Result()
		if err == nil {
			var cached CachedImpact
			if err := json.Unmarshal([]byte(val), &cached); err == nil {
				return &cached, true
			}
		}
	}

	// Local disk fallback
	safeRef1 := strings.ReplaceAll(ref1, "/", "_")
	safeRef2 := strings.ReplaceAll(ref2, "/", "_")
	fallbackPath := filepath.Join(".diffr", fmt.Sprintf("cache_%s_%s_%s.json", repoHash, safeRef1, safeRef2))
	if data, err := os.ReadFile(fallbackPath); err == nil {
		var cached CachedImpact
		if err := json.Unmarshal(data, &cached); err == nil {
			return &cached, true
		}
	}

	return nil, false
}

// SetImpact saves the resolved impact results with a 24h TTL.
func (c *Client) SetImpact(ctx context.Context, repoHash, ref1, ref2 string, impact *CachedImpact) {
	data, err := json.Marshal(impact)
	if err != nil {
		return
	}

	if c.available && c.rdb != nil {
		key := impactKey(repoHash, ref1, ref2)
		_ = c.rdb.Set(ctx, key, data, DefaultImpactTTL).Err()
	}

	// Always write local disk fallback so repeated runs without Redis stay sub-millisecond
	_ = os.MkdirAll(".diffr", 0755)
	safeRef1 := strings.ReplaceAll(ref1, "/", "_")
	safeRef2 := strings.ReplaceAll(ref2, "/", "_")
	fallbackPath := filepath.Join(".diffr", fmt.Sprintf("cache_%s_%s_%s.json", repoHash, safeRef1, safeRef2))
	_ = os.WriteFile(fallbackPath, data, 0644)
}

// GetBaseline retrieves the cached full-suite baseline execution time in ms.
func (c *Client) GetBaseline(ctx context.Context, repoHash string) (int64, bool) {
	if c.available && c.rdb != nil {
		key := baselineKey(repoHash)
		val, err := c.rdb.Get(ctx, key).Result()
		if err == nil {
			ms, err := strconv.ParseInt(val, 10, 64)
			if err == nil {
				return ms, true
			}
		}
	}

	// Local fallback file
	fallbackPath := filepath.Join(".diffr", fmt.Sprintf("baseline_%s.txt", repoHash))
	if data, err := os.ReadFile(fallbackPath); err == nil {
		ms, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
		if err == nil {
			return ms, true
		}
	}

	return 0, false
}

// SetBaseline stores the full-suite baseline execution time with a 7-day TTL.
func (c *Client) SetBaseline(ctx context.Context, repoHash string, ms int64) {
	val := strconv.FormatInt(ms, 10)
	if c.available && c.rdb != nil {
		key := baselineKey(repoHash)
		_ = c.rdb.Set(ctx, key, val, DefaultBaselineTTL).Err()
	}

	// Always write local fallback
	_ = os.MkdirAll(".diffr", 0755)
	fallbackPath := filepath.Join(".diffr", fmt.Sprintf("baseline_%s.txt", repoHash))
	_ = os.WriteFile(fallbackPath, []byte(val), 0644)
}

// Close closes the underlying Redis connection pool.
func (c *Client) Close() error {
	if c.rdb != nil {
		return c.rdb.Close()
	}
	return nil
}
