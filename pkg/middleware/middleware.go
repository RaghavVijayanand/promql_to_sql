package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"
	
	"github.com/shinro/promql-transpiler/pkg/logging"
)

// Middleware represents a transpilation middleware
// Following Chain of Responsibility pattern
type Middleware interface {
	Process(ctx context.Context, query string, next Handler) (string, error)
}

// Handler is the next handler in the chain
type Handler func(ctx context.Context, query string) (string, error)

// MiddlewareChain chains multiple middleware together
type MiddlewareChain struct {
	middlewares []Middleware
	final       Handler
}

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain(final Handler) *MiddlewareChain {
	return &MiddlewareChain{
		middlewares: make([]Middleware, 0),
		final:       final,
	}
}

// Use adds a middleware to the chain
func (c *MiddlewareChain) Use(middleware Middleware) *MiddlewareChain {
	c.middlewares = append(c.middlewares, middleware)
	return c
}

// Execute executes the middleware chain
func (c *MiddlewareChain) Execute(ctx context.Context, query string) (string, error) {
	if len(c.middlewares) == 0 {
		return c.final(ctx, query)
	}
	
	return c.buildChain(0)(ctx, query)
}

func (c *MiddlewareChain) buildChain(index int) Handler {
	if index >= len(c.middlewares) {
		return c.final
	}
	
	return func(ctx context.Context, query string) (string, error) {
		return c.middlewares[index].Process(ctx, query, c.buildChain(index+1))
	}
}

// ============================================================================
// LOGGING MIDDLEWARE
// ============================================================================

// LoggingMiddleware logs query execution
type LoggingMiddleware struct {
	logger logging.Logger
}

func NewLoggingMiddleware(logger logging.Logger) *LoggingMiddleware {
	return &LoggingMiddleware{logger: logger}
}

func (m *LoggingMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	start := time.Now()
	m.logger.Info("Processing query", logging.F("query", query))
	
	result, err := next(ctx, query)
	
	duration := time.Since(start)
	if err != nil {
		m.logger.Error("Query processing failed",
			logging.F("query", query),
			logging.F("error", err.Error()),
			logging.F("duration_ms", duration.Milliseconds()))
	} else {
		m.logger.Info("Query processed successfully",
			logging.F("query", query),
			logging.F("duration_ms", duration.Milliseconds()))
	}
	
	return result, err
}

// ============================================================================
// VALIDATION MIDDLEWARE
// ============================================================================

// ValidationMiddleware validates queries before processing
type ValidationMiddleware struct {
	maxQueryLength int
}

func NewValidationMiddleware(maxQueryLength int) *ValidationMiddleware {
	return &ValidationMiddleware{maxQueryLength: maxQueryLength}
}

func (m *ValidationMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	if len(query) > m.maxQueryLength {
		return "", fmt.Errorf("query too long: %d (max: %d)", len(query), m.maxQueryLength)
	}
	
	if query == "" {
		return "", fmt.Errorf("query cannot be empty")
	}
	
	return next(ctx, query)
}

// ============================================================================
// TIMEOUT MIDDLEWARE
// ============================================================================

// TimeoutMiddleware adds timeout to query processing
type TimeoutMiddleware struct {
	timeout time.Duration
}

func NewTimeoutMiddleware(timeout time.Duration) *TimeoutMiddleware {
	return &TimeoutMiddleware{timeout: timeout}
}

func (m *TimeoutMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()
	
	// Buffered channel prevents goroutine leak if timeout fires
	resultChan := make(chan struct {
		result string
		err    error
	}, 1)
	
	go func() {
		result, err := next(ctx, query)
		resultChan <- struct {
			result string
			err    error
		}{result, err}
	}()
	
	select {
	case <-ctx.Done():
		return "", fmt.Errorf("query processing timeout after %v", m.timeout)
	case res := <-resultChan:
		return res.result, res.err
	}
}

// ============================================================================
// CACHING MIDDLEWARE
// ============================================================================

// CachingMiddleware caches query results
type CachingMiddleware struct {
	cache Cache
}

type Cache interface {
	Get(key string) (string, bool)
	Set(key string, value string, ttl time.Duration)
}

func NewCachingMiddleware(cache Cache) *CachingMiddleware {
	return &CachingMiddleware{cache: cache}
}

func (m *CachingMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	// Try to get from cache
	if cached, ok := m.cache.Get(query); ok {
		return cached, nil
	}
	
	// Process query
	result, err := next(ctx, query)
	if err != nil {
		return result, err
	}
	
	// Cache the result
	m.cache.Set(query, result, 5*time.Minute)
	
	return result, nil
}

// ============================================================================
// METRICS MIDDLEWARE
// ============================================================================

// MetricsMiddleware collects metrics
type MetricsMiddleware struct {
	collector MetricsCollector
}

type MetricsCollector interface {
	RecordQueryDuration(duration time.Duration)
	IncrementQueryCount()
	IncrementErrorCount()
}

func NewMetricsMiddleware(collector MetricsCollector) *MetricsMiddleware {
	return &MetricsMiddleware{collector: collector}
}

func (m *MetricsMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	start := time.Now()
	m.collector.IncrementQueryCount()
	
	result, err := next(ctx, query)
	
	duration := time.Since(start)
	m.collector.RecordQueryDuration(duration)
	
	if err != nil {
		m.collector.IncrementErrorCount()
	}
	
	return result, err
}

// ============================================================================
// RATE LIMITING MIDDLEWARE
// ============================================================================

// RateLimitMiddleware implements rate limiting
type RateLimitMiddleware struct {
	limiter RateLimiter
}

type RateLimiter interface {
	Allow() bool
}

func NewRateLimitMiddleware(limiter RateLimiter) *RateLimitMiddleware {
	return &RateLimitMiddleware{limiter: limiter}
}

func (m *RateLimitMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	if !m.limiter.Allow() {
		return "", fmt.Errorf("rate limit exceeded")
	}
	
	return next(ctx, query)
}

// ============================================================================
// COMPLEXITY LIMITING MIDDLEWARE
// ============================================================================

// ComplexityMiddleware limits query complexity
type ComplexityMiddleware struct {
	maxComplexity int
}

func NewComplexityMiddleware(maxComplexity int) *ComplexityMiddleware {
	return &ComplexityMiddleware{maxComplexity: maxComplexity}
}

func (m *ComplexityMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	complexity := calculateComplexity(query)
	
	if complexity > m.maxComplexity {
		return "", fmt.Errorf("query too complex: %d (max: %d)", complexity, m.maxComplexity)
	}
	
	return next(ctx, query)
}

func calculateComplexity(query string) int {
	// Simple complexity calculation based on query length and special characters
	complexity := len(query) / 10
	
	// Add complexity for functions (use word boundary matching to avoid
	// false positives like "separate" matching "rate")
	lower := strings.ToLower(query)
	for _, fn := range []string{"rate(", "sum(", "avg(", "count("} {
		complexity += strings.Count(lower, fn) * 10
	}
	
	return complexity
}

// ============================================================================
// RETRY MIDDLEWARE
// ============================================================================

// RetryMiddleware retries failed queries
type RetryMiddleware struct {
	maxRetries int
	backoff    time.Duration
}

func NewRetryMiddleware(maxRetries int, backoff time.Duration) *RetryMiddleware {
	return &RetryMiddleware{
		maxRetries: maxRetries,
		backoff:    backoff,
	}
}

func (m *RetryMiddleware) Process(ctx context.Context, query string, next Handler) (string, error) {
	var lastErr error
	
	for attempt := 0; attempt <= m.maxRetries; attempt++ {
		result, err := next(ctx, query)
		if err == nil {
			return result, nil
		}
		
		lastErr = err
		
		if attempt < m.maxRetries {
			// Respect context cancellation during backoff
			select {
			case <-ctx.Done():
				return "", fmt.Errorf("context cancelled during retry: %w", ctx.Err())
			case <-time.After(m.backoff * time.Duration(attempt+1)):
			}
		}
	}
	
	return "", fmt.Errorf("failed after %d retries: %w", m.maxRetries, lastErr)
}
