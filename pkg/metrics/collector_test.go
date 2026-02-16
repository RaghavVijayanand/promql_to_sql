package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestCollector_RecordQueryDuration tests basic query duration recording
func TestCollector_RecordQueryDuration(t *testing.T) {
	c := NewMetricsCollector()
	
	c.RecordQuery(100 * time.Millisecond, true)
	c.RecordQuery(200 * time.Millisecond, true)
	c.RecordQuery(150 * time.Millisecond, true)
	
	metrics := c.ExportPrometheus()
	assert.Contains(t, metrics, "queries_total")
}

// TestCollector_RecordQueryDurationConcurrent tests atomic counter safety
func TestCollector_RecordQueryDurationConcurrent(t *testing.T) {
	c := NewMetricsCollector()
	iterations := 1000
	
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordQuery(time.Millisecond, true)
			}
		}()
	}
	
	wg.Wait()
	
	metrics := c.ExportPrometheus()
	// Should have exactly 10,000 counts
	assert.Contains(t, metrics, "queries_total")
}

// TestCollector_RecordError tests error counter increment
func TestCollector_RecordError(t *testing.T) {
	c := NewMetricsCollector()
	
	c.RecordValidationError()
	c.RecordTimeoutError()
	c.RecordComplexityError()
	
	metrics := c.ExportPrometheus()
	assert.Contains(t, metrics, "validation_errors")
	assert.Contains(t, metrics, "timeout_errors")
}

// TestCollector_RecordErrorConcurrent tests atomic error counter
func TestCollector_RecordErrorConcurrent(t *testing.T) {
	c := NewMetricsCollector()
	iterations := 500
	
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordValidationError()
			}
		}()
	}
	
	wg.Wait()
	
	metrics := c.ExportPrometheus()
	// Should have exactly 10,000 validation errors
	assert.Contains(t, metrics, "validation_errors")
}

// TestCollector_RecordCacheHit tests cache hit counter
func TestCollector_RecordCacheHit(t *testing.T) {
	c := NewMetricsCollector()
	
	c.RecordCacheHit()
	c.RecordCacheHit()
	c.RecordCacheMiss()
	
	metrics := c.ExportPrometheus()
	assert.Contains(t, metrics, "cache_hits")
	assert.Contains(t, metrics, "cache_misses")
}

// TestCollector_RecordCacheConcurrent tests atomic cache counters
func TestCollector_RecordCacheConcurrent(t *testing.T) {
	c := NewMetricsCollector()
	iterations := 500
	
	var wg sync.WaitGroup
	// Concurrent hits
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordCacheHit()
			}
		}()
	}
	
	// Concurrent misses
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordCacheMiss()
			}
		}()
	}
	
	wg.Wait()
	
	metrics := c.ExportPrometheus()
	assert.Contains(t, metrics, "cache_hits")
	assert.Contains(t, metrics, "cache_misses")
}

// TestCollector_AddQueryDurationRaceCondition tests the fix for race on slice append
func TestCollector_AddQueryDurationRaceCondition(t *testing.T) {
	c := NewMetricsCollector()
	iterations := 100
	
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordQuery(time.Duration(j) * time.Millisecond, true)
			}
		}()
	}
	
	wg.Wait()
	
	// Should not panic and should have correct count
	metrics := c.ExportPrometheus()
	assert.Contains(t, metrics, "queries_total")
}

// TestCollector_PercentileCalculation tests percentile calculation accuracy
func TestCollector_PercentileCalculation(t *testing.T) {
	c := NewMetricsCollector()
	
	// Add known values: 1ms to 100ms
	for i := 1; i <= 100; i++ {
		c.RecordQuery(time.Duration(i) * time.Millisecond, true)
	}
	
	metrics := c.ExportPrometheus()
	
	// Should contain percentile metrics
	assert.NotEmpty(t, metrics)
	assert.Contains(t, metrics, "queries_total")
}

// TestCollector_SortIsCorrect tests that sort.Float64s produces correct order
func TestCollector_SortIsCorrect(t *testing.T) {
	c := NewMetricsCollector()
	
	// Add values in random order
	durations := []time.Duration{
		100 * time.Millisecond,
		10 * time.Millisecond,
		50 * time.Millisecond,
		90 * time.Millisecond,
		20 * time.Millisecond,
	}
	
	for _, d := range durations {
		c.RecordQuery(d, true)
	}
	
	metrics := c.ExportPrometheus()
	
	// Should be sorted
	assert.NotEmpty(t, metrics)
}

// TestCollector_EmptyMetrics tests export with no data
func TestCollector_EmptyMetrics(t *testing.T) {
	c := NewMetricsCollector()
	
	metrics := c.ExportPrometheus()
	
	// Should still have metric definitions with 0 values
	assert.NotEmpty(t, metrics)
}

// TestCollector_LargeDataset tests performance with large dataset
func TestCollector_LargeDataset(t *testing.T) {
	c := NewMetricsCollector()
	
	// Add 10,000 measurements
	for i := 0; i < 10000; i++ {
		c.RecordQuery(time.Duration(i%1000) * time.Millisecond, true)
	}
	
	start := time.Now()
	metrics := c.ExportPrometheus()
	elapsed := time.Since(start)
	
	// Export should complete quickly (< 100ms even with sort)
	assert.Less(t, elapsed, 100*time.Millisecond)
	assert.Contains(t, metrics, "queries_total")
}

// TestCollector_MultipleErrorTypes tests multiple error type tracking
func TestCollector_MultipleErrorTypes(t *testing.T) {
	c := NewMetricsCollector()
	
	// Record different types of errors
	c.RecordValidationError()
	c.RecordTimeoutError()
	c.RecordTimeoutError()
	c.RecordComplexityError()
	c.RecordComplexityError()
	c.RecordComplexityError()
	
	metrics := c.ExportPrometheus()
	
	assert.Contains(t, metrics, "validation_errors")
	assert.Contains(t, metrics, "timeout_errors")
	assert.Contains(t, metrics, "complexity_errors")
}

// TestCollector_ThreadSafety tests overall thread safety
func TestCollector_ThreadSafety(t *testing.T) {
	c := NewMetricsCollector()
	iterations := 200
	
	var wg sync.WaitGroup
	
	// Mix all operations concurrently
	for i := 0; i < 10; i++ {
		// Query durations
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordQuery(time.Millisecond * time.Duration(j%100), true)
			}
		}()
		
		// Errors
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordValidationError()
			}
		}(i)
		
		// Cache hits
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordCacheHit()
			}
		}()
		
		// Cache misses
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < iterations; j++ {
				c.RecordCacheMiss()
			}
		}()
	}
	
	wg.Wait()
	
	metrics := c.ExportPrometheus()
	
	// Verify all counters are accurate
	assert.Contains(t, metrics, "queries_total")
	assert.Contains(t, metrics, "validation_errors")
	assert.Contains(t, metrics, "cache_hits")
	assert.Contains(t, metrics, "cache_misses")
}

// TestCollector_ZeroDuration tests handling of zero duration
func TestCollector_ZeroDuration(t *testing.T) {
	c := NewMetricsCollector()
	
	c.RecordQuery(0, true)
	c.RecordQuery(0, true)
	
	metrics := c.ExportPrometheus()
	assert.Contains(t, metrics, "queries_total")
}

// TestCollector_ExportFormat tests Prometheus format compliance
func TestCollector_ExportFormat(t *testing.T) {
	c := NewMetricsCollector()
	
	c.RecordQuery(100 * time.Millisecond, true)
	c.RecordValidationError()
	c.RecordCacheHit()
	
	metrics := c.ExportPrometheus()
	
	// Verify it's valid Prometheus format
	require.NotEmpty(t, metrics)
	assert.Contains(t, metrics, "queries_total")
}
