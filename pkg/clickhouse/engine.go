package clickhouse

import (
	"fmt"
	"strings"
	"time"
)

// ─── ENGINE TYPES ────────────────────────────────────────────────────────────
//
// ClickHouse's MergeTree family is purpose-built for time-series data.
// Each engine variant has different semantics that affect how we generate SQL:
//
//   MergeTree          – Default; no dedup; ideal for append-only metrics.
//   ReplacingMergeTree – Deduplicates by ORDER BY key on merge; queries may
//                        need FINAL to see the latest version.
//   SummingMergeTree   – Auto-sums numeric columns on merge; perfect for
//                        pre-aggregated counters.
//   AggregatingMergeTree – Stores intermediate aggregate states; used for
//                        materialized views with -State/-Merge combinators.
//
// The engine type determines:
//   1. Whether we append FINAL to table references (ReplacingMergeTree)
//   2. Whether we can rely on pre-aggregated data (Summing/Aggregating)
//   3. Which columns are safe for PREWHERE (must not be in ORDER BY key)
//   4. How partition pruning is applied
// ──────────────────────────────────────────────────────────────────────────────

// EngineType represents a ClickHouse MergeTree engine variant.
type EngineType int

const (
	// MergeTree is the default engine — append-only, no deduplication.
	MergeTree EngineType = iota

	// ReplacingMergeTree deduplicates rows by ORDER BY key during merges.
	// Queries should use FINAL to see only the latest version of each row.
	ReplacingMergeTree

	// SummingMergeTree automatically sums numeric columns with the same
	// ORDER BY key during merges. Ideal for pre-aggregated counters.
	SummingMergeTree

	// AggregatingMergeTree stores intermediate aggregate function states.
	// Used with materialized views and -State/-Merge combinators.
	AggregatingMergeTree
)

// String returns the ClickHouse engine name.
func (e EngineType) String() string {
	switch e {
	case MergeTree:
		return "MergeTree"
	case ReplacingMergeTree:
		return "ReplacingMergeTree"
	case SummingMergeTree:
		return "SummingMergeTree"
	case AggregatingMergeTree:
		return "AggregatingMergeTree"
	default:
		return "MergeTree"
	}
}

// NeedsFinal returns true if queries against this engine type should
// include FINAL to guarantee deduplication semantics.
func (e EngineType) NeedsFinal() bool {
	return e == ReplacingMergeTree
}

// SupportsPreAggregation returns true if the engine performs automatic
// aggregation on merge (Summing or Aggregating).
func (e EngineType) SupportsPreAggregation() bool {
	return e == SummingMergeTree || e == AggregatingMergeTree
}

// ─── TABLE METADATA ──────────────────────────────────────────────────────────
//
// TableMeta captures the physical layout of a ClickHouse table. The query
// planner uses this metadata to make DETERMINISTIC optimizations — no
// guessing, no probabilistic heuristics. If the ORDER BY key starts with
// (metric_name, timestamp), we KNOW that a metric_name filter will trigger
// efficient index granule skipping. If the table is partitioned by
// toYYYYMM(timestamp), we KNOW that a time range filter will prune
// partitions.
// ──────────────────────────────────────────────────────────────────────────────

// TableMeta describes the physical properties of a ClickHouse table that
// affect query optimization. All optimizations derived from this metadata
// are deterministic — they are guaranteed correct based on the table DDL.
type TableMeta struct {
	// Engine is the MergeTree variant used by the table.
	Engine EngineType

	// OrderByKey is the table's ORDER BY (sorting key) columns, in order.
	// ClickHouse stores data sorted by these columns within each part.
	// Filters on leading ORDER BY columns enable index granule skipping.
	//
	// Example: []string{"metric_name", "timestamp"}
	OrderByKey []string

	// PartitionKey is the expression used for PARTITION BY.
	// Filters that match the partition expression enable partition pruning,
	// which can skip entire data parts without reading any granules.
	//
	// Example: "toYYYYMM(timestamp)"
	PartitionKey string

	// PrimaryKey overrides the ORDER BY key for the sparse primary index.
	// If empty, defaults to OrderByKey. Filters on PrimaryKey columns
	// enable index granule skipping (typically ~8192 rows per granule).
	PrimaryKey []string

	// TTL specifies the table's TTL policy. Zero means no TTL.
	// When set, the transpiler can skip adding explicit time range filters
	// for queries that fall within the TTL window.
	TTL time.Duration

	// SkipIndexes lists data-skipping indexes (minmax, set, bloom_filter,
	// ngrambf_v1, tokenbf_v1). The query planner uses these to decide
	// whether certain filter patterns can be optimized.
	SkipIndexes []SkipIndex

	// SamplingKey is the SAMPLE BY expression, if any. When present,
	// the transpiler can add SAMPLE clauses for high-cardinality queries.
	SamplingKey string
}

// SkipIndex describes a ClickHouse data-skipping index.
type SkipIndex struct {
	// Name is the index name.
	Name string
	// Type is the index type: "minmax", "set", "bloom_filter",
	// "ngrambf_v1", "tokenbf_v1".
	Type string
	// Column is the column the index is built on.
	Column string
	// Granularity is the index granularity.
	Granularity int
}

// ─── EFFECTIVE PRIMARY KEY ───────────────────────────────────────────────────

// EffectivePrimaryKey returns the primary key columns used for the sparse
// index. If PrimaryKey is explicitly set, it is returned; otherwise,
// the ORDER BY key is used (which is ClickHouse's default behavior).
func (m *TableMeta) EffectivePrimaryKey() []string {
	if len(m.PrimaryKey) > 0 {
		return m.PrimaryKey
	}
	return m.OrderByKey
}

// LeadingKeyColumn returns the first column in the effective primary key,
// or empty string if no key is defined. Filters on this column are the
// most efficient because they enable the primary index to skip the most
// granules.
func (m *TableMeta) LeadingKeyColumn() string {
	pk := m.EffectivePrimaryKey()
	if len(pk) > 0 {
		return pk[0]
	}
	return ""
}

// HasColumnInKey returns true if the given column appears anywhere in the
// effective primary key. Filters on key columns benefit from index granule
// skipping (in decreasing efficiency as the column position increases).
func (m *TableMeta) HasColumnInKey(column string) bool {
	for _, col := range m.EffectivePrimaryKey() {
		if col == column {
			return true
		}
	}
	return false
}

// ColumnKeyPosition returns the 0-based position of a column in the
// effective primary key, or -1 if not found. Columns at position 0
// offer the best granule skipping; columns at later positions are
// progressively less selective.
func (m *TableMeta) ColumnKeyPosition(column string) int {
	for i, col := range m.EffectivePrimaryKey() {
		if col == column {
			return i
		}
	}
	return -1
}

// HasSkipIndexOn returns true if a data-skipping index of the given type
// exists for the specified column.
func (m *TableMeta) HasSkipIndexOn(column, indexType string) bool {
	for _, idx := range m.SkipIndexes {
		if idx.Column == column && idx.Type == indexType {
			return true
		}
	}
	return false
}

// CanSample returns true if the table has a SAMPLE BY key defined,
// meaning the transpiler can use SAMPLE clauses.
func (m *TableMeta) CanSample() bool {
	return m.SamplingKey != ""
}

// ─── DEFAULT TABLE METADATA ──────────────────────────────────────────────────

// DefaultTableMeta returns a TableMeta matching the standard Prometheus
// metrics schema used by most ClickHouse time-series setups:
//
//	CREATE TABLE metrics (
//	    metric_name LowCardinality(String),
//	    labels Map(String, String),
//	    timestamp DateTime,
//	    value Float64
//	) ENGINE = MergeTree
//	PARTITION BY toYYYYMM(timestamp)
//	ORDER BY (metric_name, timestamp)
//	TTL timestamp + INTERVAL 90 DAY
func DefaultTableMeta() *TableMeta {
	return &TableMeta{
		Engine:       MergeTree,
		OrderByKey:   []string{"metric_name", "timestamp"},
		PartitionKey: "toYYYYMM(timestamp)",
		TTL:          90 * 24 * time.Hour, // 90 days
		SkipIndexes: []SkipIndex{
			{Name: "idx_metric_name", Type: "set", Column: "metric_name", Granularity: 1},
		},
	}
}

// ─── PREWHERE ANALYSIS ───────────────────────────────────────────────────────
//
// PREWHERE is ClickHouse's killer feature for time-series. It reads ONLY
// the columns referenced in the PREWHERE expression first, then only reads
// the remaining columns for rows that pass. For time-series data where
// most queries filter by metric_name and time range, this can skip 90%+
// of I/O.
//
// Rules for safe PREWHERE promotion:
//   1. The column must be filterable (part of WHERE)
//   2. PREWHERE is most effective on the leading ORDER BY / partition columns
//   3. Only move conditions that are highly selective to PREWHERE
//   4. Never PREWHERE on aliased expressions or aggregates
// ──────────────────────────────────────────────────────────────────────────────

// PrewhereCandidate identifies a WHERE condition that can be safely
// promoted to PREWHERE based on the table's physical layout.
type PrewhereCandidate struct {
	Condition string
	Column    string
	Priority  int // Lower = better (0 = leading key column)
}

// FindPrewhereCandidates inspects a list of WHERE conditions and returns
// those that are safe and beneficial to promote to PREWHERE, ranked by
// priority. The decision is fully deterministic based on table metadata.
func (m *TableMeta) FindPrewhereCandidates(conditions []string, schema *Schema) []PrewhereCandidate {
	var candidates []PrewhereCandidate

	for _, cond := range conditions {
		// Check if condition references the metric name column
		if strings.Contains(cond, schema.MetricNameColumn) {
			pos := m.ColumnKeyPosition(schema.MetricNameColumn)
			if pos >= 0 {
				candidates = append(candidates, PrewhereCandidate{
					Condition: cond,
					Column:    schema.MetricNameColumn,
					Priority:  pos,
				})
			}
		}

		// Check if condition references the timestamp column (partition pruning)
		if strings.Contains(cond, schema.TimestampColumn) {
			// Timestamp conditions are excellent PREWHERE candidates when
			// the table is partitioned by time
			if m.PartitionKey != "" && strings.Contains(m.PartitionKey, schema.TimestampColumn) {
				candidates = append(candidates, PrewhereCandidate{
					Condition: cond,
					Column:    schema.TimestampColumn,
					Priority:  0, // Partition pruning is highest priority
				})
			} else {
				pos := m.ColumnKeyPosition(schema.TimestampColumn)
				if pos >= 0 {
					candidates = append(candidates, PrewhereCandidate{
						Condition: cond,
						Column:    schema.TimestampColumn,
						Priority:  pos,
					})
				}
			}
		}
	}

	// Sort by priority (lower = better)
	for i := 0; i < len(candidates); i++ {
		for j := i + 1; j < len(candidates); j++ {
			if candidates[j].Priority < candidates[i].Priority {
				candidates[i], candidates[j] = candidates[j], candidates[i]
			}
		}
	}

	return candidates
}

// ─── QUERY HINTS ─────────────────────────────────────────────────────────────

// QueryHints represents deterministic optimization hints derived from
// comparing a query's access pattern against the table's physical layout.
type QueryHints struct {
	// UseFinal should be true when querying ReplacingMergeTree tables.
	UseFinal bool

	// PrewhereConditions are WHERE conditions to promote to PREWHERE.
	PrewhereConditions []string

	// RemainingWhereConditions are the WHERE conditions that stay in WHERE.
	RemainingWhereConditions []string

	// OrderByAligned is true when the query's ORDER BY matches (a prefix
	// of) the table's ORDER BY key, meaning no re-sort is needed.
	OrderByAligned bool

	// CanUsePartitionPruning is true when the query's time range filter
	// matches the table's PARTITION BY expression.
	CanUsePartitionPruning bool

	// SuggestSample is true when the table supports sampling and the
	// query operates on a large time range.
	SuggestSample bool
	SampleRatio   float64
}

// AnalyzeQuery produces deterministic QueryHints by comparing the query's
// filter columns against the table metadata. No heuristics — every
// optimization decision is derived from known table DDL properties.
func (m *TableMeta) AnalyzeQuery(conditions []string, schema *Schema, timeRange time.Duration) *QueryHints {
	hints := &QueryHints{}

	// 1. FINAL for ReplacingMergeTree
	hints.UseFinal = m.Engine.NeedsFinal()

	// 2. PREWHERE promotion
	candidates := m.FindPrewhereCandidates(conditions, schema)
	prewhereConds := make(map[string]bool)
	for _, c := range candidates {
		prewhereConds[c.Condition] = true
		hints.PrewhereConditions = append(hints.PrewhereConditions, c.Condition)
	}
	for _, cond := range conditions {
		if !prewhereConds[cond] {
			hints.RemainingWhereConditions = append(hints.RemainingWhereConditions, cond)
		}
	}

	// 3. Partition pruning — the timestamp filter will prune partitions
	//    if the table is partitioned by a time expression.
	if m.PartitionKey != "" {
		for _, cond := range conditions {
			if strings.Contains(cond, schema.TimestampColumn) {
				hints.CanUsePartitionPruning = true
				break
			}
		}
	}

	// 4. Sampling — suggest only when the table supports it and the
	//    time range is large (>24h) to avoid unnecessary overhead on
	//    small queries.
	if m.CanSample() && timeRange > 24*time.Hour {
		hints.SuggestSample = true
		// Scale sample ratio: 24h=1.0, 7d=0.5, 30d=0.1
		hours := timeRange.Hours()
		switch {
		case hours <= 24:
			hints.SampleRatio = 1.0
		case hours <= 168: // 7 days
			hints.SampleRatio = 0.5
		case hours <= 720: // 30 days
			hints.SampleRatio = 0.1
		default:
			hints.SampleRatio = 0.01
		}
	}

	return hints
}

// ─── TABLE REFERENCE ─────────────────────────────────────────────────────────

// TableRef returns the table reference SQL fragment, including FINAL
// if required by the engine type.
func (m *TableMeta) TableRef(tableName string) string {
	if m.Engine.NeedsFinal() {
		return fmt.Sprintf("%s FINAL", tableName)
	}
	return tableName
}

// TableRefWithSample returns the table reference with optional SAMPLE
// clause when the table supports sampling.
func (m *TableMeta) TableRefWithSample(tableName string, sampleRatio float64) string {
	ref := m.TableRef(tableName)
	if m.CanSample() && sampleRatio > 0 && sampleRatio < 1 {
		return fmt.Sprintf("%s SAMPLE %.4f", ref, sampleRatio)
	}
	return ref
}
