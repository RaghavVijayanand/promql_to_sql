package validation

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidator_ValidQuery tests validation of correct PromQL queries
func TestValidator_ValidQuery(t *testing.T) {
	v := NewPromQLValidator()
	
	tests := []struct {
		name  string
		query string
	}{
		{"simple metric", "http_requests_total"},
		{"with labels", `http_requests_total{job="api"}`},
		{"rate function", "rate(http_requests_total[5m])"},
		{"sum aggregation", `sum(rate(http_requests_total[5m])) by (status)`},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Validate(tt.query)
			assert.NoError(t, err, "Query should be valid: %s", tt.query)
		})
	}
}

// TestValidator_EmptyQuery tests empty query validation
func TestValidator_EmptyQuery(t *testing.T) {
	v := NewPromQLValidator()
	
	tests := []string{
		"",
		"   ",
		"\t",
		"\n",
	}
	
	for _, query := range tests {
		err := v.Validate(query)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty")
	}
}

// TestValidator_QueryTooLong tests maximum query length
func TestValidator_QueryTooLong(t *testing.T) {
	v := NewPromQLValidator()
	
	// Create a query longer than max length (default 10KB)
	longQuery := strings.Repeat("http_requests_total ", 1000)
	
	err := v.Validate(longQuery)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "too long")
}

// TestValidator_InvalidInput tests non-string input
func TestValidator_InvalidInput(t *testing.T) {
	v := NewPromQLValidator()
	
	err := v.Validate(12345)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a string")
}

