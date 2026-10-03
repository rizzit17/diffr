package api

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"diffr/internal/store"
)

// Response matches the GET /api/runs schema in system-design.md §3.
type Response struct {
	Runs      []*store.RunRecord    `json:"runs"`
	Aggregate *store.AggregateStats `json:"aggregate"`
}

// Server handles dashboard HTTP requests and serves static web assets.
type Server struct {
	Store     *store.Store
	StaticDir string
}

// NewServer creates a new dashboard API and static asset server.
func NewServer(st *store.Store, staticDir string) *Server {
	return &Server{
		Store:     st,
		StaticDir: staticDir,
	}
}

// Handler returns the configured http.Handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/runs", s.handleRuns)
	mux.HandleFunc("/api/health", s.handleHealth)

	// Static web assets
	if s.StaticDir != "" {
		absStatic, err := filepath.Abs(s.StaticDir)
		if err == nil {
			if _, err := os.Stat(absStatic); err == nil {
				fileServer := http.FileServer(http.Dir(absStatic))
				mux.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					// Fallback to index.html for root or missing routes
					if r.URL.Path != "/" {
						target := filepath.Join(absStatic, filepath.Clean(r.URL.Path))
						if _, err := os.Stat(target); os.IsNotExist(err) {
							http.ServeFile(w, r, filepath.Join(absStatic, "index.html"))
							return
						}
					}
					fileServer.ServeHTTP(w, r)
				}))
			}
		}
	}

	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := int64(20)
	if qLimit := r.URL.Query().Get("limit"); qLimit != "" {
		if parsed, err := strconv.ParseInt(qLimit, 10, 64); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	ctx := r.Context()
	runs, err := s.Store.GetRecentRuns(ctx, limit)
	if err != nil {
		http.Error(w, "failed to query runs: "+err.Error(), http.StatusInternalServerError)
		return
	}

	aggregate, err := s.Store.GetStats(ctx)
	if err != nil {
		aggregate = &store.AggregateStats{
			TotalRuns:       len(runs),
			AvgPctTimeSaved: 0,
			TotalMsSaved:    0,
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(Response{
		Runs:      runs,
		Aggregate: aggregate,
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "ok",
		"mongo_connected": s.Store.IsConnected(),
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	})
}
