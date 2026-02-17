package transpiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/shinro/promql-transpiler/internal/cardinality"
	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/promapi"
)

// ─── CONFIGURATION ───────────────────────────────────────────────────────────

// Config holds transpiler configuration.
type Config struct {
	Schema             *clickhouse.Schema
	EnableOptimization bool
	EnableSampling     bool
	DefaultTimeRange   time.Duration
	// PrometheusURL is the base URL of Prometheus (e.g., "http://localhost:9090") - REQUIRED
	PrometheusURL string
}

// TimeRange represents the query evaluation time range.
type TimeRange struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// ─── TRANSPILER ──────────────────────────────────────────────────────────────

// Transpiler converts PromQL to ClickHouse SQL using Prometheus API for parsing.
type Transpiler struct {
	schema             *clickhouse.Schema
	qb                 *clickhouse.QueryBuilder
	optimizer          *cardinality.Optimizer
	enableOptimization bool
	enableSampling     bool
	timeRange          *TimeRange
	apiParser          *promapi.Parser // Prometheus API parser
}

// New creates a Transpiler with the given configuration.
func New(config *Config) *Transpiler {
	if config == nil {
		config = &Config{
			Schema:             clickhouse.DefaultSchema(),
			EnableOptimization: true,
			EnableSampling:     true,
			DefaultTimeRange:   time.Hour,
			PrometheusURL:      "http://localhost:9090",
		}
	}
	if config.Schema == nil {
		config.Schema = clickhouse.DefaultSchema()
	}
	if config.PrometheusURL == "" {
		config.PrometheusURL = "http://localhost:9090"
	}

	t := &Transpiler{
		schema:             config.Schema,
		qb:                 clickhouse.NewQueryBuilder(config.Schema),
		optimizer:          cardinality.NewOptimizer(),
		enableOptimization: config.EnableOptimization,
		enableSampling:     config.EnableSampling,
		apiParser:          promapi.NewParser(config.PrometheusURL),
	}

	return t
}

// SetTimeRange sets the evaluation time range.
func (t *Transpiler) SetTimeRange(start, end time.Time, step time.Duration) {
	t.timeRange = &TimeRange{Start: start, End: end, Step: step}
	t.qb.SetTimeRange(start, end, step)
}

// Transpile is the public entry point.
func (t *Transpiler) Transpile(promql string) (string, error) {
	// Parse using Prometheus API - returns JSON AST
	node, err := t.apiParser.Parse(promql)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}

	sql, err := t.transpileNode(node)
	if err != nil {
		return "", fmt.Errorf("transpile error: %w", err)
	}

	// Note: Optimization removed - can be re-added if needed for JSON AST

	return strings.TrimSpace(sql), nil
}

// ─── EXPRESSION DISPATCH ─────────────────────────────────────────────────────

func (t *Transpiler) transpileNode(node *promapi.ASTNode) (string, error) {
	if node == nil {
		return "", fmt.Errorf("nil node")
	}

	switch node.Type {
	case "vectorSelector":
		return t.transpileVectorSelectorNode(node)
	case "matrixSelector":
		return t.transpileMatrixSelectorNode(node)
	case "subqueryRange", "subquery":
		return t.transpileSubqueryRangeNode(node)
	case "aggregation":
		return t.transpileAggregationNode(node)
	case "call":
		return t.transpileCallNode(node)
	case "binaryExpr":
		return t.transpileBinaryExprNode(node)
	case "unaryExpr":
		return t.transpileUnaryExprNode(node)
	case "numberLiteral":
		return node.Val, nil
	case "stringLiteral":
		return fmt.Sprintf("'%s'", strings.ReplaceAll(node.Val, "'", "''")), nil
	case "parenExpr":
		return t.transpileNode(node.Expr)
	default:
		return "", fmt.Errorf("unsupported node type: %s", node.Type)
	}
}
