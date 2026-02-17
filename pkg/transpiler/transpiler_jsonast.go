package transpiler

import (
	"fmt"
	"strings"
	"time"

	"github.com/shinro/promql-transpiler/pkg/promapi"
)

// ═══════════════════════════════════════════════════════════════════════════════
// JSON AST Transpiler Methods
// These methods work directly with Prometheus API JSON AST (*promapi.ASTNode)
// ═══════════════════════════════════════════════════════════════════════════════

// transpileVectorSelectorNode transpiles a vectorSelector JSON AST node
func (t *Transpiler) transpileVectorSelectorNode(node *promapi.ASTNode) (string, error) {
	metricName := node.Name

	// Build WHERE conditions from matchers
	var conditions []string
	for _, matcher := range node.Matchers {
		// Skip __name__ matcher as it's handled separately
		if matcher.Name == "__name__" {
			continue
		}

		var condition string
		switch matcher.Type {
		case "=":
			condition = fmt.Sprintf("%s['%s'] = '%s'", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		case "!=":
			condition = fmt.Sprintf("%s['%s'] != '%s'", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		case "=~":
			condition = fmt.Sprintf("match(%s['%s'], '%s')", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		case "!~":
			condition = fmt.Sprintf("NOT match(%s['%s'], '%s')", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		default:
			return "", fmt.Errorf("unsupported matcher type: %s", matcher.Type)
		}
		conditions = append(conditions, condition)
	}

	// Build query
	var sql strings.Builder
	sql.WriteString("WITH base AS (\n")
	sql.WriteString(fmt.Sprintf("SELECT %s, %s, %s\n", t.schema.TimestampColumn, t.schema.ValueColumn, t.schema.LabelsColumn))
	sql.WriteString(fmt.Sprintf("FROM %s\n", t.schema.TableName))

	// PREWHERE for metric name and time range
	if t.timeRange != nil {
		sql.WriteString(fmt.Sprintf("PREWHERE %s = '%s' AND %s >= toDateTime('%s') AND %s <= toDateTime('%s')\n",
			t.schema.MetricNameColumn, metricName,
			t.schema.TimestampColumn, t.timeRange.Start.Format("2006-01-02 15:04:05"),
			t.schema.TimestampColumn, t.timeRange.End.Format("2006-01-02 15:04:05")))
	} else {
		sql.WriteString(fmt.Sprintf("PREWHERE %s = '%s'\n", t.schema.MetricNameColumn, metricName))
	}

	// WHERE for label matchers
	if len(conditions) > 0 {
		sql.WriteString("WHERE " + strings.Join(conditions, " AND ") + "\n")
	}

	sql.WriteString(fmt.Sprintf("ORDER BY %s\n", t.schema.TimestampColumn))
	sql.WriteString(")\n")
	sql.WriteString(fmt.Sprintf("SELECT %s, %s, %s\n", t.schema.TimestampColumn, t.schema.LabelsColumn, t.schema.ValueColumn))
	sql.WriteString("FROM base")

	return sql.String(), nil
}

// transpileMatrixSelectorNode transpiles a matrixSelector JSON AST node
func (t *Transpiler) transpileMatrixSelectorNode(node *promapi.ASTNode) (string, error) {
	// Matrix selector is like vector selector but with a time range
	// The range is in milliseconds
	rangeDuration := time.Duration(node.Range) * time.Millisecond

	metricName := node.Name

	// Build WHERE conditions from matchers
	var conditions []string
	for _, matcher := range node.Matchers {
		if matcher.Name == "__name__" {
			continue
		}

		var condition string
		switch matcher.Type {
		case "=":
			condition = fmt.Sprintf("%s['%s'] = '%s'", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		case "!=":
			condition = fmt.Sprintf("%s['%s'] != '%s'", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		case "=~":
			condition = fmt.Sprintf("match(%s['%s'], '%s')", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		case "!~":
			condition = fmt.Sprintf("NOT match(%s['%s'], '%s')", t.schema.LabelsColumn, matcher.Name, matcher.Value)
		}
		conditions = append(conditions, condition)
	}

	// For matrix selectors, we need to adjust the time range
	var startTime, endTime time.Time
	if t.timeRange != nil {
		endTime = t.timeRange.End
		startTime = endTime.Add(-rangeDuration)
	} else {
		endTime = time.Now()
		startTime = endTime.Add(-rangeDuration)
	}

	var sql strings.Builder
	sql.WriteString(fmt.Sprintf("SELECT %s, %s, %s\n", t.schema.TimestampColumn, t.schema.ValueColumn, t.schema.LabelsColumn))
	sql.WriteString(fmt.Sprintf("FROM %s\n", t.schema.TableName))
	sql.WriteString(fmt.Sprintf("PREWHERE %s = '%s' AND %s >= toDateTime('%s') AND %s <= toDateTime('%s')\n",
		t.schema.MetricNameColumn, metricName,
		t.schema.TimestampColumn, startTime.Format("2006-01-02 15:04:05"),
		t.schema.TimestampColumn, endTime.Format("2006-01-02 15:04:05")))

	if len(conditions) > 0 {
		sql.WriteString("WHERE " + strings.Join(conditions, " AND ") + "\n")
	}

	sql.WriteString(fmt.Sprintf("ORDER BY %s", t.schema.TimestampColumn))

	return sql.String(), nil
}

// transpileSubqueryRangeNode handles subquery range expressions like max_over_time(m[1h])[10m:1m]
func (t *Transpiler) transpileSubqueryRangeNode(node *promapi.ASTNode) (string, error) {
	// A subquery evaluates an expression over a range, producing a matrix
	// For now, we'll transpile the inner expression and treat it similarly to a matrix selector
	if node.Expr == nil {
		return "", fmt.Errorf("subquery range node missing expression")
	}

	// Transpile the inner expression (e.g., max_over_time(m[1h]))
	return t.transpileNode(node.Expr)
}

// transpileCallNode transpiles a function call JSON AST node
func (t *Transpiler) transpileCallNode(node *promapi.ASTNode) (string, error) {
	if node.Func == nil {
		return "", fmt.Errorf("call node missing function definition")
	}

	funcName := node.Func.Name

	switch funcName {
	// Rate/Counter functions
	case "rate", "irate":
		return t.transpileRateNode(node)
	case "increase":
		return t.transpileIncreaseNode(node)
	case "delta", "idelta":
		return t.transpileDeltaNode(node)
	case "resets":
		return t.transpileResetsNode(node)
	case "changes":
		return t.transpileChangesNode(node)
	case "deriv":
		return t.transpileDerivNode(node)
	case "predict_linear":
		return t.transpilePredictLinearNode(node)

	// Over-time aggregation functions
	case "sum_over_time", "avg_over_time", "min_over_time", "max_over_time", "count_over_time":
		return t.transpileOverTimeNode(node)
	case "stddev_over_time":
		return t.transpileStddevOverTimeNode(node)
	case "stdvar_over_time":
		return t.transpileStdvarOverTimeNode(node)
	case "last_over_time":
		return t.transpileLastOverTimeNode(node)
	case "present_over_time":
		return t.transpilePresentOverTimeNode(node)
	case "absent_over_time":
		return t.transpileAbsentOverTimeNode(node)
	case "quantile_over_time":
		return t.transpileQuantileOverTimeNode(node)

	// Math functions
	case "abs":
		return t.transpileMathFunctionNode(node, "abs")
	case "ceil":
		return t.transpileMathFunctionNode(node, "ceil")
	case "floor":
		return t.transpileMathFunctionNode(node, "floor")
	case "round":
		return t.transpileRoundNode(node)
	case "sqrt":
		return t.transpileMathFunctionNode(node, "sqrt")
	case "exp":
		return t.transpileMathFunctionNode(node, "exp")
	case "ln":
		return t.transpileMathFunctionNode(node, "log")
	case "log2":
		return t.transpileMathFunctionNode(node, "log2")
	case "log10":
		return t.transpileMathFunctionNode(node, "log10")
	case "sgn":
		return t.transpileMathFunctionNode(node, "sign")

	// Clamp functions
	case "clamp":
		return t.transpileClampNode(node)
	case "clamp_min":
		return t.transpileClampMinNode(node)
	case "clamp_max":
		return t.transpileClampMaxNode(node)

	// Trigonometric functions
	case "sin":
		return t.transpileMathFunctionNode(node, "sin")
	case "cos":
		return t.transpileMathFunctionNode(node, "cos")
	case "tan":
		return t.transpileMathFunctionNode(node, "tan")
	case "asin":
		return t.transpileMathFunctionNode(node, "asin")
	case "acos":
		return t.transpileMathFunctionNode(node, "acos")
	case "atan":
		return t.transpileMathFunctionNode(node, "atan")
	case "sinh":
		return t.transpileMathFunctionNode(node, "sinh")
	case "cosh":
		return t.transpileMathFunctionNode(node, "cosh")
	case "tanh":
		return t.transpileMathFunctionNode(node, "tanh")
	case "asinh":
		return t.transpileMathFunctionNode(node, "asinh")
	case "acosh":
		return t.transpileMathFunctionNode(node, "acosh")
	case "atanh":
		return t.transpileMathFunctionNode(node, "atanh")
	case "deg":
		return t.transpileMathFunctionNode(node, "degrees")
	case "rad":
		return t.transpileMathFunctionNode(node, "radians")

	// Date/Time functions
	case "day_of_month":
		return t.transpileDateTimeFunctionNode(node, "toDayOfMonth(timestamp)")
	case "day_of_week":
		return t.transpileDateTimeFunctionNode(node, "toDayOfWeek(timestamp)")
	case "day_of_year":
		return t.transpileDateTimeFunctionNode(node, "toDayOfYear(timestamp)")
	case "days_in_month":
		return t.transpileDateTimeFunctionNode(node, "toDayOfMonth(toLastDayOfMonth(timestamp))")
	case "hour":
		return t.transpileDateTimeFunctionNode(node, "toHour(timestamp)")
	case "minute":
		return t.transpileDateTimeFunctionNode(node, "toMinute(timestamp)")
	case "month":
		return t.transpileDateTimeFunctionNode(node, "toMonth(timestamp)")
	case "year":
		return t.transpileDateTimeFunctionNode(node, "toYear(timestamp)")
	case "timestamp":
		return t.transpileDateTimeFunctionNode(node, "toUnixTimestamp(timestamp)")
	case "time":
		return t.transpileTimeNode(node)
	case "pi":
		return t.transpilePiNode(node)

	// Histogram functions
	case "histogram_quantile":
		return t.transpileHistogramQuantileNode(node)

	// Sort functions
	case "sort":
		return t.transpileSortNode(node, "ASC")
	case "sort_desc":
		return t.transpileSortNode(node, "DESC")

	// Label manipulation
	case "label_replace":
		return t.transpileLabelReplaceNode(node)
	case "label_join":
		return t.transpileLabelJoinNode(node)

	// Deprecated/Special functions
	case "holt_winters":
		return t.transpileHoltWintersNode(node)

	// Other functions
	case "vector":
		return t.transpileVectorNode(node)
	case "scalar":
		return t.transpileScalarNode(node)
	case "absent":
		return t.transpileAbsentNode(node)

	default:
		return "", fmt.Errorf("unsupported function: %s", funcName)
	}
}

// transpileRateNode handles rate() and irate()
func (t *Transpiler) transpileRateNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("rate() requires exactly 1 argument")
	}

	// The argument should be a matrix selector OR a subquery
	matrixNode := &node.Args[0]

	var baseSQL string
	var err error

	// Handle different argument types
	switch matrixNode.Type {
	case "matrixSelector":
		baseSQL, err = t.transpileMatrixSelectorNode(matrixNode)
	case "subqueryRange", "subquery":
		// Subquery like max_over_time(m[1h])[10m:1m]
		baseSQL, err = t.transpileSubqueryRangeNode(matrixNode)
	case "call":
		// Call node might contain an over_time aggregation that produces a range vector
		baseSQL, err = t.transpileNode(matrixNode)
	default:
		// Fallback: try to transpile it anyway
		baseSQL, err = t.transpileNode(matrixNode)
	}

	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n),\n")
	sql.WriteString("rate_data AS (\n")
	sql.WriteString("  SELECT\n")
	sql.WriteString("    timestamp,\n")
	sql.WriteString("    value,\n")
	sql.WriteString("    labels,\n")
	sql.WriteString("    lagInFrame(value, 1, 0) OVER w AS prev_value,\n")
	sql.WriteString("    toUnixTimestamp(timestamp) - toUnixTimestamp(lagInFrame(timestamp, 1, timestamp) OVER w) AS time_diff\n")
	sql.WriteString("    FROM base_data\n")
	sql.WriteString("  WINDOW w AS (PARTITION BY labels ORDER BY timestamp)\n")
	sql.WriteString(")\n")
	sql.WriteString("SELECT timestamp, labels, ")
	sql.WriteString("if(time_diff > 0, (if(value < prev_value, value, value - prev_value)) / time_diff, 0) AS value\n")
	sql.WriteString("FROM rate_data\n")
	sql.WriteString("WHERE prev_value IS NOT NULL OR 1=1")

	return sql.String(), nil
}

// transpileIncreaseNode handles increase()
func (t *Transpiler) transpileIncreaseNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("increase() requires exactly 1 argument")
	}

	// Similar to rate but without dividing by time
	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT timestamp, labels, value - lagInFrame(value, 1, 0) OVER (PARTITION BY labels ORDER BY timestamp) AS value\n")
	sql.WriteString("FROM base_data")

	return sql.String(), nil
}

// transpileOverTimeNode handles *_over_time functions
func (t *Transpiler) transpileOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("%s() requires exactly 1 argument", node.Func.Name)
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var aggFunc string
	switch node.Func.Name {
	case "sum_over_time":
		aggFunc = "sum"
	case "avg_over_time":
		aggFunc = "avg"
	case "min_over_time":
		aggFunc = "min"
	case "max_over_time":
		aggFunc = "max"
	case "count_over_time":
		aggFunc = "count"
	default:
		return "", fmt.Errorf("unsupported over_time function: %s", node.Func.Name)
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT labels, %s(value) AS value\n", aggFunc))
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpileDeltaNode handles delta() and idelta()
func (t *Transpiler) transpileDeltaNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("%s() requires exactly 1 argument", node.Func.Name)
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT timestamp, labels, value - lagInFrame(value, 1, value) OVER (PARTITION BY labels ORDER BY timestamp) AS value\n")
	sql.WriteString("FROM base_data")

	return sql.String(), nil
}

// transpileResetsNode handles resets()
func (t *Transpiler) transpileResetsNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("resets() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, countIf(value < lagInFrame(value, 1, value) OVER w) AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("WINDOW w AS (PARTITION BY labels ORDER BY timestamp)\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpileChangesNode handles changes()
func (t *Transpiler) transpileChangesNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("changes() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, countIf(value != lagInFrame(value, 1, value) OVER w) AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("WINDOW w AS (PARTITION BY labels ORDER BY timestamp)\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpileDerivNode handles deriv()
func (t *Transpiler) transpileDerivNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("deriv() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, (simpleLinearRegression(toUnixTimestamp(timestamp), value)).1 AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpilePredictLinearNode handles predict_linear()
func (t *Transpiler) transpilePredictLinearNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 2 {
		return "", fmt.Errorf("predict_linear() requires exactly 2 arguments")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	// Second arg is the time in seconds to predict forward
	predictionTime := node.Args[1].Val

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n),\n")
	sql.WriteString("regression AS (\n")
	sql.WriteString("  SELECT\n")
	sql.WriteString("    labels,\n")
	sql.WriteString("    (simpleLinearRegression(toUnixTimestamp(timestamp), value)).1 AS slope,\n")
	sql.WriteString("    (simpleLinearRegression(toUnixTimestamp(timestamp), value)).2 AS intercept\n")
	sql.WriteString("  FROM base_data\n")
	sql.WriteString("  GROUP BY labels\n")
	sql.WriteString(")\n")
	sql.WriteString("SELECT labels, slope * (toUnixTimestamp(now()) + ")
	sql.WriteString(predictionTime)
	sql.WriteString(") + intercept AS value\n")
	sql.WriteString("FROM regression")

	return sql.String(), nil
}

// transpileStddevOverTimeNode handles stddev_over_time()
func (t *Transpiler) transpileStddevOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("stddev_over_time() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, stddevPop(value) AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpileStdvarOverTimeNode handles stdvar_over_time()
func (t *Transpiler) transpileStdvarOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("stdvar_over_time() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, varPop(value) AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpileLastOverTimeNode handles last_over_time()
func (t *Transpiler) transpileLastOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("last_over_time() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, argMax(value, timestamp) AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpilePresentOverTimeNode handles present_over_time()
func (t *Transpiler) transpilePresentOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("present_over_time() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT labels, 1 AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels\n")
	sql.WriteString("HAVING count(value) > 0")

	return sql.String(), nil
}

// transpileAbsentOverTimeNode handles absent_over_time()
func (t *Transpiler) transpileAbsentOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("absent_over_time() requires exactly 1 argument")
	}

	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("SELECT 1 AS value\n")
	sql.WriteString("WHERE (SELECT count() FROM (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)) = 0")

	return sql.String(), nil
}

// transpileQuantileOverTimeNode handles quantile_over_time()
func (t *Transpiler) transpileQuantileOverTimeNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 2 {
		return "", fmt.Errorf("quantile_over_time() requires exactly 2 arguments")
	}

	quantile := node.Args[0].Val
	matrixNode := &node.Args[1]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT labels, quantile(%s)(value) AS value\n", quantile))
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}

// transpileMathFunctionNode handles single-argument math functions
func (t *Transpiler) transpileMathFunctionNode(node *promapi.ASTNode, chFunc string) (string, error) {
	if len(node.Args) < 1 {
		return "", fmt.Errorf("%s() requires at least 1 argument", node.Func.Name)
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, %s(value) AS value\n", chFunc))
	sql.WriteString("FROM inner")

	return sql.String(), nil
}

// transpileRoundNode handles round()
func (t *Transpiler) transpileRoundNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) < 1 || len(node.Args) > 2 {
		return "", fmt.Errorf("round() requires 1 or 2 arguments")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	precision := "0"
	if len(node.Args) == 2 {
		precision = node.Args[1].Val
	}

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, round(value, %s) AS value\n", precision))
	sql.WriteString("FROM inner")

	return sql.String(), nil
}

// transpileClampNode handles clamp()
func (t *Transpiler) transpileClampNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 3 {
		return "", fmt.Errorf("clamp() requires 3 arguments")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	minVal := node.Args[1].Val
	maxVal := node.Args[2].Val

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, greatest(least(value, %s), %s) AS value\n", maxVal, minVal))
	sql.WriteString("FROM inner")

	return sql.String(), nil
}

// transpileClampMinNode handles clamp_min()
func (t *Transpiler) transpileClampMinNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 2 {
		return "", fmt.Errorf("clamp_min() requires 2 arguments")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	minVal := node.Args[1].Val

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, greatest(value, %s) AS value\n", minVal))
	sql.WriteString("FROM inner")

	return sql.String(), nil
}

// transpileClampMaxNode handles clamp_max()
func (t *Transpiler) transpileClampMaxNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 2 {
		return "", fmt.Errorf("clamp_max() requires 2 arguments")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	maxVal := node.Args[1].Val

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, least(value, %s) AS value\n", maxVal))
	sql.WriteString("FROM inner")

	return sql.String(), nil
}

// transpileDateTimeFunctionNode handles date/time functions
func (t *Transpiler) transpileDateTimeFunctionNode(node *promapi.ASTNode, chExpr string) (string, error) {
	var innerSQL string
	var err error

	if len(node.Args) == 0 {
		// No argument - return the expression directly
		return fmt.Sprintf("SELECT %s AS value", chExpr), nil
	}

	innerSQL, err = t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, %s AS value\n", chExpr))
	sql.WriteString("FROM inner")

	return sql.String(), nil
}

// transpileSortNode handles sort() and sort_desc()
func (t *Transpiler) transpileSortNode(node *promapi.ASTNode, order string) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("sort() requires exactly 1 argument")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")
	sql.WriteString(fmt.Sprintf("SELECT * FROM inner ORDER BY value %s", order))

	return sql.String(), nil
}

// transpileVectorNode handles vector()
func (t *Transpiler) transpileVectorNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("vector() requires exactly 1 argument")
	}

	val := node.Args[0].Val

	return fmt.Sprintf("SELECT now() AS timestamp, map() AS labels, %s AS value", val), nil
}

// transpileScalarNode handles scalar()
func (t *Transpiler) transpileScalarNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("scalar() requires exactly 1 argument")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("SELECT value FROM (\n%s\n) LIMIT 1", innerSQL), nil
}

// transpileAbsentNode handles absent()
func (t *Transpiler) transpileAbsentNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 1 {
		return "", fmt.Errorf("absent() requires exactly 1 argument")
	}

	innerSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("SELECT 1 AS value WHERE (SELECT count() FROM (\n%s\n)) = 0", innerSQL), nil
}

// transpileHistogramQuantileNode handles histogram_quantile()
func (t *Transpiler) transpileHistogramQuantileNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 2 {
		return "", fmt.Errorf("histogram_quantile() requires exactly 2 arguments")
	}

	// First argument is the quantile (numberLiteral)
	quantileNode := &node.Args[0]
	if quantileNode.Type != "numberLiteral" {
		return "", fmt.Errorf("histogram_quantile() first argument must be a number")
	}
	quantile := quantileNode.Val

	// Second argument is typically rate() of a bucket metric
	rateNode := &node.Args[1]
	rateSQL, err := t.transpileNode(rateNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH bucket_data AS (\n")
	sql.WriteString(rateSQL)
	sql.WriteString("\n),\n")
	sql.WriteString("-- Extract numeric bucket boundary from the 'le' label\n")
	sql.WriteString("buckets AS (\n")
	sql.WriteString("  SELECT\n")
	sql.WriteString("    timestamp,\n")
	sql.WriteString("    toFloat64OrNull(labels['le']) AS le,\n")
	sql.WriteString("    value AS bucket_count,\n")
	sql.WriteString("    arrayFilter(x -> x.1 != 'le', mapKeys(labels), mapValues(labels)) AS group_labels\n")
	sql.WriteString("  FROM bucket_data\n")
	sql.WriteString("  WHERE labels['le'] != ''\n")
	sql.WriteString(")\n")
	sql.WriteString("SELECT\n")
	sql.WriteString("  group_labels AS labels,\n")
	sql.WriteString(fmt.Sprintf("  quantileExactWeighted(%s)(le, toUInt64(bucket_count)) AS value\n", quantile))
	sql.WriteString("FROM buckets\n")
	sql.WriteString("WHERE le IS NOT NULL\n")
	sql.WriteString("GROUP BY group_labels")

	return sql.String(), nil
}

// transpileAggregationNode transpiles an aggregation JSON AST node
func (t *Transpiler) transpileAggregationNode(node *promapi.ASTNode) (string, error) {
	// Get the aggregation operator
	aggOp := strings.ToLower(node.Op)

	// Translate to ClickHouse function
	var chFunc string
	var needsParam bool
	var param string

	switch aggOp {
	case "sum":
		chFunc = "sum"
	case "avg":
		chFunc = "avg"
	case "min":
		chFunc = "min"
	case "max":
		chFunc = "max"
	case "count":
		chFunc = "count"
	case "group":
		// group() returns 1 for each group
		chFunc = "count"
	case "stddev":
		chFunc = "stddevPop"
	case "stdvar":
		chFunc = "varPop"
	case "topk", "bottomk":
		if node.Param == nil {
			return "", fmt.Errorf("%s requires a parameter", aggOp)
		}
		paramSQL, err := t.transpileNode(node.Param)
		if err != nil {
			return "", fmt.Errorf("failed to transpile %s parameter: %w", aggOp, err)
		}
		param = paramSQL
		// topk/bottomk don't use aggregation functions, just ORDER BY + LIMIT
		// We'll use 'any' to pass through values
		chFunc = "any"
		needsParam = false
	case "quantile":
		if node.Param == nil {
			return "", fmt.Errorf("quantile requires a parameter")
		}
		needsParam = true
		paramSQL, err := t.transpileNode(node.Param)
		if err != nil {
			return "", fmt.Errorf("failed to transpile quantile parameter: %w", err)
		}
		param = paramSQL
		chFunc = "quantile"
	case "count_values":
		if node.Param == nil {
			return "", fmt.Errorf("count_values requires a label name parameter")
		}
		chFunc = "count"
		// count_values is special - groups by the value itself
	default:
		return "", fmt.Errorf("unsupported aggregation operator: %s", aggOp)
	}

	// Transpile the inner expression
	innerSQL, err := t.transpileNode(node.Expr)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH inner AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")

	// Build the SELECT with grouping
	if len(node.Grouping) > 0 {
		// Group by specific labels
		var labelSelects []string
		for _, label := range node.Grouping {
			labelSelects = append(labelSelects, fmt.Sprintf("labels['%s'] AS %s", label, label))
		}
		sql.WriteString("SELECT " + strings.Join(labelSelects, ", ") + ", ")

		if aggOp == "group" {
			// group() always returns 1
			sql.WriteString("1 AS value\n")
		} else if needsParam {
			sql.WriteString(fmt.Sprintf("%s(%s)(value) AS value\n", chFunc, param))
		} else {
			sql.WriteString(fmt.Sprintf("%s(value) AS value\n", chFunc))
		}

		sql.WriteString("FROM inner\n")
		sql.WriteString("GROUP BY " + strings.Join(node.Grouping, ", "))

		// topk and bottomk use ORDER BY and LIMIT
		if aggOp == "topk" {
			sql.WriteString(fmt.Sprintf("\nORDER BY value DESC LIMIT %s", param))
		} else if aggOp == "bottomk" {
			sql.WriteString(fmt.Sprintf("\nORDER BY value ASC LIMIT %s", param))
		}
	} else {
		// No grouping - aggregate all
		if aggOp == "group" {
			// group() always returns 1
			sql.WriteString("SELECT 1 AS value\n")
		} else if needsParam {
			sql.WriteString(fmt.Sprintf("SELECT %s(%s)(value) AS value\n", chFunc, param))
		} else {
			sql.WriteString(fmt.Sprintf("SELECT %s(value) AS value\n", chFunc))
		}

		sql.WriteString("FROM inner")

		// topk and bottomk use ORDER BY and LIMIT
		if aggOp == "topk" {
			sql.WriteString(fmt.Sprintf("\nORDER BY value DESC LIMIT %s", param))
		} else if aggOp == "bottomk" {
			sql.WriteString(fmt.Sprintf("\nORDER BY value ASC LIMIT %s", param))
		}
	}

	return sql.String(), nil
}

// transpileBinaryExprNode transpiles a binaryExpr JSON AST node
func (t *Transpiler) transpileBinaryExprNode(node *promapi.ASTNode) (string, error) {
	op := node.Op

	// Transpile left and right operands
	leftSQL, err := t.transpileNode(node.LHS)
	if err != nil {
		return "", fmt.Errorf("failed to transpile left operand: %w", err)
	}

	rightSQL, err := t.transpileNode(node.RHS)
	if err != nil {
		return "", fmt.Errorf("failed to transpile right operand: %w", err)
	}

	// For simple operators with scalar or numeric literals
	if node.RHS.Type == "numberLiteral" {
		var sql strings.Builder
		sql.WriteString("WITH base AS (\n")
		sql.WriteString(leftSQL)
		sql.WriteString("\n)\n")

		var condition string
		switch op {
		case ">":
			condition = fmt.Sprintf("if(value > %s, value, NULL)", node.RHS.Val)
		case "<":
			condition = fmt.Sprintf("if(value < %s, value, NULL)", node.RHS.Val)
		case ">=":
			condition = fmt.Sprintf("if(value >= %s, value, NULL)", node.RHS.Val)
		case "<=":
			condition = fmt.Sprintf("if(value <= %s, value, NULL)", node.RHS.Val)
		case "==":
			condition = fmt.Sprintf("if(value = %s, value, NULL)", node.RHS.Val)
		case "!=":
			condition = fmt.Sprintf("if(value != %s, value, NULL)", node.RHS.Val)
		case "+":
			condition = fmt.Sprintf("value + %s", node.RHS.Val)
		case "-":
			condition = fmt.Sprintf("value - %s", node.RHS.Val)
		case "*":
			condition = fmt.Sprintf("value * %s", node.RHS.Val)
		case "/":
			condition = fmt.Sprintf("value / %s", node.RHS.Val)
		case "%":
			condition = fmt.Sprintf("modulo(value, %s)", node.RHS.Val)
		case "^":
			condition = fmt.Sprintf("pow(value, %s)", node.RHS.Val)
		default:
			return "", fmt.Errorf("unsupported binary operator: %s", op)
		}

		sql.WriteString(fmt.Sprintf("SELECT timestamp, labels, %s AS value\n", condition))
		sql.WriteString("FROM base")
		return sql.String(), nil
	}

	// For vector-to-vector operations, use JOIN
	var sql strings.Builder
	sql.WriteString("WITH left_query AS (\n")
	sql.WriteString(leftSQL)
	sql.WriteString("\n),\n")
	sql.WriteString("right_query AS (\n")
	sql.WriteString(rightSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT\n")
	sql.WriteString("  l.timestamp,\n")

	// Handle label selection based on group_left/group_right
	if node.Matching != nil && len(node.Matching.Include) > 0 {
		// group_left or group_right - merge labels from the "many" side
		// The Include field contains labels to include from the "many" side
		// Card field indicates which side is "many": "many-to-one" means left is many
		if node.Matching.Card == "many-to-one" {
			// group_left - include labels from left side (many)
			var includeLabels []string
			for _, label := range node.Matching.Include {
				includeLabels = append(includeLabels, fmt.Sprintf("'%s', l.labels['%s']", label, label))
			}
			sql.WriteString(fmt.Sprintf("  mapUpdate(l.labels, map(%s)) AS labels,\n", strings.Join(includeLabels, ", ")))
		} else {
			// group_right - include labels from right side (many)
			var includeLabels []string
			for _, label := range node.Matching.Include {
				includeLabels = append(includeLabels, fmt.Sprintf("'%s', r.labels['%s']", label, label))
			}
			sql.WriteString(fmt.Sprintf("  mapUpdate(l.labels, map(%s)) AS labels,\n", strings.Join(includeLabels, ", ")))
		}
	} else {
		sql.WriteString("  l.labels,\n")
	}

	switch op {
	case "+":
		sql.WriteString("  l.value + r.value AS value\n")
	case "-":
		sql.WriteString("  l.value - r.value AS value\n")
	case "*":
		sql.WriteString("  l.value * r.value AS value\n")
	case "/":
		sql.WriteString("  l.value / r.value AS value\n")
	case "%":
		sql.WriteString("  modulo(l.value, r.value) AS value\n")
	case "^":
		sql.WriteString("  pow(l.value, r.value) AS value\n")
	case "==":
		sql.WriteString("  if(l.value = r.value, l.value, NULL) AS value\n")
	case "!=":
		sql.WriteString("  if(l.value != r.value, l.value, NULL) AS value\n")
	case ">":
		sql.WriteString("  if(l.value > r.value, l.value, NULL) AS value\n")
	case "<":
		sql.WriteString("  if(l.value < r.value, l.value, NULL) AS value\n")
	case ">=":
		sql.WriteString("  if(l.value >= r.value, l.value, NULL) AS value\n")
	case "<=":
		sql.WriteString("  if(l.value <= r.value, l.value, NULL) AS value\n")
	case "and":
		sql.WriteString("  l.value AS value\n")
	case "or":
		sql.WriteString("  coalesce(l.value, r.value) AS value\n")
	case "unless":
		sql.WriteString("  if(r.value IS NULL, l.value, NULL) AS value\n")
	default:
		return "", fmt.Errorf("unsupported binary operator for vectors: %s", op)
	}

	// Determine JOIN type based on operator
	sql.WriteString("FROM left_query AS l\n")
	if op == "or" || op == "unless" {
		sql.WriteString("LEFT JOIN right_query AS r\n")
	} else if op == "and" {
		sql.WriteString("INNER JOIN right_query AS r\n")
	} else {
		sql.WriteString("INNER JOIN right_query AS r\n")
	}

	// Build JOIN condition based on vector matching rules
	sql.WriteString("ON l.timestamp = r.timestamp")

	if node.Matching != nil && len(node.Matching.Labels) > 0 {
		// Custom label matching with on() or ignoring()
		if node.Matching.On {
			// on(label1, label2, ...) - join only on specified labels
			for _, label := range node.Matching.Labels {
				sql.WriteString(fmt.Sprintf(" AND l.labels['%s'] = r.labels['%s']", label, label))
			}
		} else {
			// ignoring(label1, label2, ...) - join on all labels except specified
			// We need to compare label maps excluding the ignored keys
			// Use mapFilter to remove ignored labels, then compare
			var ignoredLabels []string
			for _, label := range node.Matching.Labels {
				ignoredLabels = append(ignoredLabels, fmt.Sprintf("'%s'", label))
			}

			// Create filtered label maps and compare them
			// mapFilter(map, keys_to_remove) removes specified keys from map
			ignoredList := strings.Join(ignoredLabels, ", ")
			sql.WriteString(fmt.Sprintf(" AND mapFilter((k, v) -> k NOT IN (%s), l.labels) = mapFilter((k, v) -> k NOT IN (%s), r.labels)",
				ignoredList, ignoredList))
		}
	} else {
		// Default: match on all labels
		sql.WriteString(" AND l.labels = r.labels")
	}

	return sql.String(), nil
}

// transpileUnaryExprNode transpiles a unaryExpr JSON AST node
func (t *Transpiler) transpileUnaryExprNode(node *promapi.ASTNode) (string, error) {
	innerSQL, err := t.transpileNode(node.Expr)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base AS (\n")
	sql.WriteString(innerSQL)
	sql.WriteString("\n)\n")

	switch node.Op {
	case "+":
		sql.WriteString("SELECT timestamp, labels, value\nFROM base")
	case "-":
		sql.WriteString("SELECT timestamp, labels, -value AS value\nFROM base")
	default:
		return "", fmt.Errorf("unsupported unary operator: %s", node.Op)
	}

	return sql.String(), nil
}

// transpileTimeNode returns the current Unix timestamp as a scalar
func (t *Transpiler) transpileTimeNode(node *promapi.ASTNode) (string, error) {
	// time() returns the current evaluation time as a scalar
	// In ClickHouse, we can use now() or a fixed time if set
	if t.timeRange != nil {
		return fmt.Sprintf("SELECT %d AS value", t.timeRange.End.Unix()), nil
	}
	return "SELECT toUnixTimestamp(now()) AS value", nil
}

// transpilePiNode returns pi as a scalar
func (t *Transpiler) transpilePiNode(node *promapi.ASTNode) (string, error) {
	return "SELECT pi() AS value", nil
}

// transpileLabelReplaceNode implements label_replace
func (t *Transpiler) transpileLabelReplaceNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) < 5 {
		return "", fmt.Errorf("label_replace requires 5 arguments")
	}

	// label_replace(v instant-vector, dst_label string, replacement string, src_label string, regex string)
	// Transpile the vector argument
	vectorSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	// Extract string arguments (dst_label, replacement, src_label, regex)
	dstLabel := strings.Trim(node.Args[1].Val, "\"")
	replacement := strings.Trim(node.Args[2].Val, "\"")
	srcLabel := strings.Trim(node.Args[3].Val, "\"")
	regex := strings.Trim(node.Args[4].Val, "\"")

	var sql strings.Builder
	sql.WriteString("WITH base AS (\n")
	sql.WriteString(vectorSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT\n")
	sql.WriteString("  timestamp,\n")
	sql.WriteString("  value,\n")
	sql.WriteString(fmt.Sprintf("  mapUpdate(labels, map('%s', replaceRegexpOne(labels['%s'], '%s', '%s'))) AS labels\n",
		dstLabel, srcLabel, regex, replacement))
	sql.WriteString("FROM base")

	return sql.String(), nil
}

// transpileLabelJoinNode implements label_join
func (t *Transpiler) transpileLabelJoinNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) < 4 {
		return "", fmt.Errorf("label_join requires at least 4 arguments")
	}

	// label_join(v instant-vector, dst_label string, separator string, src_label1 string, src_label2 string, ...)
	vectorSQL, err := t.transpileNode(&node.Args[0])
	if err != nil {
		return "", err
	}

	dstLabel := strings.Trim(node.Args[1].Val, "\"")
	separator := strings.Trim(node.Args[2].Val, "\"")

	// Collect source labels
	var srcLabels []string
	for i := 3; i < len(node.Args); i++ {
		srcLabel := strings.Trim(node.Args[i].Val, "\"")
		srcLabels = append(srcLabels, fmt.Sprintf("labels['%s']", srcLabel))
	}

	var sql strings.Builder
	sql.WriteString("WITH base AS (\n")
	sql.WriteString(vectorSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT\n")
	sql.WriteString("  timestamp,\n")
	sql.WriteString("  value,\n")
	sql.WriteString(fmt.Sprintf("  mapUpdate(labels, map('%s', concat(%s))) AS labels\n",
		dstLabel, strings.Join(srcLabels, fmt.Sprintf(", '%s', ", separator))))
	sql.WriteString("FROM base")

	return sql.String(), nil
}

// transpileHoltWintersNode implements holt_winters (deprecated but needed for compatibility)
func (t *Transpiler) transpileHoltWintersNode(node *promapi.ASTNode) (string, error) {
	if len(node.Args) != 3 {
		return "", fmt.Errorf("holt_winters requires exactly 3 arguments")
	}

	// holt_winters(v range-vector, sf scalar, tf scalar)
	// For simplicity, we'll just return the average over time
	matrixNode := &node.Args[0]
	baseSQL, err := t.transpileMatrixSelectorNode(matrixNode)
	if err != nil {
		return "", err
	}

	var sql strings.Builder
	sql.WriteString("WITH base_data AS (\n")
	sql.WriteString(baseSQL)
	sql.WriteString("\n)\n")
	sql.WriteString("SELECT\n")
	sql.WriteString("  max(timestamp) AS timestamp,\n")
	sql.WriteString("  labels,\n")
	sql.WriteString("  avg(value) AS value\n")
	sql.WriteString("FROM base_data\n")
	sql.WriteString("GROUP BY labels")

	return sql.String(), nil
}
