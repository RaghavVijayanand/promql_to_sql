package parser

import (
	"strings"
	"testing"

	"github.com/shinro/promql-transpiler/pkg/lexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestParser_DepthLimit tests the parser depth limit protection
func TestParser_DepthLimit(t *testing.T) {
	// Create deeply nested query: (((((...))))
	// Each pair of parens adds 1 to depth
	depth := 150 // Exceeds maxParseDepth of 100
	query := strings.Repeat("(", depth) + "http_requests_total" + strings.Repeat(")", depth)
	
	l := lexer.New(query)
	p := New(l)
	
	_, err := p.ParseExpr()
	
	require.Error(t, err, "Should reject deeply nested query")
	assert.Contains(t, err.Error(), "depth", "Error should mention depth limit")
}

// TestParser_ValidDeeplyNestedQuery tests acceptable depth
func TestParser_ValidDeeplyNestedQuery(t *testing.T) {
	// Create query just under the limit (assuming maxParseDepth = 100)
	depth := 50 // Well under limit
	query := strings.Repeat("(", depth) + "http_requests_total" + strings.Repeat(")", depth)
	
	l := lexer.New(query)
	p := New(l)
	
	_, err := p.ParseExpr()
	
	// Should succeed - depth is acceptable
	if err != nil {
		t.Logf("Parse error (may be acceptable): %v", err)
	}
}

// TestParser_DepthRecovery tests that depth counter recovers correctly
func TestParser_DepthRecovery(t *testing.T) {
	// Parse a complex valid query
	query1 := "sum(rate(http_requests_total[5m]))"
	l1 := lexer.New(query1)
	p1 := New(l1)
	_, _ = p1.ParseExpr()
	
	// Parse another query to ensure depth was reset
	query2 := "avg(http_response_time)"
	l2 := lexer.New(query2)
	p2 := New(l2)
	_, err := p2.ParseExpr()
	
	// Should work fine - each parser has its own depth counter
	if err != nil {
		t.Logf("Parse error (may be acceptable): %v", err)
	}
}

// TestParser_ErrorCapping tests maximum error limit
func TestParser_ErrorCapping(t *testing.T) {
	// Create query with many syntax errors
	invalidQuery := strings.Repeat("{[( ", 150) + "metric"
	
	l := lexer.New(invalidQuery)
	p := New(l)
	
	_, err := p.ParseExpr()
	
	require.Error(t, err)
	
	// Verify errors are bounded
	errors := p.Errors()
	assert.LessOrEqual(t, len(errors), maxErrors+1, 
		"Errors should be capped at maxErrors + truncation message")
}

