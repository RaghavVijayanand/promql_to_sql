package ast

import (
	"fmt"
	"strings"
	"time"
)

// Node represents any node in the AST
type Node interface {
	String() string
}

// Expr represents an expression node
type Expr interface {
	Node
	exprNode()
}

// MetricSelector represents a metric with label matchers
type MetricSelector struct {
	Name     string
	Matchers []*LabelMatcher
}

func (m *MetricSelector) exprNode() {}
func (m *MetricSelector) String() string {
	if len(m.Matchers) == 0 {
		return m.Name
	}
	var matchers []string
	for _, matcher := range m.Matchers {
		matchers = append(matchers, matcher.String())
	}
	return fmt.Sprintf("%s{%s}", m.Name, strings.Join(matchers, ","))
}

// LabelMatcher represents a label matching condition
type LabelMatcher struct {
	Name     string
	Operator MatchOp
	Value    string
}

func (l *LabelMatcher) String() string {
	return fmt.Sprintf("%s%s%q", l.Name, l.Operator, l.Value)
}

// MatchOp represents label matching operators
type MatchOp string

const (
	MatchEqual     MatchOp = "="
	MatchNotEqual  MatchOp = "!="
	MatchRegexp    MatchOp = "=~"
	MatchNotRegexp MatchOp = "!~"
)

// VectorSelector represents an instant vector selector
type VectorSelector struct {
	MetricSelector *MetricSelector
	Offset         time.Duration
	Timestamp      *float64 // @ modifier timestamp (Unix seconds)
}

func (v *VectorSelector) exprNode() {}
func (v *VectorSelector) String() string {
	s := v.MetricSelector.String()
	if v.Timestamp != nil {
		s += fmt.Sprintf(" @ %.0f", *v.Timestamp)
	}
	if v.Offset != 0 {
		s += fmt.Sprintf(" offset %s", v.Offset)
	}
	return s
}

// MatrixSelector represents a range vector selector
type MatrixSelector struct {
	VectorSelector *VectorSelector
	Range          time.Duration
}

func (m *MatrixSelector) exprNode() {}
func (m *MatrixSelector) String() string {
	return fmt.Sprintf("%s[%s]", m.VectorSelector.String(), m.Range)
}

// NumberLiteral represents a numeric literal
type NumberLiteral struct {
	Value float64
}

func (n *NumberLiteral) exprNode() {}
func (n *NumberLiteral) String() string {
	return fmt.Sprintf("%g", n.Value)
}

// StringLiteral represents a string literal
type StringLiteral struct {
	Value string
}

func (s *StringLiteral) exprNode() {}
func (s *StringLiteral) String() string {
	return fmt.Sprintf("%q", s.Value)
}

// BinaryExpr represents a binary operation
type BinaryExpr struct {
	Left       Expr
	Operator   BinOp
	Right      Expr
	Matching   *VectorMatching
	ReturnBool bool
}

func (b *BinaryExpr) exprNode() {}
func (b *BinaryExpr) String() string {
	return fmt.Sprintf("(%s %s %s)", b.Left.String(), b.Operator, b.Right.String())
}

// BinOp represents binary operators
type BinOp string

const (
	// Arithmetic operators
	OpAdd BinOp = "+"
	OpSub BinOp = "-"
	OpMul BinOp = "*"
	OpDiv BinOp = "/"
	OpMod BinOp = "%"
	OpPow BinOp = "^"

	// Comparison operators
	OpEql BinOp = "=="
	OpNeq BinOp = "!="
	OpLt  BinOp = "<"
	OpGt  BinOp = ">"
	OpLte BinOp = "<="
	OpGte BinOp = ">="

	// Logical operators
	OpAnd    BinOp = "and"
	OpOr     BinOp = "or"
	OpUnless BinOp = "unless"
)

// VectorMatching specifies how to match vectors in binary operations
type VectorMatching struct {
	On         []string
	Ignoring   []string
	GroupLeft  []string // non-nil means group_left was specified
	GroupRight []string // non-nil means group_right was specified
}

// UnaryExpr represents a unary operation
type UnaryExpr struct {
	Operator UnaryOp
	Expr     Expr
}

func (u *UnaryExpr) exprNode() {}
func (u *UnaryExpr) String() string {
	return fmt.Sprintf("(%s%s)", u.Operator, u.Expr.String())
}

// UnaryOp represents unary operators
type UnaryOp string

const (
	OpUnaryPlus  UnaryOp = "+"
	OpUnaryMinus UnaryOp = "-"
)

// Call represents a function call
type Call struct {
	Func FuncType
	Args []Expr
}

func (c *Call) exprNode() {}
func (c *Call) String() string {
	var args []string
	for _, arg := range c.Args {
		args = append(args, arg.String())
	}
	return fmt.Sprintf("%s(%s)", c.Func, strings.Join(args, ", "))
}

// FuncType represents function types
type FuncType string

const (
	// Rate / counter functions
	FuncRate     FuncType = "rate"
	FuncIRate    FuncType = "irate"
	FuncIncrease FuncType = "increase"
	FuncDelta    FuncType = "delta"
	FuncIDelta   FuncType = "idelta"

	// Over-time aggregation functions
	FuncAvgOverTime      FuncType = "avg_over_time"
	FuncSumOverTime      FuncType = "sum_over_time"
	FuncMinOverTime      FuncType = "min_over_time"
	FuncMaxOverTime      FuncType = "max_over_time"
	FuncCountOverTime    FuncType = "count_over_time"
	FuncQuantileOverTime FuncType = "quantile_over_time"
	FuncStddevOverTime   FuncType = "stddev_over_time"
	FuncStdvarOverTime   FuncType = "stdvar_over_time"
	FuncLastOverTime     FuncType = "last_over_time"
	FuncPresentOverTime  FuncType = "present_over_time"
	FuncAbsentOverTime   FuncType = "absent_over_time"

	// Math functions
	FuncAbs      FuncType = "abs"
	FuncCeil     FuncType = "ceil"
	FuncFloor    FuncType = "floor"
	FuncRound    FuncType = "round"
	FuncSqrt     FuncType = "sqrt"
	FuncExp      FuncType = "exp"
	FuncLn       FuncType = "ln"
	FuncLog2     FuncType = "log2"
	FuncLog10    FuncType = "log10"
	FuncSgn      FuncType = "sgn"
	FuncClamp    FuncType = "clamp"
	FuncClampMax FuncType = "clamp_max"
	FuncClampMin FuncType = "clamp_min"

	// Trigonometric functions
	FuncSin   FuncType = "sin"
	FuncCos   FuncType = "cos"
	FuncTan   FuncType = "tan"
	FuncAsin  FuncType = "asin"
	FuncAcos  FuncType = "acos"
	FuncAtan  FuncType = "atan"
	FuncSinh  FuncType = "sinh"
	FuncCosh  FuncType = "cosh"
	FuncTanh  FuncType = "tanh"
	FuncAsinh FuncType = "asinh"
	FuncAcosh FuncType = "acosh"
	FuncAtanh FuncType = "atanh"
	FuncDeg   FuncType = "deg"
	FuncRad   FuncType = "rad"
	FuncPi    FuncType = "pi"

	// Date/Time functions
	FuncTime        FuncType = "time"
	FuncTimestamp   FuncType = "timestamp"
	FuncDayOfMonth  FuncType = "day_of_month"
	FuncDayOfWeek   FuncType = "day_of_week"
	FuncDayOfYear   FuncType = "day_of_year"
	FuncDaysInMonth FuncType = "days_in_month"
	FuncHour        FuncType = "hour"
	FuncMinute      FuncType = "minute"
	FuncMonth       FuncType = "month"
	FuncYear        FuncType = "year"

	// Histogram functions
	FuncHistogramQuantile FuncType = "histogram_quantile"
	FuncHistogramCount    FuncType = "histogram_count"
	FuncHistogramSum      FuncType = "histogram_sum"

	// Label functions
	FuncLabelReplace FuncType = "label_replace"
	FuncLabelJoin    FuncType = "label_join"

	// Sort functions
	FuncSort     FuncType = "sort"
	FuncSortDesc FuncType = "sort_desc"

	// Other functions
	FuncVector        FuncType = "vector"
	FuncScalar        FuncType = "scalar"
	FuncAbsent        FuncType = "absent"
	FuncChanges       FuncType = "changes"
	FuncResets        FuncType = "resets"
	FuncDeriv         FuncType = "deriv"
	FuncPredictLinear FuncType = "predict_linear"
	FuncHoltWinters   FuncType = "holt_winters"
)

// KnownFunctions maps function name strings to FuncType.
// Used by the parser to identify function calls from identifiers.
var KnownFunctions = map[string]FuncType{
	"rate":               FuncRate,
	"irate":              FuncIRate,
	"increase":           FuncIncrease,
	"delta":              FuncDelta,
	"idelta":             FuncIDelta,
	"avg_over_time":      FuncAvgOverTime,
	"sum_over_time":      FuncSumOverTime,
	"min_over_time":      FuncMinOverTime,
	"max_over_time":      FuncMaxOverTime,
	"count_over_time":    FuncCountOverTime,
	"quantile_over_time": FuncQuantileOverTime,
	"stddev_over_time":   FuncStddevOverTime,
	"stdvar_over_time":   FuncStdvarOverTime,
	"last_over_time":     FuncLastOverTime,
	"present_over_time":  FuncPresentOverTime,
	"absent_over_time":   FuncAbsentOverTime,
	"abs":                FuncAbs,
	"ceil":               FuncCeil,
	"floor":              FuncFloor,
	"round":              FuncRound,
	"sqrt":               FuncSqrt,
	"exp":                FuncExp,
	"ln":                 FuncLn,
	"log2":               FuncLog2,
	"log10":              FuncLog10,
	"sgn":                FuncSgn,
	"clamp":              FuncClamp,
	"clamp_max":          FuncClampMax,
	"clamp_min":          FuncClampMin,
	"sin":                FuncSin,
	"cos":                FuncCos,
	"tan":                FuncTan,
	"asin":               FuncAsin,
	"acos":               FuncAcos,
	"atan":               FuncAtan,
	"sinh":               FuncSinh,
	"cosh":               FuncCosh,
	"tanh":               FuncTanh,
	"asinh":              FuncAsinh,
	"acosh":              FuncAcosh,
	"atanh":              FuncAtanh,
	"deg":                FuncDeg,
	"rad":                FuncRad,
	"pi":                 FuncPi,
	"time":               FuncTime,
	"timestamp":          FuncTimestamp,
	"day_of_month":       FuncDayOfMonth,
	"day_of_week":        FuncDayOfWeek,
	"day_of_year":        FuncDayOfYear,
	"days_in_month":      FuncDaysInMonth,
	"hour":               FuncHour,
	"minute":             FuncMinute,
	"month":              FuncMonth,
	"year":               FuncYear,
	"histogram_quantile": FuncHistogramQuantile,
	"histogram_count":    FuncHistogramCount,
	"histogram_sum":      FuncHistogramSum,
	"label_replace":      FuncLabelReplace,
	"label_join":         FuncLabelJoin,
	"sort":               FuncSort,
	"sort_desc":          FuncSortDesc,
	"vector":             FuncVector,
	"scalar":             FuncScalar,
	"absent":             FuncAbsent,
	"changes":            FuncChanges,
	"resets":             FuncResets,
	"deriv":              FuncDeriv,
	"predict_linear":     FuncPredictLinear,
	"holt_winters":       FuncHoltWinters,
}

// AggregateExpr represents an aggregation operation
type AggregateExpr struct {
	Op       AggOp
	Expr     Expr
	Param    Expr     // For quantile, topk, bottomk, count_values
	Grouping []string
	Without  bool // true if WITHOUT, false if BY
}

func (a *AggregateExpr) exprNode() {}
func (a *AggregateExpr) String() string {
	var parts []string
	parts = append(parts, string(a.Op))

	if a.Param != nil {
		parts = append(parts, fmt.Sprintf("(%s, %s)", a.Param.String(), a.Expr.String()))
	} else {
		parts = append(parts, fmt.Sprintf("(%s)", a.Expr.String()))
	}

	if len(a.Grouping) > 0 {
		if a.Without {
			parts = append(parts, fmt.Sprintf("without (%s)", strings.Join(a.Grouping, ", ")))
		} else {
			parts = append(parts, fmt.Sprintf("by (%s)", strings.Join(a.Grouping, ", ")))
		}
	}

	return strings.Join(parts, " ")
}

// AggOp represents aggregation operators
type AggOp string

const (
	AggSum         AggOp = "sum"
	AggMin         AggOp = "min"
	AggMax         AggOp = "max"
	AggAvg         AggOp = "avg"
	AggStddev      AggOp = "stddev"
	AggStdvar      AggOp = "stdvar"
	AggCount       AggOp = "count"
	AggCountValues AggOp = "count_values"
	AggBottomK     AggOp = "bottomk"
	AggTopK        AggOp = "topk"
	AggQuantile    AggOp = "quantile"
	AggGroup       AggOp = "group"
)

// ParenExpr represents a parenthesized expression
type ParenExpr struct {
	Expr Expr
}

func (p *ParenExpr) exprNode() {}
func (p *ParenExpr) String() string {
	return fmt.Sprintf("(%s)", p.Expr.String())
}

// SubqueryExpr represents a subquery
type SubqueryExpr struct {
	Expr   Expr
	Range  time.Duration
	Step   time.Duration
	Offset time.Duration
}

func (s *SubqueryExpr) exprNode() {}
func (s *SubqueryExpr) String() string {
	str := fmt.Sprintf("%s[%s:", s.Expr.String(), s.Range)
	if s.Step != 0 {
		str += fmt.Sprintf("%s", s.Step)
	}
	str += "]"
	if s.Offset != 0 {
		str += fmt.Sprintf(" offset %s", s.Offset)
	}
	return str
}
