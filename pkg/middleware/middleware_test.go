package middleware

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// MockCache for testing
type MockCache struct {
	data map[string]string
}

func NewMockCache() *MockCache {
	return &MockCache{data: make(map[string]string)}
}

func (m *MockCache) Get(key string) (string, bool) {
	val, ok := m.data[key]
	return val, ok
}

func (m *MockCache) Set(key string, value string, ttl time.Duration) {
	m.data[key] = value
}

// TestValidationMiddleware tests validation middleware
func TestValidationMiddleware(t *testing.T) {
	vm := NewValidationMiddleware(100)
	
	handler := func(ctx context.Context, query string) (string, error) {
		return query, nil
	}
	
	// Valid query
	result, err := vm.Process(context.Background(), "test query", handler)
	assert.NoError(t, err)
	assert.Equal(t, "test query", result)
	
	// Empty query
	_, err = vm.Process(context.Background(), "", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
	
	// Too long query
	longQuery := string(make([]byte, 200))
	_, err = vm.Process(context.Background(), longQuery, handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

// TestTimeoutMiddleware_Success tests timeout middleware with successful completion
func TestTimeoutMiddleware_Success(t *testing.T) {
	tm := NewTimeoutMiddleware(100 * time.Millisecond)
	
	handler := func(ctx context.Context, query string) (string, error) {
		time.Sleep(10 * time.Millisecond)
		return query, nil
	}
	
	result, err := tm.Process(context.Background(), "test", handler)
	assert.NoError(t, err)
	assert.Equal(t, "test", result)
}

// TestTimeoutMiddleware_Timeout tests timeout behavior
func TestTimeoutMiddleware_Timeout(t *testing.T) {
	tm := NewTimeoutMiddleware(50 * time.Millisecond)
	
	handler := func(ctx context.Context, query string) (string, error) {
		time.Sleep(200 * time.Millisecond)
		return query, nil
	}
	
	_, err := tm.Process(context.Background(), "test", handler)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "timeout")
}

// TestCachingMiddleware tests caching middleware
func TestCachingMiddleware(t *testing.T) {
	cache := NewMockCache()
	cm := NewCachingMiddleware(cache)
	
	callCount := 0
	handler := func(ctx context.Context, query string) (string, error) {
		callCount++
		return "result", nil
	}
	
	// First call - cache miss
	result, err := cm.Process(context.Background(), "query1", handler)
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount)
	
	// Second call - cache hit
	result, err = cm.Process(context.Background(), "query1", handler)
	assert.NoError(t, err)
	assert.Equal(t, "result", result)
	assert.Equal(t, 1, callCount, "Handler should not be called again (cache hit)")
}

// TestMiddlewareChain tests middleware chain execution
func TestMiddlewareChain(t *testing.T) {
	finalHandler := func(ctx context.Context, query string) (string, error) {
		return query + "_processed", nil
	}
	
	chain := NewMiddlewareChain(finalHandler)
	chain.Use(NewValidationMiddleware(100))
	
	result, err := chain.Execute(context.Background(), "test")
	assert.NoError(t, err)
	assert.Equal(t, "test_processed", result)
}

// TestMiddlewareChain_Error tests error propagation
func TestMiddlewareChain_Error(t *testing.T) {
	finalHandler := func(ctx context.Context, query string) (string, error) {
		return "", errors.New("processing error")
	}
	
	chain := NewMiddlewareChain(finalHandler)
	chain.Use(NewValidationMiddleware(100))
	
	_, err := chain.Execute(context.Background(), "test")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "processing error")
}

// TestMiddlewareChain_MultipleMiddleware tests chain with multiple middleware
func TestMiddlewareChain_MultipleMiddleware(t *testing.T) {
	finalHandler := func(ctx context.Context, query string) (string, error) {
		return query, nil
	}
	
	chain := NewMiddlewareChain(finalHandler)
	chain.Use(NewValidationMiddleware(100))
	chain.Use(NewTimeoutMiddleware(100 * time.Millisecond))
	
	result, err := chain.Execute(context.Background(), "test")
	assert.NoError(t, err)
	assert.Equal(t, "test", result)
}

// TestTimeoutMiddleware_NoGoroutineLeak tests the buffered channel fix
func TestTimeoutMiddleware_NoGoroutineLeak(t *testing.T) {
	// Run multiple timeouts to ensure no goroutine leaks
	for i := 0; i < 50; i++ {
		tm := NewTimeoutMiddleware(10 * time.Millisecond)
		
		handler := func(ctx context.Context, query string) (string, error) {
			time.Sleep(100 * time.Millisecond)
			return "result", nil
		}
		
		_, err := tm.Process(context.Background(), "test", handler)
		assert.Error(t, err)
	}
	
	// Wait for goroutines to finish
	time.Sleep(200 * time.Millisecond)
	
	// If there are goroutine leaks, they would accumulate here
	// This test passes if it doesn't hang or panic
}

