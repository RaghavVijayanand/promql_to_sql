package strategy

import (
	"github.com/shinro/promql-transpiler/pkg/ast"
)

// TranspilationStrategy defines the interface for different transpilation strategies
// Following Interface Segregation Principle - specific, focused interface
type TranspilationStrategy interface {
	// Transpile converts an AST expression to SQL
	Transpile(expr ast.Expr) (string, error)
	
	// CanHandle checks if this strategy can handle the given expression
	CanHandle(expr ast.Expr) bool
	
	// GetPriority returns the priority of this strategy (higher = more preferred)
	GetPriority() int
}

// ExpressionTranspiler defines the contract for expression transpilation
// Following Dependency Inversion Principle - depend on abstraction
type ExpressionTranspiler interface {
	TranspileExpression(expr ast.Expr) (string, error)
}

// FunctionStrategy handles function call transpilation
type FunctionStrategy interface {
	TranspilationStrategy
	SupportedFunctions() []string
}

// AggregationStrategy handles aggregation transpilation
type AggregationStrategy interface {
	TranspilationStrategy
	SupportedAggregations() []string
}

// BinaryOpStrategy handles binary operations
type BinaryOpStrategy interface {
	TranspilationStrategy
	SupportedOperators() []string
}

// Context holds transpilation context information
type Context struct {
	Schema       SchemaProvider
	TimeRange    TimeRangeProvider
	Optimizer    OptimizerProvider
	VariableMap  map[string]string // For Grafana variables
	AlertContext *AlertContext     // For Alertmanager
}

// SchemaProvider provides schema information
type SchemaProvider interface {
	GetTableName() string
	GetMetricColumn() string
	GetLabelsColumn() string
	GetTimestampColumn() string
	GetValueColumn() string
}

// TimeRangeProvider provides time range information
type TimeRangeProvider interface {
	GetStartTime() string
	GetEndTime() string
	GetStep() string
}

// OptimizerProvider provides optimization capabilities
type OptimizerProvider interface {
	ShouldSample() bool
	GetSampleRatio() float64
	SuggestIndexHints() []string
}

// AlertContext holds Alertmanager-specific context
type AlertContext struct {
	ForDuration   string
	Labels        map[string]string
	Annotations   map[string]string
	KeepFiringFor string
}

// StrategyType represents different strategy types
type StrategyType int

const (
	StrategyPrometheus StrategyType = iota
	StrategyAlertmanager
	StrategyGrafana
)

// TypedStrategy extends TranspilationStrategy with a type identifier
// so the registry can filter by strategy type.
type TypedStrategy interface {
	TranspilationStrategy
	GetType() StrategyType
}

// StrategyRegistry manages available strategies
type StrategyRegistry interface {
	Register(strategy TranspilationStrategy)
	GetStrategy(expr ast.Expr) (TranspilationStrategy, error)
	GetStrategiesByType(strategyType StrategyType) []TranspilationStrategy
}
