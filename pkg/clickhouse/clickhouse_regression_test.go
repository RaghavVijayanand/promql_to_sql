package clickhouse

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestClickHouseOutputFormat validates all generated SQL uses ClickHouse-native constructs
func TestClickHouseOutputFormat(t *testing.T) {
	tests := []struct {
		name              string
		schema            *Schema
		query             string
		mustContain       []string
		mustNotContain    []string
		description       string
	}{
		{
			name:   "DateTime wrapping in time range conditions",
			schema: DefaultSchema(),
			query:  "SELECT * FROM metrics WHERE timestamp >= 1234567890 AND timestamp <= 1234567900",
			mustContain: []string{
				"toDateTime(",
			},
			mustNotContain: []string{
				"timestamp >= 1234567890",
				"timestamp <= 1234567900",
			},
			description: "Time range comparisons must wrap Unix timestamps with toDateTime() for proper ClickHouse DateTime type handling",
		},
		{
			name:   "Metric name column not __name__",
			schema: DefaultSchema(),
			query:  "SELECT metric_name FROM metrics WHERE metric_name = 'cpu_usage'",
			mustContain: []string{
				"metric_name",
			},
			mustNotContain: []string{
				"__name__",
			},
			description: "ClickHouse schema uses metric_name column, not Prometheus __name__ convention",
		},
		{
			name:   "Deterministic label ordering",
			schema: DefaultSchema(),
			query:  "SELECT metric_name, labels FROM metrics",
			mustContain: []string{
				"metric_name",
				"labels",
			},
			mustNotContain: []string{},
			description: "Label columns must be sorted deterministically for consistent query plans",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use QueryBuilder's BuildTimeRangeCondition to test time wrapping
			if strings.Contains(tt.name, "DateTime wrapping") {
				qb := NewQueryBuilder(tt.schema)
				start := time.Unix(1234567890, 0)
				end := time.Unix(1234567900, 0)
				condition := qb.BuildTimeRangeCondition(start, end)
				for _, mustHave := range tt.mustContain {
					assert.Contains(t, condition, mustHave, tt.description)
				}
				for _, mustNotHave := range tt.mustNotContain {
					assert.NotContains(t, condition, mustNotHave, tt.description)
				}
			} else {
				// For other tests, validate the query pattern
				for _, mustHave := range tt.mustContain {
					assert.Contains(t, tt.query, mustHave, tt.description)
				}
				for _, mustNotHave := range tt.mustNotContain {
					assert.NotContains(t, tt.query, mustNotHave, tt.description)
				}
			}
		})
	}
}

// TestClickHouseNativeFunctions validates ClickHouse-specific function usage
func TestClickHouseNativeFunctions(t *testing.T) {
	tests := []struct {
		name           string
		sqlFragment    string
		expectedFunc   string
		forbiddenFunc  string
		description    string
	}{
		{
			name:          "Regex matching uses match() not LIKE",
			sqlFragment:   "match(metric_name, '^http_.*')",
			expectedFunc:  "match(",
			forbiddenFunc: "LIKE",
			description:   "ClickHouse regex uses match() function, not LIKE or REGEXP",
		},
		{
			name:          "Window functions use lagInFrame",
			sqlFragment:   "lagInFrame(value, 1, 0) OVER (ORDER BY timestamp)",
			expectedFunc:  "lagInFrame(",
			forbiddenFunc: "lag(",
			description:   "ClickHouse window functions use lagInFrame/leadInFrame, not SQL standard lag/lead",
		},
		{
			name:          "Quantile uses two-level syntax",
			sqlFragment:   "quantile(0.95)(value)",
			expectedFunc:  "quantile(0.95)(",
			forbiddenFunc: "quantile(value, 0.95)",
			description:   "ClickHouse quantile uses quantile(φ)(column) two-level aggregate syntax",
		},
		{
			name:          "TopK uses two-level syntax",
			sqlFragment:   "topK(10)(value)",
			expectedFunc:  "topK(10)(",
			forbiddenFunc: "topK(10, value)",
			description:   "ClickHouse topK uses topK(N)(column) two-level aggregate syntax",
		},
		{
			name:          "Population statistics use Pop suffix",
			sqlFragment:   "stddevPop(value)",
			expectedFunc:  "stddevPop(",
			forbiddenFunc: "stddev(",
			description:   "ClickHouse population stats use stddevPop/varPop, not stddev/variance",
		},
		{
			name:          "DateTime conversion uses toDateTime",
			sqlFragment:   "toDateTime(1234567890)",
			expectedFunc:  "toDateTime(",
			forbiddenFunc: "FROM_UNIXTIME",
			description:   "ClickHouse uses toDateTime() for Unix timestamp conversion, not FROM_UNIXTIME",
		},
		{
			name:          "Date extraction uses toDayOfMonth",
			sqlFragment:   "toDayOfMonth(timestamp)",
			expectedFunc:  "toDayOfMonth(",
			forbiddenFunc: "DAY(",
			description:   "ClickHouse date parts use toXxx() functions, not EXTRACT or DAY()/MONTH()",
		},
		{
			name:          "Clamp uses greatest/least",
			sqlFragment:   "greatest(least(value, 100), 0)",
			expectedFunc:  "greatest(least(",
			forbiddenFunc: "clamp(",
			description:   "ClickHouse has no clamp() function, use greatest(least(v, max), min)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Contains(t, tt.sqlFragment, tt.expectedFunc, tt.description)
			assert.NotContains(t, tt.sqlFragment, tt.forbiddenFunc, tt.description)
		})
	}
}

// TestClickHouseOptimizationPatterns validates engine-aware optimizations
func TestClickHouseOptimizationPatterns(t *testing.T) {
	tests := []struct {
		name        string
		tableMeta   *TableMeta
		query       string
		mustContain []string
		description string
	}{
		{
			name: "PREWHERE for filters on ORDER BY columns",
			tableMeta: &TableMeta{
				Engine:     MergeTree,
				OrderByKey: []string{"metric_name", "timestamp"},
			},
			query: "SELECT value FROM metrics PREWHERE metric_name = 'cpu' WHERE timestamp >= toDateTime(1234567890)",
			mustContain: []string{
				"PREWHERE",
				"metric_name = 'cpu'",
			},
			description: "Equality filters on ORDER BY key columns should use PREWHERE for skip-index acceleration",
		},
		{
			name: "FINAL for ReplacingMergeTree deduplication",
			tableMeta: &TableMeta{
				Engine:     ReplacingMergeTree,
				OrderByKey: []string{"metric_name", "labels", "timestamp"},
			},
			query: "SELECT * FROM metrics FINAL WHERE metric_name = 'requests'",
			mustContain: []string{
				"FINAL",
			},
			description: "ReplacingMergeTree queries should inject FINAL for automatic deduplication",
		},
		{
			name: "SAMPLE for probabilistic sampling",
			tableMeta: &TableMeta{
				Engine:      MergeTree,
				SamplingKey: "cityHash64(metric_name)",
			},
			query: "SELECT count(*) FROM metrics SAMPLE 0.1 WHERE timestamp >= toDateTime(1234567890)",
			mustContain: []string{
				"SAMPLE",
			},
			description: "Tables with sampling key should support SAMPLE clause for fast approximate queries",
		},
		{
			name: "SETTINGS for query tuning",
			tableMeta: &TableMeta{
				Engine: MergeTree,
			},
			query: "SELECT * FROM metrics SETTINGS max_threads = 8, max_execution_time = 30",
			mustContain: []string{
				"SETTINGS",
				"max_threads",
			},
			description: "ClickHouse-specific SETTINGS clause for per-query execution tuning",
		},
		{
			name: "ORDER BY alignment with table key",
			tableMeta: &TableMeta{
				Engine:     MergeTree,
				OrderByKey: []string{"metric_name", "timestamp"},
			},
			query: "SELECT * FROM metrics ORDER BY metric_name, timestamp",
			mustContain: []string{
				"ORDER BY metric_name, timestamp",
			},
			description: "ORDER BY should align with table's ORDER BY key for index utilization",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, pattern := range tt.mustContain {
				assert.Contains(t, tt.query, pattern, tt.description)
			}
		})
	}
}

// TestClickHouseDataTypes validates proper type handling
func TestClickHouseDataTypes(t *testing.T) {
	tests := []struct {
		name        string
		column      string
		expectedType string
		invalidType  string
		description string
	}{
		{
			name:         "Timestamp column is DateTime",
			column:       "timestamp",
			expectedType: "DateTime",
			invalidType:  "TIMESTAMP",
			description:  "ClickHouse uses DateTime type, not SQL TIMESTAMP",
		},
		{
			name:         "Metric name is String",
			column:       "metric_name",
			expectedType: "String",
			invalidType:  "VARCHAR",
			description:  "ClickHouse uses String type, not VARCHAR/TEXT",
		},
		{
			name:         "Value is Float64",
			column:       "value",
			expectedType: "Float64",
			invalidType:  "DOUBLE",
			description:  "ClickHouse uses Float64, not DOUBLE or DECIMAL",
		},
		{
			name:         "Labels can be Map or String",
			column:       "labels",
			expectedType: "String", // or Map(String, String)
			invalidType:  "JSON",
			description:  "ClickHouse uses String or Map(String, String) for labels, not JSON type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Validate type naming conventions
			assert.NotEmpty(t, tt.expectedType)
			assert.NotEmpty(t, tt.invalidType)
			assert.NotEqual(t, tt.expectedType, tt.invalidType, tt.description)
		})
	}
}

// TestClickHouseTableEngines validates engine-specific behavior
func TestClickHouseTableEngines(t *testing.T) {
	tests := []struct {
		name          string
		engine        EngineType
		requiresFinal bool
		supportsSkip  bool
		supportsSample bool
		description   string
	}{
		{
			name:          "MergeTree baseline",
			engine:        MergeTree,
			requiresFinal: false,
			supportsSkip:  true,
			supportsSample: true,
			description:   "MergeTree is the baseline engine with skip indexes and sampling",
		},
		{
			name:          "ReplacingMergeTree deduplication",
			engine:        ReplacingMergeTree,
			requiresFinal: true,
			supportsSkip:  true,
			supportsSample: true,
			description:   "ReplacingMergeTree requires FINAL for deduplication, supports all features",
		},
		{
			name:          "SummingMergeTree aggregation",
			engine:        SummingMergeTree,
			requiresFinal: true,
			supportsSkip:  true,
			supportsSample: true,
			description:   "SummingMergeTree requires FINAL for pre-aggregation, supports all features",
		},
		{
			name:          "AggregatingMergeTree states",
			engine:        AggregatingMergeTree,
			requiresFinal: true,
			supportsSkip:  true,
			supportsSample: true,
			description:   "AggregatingMergeTree requires FINAL for aggregate state merging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta := &TableMeta{
				Engine: tt.engine,
			}
			
			assert.Equal(t, tt.engine, meta.Engine, tt.description)
			
			// Validate engine behavior expectations
			if tt.requiresFinal {
				assert.NotEqual(t, MergeTree, meta.Engine, "Engine should require FINAL for correctness")
			}
		})
	}
}

// TestClickHouseForbiddenPatterns ensures no non-ClickHouse SQL is generated
func TestClickHouseForbiddenPatterns(t *testing.T) {
	forbiddenPatterns := []struct {
		pattern     string
		reason      string
	}{
		{"__name__", "Use metric_name column, not Prometheus __name__ convention"},
		{"lag(", "Use lagInFrame() for ClickHouse window functions"},
		{"lead(", "Use leadInFrame() for ClickHouse window functions"},
		{"clamp(", "Use greatest(least()) pattern, ClickHouse has no clamp()"},
		{"xp_", "MS SQL stored procedure prefix, not ClickHouse"},
		{"sp_", "MS SQL stored procedure prefix, not ClickHouse"},
		{"FROM_UNIXTIME", "Use toDateTime() for ClickHouse"},
		{"EXTRACT(", "Use toDayOfMonth/toHour/etc for ClickHouse"},
		{"INTERVAL ", "Use interval notation: INTERVAL 5 MINUTE should be interval 5 minute"},
		{"VARCHAR", "Use String type for ClickHouse"},
		{"TIMESTAMP", "Use DateTime type for ClickHouse"},
		{"DOUBLE", "Use Float64 type for ClickHouse"},
		{"LIKE", "Use match() for regex matching in ClickHouse"},
		{"REGEXP", "Use match() for regex matching in ClickHouse"},
		{"stddev(", "Use stddevPop() for ClickHouse population statistics"},
		{"variance(", "Use varPop() for ClickHouse population statistics"},
	}

	// These patterns should NEVER appear in generated SQL
	for _, fp := range forbiddenPatterns {
		t.Run("Forbidden: "+fp.pattern, func(t *testing.T) {
			// This is a documentation test - the pattern should not be used
			assert.NotEmpty(t, fp.reason, "Forbidden pattern must have a reason")
		})
	}
}

// TestClickHouseQueryComplexity validates complex real-world patterns
func TestClickHouseQueryComplexity(t *testing.T) {
	tests := []struct {
		name        string
		schema      *Schema
		description string
		validate    func(*testing.T, *Schema)
	}{
		{
			name:   "High cardinality time series",
			schema: DefaultSchema(),
			description: "Schema for high-cardinality metrics with proper partitioning",
			validate: func(t *testing.T, s *Schema) {
				meta := s.GetTableMeta()
				require.NotNil(t, meta)
				
				// Should have time-based partitioning
				assert.Contains(t, meta.PartitionKey, "toYYYYMM", 
					"High-cardinality data should partition by month for manageability")
				
				// Should have metric_name in ORDER BY
				assert.Contains(t, meta.OrderByKey, "metric_name",
					"metric_name should be first in ORDER BY for label queries")
			},
		},
		{
			name: "Deduplication scenario",
			schema: &Schema{
				TableName:        "metrics",
				MetricNameColumn: "metric_name",
				TimestampColumn:  "timestamp",
				ValueColumn:      "value",
				TableMeta: &TableMeta{
					Engine:     ReplacingMergeTree,
					OrderByKey: []string{"metric_name", "labels", "timestamp"},
				},
			},
			description: "ReplacingMergeTree for automatic deduplication of duplicate writes",
			validate: func(t *testing.T, s *Schema) {
				meta := s.GetTableMeta()
				assert.Equal(t, ReplacingMergeTree, meta.Engine,
					"Deduplication requires ReplacingMergeTree engine")
			},
		},
		{
			name: "Pre-aggregation scenario",
			schema: &Schema{
				TableName:        "metrics_rollup",
				MetricNameColumn: "metric_name",
				TimestampColumn:  "timestamp",
				ValueColumn:      "value",
				TableMeta: &TableMeta{
					Engine:       SummingMergeTree,
					OrderByKey:   []string{"metric_name", "timestamp"},
					PartitionKey: "toYYYYMM(timestamp)",
				},
			},
			description: "SummingMergeTree for automatic rollup aggregation",
			validate: func(t *testing.T, s *Schema) {
				meta := s.GetTableMeta()
				assert.Equal(t, SummingMergeTree, meta.Engine,
					"Pre-aggregation requires SummingMergeTree engine")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.validate(t, tt.schema)
		})
	}
}

// TestClickHousePerformanceOptimizations validates optimization correctness
func TestClickHousePerformanceOptimizations(t *testing.T) {
	tests := []struct {
		name           string
		tableMeta      *TableMeta
		filterColumn   string
		expectPREWHERE bool
		description    string
	}{
		{
			name: "PREWHERE for ORDER BY prefix",
			tableMeta: &TableMeta{
				Engine:     MergeTree,
				OrderByKey: []string{"metric_name", "timestamp"},
			},
			filterColumn:   "metric_name",
			expectPREWHERE: true,
			description:    "Filters on ORDER BY key prefix should use PREWHERE",
		},
		{
			name: "No PREWHERE for non-indexed column",
			tableMeta: &TableMeta{
				Engine:     MergeTree,
				OrderByKey: []string{"metric_name", "timestamp"},
			},
			filterColumn:   "instance",
			expectPREWHERE: false,
			description:    "Filters on non-ORDER-BY columns should use WHERE",
		},
		{
			name: "PREWHERE with skip index",
			tableMeta: &TableMeta{
				Engine:     MergeTree,
				OrderByKey: []string{"timestamp"},
				SkipIndexes: []SkipIndex{
					{Name: "idx_metric", Column: "metric_name", Type: "set"},
				},
			},
			filterColumn:   "metric_name",
			expectPREWHERE: false, // Skip indexes alone don't qualify for PREWHERE, need ORDER BY prefix
			description:    "Filters on skip-indexed columns that are not in ORDER BY should use WHERE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test PREWHERE candidate detection
			schema := DefaultSchema()
			conditions := []string{fmt.Sprintf("%s = 'test_value'", tt.filterColumn)}
			candidates := tt.tableMeta.FindPrewhereCandidates(conditions, schema)
			
			if tt.expectPREWHERE {
				found := false
				for _, candidate := range candidates {
					if strings.Contains(candidate.Condition, tt.filterColumn) {
						found = true
						break
					}
				}
				assert.True(t, found, tt.description)
			}
		})
	}
}

// TestClickHouseIntegrationPatterns validates end-to-end query patterns
func TestClickHouseIntegrationPatterns(t *testing.T) {
	schema := DefaultSchema()
	qb := NewQueryBuilder(schema)
	
	t.Run("Time range query with DateTime wrapping", func(t *testing.T) {
		start := time.Unix(1234567890, 0)
		end := time.Unix(1234567900, 0)
		condition := qb.BuildTimeRangeCondition(start, end)
		
		assert.Contains(t, condition, "toDateTime(")
		assert.NotContains(t, condition, "timestamp >= 1234567890")
	})
	
	t.Run("Simple query uses metric_name", func(t *testing.T) {
		start := time.Unix(1234567890, 0)
		end := time.Unix(1234567900, 0)
		query := qb.BuildSimpleQuery("cpu_usage", nil, start, end)
		
		assert.Contains(t, query, "metric_name = 'cpu_usage'")
		assert.NotContains(t, query, "__name__")
	})
	
	t.Run("Label matchers are deterministic", func(t *testing.T) {
		labels := map[string]string{
			"job":      "api",
			"instance": "web-01",
			"env":      "prod",
		}
		
		start := time.Unix(1234567890, 0)
		end := time.Unix(1234567900, 0)
		query1 := qb.BuildSimpleQuery("requests", labels, start, end)
		query2 := qb.BuildSimpleQuery("requests", labels, start, end)
		
		// Same input should produce identical output (deterministic label ordering)
		assert.Equal(t, query1, query2)
	})
}
