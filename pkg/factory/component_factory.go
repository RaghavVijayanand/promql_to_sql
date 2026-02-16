package factory

import (
	"fmt"
	
	"github.com/shinro/promql-transpiler/pkg/ast"
	"github.com/shinro/promql-transpiler/pkg/builder"
)

// ComponentFactory creates transpiler components
// Following Factory Pattern and Abstract Factory Pattern
// Following Dependency Inversion - depends on abstractions
type ComponentFactory interface {
	CreateSQLBuilder() builder.SQLBuilder
	CreateExpressionHandler(exprType string) (ExpressionHandler, error)
	CreateAggregationHandler(aggType string) (AggregationHandler, error)
	CreateFunctionHandler(funcName string) (FunctionHandler, error)
}

// ExpressionHandler handles different types of expressions
// Following Strategy Pattern
type ExpressionHandler interface {
	Handle(expr ast.Expr, ctx *TranspilationContext) (string, error)
	CanHandle(expr ast.Expr) bool
}

// AggregationHandler handles aggregation operations
type AggregationHandler interface {
	Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error)
	GetClickHouseFunction() string
}

// FunctionHandler handles PromQL functions
type FunctionHandler interface {
	Handle(call *ast.Call, ctx *TranspilationContext) (string, error)
	GetRequiredArgs() int
	GetOptionalArgs() int
}

// TranspilationContext holds context during transpilation
type TranspilationContext struct {
	SchemaName    string
	TableName     string
	TimeColumn    string
	ValueColumn   string
	LabelColumns  []string
	StartTime     int64
	EndTime       int64
	Step          int64
	Builder       builder.SQLBuilder
}

// ClickHouseComponentFactory creates components for ClickHouse
type ClickHouseComponentFactory struct {
	exprHandlers map[string]ExpressionHandler
	aggHandlers  map[string]AggregationHandler
	funcHandlers map[string]FunctionHandler
}

// NewClickHouseComponentFactory creates a new ClickHouse factory
func NewClickHouseComponentFactory() ComponentFactory {
	factory := &ClickHouseComponentFactory{
		exprHandlers: make(map[string]ExpressionHandler),
		aggHandlers:  make(map[string]AggregationHandler),
		funcHandlers: make(map[string]FunctionHandler),
	}
	
	// Register default handlers
	factory.registerDefaultHandlers()
	
	return factory
}

// CreateSQLBuilder creates a SQL builder
func (f *ClickHouseComponentFactory) CreateSQLBuilder() builder.SQLBuilder {
	return builder.NewClickHouseSQLBuilder()
}

// CreateExpressionHandler creates an expression handler
func (f *ClickHouseComponentFactory) CreateExpressionHandler(exprType string) (ExpressionHandler, error) {
	handler, exists := f.exprHandlers[exprType]
	if !exists {
		return nil, fmt.Errorf("no handler registered for expression type: %s", exprType)
	}
	return handler, nil
}

// CreateAggregationHandler creates an aggregation handler
func (f *ClickHouseComponentFactory) CreateAggregationHandler(aggType string) (AggregationHandler, error) {
	handler, exists := f.aggHandlers[aggType]
	if !exists {
		return nil, fmt.Errorf("no handler registered for aggregation: %s", aggType)
	}
	return handler, nil
}

// CreateFunctionHandler creates a function handler
func (f *ClickHouseComponentFactory) CreateFunctionHandler(funcName string) (FunctionHandler, error) {
	handler, exists := f.funcHandlers[funcName]
	if !exists {
		return nil, fmt.Errorf("no handler registered for function: %s", funcName)
	}
	return handler, nil
}

// registerDefaultHandlers registers all default handlers
func (f *ClickHouseComponentFactory) registerDefaultHandlers() {
	// Expression handlers
	f.exprHandlers["vector"] = &VectorSelectorHandler{}
	f.exprHandlers["matrix"] = &MatrixSelectorHandler{}
	f.exprHandlers["binary"] = &BinaryExprHandler{}
	f.exprHandlers["unary"] = &UnaryExprHandler{}
	f.exprHandlers["number"] = &NumberLiteralHandler{}
	f.exprHandlers["string"] = &StringLiteralHandler{}
	
	// Aggregation handlers
	f.aggHandlers["sum"] = &SumAggregationHandler{}
	f.aggHandlers["avg"] = &AvgAggregationHandler{}
	f.aggHandlers["count"] = &CountAggregationHandler{}
	f.aggHandlers["min"] = &MinAggregationHandler{}
	f.aggHandlers["max"] = &MaxAggregationHandler{}
	f.aggHandlers["stddev"] = &StddevAggregationHandler{}
	f.aggHandlers["stdvar"] = &StdvarAggregationHandler{}
	f.aggHandlers["topk"] = &TopKAggregationHandler{}
	f.aggHandlers["bottomk"] = &BottomKAggregationHandler{}
	f.aggHandlers["quantile"] = &QuantileAggregationHandler{}
	f.aggHandlers["count_values"] = &CountValuesAggregationHandler{}
	
	// Function handlers - Prometheus standard functions
	f.funcHandlers["rate"] = &RateFunctionHandler{}
	f.funcHandlers["irate"] = &IRateFunctionHandler{}
	f.funcHandlers["increase"] = &IncreaseFunctionHandler{}
	f.funcHandlers["delta"] = &DeltaFunctionHandler{}
	f.funcHandlers["idelta"] = &IDeltaFunctionHandler{}
	f.funcHandlers["deriv"] = &DerivFunctionHandler{}
	f.funcHandlers["predict_linear"] = &PredictLinearFunctionHandler{}
	f.funcHandlers["histogram_quantile"] = &HistogramQuantileFunctionHandler{}
	
	// Time functions
	f.funcHandlers["time"] = &TimeFunctionHandler{}
	f.funcHandlers["timestamp"] = &TimestampFunctionHandler{}
	
	// Math functions
	f.funcHandlers["abs"] = &AbsFunctionHandler{}
	f.funcHandlers["ceil"] = &CeilFunctionHandler{}
	f.funcHandlers["floor"] = &FloorFunctionHandler{}
	f.funcHandlers["round"] = &RoundFunctionHandler{}
	f.funcHandlers["sqrt"] = &SqrtFunctionHandler{}
	f.funcHandlers["exp"] = &ExpFunctionHandler{}
	f.funcHandlers["ln"] = &LnFunctionHandler{}
	f.funcHandlers["log2"] = &Log2FunctionHandler{}
	f.funcHandlers["log10"] = &Log10FunctionHandler{}
	
	// Aggregation over time functions
	f.funcHandlers["avg_over_time"] = &AvgOverTimeFunctionHandler{}
	f.funcHandlers["min_over_time"] = &MinOverTimeFunctionHandler{}
	f.funcHandlers["max_over_time"] = &MaxOverTimeFunctionHandler{}
	f.funcHandlers["sum_over_time"] = &SumOverTimeFunctionHandler{}
	f.funcHandlers["count_over_time"] = &CountOverTimeFunctionHandler{}
	f.funcHandlers["quantile_over_time"] = &QuantileOverTimeFunctionHandler{}
	f.funcHandlers["stddev_over_time"] = &StddevOverTimeFunctionHandler{}
	f.funcHandlers["stdvar_over_time"] = &StdvarOverTimeFunctionHandler{}
	
	// Label manipulation
	f.funcHandlers["label_replace"] = &LabelReplaceFunctionHandler{}
	f.funcHandlers["label_join"] = &LabelJoinFunctionHandler{}
	
	// Vector functions
	f.funcHandlers["vector"] = &VectorFunctionHandler{}
	f.funcHandlers["scalar"] = &ScalarFunctionHandler{}
	
	// Sorting functions
	f.funcHandlers["sort"] = &SortFunctionHandler{}
	f.funcHandlers["sort_desc"] = &SortDescFunctionHandler{}
	
	// Clamp functions
	f.funcHandlers["clamp"] = &ClampFunctionHandler{}
	f.funcHandlers["clamp_max"] = &ClampMaxFunctionHandler{}
	f.funcHandlers["clamp_min"] = &ClampMinFunctionHandler{}
	
	// Additional functions
	f.funcHandlers["absent"] = &AbsentFunctionHandler{}
	f.funcHandlers["absent_over_time"] = &AbsentOverTimeFunctionHandler{}
	f.funcHandlers["changes"] = &ChangesFunctionHandler{}
	f.funcHandlers["resets"] = &ResetsFunctionHandler{}
	
	// Alertmanager specific functions
	f.funcHandlers["ALERTS"] = &AlertsFunctionHandler{}
	f.funcHandlers["ALERTS_FOR_STATE"] = &AlertsForStateFunctionHandler{}

	// Date/time functions (ClickHouse-native: toDayOfMonth, toDayOfWeek, etc.)
	registerDateTimeFunctions(f)
}

// VectorSelectorHandler handles vector selectors
type VectorSelectorHandler struct{}

func (h *VectorSelectorHandler) Handle(expr ast.Expr, ctx *TranspilationContext) (string, error) {
	vs, ok := expr.(*ast.VectorSelector)
	if !ok {
		return "", fmt.Errorf("expected VectorSelector, got %T", expr)
	}
	
	builder := ctx.Builder
	builder.Reset()
	
	// Build label conditions
	conditions := []string{}
	if vs.MetricSelector != nil && vs.MetricSelector.Name != "" {
		// ClickHouse uses metric_name column, not Prometheus __name__
		conditions = append(conditions, fmt.Sprintf("metric_name = '%s'", vs.MetricSelector.Name))
	}
	
	if vs.MetricSelector != nil {
		for _, matcher := range vs.MetricSelector.Matchers {
			condition := buildMatcherCondition(matcher)
			if condition != "" {
				conditions = append(conditions, condition)
			}
		}
	}
	
	// Select value and labels
	builder.Select(ctx.ValueColumn)
	for _, label := range ctx.LabelColumns {
		builder.Select(label)
	}
	
	builder.From(ctx.TableName).
		Where(buildTimeRangeCondition(ctx))
	
	for _, cond := range conditions {
		builder.And(cond)
	}
	
	return builder.Build(), nil
}

func (h *VectorSelectorHandler) CanHandle(expr ast.Expr) bool {
	_, ok := expr.(*ast.VectorSelector)
	return ok
}

// MatrixSelectorHandler handles matrix selectors
type MatrixSelectorHandler struct{}

func (h *MatrixSelectorHandler) Handle(expr ast.Expr, ctx *TranspilationContext) (string, error) {
	ms, ok := expr.(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("expected MatrixSelector, got %T", expr)
	}
	
	// Matrix selector includes a time range
	// This would be similar to vector selector but with adjusted time range
	builder := ctx.Builder
	builder.Reset()
	
	// Build label conditions from vector selector
	conditions := []string{}
	if ms.VectorSelector != nil && ms.VectorSelector.MetricSelector != nil && ms.VectorSelector.MetricSelector.Name != "" {
		// ClickHouse uses metric_name column, not Prometheus __name__
		conditions = append(conditions, fmt.Sprintf("metric_name = '%s'", ms.VectorSelector.MetricSelector.Name))
	}
	
	if ms.VectorSelector != nil && ms.VectorSelector.MetricSelector != nil {
		for _, matcher := range ms.VectorSelector.MetricSelector.Matchers {
			condition := buildMatcherCondition(matcher)
			if condition != "" {
				conditions = append(conditions, condition)
			}
		}
	}
	
	builder.Select(ctx.ValueColumn, ctx.TimeColumn)
	for _, label := range ctx.LabelColumns {
		builder.Select(label)
	}
	
	// Adjust time range based on range duration
	rangeStart := ctx.StartTime - int64(ms.Range.Seconds())
	// ClickHouse: wrap Unix timestamps with toDateTime() for proper DateTime comparison
	builder.From(ctx.TableName).
		Where(fmt.Sprintf("%s >= toDateTime(%d) AND %s <= toDateTime(%d)", ctx.TimeColumn, rangeStart, ctx.TimeColumn, ctx.EndTime))
	
	for _, cond := range conditions {
		builder.And(cond)
	}
	
	return builder.Build(), nil
}

func (h *MatrixSelectorHandler) CanHandle(expr ast.Expr) bool {
	_, ok := expr.(*ast.MatrixSelector)
	return ok
}

// BinaryExprHandler handles binary expressions
type BinaryExprHandler struct{}

func (h *BinaryExprHandler) Handle(expr ast.Expr, ctx *TranspilationContext) (string, error) {
	be, ok := expr.(*ast.BinaryExpr)
	if !ok {
		return "", fmt.Errorf("expected BinaryExpr, got %T", expr)
	}
	
	// This is a placeholder - actual implementation would need to recursively handle left and right
	_ = be
	return "", fmt.Errorf("BinaryExprHandler not fully implemented")
}

func (h *BinaryExprHandler) CanHandle(expr ast.Expr) bool {
	_, ok := expr.(*ast.BinaryExpr)
	return ok
}

// UnaryExprHandler handles unary expressions
type UnaryExprHandler struct{}

func (h *UnaryExprHandler) Handle(expr ast.Expr, ctx *TranspilationContext) (string, error) {
	ue, ok := expr.(*ast.UnaryExpr)
	if !ok {
		return "", fmt.Errorf("expected UnaryExpr, got %T", expr)
	}
	
	_ = ue
	return "", fmt.Errorf("UnaryExprHandler not fully implemented")
}

func (h *UnaryExprHandler) CanHandle(expr ast.Expr) bool {
	_, ok := expr.(*ast.UnaryExpr)
	return ok
}

// NumberLiteralHandler handles number literals
type NumberLiteralHandler struct{}

func (h *NumberLiteralHandler) Handle(expr ast.Expr, ctx *TranspilationContext) (string, error) {
	nl, ok := expr.(*ast.NumberLiteral)
	if !ok {
		return "", fmt.Errorf("expected NumberLiteral, got %T", expr)
	}
	
	return fmt.Sprintf("%f", nl.Value), nil
}

func (h *NumberLiteralHandler) CanHandle(expr ast.Expr) bool {
	_, ok := expr.(*ast.NumberLiteral)
	return ok
}

// StringLiteralHandler handles string literals
type StringLiteralHandler struct{}

func (h *StringLiteralHandler) Handle(expr ast.Expr, ctx *TranspilationContext) (string, error) {
	sl, ok := expr.(*ast.StringLiteral)
	if !ok {
		return "", fmt.Errorf("expected StringLiteral, got %T", expr)
	}
	
	return fmt.Sprintf("'%s'", sl.Value), nil
}

func (h *StringLiteralHandler) CanHandle(expr ast.Expr) bool {
	_, ok := expr.(*ast.StringLiteral)
	return ok
}

// Helper functions
func buildMatcherCondition(matcher *ast.LabelMatcher) string {
	if matcher == nil {
		return ""
	}
	
	switch matcher.Operator {
	case ast.MatchEqual:
		return fmt.Sprintf("%s = '%s'", matcher.Name, matcher.Value)
	case ast.MatchNotEqual:
		return fmt.Sprintf("%s != '%s'", matcher.Name, matcher.Value)
	case ast.MatchRegexp:
		return fmt.Sprintf("match(%s, '%s')", matcher.Name, matcher.Value)
	case ast.MatchNotRegexp:
		return fmt.Sprintf("NOT match(%s, '%s')", matcher.Name, matcher.Value)
	default:
		return ""
	}
}

func buildTimeRangeCondition(ctx *TranspilationContext) string {
	// ClickHouse: wrap Unix timestamps with toDateTime() for proper DateTime comparison
	return fmt.Sprintf("%s >= toDateTime(%d) AND %s <= toDateTime(%d)", 
		ctx.TimeColumn, ctx.StartTime, 
		ctx.TimeColumn, ctx.EndTime)
}
