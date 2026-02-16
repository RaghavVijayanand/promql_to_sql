package parser

import (
	"testing"
	"time"

	"github.com/shinro/promql-transpiler/pkg/ast"
	"github.com/shinro/promql-transpiler/pkg/lexer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParser_VectorSelector(t *testing.T) {
	input := `http_requests_total`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	vs, ok := expr.(*ast.VectorSelector)
	require.True(t, ok, "expected VectorSelector")
	
	assert.Equal(t, "http_requests_total", vs.MetricSelector.Name)
	assert.Equal(t, 0, len(vs.MetricSelector.Matchers))
}

func TestParser_VectorSelectorWithLabels(t *testing.T) {
	input := `http_requests_total{job="api",status="200"}`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	vs, ok := expr.(*ast.VectorSelector)
	require.True(t, ok)
	
	assert.Equal(t, "http_requests_total", vs.MetricSelector.Name)
	assert.Equal(t, 2, len(vs.MetricSelector.Matchers))
	
	assert.Equal(t, "job", vs.MetricSelector.Matchers[0].Name)
	assert.Equal(t, ast.MatchEqual, vs.MetricSelector.Matchers[0].Operator)
	assert.Equal(t, "api", vs.MetricSelector.Matchers[0].Value)
}

func TestParser_MatrixSelector(t *testing.T) {
	input := `http_requests_total[5m]`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	ms, ok := expr.(*ast.MatrixSelector)
	require.True(t, ok)
	
	assert.Equal(t, "http_requests_total", ms.VectorSelector.MetricSelector.Name)
	assert.Equal(t, 5*time.Minute, ms.Range)
}

func TestParser_AggregateExpr(t *testing.T) {
	input := `sum(http_requests_total) by (status)`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	agg, ok := expr.(*ast.AggregateExpr)
	require.True(t, ok)
	
	assert.Equal(t, ast.AggSum, agg.Op)
	assert.Equal(t, 1, len(agg.Grouping))
	assert.Equal(t, "status", agg.Grouping[0])
	assert.False(t, agg.Without)
}

func TestParser_FunctionCall_Rate(t *testing.T) {
	input := `rate(http_requests_total[5m])`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	call, ok := expr.(*ast.Call)
	require.True(t, ok)
	
	assert.Equal(t, ast.FuncRate, call.Func)
	assert.Equal(t, 1, len(call.Args))
	
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	require.True(t, ok)
	assert.Equal(t, 5*time.Minute, ms.Range)
}

func TestParser_BinaryExpr_Addition(t *testing.T) {
	input := `http_requests_total + 10`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	be, ok := expr.(*ast.BinaryExpr)
	require.True(t, ok)
	
	assert.Equal(t, ast.OpAdd, be.Operator)
	
	_, ok = be.Left.(*ast.VectorSelector)
	assert.True(t, ok)
	
	num, ok := be.Right.(*ast.NumberLiteral)
	require.True(t, ok)
	assert.Equal(t, 10.0, num.Value)
}

func TestParser_ComplexQuery(t *testing.T) {
	input := `sum(rate(http_requests_total{job="api"}[5m])) by (status)`
	
	expr, err := Parse(input)
	require.NoError(t, err)
	
	agg, ok := expr.(*ast.AggregateExpr)
	require.True(t, ok)
	assert.Equal(t, ast.AggSum, agg.Op)
	
	call, ok := agg.Expr.(*ast.Call)
	require.True(t, ok)
	assert.Equal(t, ast.FuncRate, call.Func)
}

func TestParser_Errors(t *testing.T) {
	tests := []string{
		`http_requests_total{`,           // Unclosed brace
		`sum(`,                            // Unclosed parenthesis
		`rate()`,                          // Missing argument
		`http_requests_total[`,            // Unclosed bracket
	}
	
	for _, input := range tests {
		l := lexer.New(input)
		p := New(l)
		_, err := p.ParseExpr()
		assert.Error(t, err, "expected error for input: %s", input)
	}
}

func TestParser_LabelMatchers(t *testing.T) {
	tests := []struct {
		input    string
		operator ast.MatchOp
	}{
		{`http_requests_total{job="api"}`, ast.MatchEqual},
		{`http_requests_total{job!="api"}`, ast.MatchNotEqual},
		{`http_requests_total{job=~"api.*"}`, ast.MatchRegexp},
		{`http_requests_total{job!~"api.*"}`, ast.MatchNotRegexp},
	}
	
	for _, tt := range tests {
		expr, err := Parse(tt.input)
		require.NoError(t, err)
		
		vs, ok := expr.(*ast.VectorSelector)
		require.True(t, ok)
		require.Equal(t, 1, len(vs.MetricSelector.Matchers))
		
		assert.Equal(t, tt.operator, vs.MetricSelector.Matchers[0].Operator)
	}
}
