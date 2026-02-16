package grafana

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	
	"github.com/shinro/promql-transpiler/pkg/builder"
)

// Pre-compiled regex patterns for performance (CRITICAL-3 fix)
var (
	timeFilterRe      = regexp.MustCompile(`\$__timeFilter\(([^)]+)\)`)
	timeFromRe        = regexp.MustCompile(`\$__timeFrom\(\)`)
	timeToRe          = regexp.MustCompile(`\$__timeTo\(\)`)
	timeGroupRe       = regexp.MustCompile(`\$__timeGroup\(([^,]+),\s*([^)]+)\)`)
	unixEpochFilterRe = regexp.MustCompile(`\$__unixEpochFilter\(([^)]+)\)`)
	unixEpochFromRe   = regexp.MustCompile(`\$__unixEpochFrom\(\)`)
	unixEpochToRe     = regexp.MustCompile(`\$__unixEpochTo\(\)`)
	containsRe        = regexp.MustCompile(`\$__contains\(([^,]+),\s*([^)]+)\)`)
)

// GrafanaVariableProcessor handles Grafana template variables and macros
// Following Strategy Pattern for different variable types
// Following Single Responsibility Principle
type GrafanaVariableProcessor interface {
	ProcessVariables(query string, ctx *GrafanaContext) (string, error)
	ProcessMacros(query string, ctx *GrafanaContext) (string, error)
}

// GrafanaContext holds Grafana-specific context
type GrafanaContext struct {
	Variables     map[string]string
	TimeRange     *TimeRange
	Interval      string
	IntervalMs    int64
	Dashboard     *DashboardInfo
	Panel         *PanelInfo
}

// TimeRange represents Grafana time range
type TimeRange struct {
	From time.Time
	To   time.Time
}

// DashboardInfo holds dashboard metadata
type DashboardInfo struct {
	Name string
	UID  string
	Tags []string
}

// PanelInfo holds panel metadata
type PanelInfo struct {
	ID    int
	Title string
	Type  string
}

// DefaultGrafanaVariableProcessor implements GrafanaVariableProcessor
type DefaultGrafanaVariableProcessor struct {
	sqlBuilder builder.SQLBuilder
}

// NewGrafanaVariableProcessor creates a new Grafana variable processor
func NewGrafanaVariableProcessor(sqlBuilder builder.SQLBuilder) GrafanaVariableProcessor {
	return &DefaultGrafanaVariableProcessor{
		sqlBuilder: sqlBuilder,
	}
}

// ProcessVariables processes Grafana template variables
func (p *DefaultGrafanaVariableProcessor) ProcessVariables(query string, ctx *GrafanaContext) (string, error) {
	result := query
	
	// Process standard Grafana variables
	variableHandlers := map[string]func(*GrafanaContext) string{
		"$__interval":       p.processIntervalVariable,
		"$__interval_ms":    p.processIntervalMsVariable,
		"$__range":          p.processRangeVariable,
		"$__range_s":        p.processRangeSVariable,
		"$__range_ms":       p.processRangeMsVariable,
		"$__from":           p.processFromVariable,
		"$__to":             p.processToVariable,
		"$__dashboard":      p.processDashboardVariable,
		"$__panel":          p.processPanelVariable,
		"$__user":           p.processUserVariable,
		"$__org":            p.processOrgVariable,
	}
	
	// Replace built-in variables
	for varName, handler := range variableHandlers {
		if strings.Contains(result, varName) {
			value := handler(ctx)
			result = strings.ReplaceAll(result, varName, value)
		}
	}
	
	// Process custom variables
	for varName, varValue := range ctx.Variables {
		placeholder := fmt.Sprintf("$%s", varName)
		result = strings.ReplaceAll(result, placeholder, varValue)
		
		// Also handle ${variable} format
		placeholderBraced := fmt.Sprintf("${%s}", varName)
		result = strings.ReplaceAll(result, placeholderBraced, varValue)
	}
	
	// Process multi-value variables (comma-separated)
	result = p.processMultiValueVariables(result, ctx)
	
	return result, nil
}

// ProcessMacros processes Grafana macros
func (p *DefaultGrafanaVariableProcessor) ProcessMacros(query string, ctx *GrafanaContext) (string, error) {
	result := query
	
	// Define macro processors with pre-compiled regex
	macroProcessors := []struct {
		re      *regexp.Regexp
		handler func(string, *GrafanaContext) string
	}{
		{timeFilterRe, p.processTimeFilterMacro},
		{timeFromRe, p.processTimeFromMacro},
		{timeToRe, p.processTimeToMacro},
		{timeGroupRe, p.processTimeGroupMacro},
		{unixEpochFilterRe, p.processUnixEpochFilterMacro},
		{unixEpochFromRe, p.processUnixEpochFromMacro},
		{unixEpochToRe, p.processUnixEpochToMacro},
		{containsRe, p.processContainsMacro},
	}
	
	// Process each macro
	for _, processor := range macroProcessors {
		matches := processor.re.FindAllStringSubmatch(result, -1)
		
		for _, match := range matches {
			replacement := processor.handler(match[0], ctx)
			result = strings.ReplaceAll(result, match[0], replacement)
		}
	}
	
	return result, nil
}

// ============================================================================
// Variable Processors
// ============================================================================

func (p *DefaultGrafanaVariableProcessor) processIntervalVariable(ctx *GrafanaContext) string {
	if ctx.Interval != "" {
		return ctx.Interval
	}
	return "1m" // default
}

func (p *DefaultGrafanaVariableProcessor) processIntervalMsVariable(ctx *GrafanaContext) string {
	if ctx.IntervalMs > 0 {
		return fmt.Sprintf("%d", ctx.IntervalMs)
	}
	return "60000" // default 1 minute
}

func (p *DefaultGrafanaVariableProcessor) processRangeVariable(ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		duration := ctx.TimeRange.To.Sub(ctx.TimeRange.From)
		return formatDuration(duration)
	}
	return "1h"
}

func (p *DefaultGrafanaVariableProcessor) processRangeSVariable(ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		duration := ctx.TimeRange.To.Sub(ctx.TimeRange.From)
		return fmt.Sprintf("%d", int64(duration.Seconds()))
	}
	return "3600"
}

func (p *DefaultGrafanaVariableProcessor) processRangeMsVariable(ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		duration := ctx.TimeRange.To.Sub(ctx.TimeRange.From)
		return fmt.Sprintf("%d", duration.Milliseconds())
	}
	return "3600000"
}

func (p *DefaultGrafanaVariableProcessor) processFromVariable(ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		return fmt.Sprintf("%d", ctx.TimeRange.From.Unix())
	}
	return "0"
}

func (p *DefaultGrafanaVariableProcessor) processToVariable(ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		return fmt.Sprintf("%d", ctx.TimeRange.To.Unix())
	}
	return fmt.Sprintf("%d", time.Now().Unix())
}

func (p *DefaultGrafanaVariableProcessor) processDashboardVariable(ctx *GrafanaContext) string {
	if ctx.Dashboard != nil {
		return ctx.Dashboard.Name
	}
	return "unknown"
}

func (p *DefaultGrafanaVariableProcessor) processPanelVariable(ctx *GrafanaContext) string {
	if ctx.Panel != nil {
		return ctx.Panel.Title
	}
	return "unknown"
}

func (p *DefaultGrafanaVariableProcessor) processUserVariable(ctx *GrafanaContext) string {
	return "admin" // placeholder
}

func (p *DefaultGrafanaVariableProcessor) processOrgVariable(ctx *GrafanaContext) string {
	return "1" // placeholder
}

// ============================================================================
// Macro Processors
// ============================================================================

func (p *DefaultGrafanaVariableProcessor) processTimeFilterMacro(macro string, ctx *GrafanaContext) string {
	// Extract column name from macro using pre-compiled regex
	matches := timeFilterRe.FindStringSubmatch(macro)
	
	if len(matches) < 2 || ctx.TimeRange == nil {
		return "1=1"
	}
	
	column := strings.TrimSpace(matches[1])
	fromUnix := ctx.TimeRange.From.Unix()
	toUnix := ctx.TimeRange.To.Unix()
	
	return fmt.Sprintf("%s >= toDateTime(%d) AND %s <= toDateTime(%d)", 
		column, fromUnix, column, toUnix)
}

func (p *DefaultGrafanaVariableProcessor) processTimeFromMacro(macro string, ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		return fmt.Sprintf("toDateTime(%d)", ctx.TimeRange.From.Unix())
	}
	return "toDateTime(0)"
}

func (p *DefaultGrafanaVariableProcessor) processTimeToMacro(macro string, ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		return fmt.Sprintf("toDateTime(%d)", ctx.TimeRange.To.Unix())
	}
	return "now()"
}

func (p *DefaultGrafanaVariableProcessor) processTimeGroupMacro(macro string, ctx *GrafanaContext) string {
	// Extract column and interval from macro using pre-compiled regex
	matches := timeGroupRe.FindStringSubmatch(macro)
	
	if len(matches) < 3 {
		return "timestamp"
	}
	
	column := strings.TrimSpace(matches[1])
	interval := strings.TrimSpace(matches[2])
	
	// Convert interval to seconds for ClickHouse
	intervalSeconds := parseInterval(interval)
	
	return fmt.Sprintf("toStartOfInterval(%s, INTERVAL %d second)", column, intervalSeconds)
}

func (p *DefaultGrafanaVariableProcessor) processUnixEpochFilterMacro(macro string, ctx *GrafanaContext) string {
	// Extract column name from macro using pre-compiled regex
	matches := unixEpochFilterRe.FindStringSubmatch(macro)
	
	if len(matches) < 2 || ctx.TimeRange == nil {
		return "1=1"
	}
	
	column := strings.TrimSpace(matches[1])
	fromUnix := ctx.TimeRange.From.Unix()
	toUnix := ctx.TimeRange.To.Unix()
	
	return fmt.Sprintf("%s >= %d AND %s <= %d", column, fromUnix, column, toUnix)
}

func (p *DefaultGrafanaVariableProcessor) processUnixEpochFromMacro(macro string, ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		return fmt.Sprintf("%d", ctx.TimeRange.From.Unix())
	}
	return "0"
}

func (p *DefaultGrafanaVariableProcessor) processUnixEpochToMacro(macro string, ctx *GrafanaContext) string {
	if ctx.TimeRange != nil {
		return fmt.Sprintf("%d", ctx.TimeRange.To.Unix())
	}
	return fmt.Sprintf("%d", time.Now().Unix())
}

func (p *DefaultGrafanaVariableProcessor) processContainsMacro(macro string, ctx *GrafanaContext) string {
	// Extract column and values from macro using pre-compiled regex
	matches := containsRe.FindStringSubmatch(macro)
	
	if len(matches) < 3 {
		return "1=1"
	}
	
	column := strings.TrimSpace(matches[1])
	variable := strings.TrimSpace(matches[2])
	
	// Get variable value from context
	value, exists := ctx.Variables[strings.TrimPrefix(variable, "$")]
	if !exists {
		return "1=1"
	}
	
	// Handle multi-value variables
	values := strings.Split(value, ",")
	if len(values) == 1 {
		return fmt.Sprintf("%s = '%s'", column, value)
	}
	
	quotedValues := make([]string, len(values))
	for i, v := range values {
		quotedValues[i] = fmt.Sprintf("'%s'", strings.TrimSpace(v))
	}
	
	return fmt.Sprintf("%s IN (%s)", column, strings.Join(quotedValues, ", "))
}

// ============================================================================
// Helper Functions
// ============================================================================

func (p *DefaultGrafanaVariableProcessor) processMultiValueVariables(query string, ctx *GrafanaContext) string {
	result := query
	
	for varName, varValue := range ctx.Variables {
		// Check if it's a multi-value (contains comma)
		if strings.Contains(varValue, ",") {
			values := strings.Split(varValue, ",")
			quotedValues := make([]string, len(values))
			for i, v := range values {
				quotedValues[i] = fmt.Sprintf("'%s'", strings.TrimSpace(v))
			}
			
			// Replace ${var:csv} format
			csvPlaceholder := fmt.Sprintf("${%s:csv}", varName)
			result = strings.ReplaceAll(result, csvPlaceholder, strings.Join(quotedValues, ","))
			
			// Replace ${var:pipe} format
			pipePlaceholder := fmt.Sprintf("${%s:pipe}", varName)
			pipeValues := make([]string, len(values))
			for i, v := range values {
				pipeValues[i] = strings.TrimSpace(v)
			}
			result = strings.ReplaceAll(result, pipePlaceholder, strings.Join(pipeValues, "|"))
		}
	}
	
	return result
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

func parseInterval(interval string) int64 {
	// Parse Grafana interval format (e.g., "1m", "5m", "1h", "1d")
	interval = strings.TrimSpace(interval)
	if len(interval) < 2 {
		return 60 // default 1 minute
	}
	
	unit := interval[len(interval)-1:]
	valueStr := interval[:len(interval)-1]
	
	var value int64
	fmt.Sscanf(valueStr, "%d", &value)
	
	switch unit {
	case "s":
		return value
	case "m":
		return value * 60
	case "h":
		return value * 3600
	case "d":
		return value * 86400
	case "w":
		return value * 604800
	default:
		return 60
	}
}

// GrafanaPanelQueryBuilder builds ClickHouse queries for different Grafana panel types
// Following Builder Pattern
type GrafanaPanelQueryBuilder interface {
	BuildForGraph(promQL string, ctx *GrafanaContext) (string, error)
	BuildForTable(promQL string, ctx *GrafanaContext) (string, error)
	BuildForSingleStat(promQL string, ctx *GrafanaContext) (string, error)
	BuildForHeatmap(promQL string, ctx *GrafanaContext) (string, error)
}

// DefaultGrafanaPanelQueryBuilder implements GrafanaPanelQueryBuilder
type DefaultGrafanaPanelQueryBuilder struct {
	sqlBuilder builder.SQLBuilder
	varProcessor GrafanaVariableProcessor
}

// NewGrafanaPanelQueryBuilder creates a new panel query builder
func NewGrafanaPanelQueryBuilder(
	sqlBuilder builder.SQLBuilder,
	varProcessor GrafanaVariableProcessor,
) GrafanaPanelQueryBuilder {
	return &DefaultGrafanaPanelQueryBuilder{
		sqlBuilder:   sqlBuilder,
		varProcessor: varProcessor,
	}
}

// BuildForGraph builds query for graph panel
func (b *DefaultGrafanaPanelQueryBuilder) BuildForGraph(promQL string, ctx *GrafanaContext) (string, error) {
	// Process variables and macros
	processed, err := b.varProcessor.ProcessVariables(promQL, ctx)
	if err != nil {
		return "", err
	}
	
	processed, err = b.varProcessor.ProcessMacros(processed, ctx)
	if err != nil {
		return "", err
	}
	
	// For graphs, we need time-series data
	// This would integrate with the main transpiler
	return processed, nil
}

// BuildForTable builds query for table panel
func (b *DefaultGrafanaPanelQueryBuilder) BuildForTable(promQL string, ctx *GrafanaContext) (string, error) {
	// Process variables and macros
	processed, err := b.varProcessor.ProcessVariables(promQL, ctx)
	if err != nil {
		return "", err
	}
	
	processed, err = b.varProcessor.ProcessMacros(processed, ctx)
	if err != nil {
		return "", err
	}
	
	// Tables usually don't need time grouping
	return processed, nil
}

// BuildForSingleStat builds query for singlestat panel
func (b *DefaultGrafanaPanelQueryBuilder) BuildForSingleStat(promQL string, ctx *GrafanaContext) (string, error) {
	// Process variables and macros
	processed, err := b.varProcessor.ProcessVariables(promQL, ctx)
	if err != nil {
		return "", err
	}
	
	processed, err = b.varProcessor.ProcessMacros(processed, ctx)
	if err != nil {
		return "", err
	}
	
	// Single stat needs aggregation - usually latest value or average
	return processed, nil
}

// BuildForHeatmap builds query for heatmap panel
func (b *DefaultGrafanaPanelQueryBuilder) BuildForHeatmap(promQL string, ctx *GrafanaContext) (string, error) {
	// Process variables and macros
	processed, err := b.varProcessor.ProcessVariables(promQL, ctx)
	if err != nil {
		return "", err
	}
	
	processed, err = b.varProcessor.ProcessMacros(processed, ctx)
	if err != nil {
		return "", err
	}
	
	// Heatmaps need histogram buckets
	return processed, nil
}
