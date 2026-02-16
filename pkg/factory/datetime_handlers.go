package factory

import (
	"fmt"
	
	"github.com/shinro/promql-transpiler/pkg/ast"
)

// ============================================================================
// DATE/TIME FUNCTION HANDLERS
// ============================================================================

// DayOfMonthFunctionHandler handles day_of_month()
type DayOfMonthFunctionHandler struct{}

func (h *DayOfMonthFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("day_of_month() requires 1 argument")
	}
	return fmt.Sprintf("toDayOfMonth(%s)", ctx.TimeColumn), nil
}

func (h *DayOfMonthFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *DayOfMonthFunctionHandler) GetOptionalArgs() int { return 0 }

// DayOfWeekFunctionHandler handles day_of_week()
type DayOfWeekFunctionHandler struct{}

func (h *DayOfWeekFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("day_of_week() requires 1 argument")
	}
	return fmt.Sprintf("toDayOfWeek(%s)", ctx.TimeColumn), nil
}

func (h *DayOfWeekFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *DayOfWeekFunctionHandler) GetOptionalArgs() int { return 0 }

// DaysInMonthFunctionHandler handles days_in_month()
type DaysInMonthFunctionHandler struct{}

func (h *DaysInMonthFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("days_in_month() requires 1 argument")
	}
	return fmt.Sprintf("toDaysInMonth(%s)", ctx.TimeColumn), nil
}

func (h *DaysInMonthFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *DaysInMonthFunctionHandler) GetOptionalArgs() int { return 0 }

// HourFunctionHandler handles hour()
type HourFunctionHandler struct{}

func (h *HourFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("hour() requires 1 argument")
	}
	return fmt.Sprintf("toHour(%s)", ctx.TimeColumn), nil
}

func (h *HourFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *HourFunctionHandler) GetOptionalArgs() int { return 0 }

// MinuteFunctionHandler handles minute()
type MinuteFunctionHandler struct{}

func (h *MinuteFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("minute() requires 1 argument")
	}
	return fmt.Sprintf("toMinute(%s)", ctx.TimeColumn), nil
}

func (h *MinuteFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *MinuteFunctionHandler) GetOptionalArgs() int { return 0 }

// MonthFunctionHandler handles month()
type MonthFunctionHandler struct{}

func (h *MonthFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("month() requires 1 argument")
	}
	return fmt.Sprintf("toMonth(%s)", ctx.TimeColumn), nil
}

func (h *MonthFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *MonthFunctionHandler) GetOptionalArgs() int { return 0 }

// YearFunctionHandler handles year()
type YearFunctionHandler struct{}

func (h *YearFunctionHandler) Handle(call *ast.Call, ctx *TranspilationContext) (string, error) {
	if len(call.Args) != 1 {
		return "", fmt.Errorf("year() requires 1 argument")
	}
	return fmt.Sprintf("toYear(%s)", ctx.TimeColumn), nil
}

func (h *YearFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *YearFunctionHandler) GetOptionalArgs() int { return 0 }

// Register these handlers in the factory
func registerDateTimeFunctions(factory *ClickHouseComponentFactory) {
	factory.funcHandlers["day_of_month"] = &DayOfMonthFunctionHandler{}
	factory.funcHandlers["day_of_week"] = &DayOfWeekFunctionHandler{}
	factory.funcHandlers["days_in_month"] = &DaysInMonthFunctionHandler{}
	factory.funcHandlers["hour"] = &HourFunctionHandler{}
	factory.funcHandlers["minute"] = &MinuteFunctionHandler{}
	factory.funcHandlers["month"] = &MonthFunctionHandler{}
	factory.funcHandlers["year"] = &YearFunctionHandler{}
}
