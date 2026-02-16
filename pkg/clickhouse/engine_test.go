package clickhouse

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── ENGINE TYPE ─────────────────────────────────────────────────────────────

func TestEngineType_String(t *testing.T) {
	tests := []struct {
		engine   EngineType
		expected string
	}{
		{MergeTree, "MergeTree"},
		{ReplacingMergeTree, "ReplacingMergeTree"},
		{SummingMergeTree, "SummingMergeTree"},
		{AggregatingMergeTree, "AggregatingMergeTree"},
		{EngineType(99), "MergeTree"}, // unknown defaults to MergeTree
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expected, tt.engine.String())
	}
}

func TestEngineType_NeedsFinal(t *testing.T) {
	assert.False(t, MergeTree.NeedsFinal())
	assert.True(t, ReplacingMergeTree.NeedsFinal())
	assert.False(t, SummingMergeTree.NeedsFinal())
	assert.False(t, AggregatingMergeTree.NeedsFinal())
}

func TestEngineType_SupportsPreAggregation(t *testing.T) {
	assert.False(t, MergeTree.SupportsPreAggregation())
	assert.False(t, ReplacingMergeTree.SupportsPreAggregation())
	assert.True(t, SummingMergeTree.SupportsPreAggregation())
	assert.True(t, AggregatingMergeTree.SupportsPreAggregation())
}

// ─── TABLE META ──────────────────────────────────────────────────────────────

func TestDefaultTableMeta(t *testing.T) {
	meta := DefaultTableMeta()

	assert.Equal(t, MergeTree, meta.Engine)
	assert.Equal(t, []string{"metric_name", "timestamp"}, meta.OrderByKey)
	assert.Equal(t, "toYYYYMM(timestamp)", meta.PartitionKey)
	assert.Equal(t, 90*24*time.Hour, meta.TTL)
	require.Len(t, meta.SkipIndexes, 1)
	assert.Equal(t, "set", meta.SkipIndexes[0].Type)
	assert.Equal(t, "metric_name", meta.SkipIndexes[0].Column)
}

func TestTableMeta_EffectivePrimaryKey(t *testing.T) {
	t.Run("uses OrderByKey when PrimaryKey is empty", func(t *testing.T) {
		meta := &TableMeta{OrderByKey: []string{"a", "b"}}
		assert.Equal(t, []string{"a", "b"}, meta.EffectivePrimaryKey())
	})
	t.Run("uses PrimaryKey when set", func(t *testing.T) {
		meta := &TableMeta{
			OrderByKey: []string{"a", "b"},
			PrimaryKey: []string{"a"},
		}
		assert.Equal(t, []string{"a"}, meta.EffectivePrimaryKey())
	})
}

func TestTableMeta_LeadingKeyColumn(t *testing.T) {
	meta := DefaultTableMeta()
	assert.Equal(t, "metric_name", meta.LeadingKeyColumn())

	empty := &TableMeta{}
	assert.Equal(t, "", empty.LeadingKeyColumn())
}

func TestTableMeta_HasColumnInKey(t *testing.T) {
	meta := DefaultTableMeta()
	assert.True(t, meta.HasColumnInKey("metric_name"))
	assert.True(t, meta.HasColumnInKey("timestamp"))
	assert.False(t, meta.HasColumnInKey("value"))
	assert.False(t, meta.HasColumnInKey("labels"))
}

func TestTableMeta_ColumnKeyPosition(t *testing.T) {
	meta := DefaultTableMeta()
	assert.Equal(t, 0, meta.ColumnKeyPosition("metric_name"))
	assert.Equal(t, 1, meta.ColumnKeyPosition("timestamp"))
	assert.Equal(t, -1, meta.ColumnKeyPosition("value"))
}

func TestTableMeta_HasSkipIndexOn(t *testing.T) {
	meta := DefaultTableMeta()
	assert.True(t, meta.HasSkipIndexOn("metric_name", "set"))
	assert.False(t, meta.HasSkipIndexOn("metric_name", "bloom_filter"))
	assert.False(t, meta.HasSkipIndexOn("timestamp", "set"))
}

func TestTableMeta_CanSample(t *testing.T) {
	meta := DefaultTableMeta()
	assert.False(t, meta.CanSample())

	meta.SamplingKey = "rand()"
	assert.True(t, meta.CanSample())
}

// ─── TABLE REFERENCE ─────────────────────────────────────────────────────────

func TestTableMeta_TableRef(t *testing.T) {
	t.Run("MergeTree — no FINAL", func(t *testing.T) {
		meta := &TableMeta{Engine: MergeTree}
		assert.Equal(t, "metrics", meta.TableRef("metrics"))
	})
	t.Run("ReplacingMergeTree — adds FINAL", func(t *testing.T) {
		meta := &TableMeta{Engine: ReplacingMergeTree}
		assert.Equal(t, "metrics FINAL", meta.TableRef("metrics"))
	})
}

func TestTableMeta_TableRefWithSample(t *testing.T) {
	t.Run("no sampling key — ignores ratio", func(t *testing.T) {
		meta := &TableMeta{Engine: MergeTree}
		assert.Equal(t, "metrics", meta.TableRefWithSample("metrics", 0.1))
	})
	t.Run("with sampling key and valid ratio", func(t *testing.T) {
		meta := &TableMeta{Engine: MergeTree, SamplingKey: "rand()"}
		assert.Equal(t, "metrics SAMPLE 0.1000", meta.TableRefWithSample("metrics", 0.1))
	})
	t.Run("ratio >= 1 — no sample", func(t *testing.T) {
		meta := &TableMeta{Engine: MergeTree, SamplingKey: "rand()"}
		assert.Equal(t, "metrics", meta.TableRefWithSample("metrics", 1.0))
	})
	t.Run("ReplacingMergeTree with sampling", func(t *testing.T) {
		meta := &TableMeta{Engine: ReplacingMergeTree, SamplingKey: "rand()"}
		assert.Equal(t, "metrics FINAL SAMPLE 0.5000", meta.TableRefWithSample("metrics", 0.5))
	})
}

// ─── PREWHERE CANDIDATES ────────────────────────────────────────────────────

func TestTableMeta_FindPrewhereCandidates(t *testing.T) {
	meta := DefaultTableMeta()
	schema := DefaultSchema()

	conditions := []string{
		"metric_name = 'http_requests_total'",
		"timestamp >= '2024-01-01'",
		"labels['job'] = 'api'",
	}

	candidates := meta.FindPrewhereCandidates(conditions, schema)

	// Both metric_name (ORDER BY pos 0) and timestamp (partition key → pos 0)
	// are candidates. Labels condition is NOT a candidate (AnalyzeQuery
	// handles that). Sort is stable, so metric_name appears first.
	require.GreaterOrEqual(t, len(candidates), 2)
	assert.Equal(t, "metric_name", candidates[0].Column)
	assert.Equal(t, "timestamp", candidates[1].Column)
}

// ─── QUERY HINTS ─────────────────────────────────────────────────────────────

func TestTableMeta_AnalyzeQuery(t *testing.T) {
	t.Run("MergeTree — no FINAL", func(t *testing.T) {
		meta := DefaultTableMeta()
		schema := DefaultSchema()
		hints := meta.AnalyzeQuery(
			[]string{"metric_name = 'up'", "timestamp >= '2024-01-01'"},
			schema, time.Hour,
		)
		assert.False(t, hints.UseFinal)
		assert.True(t, hints.CanUsePartitionPruning)
		assert.False(t, hints.SuggestSample)
		assert.Len(t, hints.PrewhereConditions, 2)
		assert.Empty(t, hints.RemainingWhereConditions)
	})

	t.Run("ReplacingMergeTree — needs FINAL", func(t *testing.T) {
		meta := &TableMeta{Engine: ReplacingMergeTree, OrderByKey: []string{"metric_name"}}
		schema := DefaultSchema()
		hints := meta.AnalyzeQuery(nil, schema, time.Hour)
		assert.True(t, hints.UseFinal)
	})

	t.Run("large time range with sampling key", func(t *testing.T) {
		meta := DefaultTableMeta()
		meta.SamplingKey = "rand()"
		schema := DefaultSchema()
		hints := meta.AnalyzeQuery(
			[]string{"timestamp >= '2024-01-01'"},
			schema, 7*24*time.Hour, // 7 days
		)
		assert.True(t, hints.SuggestSample)
		assert.Equal(t, 0.5, hints.SampleRatio)
	})

	t.Run("30-day time range sampling", func(t *testing.T) {
		meta := DefaultTableMeta()
		meta.SamplingKey = "rand()"
		schema := DefaultSchema()
		hints := meta.AnalyzeQuery(nil, schema, 30*24*time.Hour)
		assert.True(t, hints.SuggestSample)
		assert.Equal(t, 0.1, hints.SampleRatio)
	})

	t.Run("labels stay in WHERE", func(t *testing.T) {
		meta := DefaultTableMeta()
		schema := DefaultSchema()
		hints := meta.AnalyzeQuery(
			[]string{
				"metric_name = 'up'",
				"labels['job'] = 'api'",
			},
			schema, time.Hour,
		)
		assert.Len(t, hints.PrewhereConditions, 1)
		assert.Contains(t, hints.PrewhereConditions[0], "metric_name")
		assert.Len(t, hints.RemainingWhereConditions, 1)
		assert.Contains(t, hints.RemainingWhereConditions[0], "labels")
	})
}

// ─── SCHEMA INTEGRATION ─────────────────────────────────────────────────────

func TestSchema_GetTableMeta(t *testing.T) {
	t.Run("returns explicit TableMeta when set", func(t *testing.T) {
		custom := &TableMeta{Engine: ReplacingMergeTree}
		schema := &Schema{TableMeta: custom}
		assert.Equal(t, ReplacingMergeTree, schema.GetTableMeta().Engine)
	})
	t.Run("falls back to DefaultTableMeta when nil", func(t *testing.T) {
		schema := &Schema{}
		meta := schema.GetTableMeta()
		assert.Equal(t, MergeTree, meta.Engine)
		assert.Equal(t, []string{"metric_name", "timestamp"}, meta.OrderByKey)
	})
}

func TestDefaultSchema_IncludesTableMeta(t *testing.T) {
	schema := DefaultSchema()
	require.NotNil(t, schema.TableMeta)
	assert.Equal(t, MergeTree, schema.TableMeta.Engine)
	assert.Equal(t, "toYYYYMM(timestamp)", schema.TableMeta.PartitionKey)
}
