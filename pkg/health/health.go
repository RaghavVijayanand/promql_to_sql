package health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// HealthChecker provides health check endpoints
type HealthChecker struct {
	startTime time.Time
	version   string
	ready     atomic.Bool // atomic for thread-safe access from HTTP handlers
}

// NewHealthChecker creates a new health checker
func NewHealthChecker(version string) *HealthChecker {
	h := &HealthChecker{
		startTime: time.Now(),
		version:   version,
	}
	h.ready.Store(true)
	return h
}

// SetReady sets the ready state (thread-safe)
func (h *HealthChecker) SetReady(ready bool) {
	h.ready.Store(ready)
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status  string    `json:"status"`
	Version string    `json:"version"`
	Uptime  string    `json:"uptime"`
	Time    time.Time `json:"time"`
}

// ReadyResponse represents readiness check response
type ReadyResponse struct {
	Ready   bool      `json:"ready"`
	Version string    `json:"version"`
	Time    time.Time `json:"time"`
}

// VersionResponse represents version info response
type VersionResponse struct {
	Version   string    `json:"version"`
	BuildTime string    `json:"build_time"`
	GoVersion string    `json:"go_version"`
	GitCommit string    `json:"git_commit"`
	Time      time.Time `json:"time"`
}

// HealthHandler returns health check handler
func (h *HealthChecker) HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := HealthResponse{
			Status:  "ok",
			Version: h.version,
			Uptime:  time.Since(h.startTime).String(),
			Time:    time.Now(),
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// ReadyHandler returns readiness check handler
func (h *HealthChecker) ReadyHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		isReady := h.ready.Load()
		response := ReadyResponse{
			Ready:   isReady,
			Version: h.version,
			Time:    time.Now(),
		}
		
		status := http.StatusOK
		if !isReady {
			status = http.StatusServiceUnavailable
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(response)
	}
}

// VersionHandler returns version info handler
func (h *HealthChecker) VersionHandler(buildTime, goVersion, gitCommit string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		response := VersionResponse{
			Version:   h.version,
			BuildTime: buildTime,
			GoVersion: goVersion,
			GitCommit: gitCommit,
			Time:      time.Now(),
		}
		
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(response)
	}
}

// RegisterHandlers registers health check handlers to mux
func (h *HealthChecker) RegisterHandlers(mux *http.ServeMux, buildTime, goVersion, gitCommit string) {
	mux.HandleFunc("/health", h.HealthHandler())
	mux.HandleFunc("/ready", h.ReadyHandler())
	mux.HandleFunc("/version", h.VersionHandler(buildTime, goVersion, gitCommit))
}

// SimpleHealthCheck performs a simple health check
func SimpleHealthCheck() error {
	// Check basic functionality
	return nil
}

// DetailedHealthCheck performs detailed health checks
func DetailedHealthCheck() map[string]error {
	checks := make(map[string]error)
	
	// Check memory
	checks["memory"] = checkMemory()
	
	// Check goroutines
	checks["goroutines"] = checkGoroutines()
	
	return checks
}

func checkMemory() error {
	// Simple memory check - in production would check actual memory usage
	return nil
}

func checkGoroutines() error {
	// Check goroutine count - in production would check for goroutine leaks
	return nil
}

// PrintHealthInfo prints health information
func (h *HealthChecker) PrintHealthInfo() {
	fmt.Printf("Health Check:\n")
	fmt.Printf("  Status: ok\n")
	fmt.Printf("  Version: %s\n", h.version)
	fmt.Printf("  Uptime: %s\n", time.Since(h.startTime).String())
	fmt.Printf("  Ready: %v\n", h.ready.Load())
}
