package optimization

import (
	"testing"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/stretchr/testify/assert"
)

// ─── FINAL INJECTION ─────────────────────────────────────────────────────────

func TestEngineOptimizer_InjectFinal(t *testing.T) {
	t.Run("ReplacingMergeTree gets FINAL", func(t *testing.T) {
		meta := &clickhouse.TableMeta{
			Engine:     clickhouse.ReplacingMergeTree,
			OrderByKey: []string{"metric_name", "timestamp"},
		}
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics WHERE metric_name = 'up'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "FROM metrics FINAL")
	})

	t.Run("MergeTree does not get FINAL", func(t *testing.T) {
		meta := clickhouse.DefaultTableMeta()
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics WHERE metric_name = 'up'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.NotContains(t, result, "FINAL")
	})

	t.Run("does not double-add FINAL", func(t *testing.T) {
		meta := &clickhouse.TableMeta{Engine: clickhouse.ReplacingMergeTree}
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics FINAL WHERE metric_name = 'up'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		// Should still have exactly one FINAL
		assert.Equal(t, 1, countOccurrences(result, "FINAL"))
	})
}

// ─── PREWHERE PROMOTION ──────────────────────────────────────────────────────

func TestEngineOptimizer_PrewherePromotion(t *testing.T) {
	meta := clickhouse.DefaultTableMeta()
	schema := clickhouse.DefaultSchema()
	opt := NewEngineOptimizer(meta, schema)

	t.Run("promotes metric_name to PREWHERE", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE metric_name = 'up' AND labels['job'] = 'api'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "PREWHERE metric_name = 'up'")
		assert.Contains(t, result, "WHERE labels['job'] = 'api'")
	})

	t.Run("promotes timestamp to PREWHERE", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE timestamp >= '2024-01-01' AND labels['env'] = 'prod'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "PREWHERE timestamp >= '2024-01-01'")
		assert.Contains(t, result, "WHERE labels['env'] = 'prod'")
	})

	t.Run("does not promote labels to PREWHERE", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE labels['job'] = 'api'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.NotContains(t, result, "PREWHERE")
		assert.Contains(t, result, "WHERE labels['job'] = 'api'")
	})

	t.Run("promotes multiple key conditions", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE metric_name = 'up' AND timestamp >= '2024-01-01' AND labels['job'] = 'api'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "PREWHERE")
		assert.Contains(t, result, "metric_name = 'up'")
		assert.Contains(t, result, "timestamp >= '2024-01-01'")
		// Labels should stay in WHERE
		assert.Contains(t, result, "WHERE labels['job'] = 'api'")
	})

	t.Run("all conditions promoted — no WHERE remains", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE metric_name = 'up'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "PREWHERE metric_name = 'up'")
		// No standalone WHERE clause should remain (PREWHERE contains WHERE
		// as a substring, so we check for "\nWHERE " specifically)
		assert.NotContains(t, result, "\nWHERE ")
	})

	t.Run("does not double-add PREWHERE", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nPREWHERE metric_name = 'up'\nWHERE labels['job'] = 'api'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Equal(t, 1, countOccurrences(result, "PREWHERE"))
	})

	t.Run("no WHERE clause — no change", func(t *testing.T) {
		sql := "SELECT value FROM metrics"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.NotContains(t, result, "PREWHERE")
	})
}

// ─── ORDER BY OPTIMIZATION ───────────────────────────────────────────────────

func TestEngineOptimizer_OrderByAlignment(t *testing.T) {
	meta := clickhouse.DefaultTableMeta() // ORDER BY (metric_name, timestamp)
	schema := clickhouse.DefaultSchema()
	opt := NewEngineOptimizer(meta, schema)

	t.Run("aligned ORDER BY gets optimize_read_in_order", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE metric_name = 'up'\nORDER BY timestamp"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "optimize_read_in_order = 1")
	})

	t.Run("non-aligned ORDER BY does NOT get setting", func(t *testing.T) {
		sql := "SELECT value FROM metrics\nWHERE metric_name = 'up'\nORDER BY value DESC"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.NotContains(t, result, "optimize_read_in_order")
	})
}

// ─── SETTINGS ────────────────────────────────────────────────────────────────

func TestEngineOptimizer_Settings(t *testing.T) {
	t.Run("ReplacingMergeTree adds partition-select-final setting", func(t *testing.T) {
		meta := &clickhouse.TableMeta{Engine: clickhouse.ReplacingMergeTree}
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics WHERE metric_name = 'up'"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.Contains(t, result, "do_not_merge_across_partitions_select_final = 1")
	})

	t.Run("MergeTree does not add extra settings", func(t *testing.T) {
		meta := clickhouse.DefaultTableMeta()
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		assert.NotContains(t, result, "do_not_merge_across_partitions")
	})

	t.Run("appends to existing SETTINGS correctly", func(t *testing.T) {
		meta := &clickhouse.TableMeta{
			Engine:     clickhouse.ReplacingMergeTree,
			OrderByKey: []string{"timestamp"},
		}
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics WHERE metric_name = 'up'\nORDER BY timestamp\nSETTINGS max_threads = 4"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)
		// Should have all settings in one SETTINGS clause
		assert.Contains(t, result, "SETTINGS")
		assert.Contains(t, result, "max_threads = 4")
		assert.Contains(t, result, "do_not_merge_across_partitions_select_final = 1")
		// Should NOT duplicate SETTINGS keyword
		assert.Equal(t, 1, countOccurrences(result, "SETTINGS"))
	})
}

// ─── EMPTY / NIL ─────────────────────────────────────────────────────────────

func TestEngineOptimizer_EmptyQuery(t *testing.T) {
	opt := NewEngineOptimizer(nil, nil)
	result, err := opt.Optimize("", nil)
	assert.NoError(t, err)
	assert.Equal(t, "", result)
}

func TestEngineOptimizer_NilDefaults(t *testing.T) {
	opt := NewEngineOptimizer(nil, nil)
	assert.NotNil(t, opt.meta)
	assert.NotNil(t, opt.schema)
}

// ─── SPLIT HELPER ────────────────────────────────────────────────────────────

func TestSplitTopLevelAND(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"a = 1 AND b = 2", []string{"a = 1", "b = 2"}},
		{"a = 1 AND b = 2 AND c = 3", []string{"a = 1", "b = 2", "c = 3"}},
		{"(a AND b) AND c", []string{"(a AND b)", "c"}},
		{"a = 1", []string{"a = 1"}},
		{"", nil},
	}

	for _, tt := range tests {
		result := splitTopLevelAND(tt.input)
		assert.Equal(t, tt.expected, result, "input: %q", tt.input)
	}
}

// ─── INTEGRATION: FULL PIPELINE ──────────────────────────────────────────────

func TestEngineOptimizer_FullPipeline(t *testing.T) {
	t.Run("ReplacingMergeTree + PREWHERE + FINAL", func(t *testing.T) {
		meta := &clickhouse.TableMeta{
			Engine:       clickhouse.ReplacingMergeTree,
			OrderByKey:   []string{"metric_name", "timestamp"},
			PartitionKey: "toYYYYMM(timestamp)",
		}
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT value FROM metrics\nWHERE metric_name = 'up' AND timestamp >= '2024-01-01' AND labels['job'] = 'api'\nORDER BY timestamp"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)

		// Should have FINAL
		assert.Contains(t, result, "FROM metrics FINAL")
		// Should have PREWHERE
		assert.Contains(t, result, "PREWHERE")
		// Labels should be in WHERE
		assert.Contains(t, result, "WHERE labels['job'] = 'api'")
		// Should have read-in-order (timestamp is in ORDER BY key)
		assert.Contains(t, result, "optimize_read_in_order = 1")
		// Should have partition-final setting
		assert.Contains(t, result, "do_not_merge_across_partitions_select_final = 1")
	})

	t.Run("standard MergeTree time-series query", func(t *testing.T) {
		meta := clickhouse.DefaultTableMeta()
		schema := clickhouse.DefaultSchema()
		opt := NewEngineOptimizer(meta, schema)

		sql := "SELECT avg(value) FROM metrics\nWHERE metric_name = 'cpu_usage' AND timestamp >= '2024-01-01' AND timestamp <= '2024-01-31'\nGROUP BY toStartOfHour(timestamp)\nORDER BY timestamp"
		result, err := opt.Optimize(sql, nil)
		assert.NoError(t, err)

		// No FINAL for MergeTree
		assert.NotContains(t, result, "FINAL")
		// PREWHERE on metric_name and timestamp (both are ORDER BY key columns)
		assert.Contains(t, result, "PREWHERE")
		// ORDER BY aligns with table key
		assert.Contains(t, result, "optimize_read_in_order = 1")
	})
}

// ─── HELPER ──────────────────────────────────────────────────────────────────

func countOccurrences(s, substr string) int {
	count := 0
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			count++
		}
	}
	return count
}
