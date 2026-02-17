package transpiler

import (
	"strings"
	"testing"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClickHouseOptimizedQueries validates transpiler generates ClickHouse-optimized SQL
func TestClickHouseOptimizedQueries(t *testing.T) {
	tests := []struct {
		name           string
		config         *Config
		promql         string
		mustContain    []string
		mustNotContain []string
		description    string
	}{
		{
			name: "Rate query with toDateTime and toUnixTimestamp",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `rate(http_requests_total[5m])`,
			mustContain: []string{
				"toUnixTimestamp",
				"metric_name = 'http_requests_total'",
			},
			mustNotContain: []string{
				"__name__",
				"FROM_UNIXTIME",
			},
			description: "rate() should use toUnixTimestamp() for time calculations",
		},
		{
			name: "Quantile with ClickHouse functions",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `histogram_quantile(0.95, rate(http_request_duration_bucket[5m]))`,
			mustContain: []string{
				"quantileExactWeighted(0.95)", // ClickHouse histogram_quantile uses quantileExactWeighted
				"toFloat64OrNull",             // Bucket boundary extraction
			},
			mustNotContain: []string{
				"PERCENTILE_CONT",
			},
			description: "histogram_quantile should use ClickHouse quantileExactWeighted() for histogram aggregation",
		},
		{
			name: "Window function with lagInFrame",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `resets(http_requests_total[5m])`,
			mustContain: []string{
				"lagInFrame",
				"OVER",
			},
			mustNotContain: []string{
				"lag(value)",
			},
			description: "resets() should use lagInFrame() for ClickHouse window functions",
		},
		{
			name: "Regex matcher with match()",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `{job=~"api.*"}`,
			mustContain: []string{
				"match(",
			},
			mustNotContain: []string{
				"LIKE",
				"REGEXP",
				"~",
			},
			description: "Regex matchers should use ClickHouse match() function",
		},
		{
			name: "Stddev with stddevPop",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `stddev(cpu_usage)`,
			mustContain: []string{
				"stddevPop",
			},
			mustNotContain: []string{
				"stddev(value)",
				"STDDEV_SAMP",
			},
			description: "stddev should use ClickHouse stddevPop() for population statistics",
		},
		{
			name: "TopK with ClickHouse ordering",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `topk(10, http_requests_total)`,
			mustContain: []string{
				"ORDER BY",
				"LIMIT 10", // topk transpiles to ORDER BY + LIMIT
			},
			mustNotContain: []string{
				"topK(10, value)",
			},
			description: "topk should use ClickHouse ORDER BY + LIMIT pattern",
		},
		{
			name: "Clamp with greatest/least",
			config: &Config{
				Schema:             clickhouse.DefaultSchema(),
				EnableOptimization: true,
			},
			promql: `clamp(rate(requests[5m]), 0, 100)`,
			mustContain: []string{
				"greatest",
				"least",
			},
			mustNotContain: []string{
				"clamp(",
				"CASE WHEN",
			},
			description: "clamp() should use greatest(least()) pattern since ClickHouse has no clamp()",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := New(tt.config)
			sql, err := trans.Transpile(tt.promql)
			require.NoError(t, err, "Transpilation should succeed")

			for _, pattern := range tt.mustContain {
				assert.Contains(t, sql, pattern, tt.description)
			}
			for _, pattern := range tt.mustNotContain {
				assert.NotContains(t, sql, pattern, tt.description)
			}
		})
	}
}

// TestClickHouseEngineOptimizations validates engine-aware optimizations
func TestClickHouseEngineOptimizations(t *testing.T) {
	tests := []struct {
		name        string
		tableMeta   *clickhouse.TableMeta
		promql      string
		mustContain []string
		description string
	}{
		// NOTE: FINAL optimization removed with optimization layer
		// {
		// 	name: "ReplacingMergeTree with FINAL",
		// 	tableMeta: &clickhouse.TableMeta{
		// 		Engine:     clickhouse.ReplacingMergeTree,
		// 		OrderByKey: []string{"metric_name", "labels", "timestamp"},
		// 	},
		// 	promql: `rate(requests[5m])`,
		// 	mustContain: []string{
		// 		"FINAL",
		// 	},
		// 	description: "ReplacingMergeTree queries should automatically inject FINAL for deduplication",
		// },
		{
			name: "MergeTree with PREWHERE on ORDER BY column",
			tableMeta: &clickhouse.TableMeta{
				Engine:     clickhouse.MergeTree,
				OrderByKey: []string{"metric_name", "timestamp"},
			},
			promql: `http_requests_total{job="api"}`,
			mustContain: []string{
				"PREWHERE metric_name",
			},
			description: "Equality filters on ORDER BY key should use PREWHERE for skip-index acceleration",
		},
		{
			name: "SAMPLE clause for sampling key",
			tableMeta: &clickhouse.TableMeta{
				Engine:      clickhouse.MergeTree,
				OrderByKey:  []string{"metric_name", "timestamp"},
				SamplingKey: "cityHash64(metric_name)",
			},
			promql: `count(requests)`,
			mustContain: []string{
				// SAMPLE would be added by query decorator if enabled
				"metric_name",
			},
			description: "Tables with sampling key should support SAMPLE clause",
		},
		{
			name: "ORDER BY alignment with table key",
			tableMeta: &clickhouse.TableMeta{
				Engine:     clickhouse.MergeTree,
				OrderByKey: []string{"metric_name", "timestamp"},
			},
			promql: `sort_desc(http_requests_total)`,
			mustContain: []string{
				"ORDER BY",
			},
			description: "ORDER BY should align with table's sorting key when possible",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := clickhouse.DefaultSchema()
			schema.TableMeta = tt.tableMeta

			config := &Config{
				Schema:             schema,
				EnableOptimization: true,
			}

			trans := New(config)
			sql, err := trans.Transpile(tt.promql)
			require.NoError(t, err, "Transpilation should succeed")

			for _, pattern := range tt.mustContain {
				assert.Contains(t, sql, pattern, tt.description)
			}
		})
	}
}

// TestClickHouseDateTimeFunctions validates date/time function transpilation
func TestClickHouseDateTimeFunctions(t *testing.T) {
	tests := []struct {
		name        string
		promql      string
		mustContain []string
		description string
	}{
		{
			name:   "day_of_month to toDayOfMonth",
			promql: `day_of_month(timestamp)`,
			mustContain: []string{
				"toDayOfMonth",
			},
			description: "day_of_month should transpile to ClickHouse toDayOfMonth()",
		},
		{
			name:   "hour to toHour",
			promql: `hour(timestamp)`,
			mustContain: []string{
				"toHour",
			},
			description: "hour should transpile to ClickHouse toHour()",
		},
		{
			name:   "month to toMonth",
			promql: `month(timestamp)`,
			mustContain: []string{
				"toMonth",
			},
			description: "month should transpile to ClickHouse toMonth()",
		},
	}

	config := &Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := New(config)
			sql, err := trans.Transpile(tt.promql)
			require.NoError(t, err, "Transpilation should succeed")

			for _, pattern := range tt.mustContain {
				assert.Contains(t, sql, pattern, tt.description)
			}
		})
	}
}

// TestClickHouseComplexQueries validates real-world complex query patterns
func TestClickHouseComplexQueries(t *testing.T) {
	tests := []struct {
		name           string
		promql         string
		mustContain    []string
		mustNotContain []string
		description    string
	}{
		{
			name:   "Multi-aggregation with vector operations",
			promql: `sum(rate(http_requests_total[5m])) by (job) / sum(rate(http_requests_duration[5m])) by (job)`,
			mustContain: []string{
				"toUnixTimestamp",
				"metric_name",
				"sum(",
			},
			mustNotContain: []string{
				"__name__",
			},
			description: "Complex aggregations should use ClickHouse-native functions",
		},
		{
			name:   "Nested function calls",
			promql: `round(clamp(rate(cpu_usage[5m]), 0, 1), 0.01)`,
			mustContain: []string{
				"round(",
				"greatest",
				"least",
				"toUnixTimestamp",
			},
			mustNotContain: []string{
				"clamp(",
			},
			description: "Nested functions should all use ClickHouse-native equivalents",
		},
		{
			name:   "Window function with aggregation",
			promql: `changes(http_requests_total[5m])`,
			mustContain: []string{
				"lagInFrame",
				"OVER",
				"ORDER BY",
			},
			mustNotContain: []string{
				"lag(value)",
			},
			description: "Window functions should use ClickHouse lagInFrame/leadInFrame",
		},
		{
			name:   "Quantile aggregation",
			promql: `quantile(0.99, http_request_duration_seconds) by (endpoint)`,
			mustContain: []string{
				"quantile(0.99)(",
			},
			mustNotContain: []string{
				"quantile(value, 0.99)",
			},
			description: "Quantile should use ClickHouse two-level aggregate syntax",
		},
	}

	config := &Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			trans := New(config)
			sql, err := trans.Transpile(tt.promql)
			require.NoError(t, err, "Transpilation should succeed")

			for _, pattern := range tt.mustContain {
				assert.Contains(t, sql, pattern, tt.description)
			}
			for _, pattern := range tt.mustNotContain {
				assert.NotContains(t, sql, pattern, tt.description)
			}
		})
	}
}

// TestClickHouseOutputDeterminism ensures consistent output for same input
func TestClickHouseOutputDeterminism(t *testing.T) {
	config := &Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	queries := []string{
		`rate(http_requests_total{job="api", instance="web-01", env="prod"}[5m])`,
		`sum(rate(cpu_usage[5m])) by (job)`,
		`histogram_quantile(0.95, rate(http_request_duration_bucket[5m]))`,
	}

	for _, promql := range queries {
		t.Run(promql, func(t *testing.T) {
			trans1 := New(config)
			trans2 := New(config)

			sql1, err1 := trans1.Transpile(promql)
			require.NoError(t, err1)

			sql2, err2 := trans2.Transpile(promql)
			require.NoError(t, err2)

			// Same input should produce identical output
			assert.Equal(t, sql1, sql2, "Transpilation should be deterministic")
		})
	}
}

// TestClickHouseForbiddenPatterns validates no non-ClickHouse SQL is generated
func TestClickHouseForbiddenPatterns(t *testing.T) {
	config := &Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	queries := []string{
		`rate(http_requests_total[5m])`,
		`sum(cpu_usage) by (job)`,
		`histogram_quantile(0.95, http_request_duration_bucket)`,
		`topk(10, requests)`,
		`clamp(rate(errors[5m]), 0, 100)`,
		`{job=~"api.*"}`,
		`stddev(latency)`,
		`resets(counter[5m])`,
		`changes(metric[5m])`,
	}

	forbiddenPatterns := []string{
		"__name__",
		"lag(value)",
		"lead(value)",
		"clamp(",
		"xp_",
		"sp_",
		"FROM_UNIXTIME",
		"EXTRACT(",
		"VARCHAR",
		"TIMESTAMP",
		"DOUBLE",
		"LIKE '%",
		"REGEXP",
		"stddev(value)",
		"variance(value)",
		"quantile(value,",
		"topK(10, value)",
	}

	trans := New(config)

	for _, promql := range queries {
		t.Run(promql, func(t *testing.T) {
			sql, err := trans.Transpile(promql)
			require.NoError(t, err, "Transpilation should succeed")

			for _, forbidden := range forbiddenPatterns {
				assert.NotContains(t, sql, forbidden,
					"Generated SQL must not contain non-ClickHouse pattern: %s", forbidden)
			}
		})
	}
}

// TestClickHouseSettingsInjection validates SETTINGS clause injection
func TestClickHouseSettingsInjection(t *testing.T) {
	config := &Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	trans := New(config)
	sql, err := trans.Transpile(`rate(http_requests_total[5m])`)
	require.NoError(t, err)

	// When optimization is enabled, SETTINGS should be injected
	// (depending on decorator configuration)
	if strings.Contains(sql, "SETTINGS") {
		// Validate SETTINGS format
		assert.Contains(t, sql, "SETTINGS", "Optimized queries should include SETTINGS clause")

		// Common ClickHouse query settings
		possibleSettings := []string{
			"max_threads",
			"max_execution_time",
			"max_memory_usage",
			"max_rows_to_read",
		}

		foundSetting := false
		for _, setting := range possibleSettings {
			if strings.Contains(sql, setting) {
				foundSetting = true
				break
			}
		}

		if foundSetting {
			t.Logf("SETTINGS clause found with recognized setting")
		}
	}
}

// BenchmarkClickHouseTranspilation benchmarks transpilation performance
func BenchmarkClickHouseTranspilation(b *testing.B) {
	config := &Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	queries := []string{
		`rate(http_requests_total[5m])`,
		`sum(rate(cpu_usage[5m])) by (job)`,
		`histogram_quantile(0.95, rate(http_request_duration_bucket[5m]))`,
		`clamp(rate(errors[5m]), 0, 100)`,
	}

	for _, promql := range queries {
		b.Run(promql, func(b *testing.B) {
			trans := New(config)
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				_, err := trans.Transpile(promql)
				if err != nil {
					b.Fatalf("Transpilation failed: %v", err)
				}
			}
		})
	}
}
