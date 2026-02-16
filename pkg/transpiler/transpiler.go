package transpiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/shinro/promql-transpiler/internal/cardinality"
	"github.com/shinro/promql-transpiler/pkg/ast"
	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/optimization"
	"github.com/shinro/promql-transpiler/pkg/parser"
)

// ─── CONFIGURATION ───────────────────────────────────────────────────────────

// Config holds transpiler configuration.
type Config struct {
	Schema             *clickhouse.Schema
	EnableOptimization bool
	EnableSampling     bool
	DefaultTimeRange   time.Duration
}

// TimeRange represents the query evaluation time range.
type TimeRange struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// ─── TRANSPILER ──────────────────────────────────────────────────────────────

// Transpiler converts PromQL to ClickHouse SQL.
type Transpiler struct {
	schema             *clickhouse.Schema
	qb                 *clickhouse.QueryBuilder
	optimizer          *cardinality.Optimizer
	engineOptimizer    *optimization.EngineOptimizer
	enableOptimization bool
	enableSampling     bool
	timeRange          *TimeRange
}

// New creates a Transpiler with the given configuration.
func New(config *Config) *Transpiler {
	if config == nil {
		config = &Config{
			Schema:             clickhouse.DefaultSchema(),
			EnableOptimization: true,
			EnableSampling:     true,
			DefaultTimeRange:   time.Hour,
		}
	}
	if config.Schema == nil {
		config.Schema = clickhouse.DefaultSchema()
	}

	tableMeta := config.Schema.GetTableMeta()

	return &Transpiler{
		schema:             config.Schema,
		qb:                 clickhouse.NewQueryBuilder(config.Schema),
		optimizer:          cardinality.NewOptimizer(),
		engineOptimizer:    optimization.NewEngineOptimizer(tableMeta, config.Schema),
		enableOptimization: config.EnableOptimization,
		enableSampling:     config.EnableSampling,
	}
}

// SetTimeRange sets the evaluation time range.
func (t *Transpiler) SetTimeRange(start, end time.Time, step time.Duration) {
	t.timeRange = &TimeRange{Start: start, End: end, Step: step}
	t.qb.SetTimeRange(start, end, step)
}

// Transpile is the public entry point.
func (t *Transpiler) Transpile(promql string) (string, error) {
	expr, err := parser.Parse(promql)
	if err != nil {
		return "", fmt.Errorf("parse error: %w", err)
	}
	sql, err := t.transpileExpr(expr)
	if err != nil {
		return "", fmt.Errorf("transpile error: %w", err)
	}

	// Apply engine-aware optimizations when enabled.
	// These are deterministic transformations based on the table's DDL
	// (PREWHERE promotion, FINAL injection, ORDER BY alignment).
	if t.enableOptimization && t.engineOptimizer != nil {
		sql, err = t.engineOptimizer.Optimize(sql, expr)
		if err != nil {
			return "", fmt.Errorf("optimization error: %w", err)
		}
	}

	return strings.TrimSpace(sql), nil
}

// ─── EXPRESSION DISPATCH ─────────────────────────────────────────────────────

func (t *Transpiler) transpileExpr(expr ast.Expr) (string, error) {
	if expr == nil {
		return "", fmt.Errorf("nil expression")
	}
	switch e := expr.(type) {
	case *ast.VectorSelector:
		return t.transpileVectorSelector(e)
	case *ast.MatrixSelector:
		return t.transpileMatrixSelector(e)
	case *ast.AggregateExpr:
		return t.transpileAggregateExpr(e)
	case *ast.Call:
		return t.transpileCall(e)
	case *ast.BinaryExpr:
		return t.transpileBinaryExpr(e)
	case *ast.UnaryExpr:
		return t.transpileUnaryExpr(e)
	case *ast.NumberLiteral:
		return fmt.Sprintf("%g", e.Value), nil
	case *ast.StringLiteral:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(e.Value, "'", "''")), nil
	case *ast.ParenExpr:
		inner, err := t.transpileExpr(e.Expr)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("(%s)", inner), nil
	case *ast.SubqueryExpr:
		return t.transpileSubqueryExpr(e)
	default:
		return "", fmt.Errorf("unsupported expression type: %T", expr)
	}
}

// ─── HELPERS ─────────────────────────────────────────────────────────────────

// col is a convenience shorthand for accessing schema columns.
func (t *Transpiler) ts() string  { return t.schema.TimestampColumn }
func (t *Transpiler) val() string { return t.schema.ValueColumn }
func (t *Transpiler) lbl() string { return t.schema.LabelsColumn }
func (t *Transpiler) tbl() string { return t.schema.TableName }

func (t *Transpiler) labelCol(name string) string {
	return fmt.Sprintf("%s['%s']", t.lbl(), name)
}

func (t *Transpiler) getTimeRange(offset time.Duration) (time.Time, time.Time) {
	if t.timeRange == nil {
		end := time.Now().Add(-offset)
		start := end.Add(-time.Hour)
		return start, end
	}
	return t.timeRange.Start.Add(-offset), t.timeRange.End.Add(-offset)
}

func (t *Transpiler) getTimeRangeForMatrix(rangeD, offset time.Duration) (time.Time, time.Time) {
	start, end := t.getTimeRange(offset)
	return start.Add(-rangeD), end
}

// buildWhereClauses builds conditions from a metric selector.
func (t *Transpiler) buildWhereClauses(ms *ast.MetricSelector, start, end time.Time) []string {
	var conds []string
	if ms.Name != "" {
		conds = append(conds, t.qb.BuildMetricNameCondition(ms.Name))
	}
	for _, m := range ms.Matchers {
		c := t.qb.BuildLabelCondition(m.Name, string(m.Operator), m.Value)
		if c != "" {
			conds = append(conds, c)
		}
	}
	tc := t.qb.BuildTimeRangeCondition(start, end)
	if tc != "" {
		conds = append(conds, tc)
	}
	return conds
}

// ─── VECTOR SELECTOR ─────────────────────────────────────────────────────────

func (t *Transpiler) transpileVectorSelector(vs *ast.VectorSelector) (string, error) {
	var start, end time.Time
	if vs.Timestamp != nil {
		// @ modifier: evaluate at a fixed point in time
		at := time.Unix(int64(*vs.Timestamp), 0).UTC()
		start = at.Add(-vs.Offset)
		end = at.Add(-vs.Offset)
	} else {
		start, end = t.getTimeRange(vs.Offset)
	}
	conds := t.buildWhereClauses(vs.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf("SELECT %s, %s, %s\nFROM %s\n%s\nORDER BY %s",
		t.ts(), t.val(), t.lbl(), t.tbl(), where, t.ts()), nil
}

// ─── MATRIX SELECTOR ─────────────────────────────────────────────────────────

func (t *Transpiler) transpileMatrixSelector(ms *ast.MatrixSelector) (string, error) {
	var start, end time.Time
	if ms.VectorSelector.Timestamp != nil {
		at := time.Unix(int64(*ms.VectorSelector.Timestamp), 0).UTC()
		start = at.Add(-ms.Range).Add(-ms.VectorSelector.Offset)
		end = at.Add(-ms.VectorSelector.Offset)
	} else {
		start, end = t.getTimeRangeForMatrix(ms.Range, ms.VectorSelector.Offset)
	}
	conds := t.buildWhereClauses(ms.VectorSelector.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf("SELECT %s, %s, %s\nFROM %s\n%s\nORDER BY %s",
		t.ts(), t.val(), t.lbl(), t.tbl(), where, t.ts()), nil
}

// ─── FUNCTION CALL DISPATCH ──────────────────────────────────────────────────

func (t *Transpiler) transpileCall(call *ast.Call) (string, error) {
	switch call.Func {

	// ── Rate / Counter functions ──────────────────────────────────────────
	case ast.FuncRate:
		return t.transpileRateFamily(call, true)
	case ast.FuncIRate:
		return t.transpileIRate(call)
	case ast.FuncIncrease:
		return t.transpileRateFamily(call, false)
	case ast.FuncDelta:
		return t.transpileDeltaFamily(call, false)
	case ast.FuncIDelta:
		return t.transpileDeltaFamily(call, true)

	// ── Over-time aggregation functions ───────────────────────────────────
	case ast.FuncAvgOverTime:
		return t.transpileOverTime(call, "avg")
	case ast.FuncSumOverTime:
		return t.transpileOverTime(call, "sum")
	case ast.FuncMinOverTime:
		return t.transpileOverTime(call, "min")
	case ast.FuncMaxOverTime:
		return t.transpileOverTime(call, "max")
	case ast.FuncCountOverTime:
		return t.transpileOverTime(call, "count")
	case ast.FuncStddevOverTime:
		return t.transpileOverTime(call, "stddevPop")
	case ast.FuncStdvarOverTime:
		return t.transpileOverTime(call, "varPop")
	case ast.FuncLastOverTime:
		return t.transpileLastOverTime(call)
	case ast.FuncPresentOverTime:
		return t.transpilePresentOverTime(call)
	case ast.FuncAbsentOverTime:
		return t.transpileAbsentOverTime(call)
	case ast.FuncQuantileOverTime:
		return t.transpileQuantileOverTime(call)

	// ── Math functions (1 arg → 1 ClickHouse function) ───────────────────
	case ast.FuncAbs:
		return t.transpileSimpleMath(call, "abs")
	case ast.FuncCeil:
		return t.transpileSimpleMath(call, "ceil")
	case ast.FuncFloor:
		return t.transpileSimpleMath(call, "floor")
	case ast.FuncRound:
		return t.transpileRound(call)
	case ast.FuncSqrt:
		return t.transpileSimpleMath(call, "sqrt")
	case ast.FuncExp:
		return t.transpileSimpleMath(call, "exp")
	case ast.FuncLn:
		return t.transpileSimpleMath(call, "log")
	case ast.FuncLog2:
		return t.transpileSimpleMath(call, "log2")
	case ast.FuncLog10:
		return t.transpileSimpleMath(call, "log10")
	case ast.FuncSgn:
		return t.transpileSimpleMath(call, "sign")

	// ── Clamp functions ──────────────────────────────────────────────────
	case ast.FuncClamp:
		return t.transpileClamp(call)
	case ast.FuncClampMin:
		return t.transpileClampMin(call)
	case ast.FuncClampMax:
		return t.transpileClampMax(call)

	// ── Trigonometric functions ───────────────────────────────────────────
	case ast.FuncSin:
		return t.transpileSimpleMath(call, "sin")
	case ast.FuncCos:
		return t.transpileSimpleMath(call, "cos")
	case ast.FuncTan:
		return t.transpileSimpleMath(call, "tan")
	case ast.FuncAsin:
		return t.transpileSimpleMath(call, "asin")
	case ast.FuncAcos:
		return t.transpileSimpleMath(call, "acos")
	case ast.FuncAtan:
		return t.transpileSimpleMath(call, "atan")
	case ast.FuncSinh:
		return t.transpileHyperbolic(call, "sinh")
	case ast.FuncCosh:
		return t.transpileHyperbolic(call, "cosh")
	case ast.FuncTanh:
		return t.transpileHyperbolic(call, "tanh")
	case ast.FuncAsinh:
		return t.transpileHyperbolic(call, "asinh")
	case ast.FuncAcosh:
		return t.transpileHyperbolic(call, "acosh")
	case ast.FuncAtanh:
		return t.transpileHyperbolic(call, "atanh")
	case ast.FuncDeg:
		return t.transpileSimpleMath(call, "degrees")
	case ast.FuncRad:
		return t.transpileSimpleMath(call, "radians")
	case ast.FuncPi:
		return "SELECT pi() AS value", nil

	// ── Date/Time functions ──────────────────────────────────────────────
	case ast.FuncTime:
		return t.transpileTimeFn()
	case ast.FuncTimestamp:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toUnixTimestamp(%s)", t.ts()))
	case ast.FuncDayOfMonth:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toDayOfMonth(%s)", t.ts()))
	case ast.FuncDayOfWeek:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toDayOfWeek(%s)", t.ts()))
	case ast.FuncDayOfYear:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toDayOfYear(%s)", t.ts()))
	case ast.FuncDaysInMonth:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toDayOfMonth(toLastDayOfMonth(%s))", t.ts()))
	case ast.FuncHour:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toHour(%s)", t.ts()))
	case ast.FuncMinute:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toMinute(%s)", t.ts()))
	case ast.FuncMonth:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toMonth(%s)", t.ts()))
	case ast.FuncYear:
		return t.transpileDateTimeFn(call, fmt.Sprintf("toYear(%s)", t.ts()))

	// ── Histogram functions ──────────────────────────────────────────────
	case ast.FuncHistogramQuantile:
		return t.transpileHistogramQuantile(call)
	case ast.FuncHistogramCount:
		return t.transpileHistogramCountSum(call, "count")
	case ast.FuncHistogramSum:
		return t.transpileHistogramCountSum(call, "sum")

	// ── Label functions ──────────────────────────────────────────────────
	case ast.FuncLabelReplace:
		return t.transpileLabelReplace(call)
	case ast.FuncLabelJoin:
		return t.transpileLabelJoin(call)

	// ── Sort ─────────────────────────────────────────────────────────────
	case ast.FuncSort:
		return t.transpileSort(call, "ASC")
	case ast.FuncSortDesc:
		return t.transpileSort(call, "DESC")

	// ── Other functions ──────────────────────────────────────────────────
	case ast.FuncVector:
		return t.transpileVector(call)
	case ast.FuncScalar:
		return t.transpileScalar(call)
	case ast.FuncAbsent:
		return t.transpileAbsent(call)
	case ast.FuncChanges:
		return t.transpileChanges(call)
	case ast.FuncResets:
		return t.transpileResets(call)
	case ast.FuncDeriv:
		return t.transpileDeriv(call)
	case ast.FuncPredictLinear:
		return t.transpilePredictLinear(call)
	case ast.FuncHoltWinters:
		return t.transpileHoltWinters(call)

	default:
		return "", fmt.Errorf("unsupported function: %s", call.Func)
	}
}

// ─── RATE / COUNTER FUNCTIONS ────────────────────────────────────────────────

func (t *Transpiler) transpileRateFamily(call *ast.Call, perSecond bool) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("%s() requires exactly 1 argument", call.Func)
	}
	// Handle subquery argument: e.g. rate(expr[30m:1m])
	if sq, ok := call.Args[0].(*ast.SubqueryExpr); ok {
		return t.transpileRateFamilySubquery(call, perSecond, sq)
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("%s() requires a range vector or subquery", call.Func)
	}

	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}

	valueExpr := fmt.Sprintf(
		"if(%[1]s < prev_%[1]s, %[1]s, %[1]s - prev_%[1]s)",
		t.val(),
	)
	if perSecond {
		valueExpr = fmt.Sprintf(
			"if(time_diff > 0, (%s) / time_diff, 0)", valueExpr,
		)
	}

	return fmt.Sprintf(`WITH base_data AS (
%s
),
rate_data AS (
  SELECT
    %s,
    %s,
    %s,
    lagInFrame(%s, 1, 0) OVER w AS prev_%s,
    toUnixTimestamp(%s) - toUnixTimestamp(lagInFrame(%s, 1, %s) OVER w) AS time_diff
  FROM base_data
  WINDOW w AS (PARTITION BY %s ORDER BY %s)
)
SELECT %s, %s, %s AS value
FROM rate_data
WHERE prev_%s IS NOT NULL OR 1=1`,
		baseSQL,
		t.ts(), t.val(), t.lbl(),
		t.val(), t.val(),
		t.ts(), t.ts(), t.ts(),
		t.lbl(), t.ts(),
		t.ts(), t.lbl(), valueExpr, t.val()), nil
}

// transpileRateFamilySubquery handles rate/irate/increase with a subquery argument.
func (t *Transpiler) transpileRateFamilySubquery(call *ast.Call, perSecond bool, sq *ast.SubqueryExpr) (string, error) {
	innerSQL, err := t.transpileExpr(sq.Expr)
	if err != nil {
		return "", err
	}
	if !isSelectQuery(innerSQL) {
		return "", fmt.Errorf("%s() subquery inner must produce a series", call.Func)
	}

	valueExpr := fmt.Sprintf(
		"if(%[1]s < prev_%[1]s, %[1]s, %[1]s - prev_%[1]s)",
		t.val(),
	)
	if perSecond {
		valueExpr = fmt.Sprintf(
			"if(time_diff > 0, (%s) / time_diff, 0)", valueExpr,
		)
	}

	return fmt.Sprintf(`WITH base_data AS (
%s
),
rate_data AS (
  SELECT
    %s,
    %s,
    %s,
    lagInFrame(%s, 1, 0) OVER w AS prev_%s,
    toUnixTimestamp(%s) - toUnixTimestamp(lagInFrame(%s, 1, %s) OVER w) AS time_diff
  FROM base_data
  WINDOW w AS (PARTITION BY %s ORDER BY %s)
)
SELECT %s, %s, %s AS value
FROM rate_data
WHERE prev_%s IS NOT NULL OR 1=1`,
		innerSQL,
		t.ts(), t.val(), t.lbl(),
		t.val(), t.val(),
		t.ts(), t.ts(), t.ts(),
		t.lbl(), t.ts(),
		t.ts(), t.lbl(), valueExpr, t.val()), nil
}

func (t *Transpiler) transpileIRate(call *ast.Call) (string, error) {
	// irate uses only the last two data points; we use lagInFrame with 1 row
	return t.transpileRateFamily(call, true)
}

func (t *Transpiler) transpileDeltaFamily(call *ast.Call, instantOnly bool) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("%s() requires exactly 1 argument", call.Func)
	}
	// Handle subquery argument
	if sq, ok := call.Args[0].(*ast.SubqueryExpr); ok {
		innerSQL, err := t.transpileExpr(sq.Expr)
		if err != nil {
			return "", err
		}
		if isSelectQuery(innerSQL) {
			return fmt.Sprintf(`WITH base_data AS (
%s
)
SELECT
  %s,
  %s,
  %s - lagInFrame(%s, 1, %s) OVER w AS value
FROM base_data
WINDOW w AS (PARTITION BY %s ORDER BY %s)`,
				innerSQL, t.ts(), t.lbl(), t.val(), t.val(), t.val(), t.lbl(), t.ts()), nil
		}
		return "", fmt.Errorf("%s() subquery inner must produce a series", call.Func)
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("%s() requires a range vector or subquery", call.Func)
	}

	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`WITH base_data AS (
%s
)
SELECT
  %s,
  %s,
  %s - lagInFrame(%s, 1, %s) OVER w AS value
FROM base_data
WINDOW w AS (PARTITION BY %s ORDER BY %s)`,
		baseSQL,
		t.ts(), t.lbl(),
		t.val(), t.val(), t.val(),
		t.lbl(), t.ts()), nil
}

// ─── OVER-TIME AGGREGATION FUNCTIONS ─────────────────────────────────────────

// transpileSubqueryExpr handles subquery expressions like expr[30m:1m].
// The subquery range and step are used by the enclosing over-time function;
// here we simply transpile the inner expression.
func (t *Transpiler) transpileSubqueryExpr(sq *ast.SubqueryExpr) (string, error) {
	return t.transpileExpr(sq.Expr)
}

// transpileOverTimeSubquery handles over-time functions whose argument is a subquery.
func (t *Transpiler) transpileOverTimeSubquery(call *ast.Call, chFunc string, sq *ast.SubqueryExpr) (string, error) {
	innerSQL, err := t.transpileExpr(sq.Expr)
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH subq AS (
%s
)
SELECT
  %s,
  %s(%s) AS value
FROM subq
GROUP BY %s`,
			innerSQL, t.lbl(), chFunc, t.val(), t.lbl()), nil
	}
	return fmt.Sprintf("SELECT %s(%s) AS value FROM (%s) GROUP BY %s",
		chFunc, t.val(), innerSQL, t.lbl()), nil
}

// transpileOverTime handles avg_over_time, sum_over_time, min_over_time, etc.
func (t *Transpiler) transpileOverTime(call *ast.Call, chFunc string) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("%s() requires exactly 1 argument", call.Func)
	}
	// Handle subquery argument: e.g. max_over_time(expr[30m:1m])
	if sq, ok := call.Args[0].(*ast.SubqueryExpr); ok {
		return t.transpileOverTimeSubquery(call, chFunc, sq)
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("%s() requires a range vector or subquery", call.Func)
	}

	start, end := t.getTimeRangeForMatrix(ms.Range, ms.VectorSelector.Offset)
	conds := t.buildWhereClauses(ms.VectorSelector.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf(`SELECT
  %s,
  %s(%s) AS value
FROM %s
%s
GROUP BY %s
ORDER BY %s`,
		t.lbl(), chFunc, t.val(), t.tbl(), where, t.lbl(), t.lbl()), nil
}

func (t *Transpiler) transpileQuantileOverTime(call *ast.Call) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("quantile_over_time() requires exactly 2 arguments")
	}
	phi, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}

	// Handle subquery argument: e.g. quantile_over_time(0.99, expr[1h:5m])
	if sq, ok := call.Args[1].(*ast.SubqueryExpr); ok {
		innerSQL, err := t.transpileExpr(sq.Expr)
		if err != nil {
			return "", err
		}
		if isSelectQuery(innerSQL) {
			return fmt.Sprintf(`WITH subq AS (
%s
)
SELECT
  %s,
  quantile(%s)(%s) AS value
FROM subq
GROUP BY %s`,
				innerSQL, t.lbl(), phi, t.val(), t.lbl()), nil
		}
		return fmt.Sprintf("SELECT quantile(%s)(%s) AS value FROM (%s) GROUP BY %s",
			phi, t.val(), innerSQL, t.lbl()), nil
	}

	ms, ok := call.Args[1].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("quantile_over_time() second argument must be a range vector or subquery")
	}

	start, end := t.getTimeRangeForMatrix(ms.Range, ms.VectorSelector.Offset)
	conds := t.buildWhereClauses(ms.VectorSelector.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf(`SELECT
  %s,
  quantile(%s)(%s) AS value
FROM %s
%s
GROUP BY %s`,
		t.lbl(), phi, t.val(), t.tbl(), where, t.lbl()), nil
}

func (t *Transpiler) transpileLastOverTime(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("last_over_time() requires exactly 1 argument")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("last_over_time() requires a range vector")
	}

	start, end := t.getTimeRangeForMatrix(ms.Range, ms.VectorSelector.Offset)
	conds := t.buildWhereClauses(ms.VectorSelector.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf(`SELECT
  %s,
  argMax(%s, %s) AS value
FROM %s
%s
GROUP BY %s`,
		t.lbl(), t.val(), t.ts(), t.tbl(), where, t.lbl()), nil
}

func (t *Transpiler) transpilePresentOverTime(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("present_over_time() requires exactly 1 argument")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("present_over_time() requires a range vector")
	}

	start, end := t.getTimeRangeForMatrix(ms.Range, ms.VectorSelector.Offset)
	conds := t.buildWhereClauses(ms.VectorSelector.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf(`SELECT %s, 1 AS value
FROM %s
%s
GROUP BY %s
HAVING count(%s) > 0`,
		t.lbl(), t.tbl(), where, t.lbl(), t.val()), nil
}

func (t *Transpiler) transpileAbsentOverTime(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("absent_over_time() requires exactly 1 argument")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("absent_over_time() requires a range vector")
	}

	start, end := t.getTimeRangeForMatrix(ms.Range, ms.VectorSelector.Offset)
	conds := t.buildWhereClauses(ms.VectorSelector.MetricSelector, start, end)
	where := t.qb.BuildWhere(conds...)

	return fmt.Sprintf(`SELECT 1 AS value
WHERE (SELECT count() FROM %s %s) = 0`,
		t.tbl(), where), nil
}

// ─── SIMPLE MATH FUNCTIONS ───────────────────────────────────────────────────

// transpileSimpleMath wraps a single-arg PromQL function with the ClickHouse equivalent.
func (t *Transpiler) transpileSimpleMath(call *ast.Call, chFunc string) (string, error) {
	if len(call.Args) < 1 {
		return "", fmt.Errorf("%s() requires at least 1 argument", call.Func)
	}

	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}

	// If the inner SQL is itself a SELECT, wrap it in a CTE
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, %s(%s) AS value
FROM inner`,
			innerSQL, t.ts(), t.lbl(), chFunc, t.val()), nil
	}

	// Inner is a scalar expression
	return fmt.Sprintf("SELECT %s(%s) AS value", chFunc, innerSQL), nil
}

func (t *Transpiler) transpileRound(call *ast.Call) (string, error) {
	if len(call.Args) < 1 || len(call.Args) > 2 {
		return "", fmt.Errorf("round() requires 1 or 2 arguments")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}

	precision := "0"
	if len(call.Args) == 2 {
		p, err := t.transpileExpr(call.Args[1])
		if err != nil {
			return "", err
		}
		precision = p
	}

	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, round(%s, %s) AS value
FROM inner`,
			innerSQL, t.ts(), t.lbl(), t.val(), precision), nil
	}
	return fmt.Sprintf("SELECT round(%s, %s) AS value", innerSQL, precision), nil
}

// ─── CLAMP FUNCTIONS ─────────────────────────────────────────────────────────

func (t *Transpiler) transpileClamp(call *ast.Call) (string, error) {
	if len(call.Args) != 3 {
		return "", fmt.Errorf("clamp() requires 3 arguments")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	minVal, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}
	maxVal, err := t.transpileExpr(call.Args[2])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, greatest(least(%s, %s), %s) AS value
FROM inner`,
			innerSQL, t.ts(), t.lbl(), t.val(), maxVal, minVal), nil
	}
	return fmt.Sprintf("SELECT greatest(least(%s, %s), %s) AS value", innerSQL, maxVal, minVal), nil
}

func (t *Transpiler) transpileClampMin(call *ast.Call) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("clamp_min() requires 2 arguments")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	minVal, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, greatest(%s, %s) AS value
FROM inner`,
			innerSQL, t.ts(), t.lbl(), t.val(), minVal), nil
	}
	return fmt.Sprintf("SELECT greatest(%s, %s) AS value", innerSQL, minVal), nil
}

func (t *Transpiler) transpileClampMax(call *ast.Call) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("clamp_max() requires 2 arguments")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	maxVal, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, least(%s, %s) AS value
FROM inner`,
			innerSQL, t.ts(), t.lbl(), t.val(), maxVal), nil
	}
	return fmt.Sprintf("SELECT least(%s, %s) AS value", innerSQL, maxVal), nil
}

// ─── HYPERBOLIC TRIG (emulated where ClickHouse lacks native support) ────────

func (t *Transpiler) transpileHyperbolic(call *ast.Call, fn string) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("%s() requires exactly 1 argument", fn)
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}

	// ClickHouse 23.8+ supports sinh, cosh, tanh, etc. natively.
	// For older versions, use the math definitions.
	var expr string
	switch fn {
	case "sinh":
		expr = fmt.Sprintf("(exp(%[1]s) - exp(-(%[1]s))) / 2", t.val())
	case "cosh":
		expr = fmt.Sprintf("(exp(%[1]s) + exp(-(%[1]s))) / 2", t.val())
	case "tanh":
		expr = fmt.Sprintf("(exp(2 * %[1]s) - 1) / (exp(2 * %[1]s) + 1)", t.val())
	case "asinh":
		expr = fmt.Sprintf("log(%[1]s + sqrt(%[1]s * %[1]s + 1))", t.val())
	case "acosh":
		expr = fmt.Sprintf("log(%[1]s + sqrt(%[1]s * %[1]s - 1))", t.val())
	case "atanh":
		expr = fmt.Sprintf("0.5 * log((1 + %[1]s) / (1 - %[1]s))", t.val())
	default:
		expr = fmt.Sprintf("%s(%s)", fn, t.val())
	}

	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, %s AS value
FROM inner`, innerSQL, t.ts(), t.lbl(), expr), nil
	}
	return fmt.Sprintf("SELECT %s AS value", expr), nil
}

// ─── DATE/TIME FUNCTIONS ─────────────────────────────────────────────────────

func (t *Transpiler) transpileTimeFn() (string, error) {
	return "SELECT toUnixTimestamp(now()) AS value", nil
}

// transpileDateTimeFn applies a ClickHouse date/time expression to each row.
func (t *Transpiler) transpileDateTimeFn(call *ast.Call, chExpr string) (string, error) {
	if len(call.Args) == 0 {
		// No argument → apply to now()
		return fmt.Sprintf("SELECT %s AS value", chExpr), nil
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, %s AS value
FROM inner`, innerSQL, t.ts(), t.lbl(), chExpr), nil
	}
	return fmt.Sprintf("SELECT %s AS value", chExpr), nil
}

// ─── HISTOGRAM FUNCTIONS ─────────────────────────────────────────────────────

func (t *Transpiler) transpileHistogramQuantile(call *ast.Call) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("histogram_quantile() requires 2 arguments")
	}
	phi, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	innerSQL, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}

	if !isSelectQuery(innerSQL) {
		return "", fmt.Errorf("histogram_quantile() second argument must produce a time series")
	}

	// ClickHouse implementation: use the le (less-than-or-equal) bucket label
	// to reconstruct the histogram and interpolate the quantile.
	return fmt.Sprintf(`WITH bucket_data AS (
%s
),
-- Extract numeric bucket boundary from the 'le' label
buckets AS (
  SELECT
    %s,
    toFloat64OrNull(%s['le']) AS le,
    %s AS bucket_count,
    arrayFilter(x -> x.1 != 'le', mapKeys(%s), mapValues(%s)) AS group_labels
  FROM bucket_data
  WHERE %s['le'] != ''
)
SELECT
  group_labels AS %s,
  quantileExactWeighted(%s)(le, toUInt64(bucket_count)) AS value
FROM buckets
WHERE le IS NOT NULL
GROUP BY group_labels`,
		innerSQL,
		t.ts(), t.lbl(), t.val(), t.lbl(), t.lbl(),
		t.lbl(),
		t.lbl(), phi), nil
}

func (t *Transpiler) transpileHistogramCountSum(call *ast.Call, which string) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("histogram_%s() requires exactly 1 argument", which)
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		suffix := "_count"
		if which == "sum" {
			suffix = "_sum"
		}
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, %s AS value
FROM inner
-- Filter for the %s series (metric_name ends with '%s')`,
			innerSQL, t.ts(), t.lbl(), t.val(), which, suffix), nil
	}
	return "", fmt.Errorf("histogram_%s() argument must produce a time series", which)
}

// ─── LABEL FUNCTIONS ─────────────────────────────────────────────────────────

func (t *Transpiler) transpileLabelReplace(call *ast.Call) (string, error) {
	if len(call.Args) != 5 {
		return "", fmt.Errorf("label_replace() requires 5 arguments")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	dstLabel, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}
	replacement, _ := t.transpileExpr(call.Args[2])
	srcLabel, _ := t.transpileExpr(call.Args[3])
	regex, _ := t.transpileExpr(call.Args[4])

	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT
  %s,
  mapSet(mapApply((k, v) -> (k, if(k = %s, replaceRegexpOne(v, %s, %s), v)), %s), %s, replaceRegexpOne(mapValues(%s, %s)[1], %s, %s)) AS %s,
  %s
FROM inner`,
			innerSQL,
			t.ts(),
			srcLabel, regex, replacement, t.lbl(),
			dstLabel, t.lbl(), srcLabel, regex, replacement, t.lbl(),
			t.val()), nil
	}
	return "", fmt.Errorf("label_replace() requires a vector expression as first argument")
}

func (t *Transpiler) transpileLabelJoin(call *ast.Call) (string, error) {
	if len(call.Args) < 4 {
		return "", fmt.Errorf("label_join() requires at least 4 arguments")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	dstLabel, _ := t.transpileExpr(call.Args[1])
	separator, _ := t.transpileExpr(call.Args[2])

	var srcParts []string
	for i := 3; i < len(call.Args); i++ {
		srcArg, _ := t.transpileExpr(call.Args[i])
		srcParts = append(srcParts, fmt.Sprintf("%s[%s]", t.lbl(), srcArg))
	}
	concatExpr := fmt.Sprintf("arrayStringConcat([%s], %s)", strings.Join(srcParts, ", "), separator)

	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT
  %s,
  mapSet(%s, %s, %s) AS %s,
  %s
FROM inner`,
			innerSQL,
			t.ts(),
			t.lbl(), dstLabel, concatExpr, t.lbl(),
			t.val()), nil
	}
	return "", fmt.Errorf("label_join() requires a vector expression")
}

// ─── SORT ────────────────────────────────────────────────────────────────────

func (t *Transpiler) transpileSort(call *ast.Call, order string) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("sort() requires exactly 1 argument")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT * FROM inner ORDER BY %s %s`, innerSQL, t.val(), order), nil
	}
	return innerSQL, nil
}

// ─── ABSENT ──────────────────────────────────────────────────────────────────

func (t *Transpiler) transpileAbsent(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("absent() requires exactly 1 argument")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`SELECT 1 AS value
WHERE (SELECT count() FROM (%s)) = 0`, innerSQL), nil
	}
	return "", fmt.Errorf("absent() requires a selector")
}

// ─── CHANGES / RESETS ────────────────────────────────────────────────────────

func (t *Transpiler) transpileChanges(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("changes() requires exactly 1 argument")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("changes() requires a range vector")
	}
	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`WITH base_data AS (
%s
)
SELECT
  %s,
  countIf(%s != lagInFrame(%s, 1, %s) OVER w) AS value
FROM base_data
WINDOW w AS (PARTITION BY %s ORDER BY %s)
GROUP BY %s`,
		baseSQL,
		t.lbl(), t.val(), t.val(), t.val(),
		t.lbl(), t.ts(),
		t.lbl()), nil
}

func (t *Transpiler) transpileResets(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("resets() requires exactly 1 argument")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("resets() requires a range vector")
	}
	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`WITH base_data AS (
%s
)
SELECT
  %s,
  countIf(%s < lagInFrame(%s, 1, %s) OVER w) AS value
FROM base_data
WINDOW w AS (PARTITION BY %s ORDER BY %s)
GROUP BY %s`,
		baseSQL,
		t.lbl(), t.val(), t.val(), t.val(),
		t.lbl(), t.ts(),
		t.lbl()), nil
}

// ─── DERIV / PREDICT_LINEAR ──────────────────────────────────────────────────

func (t *Transpiler) transpileDeriv(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("deriv() requires exactly 1 argument")
	}
	// Handle subquery argument: e.g. deriv(expr[1h:1m])
	if sq, ok := call.Args[0].(*ast.SubqueryExpr); ok {
		innerSQL, err := t.transpileExpr(sq.Expr)
		if err != nil {
			return "", err
		}
		if isSelectQuery(innerSQL) {
			return fmt.Sprintf(`WITH base_data AS (
%s
)
SELECT
  %s,
  (simpleLinearRegression(toUnixTimestamp(%s), %s)).1 AS value
FROM base_data
GROUP BY %s`,
				innerSQL, t.lbl(), t.ts(), t.val(), t.lbl()), nil
		}
		return "", fmt.Errorf("deriv() subquery inner must produce a series")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("deriv() requires a range vector or subquery")
	}
	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}

	// Use ClickHouse's simpleLinearRegression to compute the slope
	return fmt.Sprintf(`WITH base_data AS (
%s
)
SELECT
  %s,
  (simpleLinearRegression(toUnixTimestamp(%s), %s)).1 AS value
FROM base_data
GROUP BY %s`,
		baseSQL,
		t.lbl(),
		t.ts(), t.val(),
		t.lbl()), nil
}

func (t *Transpiler) transpilePredictLinear(call *ast.Call) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("predict_linear() requires 2 arguments")
	}
	tSec, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}
	// Handle subquery argument: e.g. predict_linear(expr[1h:5m], 3600)
	if sq, ok := call.Args[0].(*ast.SubqueryExpr); ok {
		innerSQL, err := t.transpileExpr(sq.Expr)
		if err != nil {
			return "", err
		}
		if isSelectQuery(innerSQL) {
			return fmt.Sprintf(`WITH base_data AS (
%s
),
regression AS (
  SELECT
    %s,
    (simpleLinearRegression(toUnixTimestamp(%s), %s)).1 AS slope,
    (simpleLinearRegression(toUnixTimestamp(%s), %s)).2 AS intercept
  FROM base_data
  GROUP BY %s
)
SELECT
  %s,
  slope * (toUnixTimestamp(now()) + %s) + intercept AS value
FROM regression`,
				innerSQL,
				t.lbl(), t.ts(), t.val(), t.ts(), t.val(), t.lbl(),
				t.lbl(), tSec), nil
		}
		return "", fmt.Errorf("predict_linear() subquery inner must produce a series")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("predict_linear() first argument must be a range vector or subquery")
	}
	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`WITH base_data AS (
%s
),
regression AS (
  SELECT
    %s,
    (simpleLinearRegression(toUnixTimestamp(%s), %s)).1 AS slope,
    (simpleLinearRegression(toUnixTimestamp(%s), %s)).2 AS intercept
  FROM base_data
  GROUP BY %s
)
SELECT
  %s,
  slope * (toUnixTimestamp(now()) + %s) + intercept AS value
FROM regression`,
		baseSQL,
		t.lbl(),
		t.ts(), t.val(),
		t.ts(), t.val(),
		t.lbl(),
		t.lbl(), tSec), nil
}

func (t *Transpiler) transpileHoltWinters(call *ast.Call) (string, error) {
	if len(call.Args) != 3 {
		return "", fmt.Errorf("holt_winters() requires 3 arguments")
	}
	ms, ok := call.Args[0].(*ast.MatrixSelector)
	if !ok {
		return "", fmt.Errorf("holt_winters() first argument must be a range vector")
	}
	sf, err := t.transpileExpr(call.Args[1])
	if err != nil {
		return "", err
	}
	tf, err := t.transpileExpr(call.Args[2])
	if err != nil {
		return "", err
	}
	baseSQL, err := t.transpileMatrixSelector(ms)
	if err != nil {
		return "", err
	}
	_ = sf
	_ = tf

	// Holt-Winters double exponential smoothing — approximation via predict_linear.
	return fmt.Sprintf(`WITH base_data AS (
%s
),
regression AS (
  SELECT
    %s,
    (simpleLinearRegression(toUnixTimestamp(%s), %s)).1 AS slope,
    (simpleLinearRegression(toUnixTimestamp(%s), %s)).2 AS intercept
  FROM base_data
  GROUP BY %s
)
SELECT
  %s,
  slope * toUnixTimestamp(now()) + intercept AS value
FROM regression`,
		baseSQL,
		t.lbl(),
		t.ts(), t.val(),
		t.ts(), t.val(),
		t.lbl(),
		t.lbl()), nil
}

// ─── VECTOR / SCALAR ─────────────────────────────────────────────────────────

func (t *Transpiler) transpileVector(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("vector() requires exactly 1 argument")
	}
	val, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("SELECT now() AS %s, map() AS %s, %s AS %s",
		t.ts(), t.lbl(), val, t.val()), nil
}

func (t *Transpiler) transpileScalar(call *ast.Call) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("scalar() requires exactly 1 argument")
	}
	innerSQL, err := t.transpileExpr(call.Args[0])
	if err != nil {
		return "", err
	}
	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`SELECT %s AS value FROM (%s) LIMIT 1`, t.val(), innerSQL), nil
	}
	return fmt.Sprintf("SELECT %s AS value", innerSQL), nil
}

// ─── AGGREGATION ─────────────────────────────────────────────────────────────

func (t *Transpiler) transpileAggregateExpr(agg *ast.AggregateExpr) (string, error) {
	innerSQL, err := t.transpileExpr(agg.Expr)
	if err != nil {
		return "", err
	}

	// Determine the ClickHouse aggregation function
	var aggFunc string
	switch agg.Op {
	case ast.AggSum:
		aggFunc = "sum"
	case ast.AggMin:
		aggFunc = "min"
	case ast.AggMax:
		aggFunc = "max"
	case ast.AggAvg:
		aggFunc = "avg"
	case ast.AggCount:
		aggFunc = "count"
	case ast.AggStddev:
		aggFunc = "stddevPop"
	case ast.AggStdvar:
		aggFunc = "varPop"
	case ast.AggGroup:
		// group() returns 1 per group
		aggFunc = "any"
	case ast.AggTopK:
		return t.transpileTopBottomK(agg, innerSQL, "DESC")
	case ast.AggBottomK:
		return t.transpileTopBottomK(agg, innerSQL, "ASC")
	case ast.AggQuantile:
		return t.transpileAggQuantile(agg, innerSQL)
	case ast.AggCountValues:
		return t.transpileCountValues(agg, innerSQL)
	default:
		return "", fmt.Errorf("unsupported aggregation operator: %s", agg.Op)
	}

	// Build grouping columns
	var groupByCols []string
	var selectGroupCols []string
	for _, label := range agg.Grouping {
		col := t.labelCol(label)
		groupByCols = append(groupByCols, col)
		selectGroupCols = append(selectGroupCols, fmt.Sprintf("%s AS %s", col, label))
	}

	selectCols := strings.Join(selectGroupCols, ", ")
	if selectCols != "" {
		selectCols += ", "
	}

	groupByClause := ""
	if len(groupByCols) > 0 {
		groupByClause = "GROUP BY " + strings.Join(groupByCols, ", ")
	}

	aggExpr := fmt.Sprintf("%s(%s)", aggFunc, t.val())
	if agg.Op == ast.AggGroup {
		aggExpr = "1"
	}

	if isSelectQuery(innerSQL) {
		return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s%s AS value
FROM inner
%s`,
			innerSQL, selectCols, aggExpr, groupByClause), nil
	}

	// Direct metric query → aggregate inline
	return fmt.Sprintf("SELECT %s%s AS value FROM (%s) %s",
		selectCols, aggExpr, innerSQL, groupByClause), nil
}

func (t *Transpiler) transpileTopBottomK(agg *ast.AggregateExpr, innerSQL, order string) (string, error) {
	if agg.Param == nil {
		return "", fmt.Errorf("%s requires a parameter", agg.Op)
	}
	k, err := t.transpileExpr(agg.Param)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, %s
FROM inner
ORDER BY %s %s
LIMIT %s`,
		innerSQL, t.ts(), t.lbl(), t.val(), t.val(), order, k), nil
}

func (t *Transpiler) transpileAggQuantile(agg *ast.AggregateExpr, innerSQL string) (string, error) {
	if agg.Param == nil {
		return "", fmt.Errorf("quantile requires a parameter")
	}
	phi, err := t.transpileExpr(agg.Param)
	if err != nil {
		return "", err
	}

	var groupByCols []string
	var selectGroupCols []string
	for _, label := range agg.Grouping {
		col := t.labelCol(label)
		groupByCols = append(groupByCols, col)
		selectGroupCols = append(selectGroupCols, fmt.Sprintf("%s AS %s", col, label))
	}

	selectCols := strings.Join(selectGroupCols, ", ")
	if selectCols != "" {
		selectCols += ", "
	}
	groupByClause := ""
	if len(groupByCols) > 0 {
		groupByClause = "GROUP BY " + strings.Join(groupByCols, ", ")
	}

	return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %squantile(%s)(%s) AS value
FROM inner
%s`,
		innerSQL, selectCols, phi, t.val(), groupByClause), nil
}

func (t *Transpiler) transpileCountValues(agg *ast.AggregateExpr, innerSQL string) (string, error) {
	if agg.Param == nil {
		return "", fmt.Errorf("count_values requires a label parameter")
	}
	labelName, err := t.transpileExpr(agg.Param)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT
  toString(%s) AS %s,
  count() AS value
FROM inner
GROUP BY %s`,
		innerSQL, t.val(), labelName, t.val()), nil
}

// ─── BINARY EXPRESSIONS ─────────────────────────────────────────────────────

func (t *Transpiler) transpileBinaryExpr(be *ast.BinaryExpr) (string, error) {
	leftSQL, err := t.transpileExpr(be.Left)
	if err != nil {
		return "", err
	}
	rightSQL, err := t.transpileExpr(be.Right)
	if err != nil {
		return "", err
	}

	_, leftIsScalar := be.Left.(*ast.NumberLiteral)
	_, rightIsScalar := be.Right.(*ast.NumberLiteral)

	op := string(be.Operator)
	// Translate logical operators to SQL
	switch be.Operator {
	case ast.OpAnd:
		return t.transpileLogicalOp(leftSQL, rightSQL, be.Matching, "INNER JOIN")
	case ast.OpOr:
		return t.transpileLogicalOp(leftSQL, rightSQL, be.Matching, "FULL OUTER JOIN")
	case ast.OpUnless:
		return t.transpileLogicalOp(leftSQL, rightSQL, be.Matching, "LEFT ANTI JOIN")
	}

	// ── Scalar on one or both sides ──
	if leftIsScalar && rightIsScalar {
		return fmt.Sprintf("SELECT %s %s %s AS value", leftSQL, op, rightSQL), nil
	}

	if leftIsScalar || rightIsScalar {
		return t.transpileScalarBinaryExpr(be, leftSQL, rightSQL, leftIsScalar)
	}

	// ── Vector × Vector ──
	return t.transpileVectorBinaryExpr(be, leftSQL, rightSQL)
}

func (t *Transpiler) transpileScalarBinaryExpr(be *ast.BinaryExpr, leftSQL, rightSQL string, leftIsScalar bool) (string, error) {
	var baseSQL, scalarVal string
	if leftIsScalar {
		baseSQL = rightSQL
		scalarVal = leftSQL
	} else {
		baseSQL = leftSQL
		scalarVal = rightSQL
	}

	op := string(be.Operator)
	var expr string
	if be.ReturnBool {
		// bool modifier: return 1/0
		if leftIsScalar {
			expr = fmt.Sprintf("if(%s %s %s, 1, 0)", scalarVal, op, t.val())
		} else {
			expr = fmt.Sprintf("if(%s %s %s, 1, 0)", t.val(), op, scalarVal)
		}
	} else if isComparisonOp(be.Operator) {
		// Comparison filters
		if leftIsScalar {
			expr = fmt.Sprintf("if(%s %s %s, %s, NULL)", scalarVal, op, t.val(), t.val())
		} else {
			expr = fmt.Sprintf("if(%s %s %s, %s, NULL)", t.val(), op, scalarVal, t.val())
		}
	} else {
		// Arithmetic
		if leftIsScalar {
			expr = fmt.Sprintf("%s %s %s", scalarVal, op, t.val())
		} else {
			expr = fmt.Sprintf("%s %s %s", t.val(), op, scalarVal)
		}
	}

	if isSelectQuery(baseSQL) {
		return fmt.Sprintf(`WITH base AS (
%s
)
SELECT %s, %s, %s AS value
FROM base`, baseSQL, t.ts(), t.lbl(), expr), nil
	}

	return fmt.Sprintf("SELECT %s AS value FROM (%s)", expr, baseSQL), nil
}

func (t *Transpiler) transpileVectorBinaryExpr(be *ast.BinaryExpr, leftSQL, rightSQL string) (string, error) {
	op := string(be.Operator)

	joinConds := []string{
		fmt.Sprintf("l.%s = r.%s", t.ts(), t.ts()),
	}

	// Handle vector matching: on(...) or ignoring(...)
	if be.Matching != nil {
		for _, label := range be.Matching.On {
			joinConds = append(joinConds,
				fmt.Sprintf("l.%s = r.%s", t.labelCol(label), t.labelCol(label)))
		}
		if len(be.Matching.Ignoring) > 0 {
			// Without explicit on(...), match on ALL labels except the ignored ones
			joinConds = append(joinConds,
				fmt.Sprintf("l.%s = r.%s", t.lbl(), t.lbl()))
		}
	} else {
		// Default: match on all labels
		joinConds = append(joinConds,
			fmt.Sprintf("l.%s = r.%s", t.lbl(), t.lbl()))
	}

	var valueExpr string
	if be.ReturnBool {
		valueExpr = fmt.Sprintf("if(l.%s %s r.%s, 1, 0)", t.val(), op, t.val())
	} else if isComparisonOp(be.Operator) {
		valueExpr = fmt.Sprintf("if(l.%s %s r.%s, l.%s, NULL)", t.val(), op, t.val(), t.val())
	} else {
		valueExpr = fmt.Sprintf("l.%s %s r.%s", t.val(), op, t.val())
	}

	joinType := "INNER JOIN"
	if be.Matching != nil && be.Matching.GroupLeft != nil {
		joinType = "LEFT JOIN"
	} else if be.Matching != nil && be.Matching.GroupRight != nil {
		joinType = "RIGHT JOIN"
	}

	return fmt.Sprintf(`WITH left_query AS (
%s
),
right_query AS (
%s
)
SELECT
  l.%s,
  l.%s,
  %s AS value
FROM left_query AS l
%s right_query AS r
ON %s`,
		leftSQL, rightSQL,
		t.ts(), t.lbl(),
		valueExpr,
		joinType,
		strings.Join(joinConds, " AND ")), nil
}

func (t *Transpiler) transpileLogicalOp(leftSQL, rightSQL string, matching *ast.VectorMatching, joinType string) (string, error) {
	joinConds := []string{
		fmt.Sprintf("l.%s = r.%s", t.ts(), t.ts()),
		fmt.Sprintf("l.%s = r.%s", t.lbl(), t.lbl()),
	}

	if matching != nil {
		for _, label := range matching.On {
			joinConds = append(joinConds,
				fmt.Sprintf("l.%s = r.%s", t.labelCol(label), t.labelCol(label)))
		}
	}

	return fmt.Sprintf(`WITH left_query AS (
%s
),
right_query AS (
%s
)
SELECT l.%s, l.%s, l.%s
FROM left_query AS l
%s right_query AS r
ON %s`,
		leftSQL, rightSQL,
		t.ts(), t.lbl(), t.val(),
		joinType,
		strings.Join(joinConds, " AND ")), nil
}

// ─── UNARY EXPRESSION ────────────────────────────────────────────────────────

func (t *Transpiler) transpileUnaryExpr(ue *ast.UnaryExpr) (string, error) {
	innerSQL, err := t.transpileExpr(ue.Expr)
	if err != nil {
		return "", err
	}

	if ue.Operator == ast.OpUnaryMinus {
		if isSelectQuery(innerSQL) {
			return fmt.Sprintf(`WITH inner AS (
%s
)
SELECT %s, %s, -(%s) AS value
FROM inner`, innerSQL, t.ts(), t.lbl(), t.val()), nil
		}
		return fmt.Sprintf("SELECT -(%s) AS value", innerSQL), nil
	}

	return innerSQL, nil
}

// ─── UTILITY ─────────────────────────────────────────────────────────────────

// isSelectQuery returns true if the SQL looks like a SELECT statement (not a scalar).
func isSelectQuery(sql string) bool {
	trimmed := strings.TrimSpace(sql)
	upper := strings.ToUpper(trimmed)
	return strings.HasPrefix(upper, "SELECT") || strings.HasPrefix(upper, "WITH")
}

// isComparisonOp checks if a BinOp is a comparison.
func isComparisonOp(op ast.BinOp) bool {
	switch op {
	case ast.OpEql, ast.OpNeq, ast.OpLt, ast.OpGt, ast.OpLte, ast.OpGte:
		return true
	}
	return false
}
