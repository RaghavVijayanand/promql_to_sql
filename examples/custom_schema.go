//go:build ignore

package main

import (
	"fmt"
	"log"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
	// =========================================================================
	// Example 1: Custom schema with engine-aware ClickHouse optimization
	// =========================================================================

	// Define the physical table layout — this drives PREWHERE, FINAL, and
	// ORDER BY optimizations.  Match the actual CREATE TABLE definition.
	tableMeta := &clickhouse.TableMeta{
		Engine: clickhouse.ReplacingMergeTree, // enables automatic FINAL injection
		OrderByKey: []string{"metric_name", "labels", "timestamp"},
		PartitionKey: "toYYYYMM(timestamp)",
		PrimaryKey:   []string{"metric_name", "labels"},
		SamplingKey:  "cityHash64(metric_name)", // enables SAMPLE clause
		SkipIndexes: []clickhouse.SkipIndex{
			{Name: "idx_metric", Expression: "metric_name", Type: "set(0)", Granularity: 1},
			{Name: "idx_labels", Expression: "labels", Type: "ngrambf_v1(3, 256, 2, 0)", Granularity: 1},
		},
	}

	customSchema := &clickhouse.Schema{
		TableName:        "custom_metrics",
		MetricNameColumn: "metric",
		LabelsColumn:     "tags",
		TimestampColumn:  "time",
		ValueColumn:      "val",
		CardinalityTable: "custom_metrics_cardinality",
		TableMeta:        tableMeta,
	}

	// Enable optimizations — the engine optimizer will:
	//   • Inject FINAL for ReplacingMergeTree (deduplication)
	//   • Promote equality filters to PREWHERE (skip-index alignment)
	//   • Align ORDER BY with the table's sorting key
	//   • Add ClickHouse SETTINGS for parallel execution
	config := &transpiler.Config{
		Schema:             customSchema,
		EnableOptimization: true,
		EnableSampling:     true,
	}

	t := transpiler.New(config)

	promql := `rate(http_requests_total{job="api"}[5m])`

	sql, err := t.Transpile(promql)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("ClickHouse Engine-Aware Example\n")
	fmt.Printf("===============================\n\n")
	fmt.Printf("PromQL:\n  %s\n\n", promql)
	fmt.Printf("ClickHouse SQL (ReplacingMergeTree + optimizations):\n%s\n\n", sql)

	// =========================================================================
	// Example 2: MergeTree with default layout (no FINAL, basic PREWHERE)
	// =========================================================================

	defaultSchema := clickhouse.DefaultSchema() // MergeTree, ORDER BY (metric_name, timestamp)
	configDefault := &transpiler.Config{
		Schema:             defaultSchema,
		EnableOptimization: true,
	}

	t2 := transpiler.New(configDefault)

	sql2, err := t2.Transpile(`avg_over_time(cpu_usage{instance="web-01"}[10m])`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	fmt.Printf("Default MergeTree Example\n")
	fmt.Printf("=========================\n\n")
	fmt.Printf("ClickHouse SQL:\n%s\n", sql2)

	// Note: The generated SQL will use ClickHouse-native constructs:
	//   • toDateTime() for timestamp comparisons
	//   • toUnixTimestamp() for epoch conversions
	//   • match() for regex label matchers
	//   • quantile(φ)() two-level aggregate syntax
	//   • lagInFrame() / leadInFrame() for window functions
	//   • stddevPop() / varPop() for population statistics
	//   • PREWHERE for skip-index-aligned filters
	//   • FINAL for ReplacingMergeTree deduplication
	//   • SAMPLE for probabilistic sampling
	//   • SETTINGS for query-level tuning
}
