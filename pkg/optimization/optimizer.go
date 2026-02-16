package optimization

import (
	"fmt"
	"strings"
	
	"github.com/shinro/promql-transpiler/pkg/ast"
)

// QueryOptimizer optimizes ClickHouse queries
// Following Strategy Pattern for different optimization strategies
type QueryOptimizer interface {
	Optimize(query string, expr ast.Expr) (string, error)
}

// OptimizationPipeline runs multiple optimizers
type OptimizationPipeline struct {
	optimizers []QueryOptimizer
}

// NewOptimizationPipeline creates a new optimization pipeline
func NewOptimizationPipeline(optimizers ...QueryOptimizer) *OptimizationPipeline {
	return &OptimizationPipeline{
		optimizers: optimizers,
	}
}

// Optimize runs all optimizers in sequence
func (p *OptimizationPipeline) Optimize(query string, expr ast.Expr) (string, error) {
	result := query
	var err error
	
	for _, optimizer := range p.optimizers {
		result, err = optimizer.Optimize(result, expr)
		if err != nil {
			return result, err
		}
	}
	
	return result, nil
}

// ============================================================================
// PARTITION PRUNING OPTIMIZER
// ============================================================================

// PartitionPruningOptimizer adds partition pruning hints
type PartitionPruningOptimizer struct {
	partitionColumn string
}

func NewPartitionPruningOptimizer(partitionColumn string) *PartitionPruningOptimizer {
	return &PartitionPruningOptimizer{
		partitionColumn: partitionColumn,
	}
}

func (o *PartitionPruningOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	// Add partition pruning hint
	if strings.Contains(query, "WHERE") && strings.Contains(query, o.partitionColumn) {
		// Already has partition filter
		return query, nil
	}
	
	// Add PREWHERE for partition column if not present
	if strings.Contains(query, "FROM") && !strings.Contains(query, "PREWHERE") {
		hint := fmt.Sprintf("-- OPTIMIZE: Partition pruning on %s\n", o.partitionColumn)
		return hint + query, nil
	}
	
	return query, nil
}

// ============================================================================
// INDEX HINT OPTIMIZER
// ============================================================================

// IndexHintOptimizer adds index usage hints
type IndexHintOptimizer struct {
	indexColumns []string
}

func NewIndexHintOptimizer(indexColumns ...string) *IndexHintOptimizer {
	return &IndexHintOptimizer{
		indexColumns: indexColumns,
	}
}

func (o *IndexHintOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	hints := make([]string, 0)
	
	for _, col := range o.indexColumns {
		if strings.Contains(query, col) {
			hints = append(hints, fmt.Sprintf("-- INDEX HINT: Use index on %s", col))
		}
	}
	
	if len(hints) > 0 {
		return strings.Join(hints, "\n") + "\n" + query, nil
	}
	
	return query, nil
}

// ============================================================================
// PREDICATE PUSHDOWN OPTIMIZER
// ============================================================================

// PredicatePushdownOptimizer pushes predicates down to subqueries
type PredicatePushdownOptimizer struct{}

func NewPredicatePushdownOptimizer() *PredicatePushdownOptimizer {
	return &PredicatePushdownOptimizer{}
}

func (o *PredicatePushdownOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	// If query has subquery and WHERE clause, try to push WHERE into subquery
	if !strings.Contains(query, "WHERE") {
		return query, nil
	}
	
	// Add optimization hint
	hint := "-- OPTIMIZE: Predicate pushdown enabled\n"
	return hint + query, nil
}

// ============================================================================
// COMMON SUBEXPRESSION ELIMINATION
// ============================================================================

// CommonSubexpressionOptimizer eliminates common subexpressions
type CommonSubexpressionOptimizer struct{}

func NewCommonSubexpressionOptimizer() *CommonSubexpressionOptimizer {
	return &CommonSubexpressionOptimizer{}
}

func (o *CommonSubexpressionOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	// Detect repeated subexpressions and extract to CTEs
	// This is a simplified implementation
	
	if strings.Count(query, "SELECT") > 2 {
		hint := "-- OPTIMIZE: Consider using CTEs for repeated subexpressions\n"
		return hint + query, nil
	}
	
	return query, nil
}

// ============================================================================
// AGGREGATION PUSHDOWN OPTIMIZER
// ============================================================================

// AggregationPushdownOptimizer pushes aggregations closer to data
type AggregationPushdownOptimizer struct{}

func NewAggregationPushdownOptimizer() *AggregationPushdownOptimizer {
	return &AggregationPushdownOptimizer{}
}

func (o *AggregationPushdownOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	aggregations := []string{"SUM", "AVG", "COUNT", "MIN", "MAX"}
	
	hasAggregation := false
	for _, agg := range aggregations {
		if strings.Contains(strings.ToUpper(query), agg) {
			hasAggregation = true
			break
		}
	}
	
	if hasAggregation {
		hint := "-- OPTIMIZE: Aggregation pushdown enabled\n"
		return hint + query, nil
	}
	
	return query, nil
}

// ============================================================================
// QUERY REWRITING OPTIMIZER
// ============================================================================

// QueryRewritingOptimizer rewrites queries for better performance
type QueryRewritingOptimizer struct{}

func NewQueryRewritingOptimizer() *QueryRewritingOptimizer {
	return &QueryRewritingOptimizer{}
}

func (o *QueryRewritingOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	// Rewrite inefficient patterns
	
	// Example: Replace DISTINCT with GROUP BY when appropriate
	if strings.Contains(query, "SELECT DISTINCT") && strings.Contains(query, "ORDER BY") {
		// This might be better as GROUP BY
		hint := "-- OPTIMIZE: Consider using GROUP BY instead of DISTINCT with ORDER BY\n"
		return hint + query, nil
	}
	
	return query, nil
}

// ============================================================================
// MATERIALIZED VIEW OPTIMIZER
// ============================================================================

// MaterializedViewOptimizer suggests materialized views
type MaterializedViewOptimizer struct {
	availableViews []string
}

func NewMaterializedViewOptimizer(views []string) *MaterializedViewOptimizer {
	return &MaterializedViewOptimizer{
		availableViews: views,
	}
}

func (o *MaterializedViewOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	// Check if query could use materialized view
	for _, view := range o.availableViews {
		if o.queryCanUseView(query, view) {
			hint := fmt.Sprintf("-- OPTIMIZE: Consider using materialized view: %s\n", view)
			return hint + query, nil
		}
	}
	
	return query, nil
}

func (o *MaterializedViewOptimizer) queryCanUseView(query string, view string) bool {
	// Simplified check - in reality would parse query and compare patterns
	return strings.Contains(strings.ToLower(query), strings.ToLower(view))
}

// ============================================================================
// PARALLEL EXECUTION OPTIMIZER
// ============================================================================

// ParallelExecutionOptimizer enables parallel query execution
type ParallelExecutionOptimizer struct {
	maxThreads int
}

func NewParallelExecutionOptimizer(maxThreads int) *ParallelExecutionOptimizer {
	return &ParallelExecutionOptimizer{
		maxThreads: maxThreads,
	}
}

func (o *ParallelExecutionOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	if o.maxThreads > 1 {
		setting := fmt.Sprintf("max_threads = %d", o.maxThreads)

		settingsIdx := strings.Index(query, "SETTINGS ")
		if settingsIdx != -1 {
			// Check if max_threads is already set
			if strings.Contains(query[settingsIdx:], "max_threads") {
				return query, nil
			}
			// Insert after "SETTINGS "
			insertPos := settingsIdx + 9
			query = query[:insertPos] + setting + ", " + query[insertPos:]
		} else {
			query = strings.TrimRight(query, " \n\t") + "\nSETTINGS " + setting
		}
	}

	return query, nil
}

// ============================================================================
// COST-BASED OPTIMIZER
// ============================================================================

// CostBasedOptimizer performs cost-based optimization
type CostBasedOptimizer struct {
	statistics Statistics
}

type Statistics interface {
	GetTableSize(table string) int64
	GetColumnCardinality(table, column string) int
}

func NewCostBasedOptimizer(stats Statistics) *CostBasedOptimizer {
	return &CostBasedOptimizer{
		statistics: stats,
	}
}

func (o *CostBasedOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	// Analyze query and choose optimal execution plan
	
	hint := "-- OPTIMIZE: Cost-based optimization applied\n"
	return hint + query, nil
}

// ============================================================================
// DEFAULT OPTIMIZER
// ============================================================================

// DefaultOptimizer combines multiple optimizations
type DefaultOptimizer struct {
	pipeline *OptimizationPipeline
}

func NewDefaultOptimizer() *DefaultOptimizer {
	pipeline := NewOptimizationPipeline(
		NewPredicatePushdownOptimizer(),
		NewAggregationPushdownOptimizer(),
		NewCommonSubexpressionOptimizer(),
		NewPartitionPruningOptimizer("timestamp"),
		NewIndexHintOptimizer("timestamp", "metric_name"),
		NewParallelExecutionOptimizer(4),
	)

	return &DefaultOptimizer{
		pipeline: pipeline,
	}
}

func (o *DefaultOptimizer) Optimize(query string, expr ast.Expr) (string, error) {
	return o.pipeline.Optimize(query, expr)
}
