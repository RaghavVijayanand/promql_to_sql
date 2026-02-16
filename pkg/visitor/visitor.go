package visitor

import (
	"fmt"
	"strings"

	"github.com/shinro/promql-transpiler/pkg/ast"
)

// Visitor defines the interface for AST visitors
// Following Visitor Pattern for clean AST traversal
// Following Interface Segregation Principle - focused interface
type Visitor interface {
	VisitVectorSelector(v *ast.VectorSelector) (interface{}, error)
	VisitMatrixSelector(m *ast.MatrixSelector) (interface{}, error)
	VisitAggregateExpr(a *ast.AggregateExpr) (interface{}, error)
	VisitCall(c *ast.Call) (interface{}, error)
	VisitBinaryExpr(b *ast.BinaryExpr) (interface{}, error)
	VisitUnaryExpr(u *ast.UnaryExpr) (interface{}, error)
	VisitNumberLiteral(n *ast.NumberLiteral) (interface{}, error)
	VisitStringLiteral(s *ast.StringLiteral) (interface{}, error)
	VisitParenExpr(p *ast.ParenExpr) (interface{}, error)
}

// SQLGeneratorVisitor generates SQL from AST nodes
// Following Single Responsibility Principle - only generates SQL
type SQLGeneratorVisitor struct {
	schema        SchemaInfo
	sqlBuilder    SQLBuilder
	contextStack  []VisitorContext
	errorHandler  ErrorHandler
}

// SchemaInfo provides schema information to the visitor
type SchemaInfo interface {
	GetTableName() string
	GetMetricColumn() string
	GetLabelsColumn() string
	GetTimestampColumn() string
	GetValueColumn() string
}

// SQLBuilder builds SQL components
type SQLBuilder interface {
	BuildSelect(columns ...string) string
	BuildFrom(table string) string
	BuildWhere(conditions ...string) string
	BuildGroupBy(columns ...string) string
	BuildOrderBy(columns ...string) string
	BuildWindow(name, partition, orderBy string) string
}

// VisitorContext holds context during traversal
type VisitorContext struct {
	ParentExpr ast.Expr
	Depth      int
	Metadata   map[string]interface{}
}

// ErrorHandler handles errors during visitation
type ErrorHandler interface {
	HandleError(err error, node ast.Expr) error
	GetErrors() []error
	HasErrors() bool
}

// NewSQLGeneratorVisitor creates a new SQL generator visitor
func NewSQLGeneratorVisitor(schema SchemaInfo, builder SQLBuilder) *SQLGeneratorVisitor {
	return &SQLGeneratorVisitor{
		schema:       schema,
		sqlBuilder:   builder,
		contextStack: make([]VisitorContext, 0),
		errorHandler: NewDefaultErrorHandler(),
	}
}

// Accept allows expressions to accept visitors
// This would ideally be added to ast.Expr interface
func Accept(expr ast.Expr, visitor Visitor) (interface{}, error) {
	switch e := expr.(type) {
	case *ast.VectorSelector:
		return visitor.VisitVectorSelector(e)
	case *ast.MatrixSelector:
		return visitor.VisitMatrixSelector(e)
	case *ast.AggregateExpr:
		return visitor.VisitAggregateExpr(e)
	case *ast.Call:
		return visitor.VisitCall(e)
	case *ast.BinaryExpr:
		return visitor.VisitBinaryExpr(e)
	case *ast.UnaryExpr:
		return visitor.VisitUnaryExpr(e)
	case *ast.NumberLiteral:
		return visitor.VisitNumberLiteral(e)
	case *ast.StringLiteral:
		return visitor.VisitStringLiteral(e)
	case *ast.ParenExpr:
		return visitor.VisitParenExpr(e)
	default:
		return nil, &UnsupportedExpressionError{ExprType: expr}
	}
}

// VisitVectorSelector visits a vector selector node
func (v *SQLGeneratorVisitor) VisitVectorSelector(vs *ast.VectorSelector) (interface{}, error) {
	v.pushContext(VisitorContext{ParentExpr: vs, Depth: v.getCurrentDepth() + 1})
	defer v.popContext()
	
	var conditions []string
	
	// Build metric name condition
	if vs.MetricSelector.Name != "" {
		conditions = append(conditions, 
			fmt.Sprintf("%s = '%s'", v.schema.GetMetricColumn(), vs.MetricSelector.Name))
	}
	
	// Build label conditions
	for _, matcher := range vs.MetricSelector.Matchers {
		condition, err := v.buildLabelCondition(matcher)
		if err != nil {
			return nil, v.errorHandler.HandleError(err, vs)
		}
		conditions = append(conditions, condition)
	}
	
	// Build complete query
	sql := fmt.Sprintf("%s\n%s\n%s",
		v.sqlBuilder.BuildSelect(v.schema.GetTimestampColumn(), v.schema.GetValueColumn(), v.schema.GetLabelsColumn()),
		v.sqlBuilder.BuildFrom(v.schema.GetTableName()),
		v.sqlBuilder.BuildWhere(conditions...))
	
	return sql, nil
}

// VisitMatrixSelector visits a matrix selector node
func (v *SQLGeneratorVisitor) VisitMatrixSelector(ms *ast.MatrixSelector) (interface{}, error) {
	v.pushContext(VisitorContext{ParentExpr: ms, Depth: v.getCurrentDepth() + 1})
	defer v.popContext()
	
	// Visit the underlying vector selector
	vectorSQL, err := v.VisitVectorSelector(ms.VectorSelector)
	if err != nil {
		return nil, v.errorHandler.HandleError(err, ms)
	}
	
	// Add time range filter based on matrix range
	// This is a simplified version
	return vectorSQL, nil
}

// VisitAggregateExpr visits an aggregation expression
func (v *SQLGeneratorVisitor) VisitAggregateExpr(ae *ast.AggregateExpr) (interface{}, error) {
	v.pushContext(VisitorContext{ParentExpr: ae, Depth: v.getCurrentDepth() + 1})
	defer v.popContext()
	
	// Visit inner expression
	innerResult, err := Accept(ae.Expr, v)
	if err != nil {
		return nil, v.errorHandler.HandleError(err, ae)
	}
	
	// Safe type assertion
	innerSQL, ok := innerResult.(string)
	if !ok {
		return nil, v.errorHandler.HandleError(fmt.Errorf("aggregate inner expression did not produce a string (got %T)", innerResult), ae)
	}
	
	// Build aggregation
	aggFunc := v.getAggregationFunction(ae.Op)
	groupBy := v.buildGroupByClause(ae.Grouping)
	
	sql := fmt.Sprintf("WITH inner_query AS (\n%s\n)\nSELECT %s(%s) AS value FROM inner_query %s",
		innerSQL, aggFunc, v.schema.GetValueColumn(), groupBy)
	
	return sql, nil
}

// VisitCall visits a function call
func (v *SQLGeneratorVisitor) VisitCall(c *ast.Call) (interface{}, error) {
	v.pushContext(VisitorContext{ParentExpr: c, Depth: v.getCurrentDepth() + 1})
	defer v.popContext()
	
	// Delegate to function-specific visitor
	return v.visitFunctionCall(c)
}

// VisitBinaryExpr visits a binary expression
func (v *SQLGeneratorVisitor) VisitBinaryExpr(be *ast.BinaryExpr) (interface{}, error) {
	v.pushContext(VisitorContext{ParentExpr: be, Depth: v.getCurrentDepth() + 1})
	defer v.popContext()
	
	leftResult, err := Accept(be.Left, v)
	if err != nil {
		return nil, v.errorHandler.HandleError(err, be)
	}
	
	rightResult, err := Accept(be.Right, v)
	if err != nil {
		return nil, v.errorHandler.HandleError(err, be)
	}
	
	// Safe type assertion to prevent panics
	leftSQL, ok := leftResult.(string)
	if !ok {
		return nil, v.errorHandler.HandleError(fmt.Errorf("left operand did not produce a string (got %T)", leftResult), be)
	}
	rightSQL, ok := rightResult.(string)
	if !ok {
		return nil, v.errorHandler.HandleError(fmt.Errorf("right operand did not produce a string (got %T)", rightResult), be)
	}
	
	// Build binary operation
	return v.buildBinaryOperation(leftSQL, be.Operator, rightSQL), nil
}

// VisitUnaryExpr visits a unary expression
func (v *SQLGeneratorVisitor) VisitUnaryExpr(ue *ast.UnaryExpr) (interface{}, error) {
	v.pushContext(VisitorContext{ParentExpr: ue, Depth: v.getCurrentDepth() + 1})
	defer v.popContext()
	
	innerResult, err := Accept(ue.Expr, v)
	if err != nil {
		return nil, v.errorHandler.HandleError(err, ue)
	}
	
	// Safe type assertion to prevent panics
	innerSQL, ok := innerResult.(string)
	if !ok {
		return nil, v.errorHandler.HandleError(fmt.Errorf("unary operand did not produce a string (got %T)", innerResult), ue)
	}
	
	return v.buildUnaryOperation(ue.Operator, innerSQL), nil
}

// VisitNumberLiteral visits a number literal
func (v *SQLGeneratorVisitor) VisitNumberLiteral(n *ast.NumberLiteral) (interface{}, error) {
	return fmt.Sprintf("%g", n.Value), nil
}

// VisitStringLiteral visits a string literal
func (v *SQLGeneratorVisitor) VisitStringLiteral(s *ast.StringLiteral) (interface{}, error) {
	return fmt.Sprintf("'%s'", v.escapeSQLString(s.Value)), nil
}

// VisitParenExpr visits a parenthesized expression
func (v *SQLGeneratorVisitor) VisitParenExpr(p *ast.ParenExpr) (interface{}, error) {
	return Accept(p.Expr, v)
}

// Helper methods

func (v *SQLGeneratorVisitor) pushContext(ctx VisitorContext) {
	v.contextStack = append(v.contextStack, ctx)
}

func (v *SQLGeneratorVisitor) popContext() {
	if len(v.contextStack) > 0 {
		v.contextStack = v.contextStack[:len(v.contextStack)-1]
	}
}

func (v *SQLGeneratorVisitor) getCurrentDepth() int {
	return len(v.contextStack)
}

func (v *SQLGeneratorVisitor) buildLabelCondition(matcher *ast.LabelMatcher) (string, error) {
	lblCol := fmt.Sprintf("%s['%s']", v.schema.GetLabelsColumn(), matcher.Name)
	escapedVal := v.escapeSQLString(matcher.Value)
	
	switch matcher.Operator {
	case ast.MatchEqual:
		return fmt.Sprintf("%s = '%s'", lblCol, escapedVal), nil
	case ast.MatchNotEqual:
		return fmt.Sprintf("%s != '%s'", lblCol, escapedVal), nil
	case ast.MatchRegexp:
		return fmt.Sprintf("match(%s, '%s')", lblCol, escapedVal), nil
	case ast.MatchNotRegexp:
		return fmt.Sprintf("NOT match(%s, '%s')", lblCol, escapedVal), nil
	default:
		return "", fmt.Errorf("unsupported label match operator: %s", matcher.Operator)
	}
}

func (v *SQLGeneratorVisitor) getAggregationFunction(op ast.AggOp) string {
	switch op {
	case ast.AggSum:
		return "sum"
	case ast.AggAvg:
		return "avg"
	case ast.AggMax:
		return "max"
	case ast.AggMin:
		return "min"
	case ast.AggCount:
		return "count"
	default:
		return string(op)
	}
}

func (v *SQLGeneratorVisitor) buildGroupByClause(grouping []string) string {
	if len(grouping) == 0 {
		return ""
	}
	
	columns := make([]string, len(grouping))
	for i, label := range grouping {
		columns[i] = fmt.Sprintf("%s['%s']", v.schema.GetLabelsColumn(), label)
	}
	
	return v.sqlBuilder.BuildGroupBy(columns...)
}

func (v *SQLGeneratorVisitor) visitFunctionCall(c *ast.Call) (interface{}, error) {
	// Visit arguments first
	var argSQLs []string
	for _, arg := range c.Args {
		result, err := Accept(arg, v)
		if err != nil {
			return nil, err
		}
		if s, ok := result.(string); ok {
			argSQLs = append(argSQLs, s)
		}
	}

	// Map common PromQL functions to ClickHouse equivalents
	switch c.Func {
	case ast.FuncRate, ast.FuncIRate, ast.FuncIncrease,
		ast.FuncDelta, ast.FuncIDelta:
		// These require CTE-based transpilation handled by the main transpiler.
		// The visitor provides a simplified stub.
		if len(argSQLs) > 0 {
			return argSQLs[0], nil
		}
		return "", fmt.Errorf("function %s requires at least 1 argument", c.Func)
	case ast.FuncAbs, ast.FuncCeil, ast.FuncFloor, ast.FuncSqrt,
		ast.FuncExp, ast.FuncLn, ast.FuncLog2, ast.FuncLog10:
		if len(argSQLs) > 0 {
			return fmt.Sprintf("%s(%s)", c.Func, argSQLs[0]), nil
		}
		return "", fmt.Errorf("function %s requires at least 1 argument", c.Func)
	default:
		// Fallback: emit function name with arguments
		return fmt.Sprintf("%s(%s)", c.Func, strings.Join(argSQLs, ", ")), nil
	}
}

func (v *SQLGeneratorVisitor) buildBinaryOperation(left string, op ast.BinOp, right string) string {
	return fmt.Sprintf("(%s %s %s)", left, op, right)
}

func (v *SQLGeneratorVisitor) buildUnaryOperation(op ast.UnaryOp, expr string) string {
	return fmt.Sprintf("%s%s", op, expr)
}

func (v *SQLGeneratorVisitor) escapeSQLString(s string) string {
	return strings.ReplaceAll(s, "'", "''")
}

// Error types

type UnsupportedExpressionError struct {
	ExprType ast.Expr
}

func (e *UnsupportedExpressionError) Error() string {
	return fmt.Sprintf("unsupported expression type: %T", e.ExprType)
}

// DefaultErrorHandler implements ErrorHandler
type DefaultErrorHandler struct {
	errors []error
}

func NewDefaultErrorHandler() *DefaultErrorHandler {
	return &DefaultErrorHandler{
		errors: make([]error, 0),
	}
}

func (h *DefaultErrorHandler) HandleError(err error, node ast.Expr) error {
	h.errors = append(h.errors, err)
	return err
}

func (h *DefaultErrorHandler) GetErrors() []error {
	return h.errors
}

func (h *DefaultErrorHandler) HasErrors() bool {
	return len(h.errors) > 0
}
