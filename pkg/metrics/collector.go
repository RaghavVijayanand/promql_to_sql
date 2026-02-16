package metrics

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector collects and exports metrics in Prometheus format
type MetricsCollector struct {
	mu               sync.RWMutex // Protects queryDuration slice only
	// Atomic counters for high-frequency operations (no lock needed)
	queriesTotal     int64
	queriesSuccess   int64
	queriesFailed    int64
	queryDuration    []float64 // Protected by mu
	sumDuration      float64   // Protected by mu - accurate total of all durations
	cacheHits        int64
	cacheMisses      int64
	validationErrors int64
	timeoutErrors    int64
	complexityErrors int64
	lastReset        time.Time
}

// NewMetricsCollector creates a new metrics collector
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{
		queryDuration: make([]float64, 0, 1000),
		lastReset:     time.Now(),
	}
}

// RecordQuery records a query execution
func (m *MetricsCollector) RecordQuery(duration time.Duration, success bool) {
	// Use atomic operations for counters (faster, no lock contention)
	atomic.AddInt64(&m.queriesTotal, 1)
	if success {
		atomic.AddInt64(&m.queriesSuccess, 1)
	} else {
		atomic.AddInt64(&m.queriesFailed, 1)
	}
	
	// Only lock for slice operations
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Track accurate total duration for _sum metric
	m.sumDuration += duration.Seconds()
	
	// Trim before append to prevent unbounded growth (race condition fix)
	if len(m.queryDuration) >= 1000 {
		m.queryDuration = m.queryDuration[len(m.queryDuration)-999:]
	}
	m.queryDuration = append(m.queryDuration, duration.Seconds())
}

// RecordCacheHit records a cache hit
func (m *MetricsCollector) RecordCacheHit() {
	atomic.AddInt64(&m.cacheHits, 1)
}

// RecordCacheMiss records a cache miss
func (m *MetricsCollector) RecordCacheMiss() {
	atomic.AddInt64(&m.cacheMisses, 1)
}

// RecordValidationError records a validation error
func (m *MetricsCollector) RecordValidationError() {
	atomic.AddInt64(&m.validationErrors, 1)
}

// RecordTimeoutError records a timeout error
func (m *MetricsCollector) RecordTimeoutError() {
	atomic.AddInt64(&m.timeoutErrors, 1)
}

// RecordComplexityError records a complexity error
func (m *MetricsCollector) RecordComplexityError() {
	atomic.AddInt64(&m.complexityErrors, 1)
}

// GetMetrics returns current metrics snapshot
func (m *MetricsCollector) GetMetrics() Metrics {
	// Read atomic counters without lock
	queriesTotal := atomic.LoadInt64(&m.queriesTotal)
	queriesSuccess := atomic.LoadInt64(&m.queriesSuccess)
	queriesFailed := atomic.LoadInt64(&m.queriesFailed)
	cacheHits := atomic.LoadInt64(&m.cacheHits)
	cacheMisses := atomic.LoadInt64(&m.cacheMisses)
	validationErrors := atomic.LoadInt64(&m.validationErrors)
	timeoutErrors := atomic.LoadInt64(&m.timeoutErrors)
	complexityErrors := atomic.LoadInt64(&m.complexityErrors)
	
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return Metrics{
		QueriesTotal:     queriesTotal,
		QueriesSuccess:   queriesSuccess,
		QueriesFailed:    queriesFailed,
		AvgQueryDuration: m.calculateAvgDuration(),
		SumQueryDuration: m.sumDuration,
		P50QueryDuration: m.calculatePercentile(50),
		P95QueryDuration: m.calculatePercentile(95),
		P99QueryDuration: m.calculatePercentile(99),
		CacheHits:        cacheHits,
		CacheMisses:      cacheMisses,
		CacheHitRate:     calculateCacheHitRate(cacheHits, cacheMisses),
		ValidationErrors: validationErrors,
		TimeoutErrors:    timeoutErrors,
		ComplexityErrors: complexityErrors,
		Uptime:           time.Since(m.lastReset).Seconds(),
	}
}

// ExportPrometheus exports metrics in Prometheus format
func (m *MetricsCollector) ExportPrometheus() string {
	metrics := m.GetMetrics()
	
	return `# HELP transpiler_queries_total Total number of queries processed
# TYPE transpiler_queries_total counter
transpiler_queries_total ` + formatInt64(metrics.QueriesTotal) + `

# HELP transpiler_queries_success Total number of successful queries
# TYPE transpiler_queries_success counter
transpiler_queries_success ` + formatInt64(metrics.QueriesSuccess) + `

# HELP transpiler_queries_failed Total number of failed queries
# TYPE transpiler_queries_failed counter
transpiler_queries_failed ` + formatInt64(metrics.QueriesFailed) + `

# HELP transpiler_query_duration_seconds Query duration in seconds
# TYPE transpiler_query_duration_seconds summary
transpiler_query_duration_seconds{quantile="0.5"} ` + formatFloat64(metrics.P50QueryDuration) + `
transpiler_query_duration_seconds{quantile="0.95"} ` + formatFloat64(metrics.P95QueryDuration) + `
transpiler_query_duration_seconds{quantile="0.99"} ` + formatFloat64(metrics.P99QueryDuration) + `
transpiler_query_duration_seconds_sum ` + formatFloat64(metrics.SumQueryDuration) + `
transpiler_query_duration_seconds_count ` + formatInt64(metrics.QueriesTotal) + `

# HELP transpiler_cache_hits Total number of cache hits
# TYPE transpiler_cache_hits counter
transpiler_cache_hits ` + formatInt64(metrics.CacheHits) + `

# HELP transpiler_cache_misses Total number of cache misses
# TYPE transpiler_cache_misses counter
transpiler_cache_misses ` + formatInt64(metrics.CacheMisses) + `

# HELP transpiler_cache_hit_rate Cache hit rate (0-1)
# TYPE transpiler_cache_hit_rate gauge
transpiler_cache_hit_rate ` + formatFloat64(metrics.CacheHitRate) + `

# HELP transpiler_validation_errors Total number of validation errors
# TYPE transpiler_validation_errors counter
transpiler_validation_errors ` + formatInt64(metrics.ValidationErrors) + `

# HELP transpiler_timeout_errors Total number of timeout errors
# TYPE transpiler_timeout_errors counter
transpiler_timeout_errors ` + formatInt64(metrics.TimeoutErrors) + `

# HELP transpiler_complexity_errors Total number of complexity errors
# TYPE transpiler_complexity_errors counter
transpiler_complexity_errors ` + formatInt64(metrics.ComplexityErrors) + `

# HELP transpiler_uptime_seconds Transpiler uptime in seconds
# TYPE transpiler_uptime_seconds gauge
transpiler_uptime_seconds ` + formatFloat64(metrics.Uptime) + `
`
}

// Reset resets all metrics
func (m *MetricsCollector) Reset() {
	// Reset atomic counters
	atomic.StoreInt64(&m.queriesTotal, 0)
	atomic.StoreInt64(&m.queriesSuccess, 0)
	atomic.StoreInt64(&m.queriesFailed, 0)
	atomic.StoreInt64(&m.cacheHits, 0)
	atomic.StoreInt64(&m.cacheMisses, 0)
	atomic.StoreInt64(&m.validationErrors, 0)
	atomic.StoreInt64(&m.timeoutErrors, 0)
	atomic.StoreInt64(&m.complexityErrors, 0)
	
	// Reset slice under lock
	m.mu.Lock()
	m.queryDuration = make([]float64, 0, 1000)
	m.sumDuration = 0
	m.lastReset = time.Now()
	m.mu.Unlock()
}

// calculateAvgDuration calculates average query duration
func (m *MetricsCollector) calculateAvgDuration() float64 {
	if len(m.queryDuration) == 0 {
		return 0
	}
	
	var sum float64
	for _, d := range m.queryDuration {
		sum += d
	}
	
	return sum / float64(len(m.queryDuration))
}

// calculatePercentile calculates percentile query duration
func (m *MetricsCollector) calculatePercentile(percentile int) float64 {
	if len(m.queryDuration) == 0 {
		return 0
	}
	
	// Percentile calculation using stdlib sort (O(n log n) vs O(n²) bubble sort)
	sorted := make([]float64, len(m.queryDuration))
	copy(sorted, m.queryDuration)
	
	// Use stdlib sort for efficiency
	sort.Float64s(sorted)
	
	index := (percentile * len(sorted)) / 100
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	
	return sorted[index]
}

// calculateCacheHitRate calculates cache hit rate
func calculateCacheHitRate(hits, misses int64) float64 {
	total := hits + misses
	if total == 0 {
		return 0
	}
	
	return float64(hits) / float64(total)
}

// Metrics represents a snapshot of metrics
type Metrics struct {
	QueriesTotal     int64
	QueriesSuccess   int64
	QueriesFailed    int64
	AvgQueryDuration float64
	SumQueryDuration float64
	P50QueryDuration float64
	P95QueryDuration float64
	P99QueryDuration float64
	CacheHits        int64
	CacheMisses      int64
	CacheHitRate     float64
	ValidationErrors int64
	TimeoutErrors    int64
	ComplexityErrors int64
	Uptime           float64
}

// Helper functions for formatting
func formatInt64(v int64) string {
	return fmt.Sprintf("%d", v)
}

func formatFloat64(v float64) string {
	return fmt.Sprintf("%.6f", v)
}
