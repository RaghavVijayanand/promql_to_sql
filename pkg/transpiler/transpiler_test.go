package transpiler

import (
	"testing"
	"time"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTranspiler_SimpleMetric(t *testing.T) {
	trans := New(nil)
	
	promql := `http_requests_total`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "SELECT")
	assert.Contains(t, sql, "FROM metrics")
	assert.Contains(t, sql, "metric_name = 'http_requests_total'")
}

func TestTranspiler_MetricWithLabels(t *testing.T) {
	trans := New(nil)
	
	promql := `http_requests_total{job="api",status="200"}`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "metric_name = 'http_requests_total'")
	assert.Contains(t, sql, "labels['job'] = 'api'")
	assert.Contains(t, sql, "labels['status'] = '200'")
}

func TestTranspiler_Rate(t *testing.T) {
	trans := New(nil)
	
	promql := `rate(http_requests_total[5m])`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "WITH base_data AS")
	assert.Contains(t, sql, "lagInFrame")
	assert.Contains(t, sql, "WINDOW w AS")
}

func TestTranspiler_SumAggregation(t *testing.T) {
	trans := New(nil)
	
	promql := `sum(http_requests_total) by (status)`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "sum(value)")
	assert.Contains(t, sql, "GROUP BY")
	assert.Contains(t, sql, "labels['status']")
}

func TestTranspiler_ComplexQuery(t *testing.T) {
	trans := New(nil)
	trans.SetTimeRange(
		time.Now().Add(-1*time.Hour),
		time.Now(),
		15*time.Second,
	)
	
	promql := `sum(rate(http_requests_total{job="api"}[5m])) by (status)`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.NotEmpty(t, sql)
	assert.Contains(t, sql, "WITH")
	assert.Contains(t, sql, "sum(")
	assert.Contains(t, sql, "GROUP BY")
}

func TestTranspiler_BinaryExpr_Scalar(t *testing.T) {
	trans := New(nil)
	
	promql := `http_requests_total + 10`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "+ 10")
}

func TestTranspiler_MathFunctions(t *testing.T) {
	tests := []struct {
		promql   string
		expected string
	}{
		{`abs(http_requests_total)`, "abs(value)"},
		{`ceil(http_requests_total)`, "ceil(value)"},
		{`floor(http_requests_total)`, "floor(value)"},
		{`round(http_requests_total)`, "round(value"},
	}
	
	for _, tt := range tests {
		trans := New(nil)
		sql, err := trans.Transpile(tt.promql)
		
		require.NoError(t, err, "failed for promql: %s", tt.promql)
		assert.Contains(t, sql, tt.expected, "expected to find %s in sql for promql: %s", tt.expected, tt.promql)
	}
}

func TestTranspiler_CustomSchema(t *testing.T) {
	schema := &clickhouse.Schema{
		TableName:        "custom_metrics",
		MetricNameColumn: "name",
		LabelsColumn:     "tags",
		TimestampColumn:  "ts",
		ValueColumn:      "val",
	}
	
	config := &Config{
		Schema:             schema,
		EnableOptimization: true,
	}
	
	trans := New(config)
	
	promql := `http_requests_total`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "FROM custom_metrics")
	assert.Contains(t, sql, "name = 'http_requests_total'")
}

func TestTranspiler_Increase(t *testing.T) {
	trans := New(nil)
	
	promql := `increase(http_requests_total[5m])`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "lagInFrame")
	assert.Contains(t, sql, "WITH base_data AS")
}

func TestTranspiler_Delta(t *testing.T) {
	trans := New(nil)
	
	promql := `delta(temperature[1h])`
	sql, err := trans.Transpile(promql)
	
	require.NoError(t, err)
	assert.Contains(t, sql, "lagInFrame")
	assert.Contains(t, sql, "WITH base_data AS")
}

func TestTranspiler_MultipleAggregations(t *testing.T) {
	tests := []struct {
		promql string
		aggFunc string
	}{
		{`avg(http_requests_total)`, "avg(value)"},
		{`max(http_requests_total)`, "max(value)"},
		{`min(http_requests_total)`, "min(value)"},
		{`count(http_requests_total)`, "count(value)"},
	}
	
	for _, tt := range tests {
		trans := New(nil)
		sql, err := trans.Transpile(tt.promql)
		
		require.NoError(t, err)
		assert.Contains(t, sql, tt.aggFunc)
	}
}
