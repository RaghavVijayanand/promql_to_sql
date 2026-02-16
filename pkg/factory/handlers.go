package factory

import (
	"fmt"
	
	"github.com/shinro/promql-transpiler/pkg/ast"
)

// ============================================================================
// AGGREGATION HANDLERS
// ============================================================================

// SumAggregationHandler handles sum() aggregations
type SumAggregationHandler struct{}

func (h *SumAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("sum(%s)", ctx.ValueColumn), nil
}

func (h *SumAggregationHandler) GetClickHouseFunction() string {
	return "sum"
}

// AvgAggregationHandler handles avg() aggregations
type AvgAggregationHandler struct{}

func (h *AvgAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("avg(%s)", ctx.ValueColumn), nil
}

func (h *AvgAggregationHandler) GetClickHouseFunction() string {
	return "avg"
}

// CountAggregationHandler handles count() aggregations
type CountAggregationHandler struct{}

func (h *CountAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("count(%s)", ctx.ValueColumn), nil
}

func (h *CountAggregationHandler) GetClickHouseFunction() string {
	return "count"
}

// MinAggregationHandler handles min() aggregations
type MinAggregationHandler struct{}

func (h *MinAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("min(%s)", ctx.ValueColumn), nil
}

func (h *MinAggregationHandler) GetClickHouseFunction() string {
	return "min"
}

// MaxAggregationHandler handles max() aggregations
type MaxAggregationHandler struct{}

func (h *MaxAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("max(%s)", ctx.ValueColumn), nil
}

func (h *MaxAggregationHandler) GetClickHouseFunction() string {
	return "max"
}

// StddevAggregationHandler handles stddev() aggregations
type StddevAggregationHandler struct{}

func (h *StddevAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("stddevPop(%s)", ctx.ValueColumn), nil
}

func (h *StddevAggregationHandler) GetClickHouseFunction() string {
	return "stddevPop"
}

// StdvarAggregationHandler handles stdvar() aggregations
type StdvarAggregationHandler struct{}

func (h *StdvarAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("varPop(%s)", ctx.ValueColumn), nil
}

func (h *StdvarAggregationHandler) GetClickHouseFunction() string {
	return "varPop"
}

// TopKAggregationHandler handles topk() aggregations
type TopKAggregationHandler struct{}

func (h *TopKAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	if aggExpr.Param == nil {
		return "", fmt.Errorf("topk requires a parameter")
	}
	// ClickHouse topK(N)(column) — two-level call syntax
	return fmt.Sprintf("topK(%s)(%s)", getParamValue(aggExpr.Param), ctx.ValueColumn), nil
}

func (h *TopKAggregationHandler) GetClickHouseFunction() string {
	return "topK"
}

// BottomKAggregationHandler handles bottomk() aggregations
type BottomKAggregationHandler struct{}

func (h *BottomKAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	if aggExpr.Param == nil {
		return "", fmt.Errorf("bottomk requires a parameter")
	}
	// ClickHouse has no bottomK — select the column; caller applies ORDER BY ASC LIMIT N
	return fmt.Sprintf("%s", ctx.ValueColumn), nil
}

func (h *BottomKAggregationHandler) GetClickHouseFunction() string {
	return "topK" // with arrayReverse
}

// QuantileAggregationHandler handles quantile() aggregations
type QuantileAggregationHandler struct{}

func (h *QuantileAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	if aggExpr.Param == nil {
		return "", fmt.Errorf("quantile requires a parameter")
	}
	return fmt.Sprintf("quantile(%s)(%s)", getParamValue(aggExpr.Param), ctx.ValueColumn), nil
}

func (h *QuantileAggregationHandler) GetClickHouseFunction() string {
	return "quantile"
}

// CountValuesAggregationHandler handles count_values() aggregations
type CountValuesAggregationHandler struct{}

func (h *CountValuesAggregationHandler) Handle(aggExpr *ast.AggregateExpr, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("groupArray(%s)", ctx.ValueColumn), nil
}

func (h *CountValuesAggregationHandler) GetClickHouseFunction() string {
	return "groupArray"
}

// ============================================================================
// FUNCTION HANDLERS - Rate/Increase Functions
// ============================================================================

// RateFunctionHandler handles rate()
type RateFunctionHandler struct{}

func (h *RateFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("rate() requires 1 argument")
	}
	
	// rate = (last_value - first_value) / time_range
	sql := fmt.Sprintf(`
		(max(%s) - min(%s)) / 
		(max(toUnixTimestamp(%s)) - min(toUnixTimestamp(%s)))`,
		ctx.ValueColumn, ctx.ValueColumn, ctx.TimeColumn, ctx.TimeColumn)
	
	return sql, nil
}

func (h *RateFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *RateFunctionHandler) GetOptionalArgs() int { return 0 }

// IRateFunctionHandler handles irate() - instant rate
type IRateFunctionHandler struct{}

func (h *IRateFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("irate() requires 1 argument")
	}
	
	// irate uses last two points
	sql := fmt.Sprintf(`
		(arrayElement(groupArray(%s), -1) - arrayElement(groupArray(%s), -2)) / 
		(arrayElement(groupArray(toUnixTimestamp(%s)), -1) - arrayElement(groupArray(toUnixTimestamp(%s)), -2))`,
		ctx.ValueColumn, ctx.ValueColumn, ctx.TimeColumn, ctx.TimeColumn)
	
	return sql, nil
}

func (h *IRateFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *IRateFunctionHandler) GetOptionalArgs() int { return 0 }

// IncreaseFunctionHandler handles increase()
type IncreaseFunctionHandler struct{}

func (h *IncreaseFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("increase() requires 1 argument")
	}
	
	sql := fmt.Sprintf("max(%s) - min(%s)", ctx.ValueColumn, ctx.ValueColumn)
	return sql, nil
}

func (h *IncreaseFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *IncreaseFunctionHandler) GetOptionalArgs() int { return 0 }

// DeltaFunctionHandler handles delta()
type DeltaFunctionHandler struct{}

func (h *DeltaFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("delta() requires 1 argument")
	}
	
	sql := fmt.Sprintf("max(%s) - min(%s)", ctx.ValueColumn, ctx.ValueColumn)
	return sql, nil
}

func (h *DeltaFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *DeltaFunctionHandler) GetOptionalArgs() int { return 0 }

// IDeltaFunctionHandler handles idelta()
type IDeltaFunctionHandler struct{}

func (h *IDeltaFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("idelta() requires 1 argument")
	}
	
	sql := fmt.Sprintf(`
		arrayElement(groupArray(%s), -1) - arrayElement(groupArray(%s), -2)`,
		ctx.ValueColumn, ctx.ValueColumn)
	
	return sql, nil
}

func (h *IDeltaFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *IDeltaFunctionHandler) GetOptionalArgs() int { return 0 }

// DerivFunctionHandler handles deriv()
type DerivFunctionHandler struct{}

func (h *DerivFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("deriv() requires 1 argument")
	}
	
	// Linear regression slope
	sql := fmt.Sprintf(`
		covarPop(toUnixTimestamp(%s), %s) / varPop(toUnixTimestamp(%s))`,
		ctx.TimeColumn, ctx.ValueColumn, ctx.TimeColumn)
	
	return sql, nil
}

func (h *DerivFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *DerivFunctionHandler) GetOptionalArgs() int { return 0 }

// PredictLinearFunctionHandler handles predict_linear()
type PredictLinearFunctionHandler struct{}

func (h *PredictLinearFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("predict_linear() requires 2 arguments")
	}
	
	// predict_linear(v, t) = slope * t + intercept
	sql := fmt.Sprintf(`
		(covarPop(toUnixTimestamp(%s), %s) / varPop(toUnixTimestamp(%s))) * %s + 
		(avg(%s) - (covarPop(toUnixTimestamp(%s), %s) / varPop(toUnixTimestamp(%s))) * avg(toUnixTimestamp(%s)))`,
		ctx.TimeColumn, ctx.ValueColumn, ctx.TimeColumn, "t", // t is the prediction time parameter
		ctx.ValueColumn, ctx.TimeColumn, ctx.ValueColumn, ctx.TimeColumn, ctx.TimeColumn)
	
	return sql, nil
}

func (h *PredictLinearFunctionHandler) GetRequiredArgs() int { return 2 }
func (h *PredictLinearFunctionHandler) GetOptionalArgs() int { return 0 }

// HistogramQuantileFunctionHandler handles histogram_quantile()
type HistogramQuantileFunctionHandler struct{}

func (h *HistogramQuantileFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("histogram_quantile() requires 2 arguments")
	}
	// ClickHouse quantile()() syntax with actual phi parameter
	phi := getParamValue(call.Args[0])
	return fmt.Sprintf("quantile(%s)(%s)", phi, ctx.ValueColumn), nil
}

func (h *HistogramQuantileFunctionHandler) GetRequiredArgs() int { return 2 }
func (h *HistogramQuantileFunctionHandler) GetOptionalArgs() int { return 0 }

// ============================================================================
// FUNCTION HANDLERS - Time Functions
// ============================================================================

// TimeFunctionHandler handles time()
type TimeFunctionHandler struct{}

func (h *TimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return "toUnixTimestamp(now())", nil
}

func (h *TimeFunctionHandler) GetRequiredArgs() int { return 0 }
func (h *TimeFunctionHandler) GetOptionalArgs() int { return 0 }

// TimestampFunctionHandler handles timestamp()
type TimestampFunctionHandler struct{}

func (h *TimestampFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("timestamp() requires 1 argument")
	}
	
	return fmt.Sprintf("toUnixTimestamp(%s)", ctx.TimeColumn), nil
}

func (h *TimestampFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *TimestampFunctionHandler) GetOptionalArgs() int { return 0 }

// ============================================================================
// FUNCTION HANDLERS - Math Functions
// ============================================================================

// AbsFunctionHandler handles abs()
type AbsFunctionHandler struct{}

func (h *AbsFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("abs() requires 1 argument")
	}
	return fmt.Sprintf("abs(%s)", ctx.ValueColumn), nil
}

func (h *AbsFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *AbsFunctionHandler) GetOptionalArgs() int { return 0 }

// CeilFunctionHandler handles ceil()
type CeilFunctionHandler struct{}

func (h *CeilFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("ceil() requires 1 argument")
	}
	return fmt.Sprintf("ceil(%s)", ctx.ValueColumn), nil
}

func (h *CeilFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *CeilFunctionHandler) GetOptionalArgs() int { return 0 }

// FloorFunctionHandler handles floor()
type FloorFunctionHandler struct{}

func (h *FloorFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("floor() requires 1 argument")
	}
	return fmt.Sprintf("floor(%s)", ctx.ValueColumn), nil
}

func (h *FloorFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *FloorFunctionHandler) GetOptionalArgs() int { return 0 }

// RoundFunctionHandler handles round()
type RoundFunctionHandler struct{}

func (h *RoundFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) < 1 || len(call.Args) > 2 {
		return "", fmt.Errorf("round() requires 1-2 arguments")
	}
	
	if len(call.Args) == 2 {
		// ClickHouse round(x, N) — extract precision from actual argument
		precision := getParamValue(call.Args[1])
		return fmt.Sprintf("round(%s, %s)", ctx.ValueColumn, precision), nil
	}
	return fmt.Sprintf("round(%s)", ctx.ValueColumn), nil
}

func (h *RoundFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *RoundFunctionHandler) GetOptionalArgs() int { return 1 }

// SqrtFunctionHandler handles sqrt()
type SqrtFunctionHandler struct{}

func (h *SqrtFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("sqrt() requires 1 argument")
	}
	return fmt.Sprintf("sqrt(%s)", ctx.ValueColumn), nil
}

func (h *SqrtFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *SqrtFunctionHandler) GetOptionalArgs() int { return 0 }

// ExpFunctionHandler handles exp()
type ExpFunctionHandler struct{}

func (h *ExpFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("exp() requires 1 argument")
	}
	return fmt.Sprintf("exp(%s)", ctx.ValueColumn), nil
}

func (h *ExpFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *ExpFunctionHandler) GetOptionalArgs() int { return 0 }

// LnFunctionHandler handles ln()
type LnFunctionHandler struct{}

func (h *LnFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("ln() requires 1 argument")
	}
	return fmt.Sprintf("log(%s)", ctx.ValueColumn), nil
}

func (h *LnFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *LnFunctionHandler) GetOptionalArgs() int { return 0 }

// Log2FunctionHandler handles log2()
type Log2FunctionHandler struct{}

func (h *Log2FunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("log2() requires 1 argument")
	}
	return fmt.Sprintf("log2(%s)", ctx.ValueColumn), nil
}

func (h *Log2FunctionHandler) GetRequiredArgs() int { return 1 }
func (h *Log2FunctionHandler) GetOptionalArgs() int { return 0 }

// Log10FunctionHandler handles log10()
type Log10FunctionHandler struct{}

func (h *Log10FunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("log10() requires 1 argument")
	}
	return fmt.Sprintf("log10(%s)", ctx.ValueColumn), nil
}

func (h *Log10FunctionHandler) GetRequiredArgs() int { return 1 }
func (h *Log10FunctionHandler) GetOptionalArgs() int { return 0 }

// Helper function to get parameter value
func getParamValue(param ast.Expr) string {
	switch p := param.(type) {
	case *ast.NumberLiteral:
		return fmt.Sprintf("%f", p.Value)
	case *ast.StringLiteral:
		return fmt.Sprintf("'%s'", p.Value)
	default:
		return "1"
	}
}

// Placeholder handlers for remaining functions
// These would need full implementations

type AvgOverTimeFunctionHandler struct{}
func (h *AvgOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("avg(%s)", ctx.ValueColumn), nil
}
func (h *AvgOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *AvgOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type MinOverTimeFunctionHandler struct{}
func (h *MinOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("min(%s)", ctx.ValueColumn), nil
}
func (h *MinOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *MinOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type MaxOverTimeFunctionHandler struct{}
func (h *MaxOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("max(%s)", ctx.ValueColumn), nil
}
func (h *MaxOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *MaxOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type SumOverTimeFunctionHandler struct{}
func (h *SumOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("sum(%s)", ctx.ValueColumn), nil
}
func (h *SumOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *SumOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type CountOverTimeFunctionHandler struct{}
func (h *CountOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("count(%s)", ctx.ValueColumn), nil
}
func (h *CountOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *CountOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type QuantileOverTimeFunctionHandler struct{}
func (h *QuantileOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ClickHouse quantile()() syntax — extract phi from first arg
	phi := "0.95" // default
	if len(call.Args) >= 1 {
		phi = getParamValue(call.Args[0])
	}
	return fmt.Sprintf("quantile(%s)(%s)", phi, ctx.ValueColumn), nil
}
func (h *QuantileOverTimeFunctionHandler) GetRequiredArgs() int { return 2 }
func (h *QuantileOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type StddevOverTimeFunctionHandler struct{}
func (h *StddevOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("stddevPop(%s)", ctx.ValueColumn), nil
}
func (h *StddevOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *StddevOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type StdvarOverTimeFunctionHandler struct{}
func (h *StdvarOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return fmt.Sprintf("varPop(%s)", ctx.ValueColumn), nil
}
func (h *StdvarOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *StdvarOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type LabelReplaceFunctionHandler struct{}
func (h *LabelReplaceFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return "", fmt.Errorf("label_replace not yet implemented")
}
func (h *LabelReplaceFunctionHandler) GetRequiredArgs() int { return 5 }
func (h *LabelReplaceFunctionHandler) GetOptionalArgs() int { return 0 }

type LabelJoinFunctionHandler struct{}
func (h *LabelJoinFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	return "", fmt.Errorf("label_join not yet implemented")
}
func (h *LabelJoinFunctionHandler) GetRequiredArgs() int { return 3 }
func (h *LabelJoinFunctionHandler) GetOptionalArgs() int { return 1 }

type VectorFunctionHandler struct{}
func (h *VectorFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("vector() requires 1 argument")
	}
	return getParamValue(call.Args[0]), nil
}
func (h *VectorFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *VectorFunctionHandler) GetOptionalArgs() int { return 0 }

type ScalarFunctionHandler struct{}
func (h *ScalarFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("scalar() requires 1 argument")
	}
	// ClickHouse any() returns an arbitrary single value from the group
	return fmt.Sprintf("any(%s)", ctx.ValueColumn), nil
}
func (h *ScalarFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *ScalarFunctionHandler) GetOptionalArgs() int { return 0 }

type SortFunctionHandler struct{}
func (h *SortFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// sort() is a pass-through — the ORDER BY is applied at query level
	return ctx.ValueColumn, nil
}
func (h *SortFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *SortFunctionHandler) GetOptionalArgs() int { return 0 }

type SortDescFunctionHandler struct{}
func (h *SortDescFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// sort_desc() is a pass-through — the ORDER BY DESC is applied at query level
	return ctx.ValueColumn, nil
}
func (h *SortDescFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *SortDescFunctionHandler) GetOptionalArgs() int { return 0 }

type ClampFunctionHandler struct{}
func (h *ClampFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 3 {
		return "", fmt.Errorf("clamp() requires 3 arguments")
	}
	// ClickHouse has no clamp() — emulate with greatest(least(v, max), min)
	minVal := getParamValue(call.Args[1])
	maxVal := getParamValue(call.Args[2])
	return fmt.Sprintf("greatest(least(%s, %s), %s)", ctx.ValueColumn, maxVal, minVal), nil
}
func (h *ClampFunctionHandler) GetRequiredArgs() int { return 3 }
func (h *ClampFunctionHandler) GetOptionalArgs() int { return 0 }

type ClampMaxFunctionHandler struct{}
func (h *ClampMaxFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("clamp_max() requires 2 arguments")
	}
	maxVal := getParamValue(call.Args[1])
	return fmt.Sprintf("least(%s, %s)", ctx.ValueColumn, maxVal), nil
}
func (h *ClampMaxFunctionHandler) GetRequiredArgs() int { return 2 }
func (h *ClampMaxFunctionHandler) GetOptionalArgs() int { return 0 }

type ClampMinFunctionHandler struct{}
func (h *ClampMinFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 2 {
		return "", fmt.Errorf("clamp_min() requires 2 arguments")
	}
	minVal := getParamValue(call.Args[1])
	return fmt.Sprintf("greatest(%s, %s)", ctx.ValueColumn, minVal), nil
}
func (h *ClampMinFunctionHandler) GetRequiredArgs() int { return 2 }
func (h *ClampMinFunctionHandler) GetOptionalArgs() int { return 0 }

type AbsentFunctionHandler struct{}
func (h *AbsentFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ClickHouse: returns 1 if no rows match, 0 otherwise
	return fmt.Sprintf("if(count(%s) = 0, 1, 0)", ctx.ValueColumn), nil
}
func (h *AbsentFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *AbsentFunctionHandler) GetOptionalArgs() int { return 0 }

type AbsentOverTimeFunctionHandler struct{}
func (h *AbsentOverTimeFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ClickHouse: returns 1 if no rows in the time range, 0 otherwise
	return fmt.Sprintf("if(count(%s) = 0, 1, 0)", ctx.ValueColumn), nil
}
func (h *AbsentOverTimeFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *AbsentOverTimeFunctionHandler) GetOptionalArgs() int { return 0 }

type ChangesFunctionHandler struct{}
func (h *ChangesFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ClickHouse: count transitions where value differs from previous row
	return fmt.Sprintf("countIf(%s != lagInFrame(%s, 1, %s) OVER (ORDER BY %s))",
		ctx.ValueColumn, ctx.ValueColumn, ctx.ValueColumn, ctx.TimeColumn), nil
}
func (h *ChangesFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *ChangesFunctionHandler) GetOptionalArgs() int { return 0 }

type ResetsFunctionHandler struct{}
func (h *ResetsFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ClickHouse uses lagInFrame() not lag() for window functions
	return fmt.Sprintf("countIf(%s < lagInFrame(%s, 1, 0) OVER (ORDER BY %s))",
		ctx.ValueColumn, ctx.ValueColumn, ctx.TimeColumn), nil
}
func (h *ResetsFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *ResetsFunctionHandler) GetOptionalArgs() int { return 0 }

// ============================================================================
// ALERTMANAGER SPECIFIC HANDLERS
// ============================================================================

type AlertsFunctionHandler struct{}
func (h *AlertsFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ALERTS is a special metric in Alertmanager
	// Returns currently active alerts
	builder := ctx.Builder
	builder.Reset()
	
	builder.Select("alertname", "alertstate", "labels", "value").
		From("alerts").
		Where("alertstate = 'firing'")
	
	return builder.Build(), nil
}
func (h *AlertsFunctionHandler) GetRequiredArgs() int { return 0 }
func (h *AlertsFunctionHandler) GetOptionalArgs() int { return 0 }

type AlertsForStateFunctionHandler struct{}
func (h *AlertsForStateFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	// ALERTS_FOR_STATE returns alerts in specific state
	if len(call.Args) < 1 {
		return "", fmt.Errorf("ALERTS_FOR_STATE requires at least 1 argument")
	}
	
	builder := ctx.Builder
	builder.Reset()
	
	state := "firing" // default, would extract from args
	builder.Select("alertname", "alertstate", "labels", "value", "duration").
		From("alerts").
		Where(fmt.Sprintf("alertstate = '%s'", state))
	
	return builder.Build(), nil
}
func (h *AlertsForStateFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *AlertsForStateFunctionHandler) GetOptionalArgs() int { return 1 }
