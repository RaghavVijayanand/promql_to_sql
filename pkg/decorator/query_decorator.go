package decorator

import (
	"fmt"
	"strings"
	
	"github.com/shinro/promql-transpiler/pkg/builder"
)

// QueryDecorator implements the Decorator Pattern
// Allows adding features to queries dynamically without modifying the base query builder
// Following Open/Closed Principle - open for extension, closed for modification
type QueryDecorator interface {
	Decorate(query string) string
	SetNext(decorator QueryDecorator) QueryDecorator
}

// BaseDecorator provides common functionality for all decorators
type BaseDecorator struct {
	next QueryDecorator
}

func (d *BaseDecorator) SetNext(decorator QueryDecorator) QueryDecorator {
	d.next = decorator
	return decorator
}

func (d *BaseDecorator) callNext(query string) string {
	if d.next != nil {
		return d.next.Decorate(query)
	}
	return query
}

// ============================================================================
// SAMPLING DECORATOR
// ============================================================================

// SamplingDecorator adds SAMPLE clause for performance optimization
type SamplingDecorator struct {
	BaseDecorator
	ratio float64
}

// NewSamplingDecorator creates a new sampling decorator
func NewSamplingDecorator(ratio float64) *SamplingDecorator {
	return &SamplingDecorator{
		ratio: ratio,
	}
}

// Decorate adds SAMPLE clause to the query
func (d *SamplingDecorator) Decorate(query string) string {
	if d.ratio <= 0 || d.ratio >= 1 {
		return d.callNext(query)
	}

	// Find "FROM <table>" using a case-sensitive search for the SQL keyword.
	// We look for "\nFROM " or " FROM " to avoid matching column names that
	// happen to contain "FROM".
	fromIdx := -1
	for _, prefix := range []string{"\nFROM ", " FROM "} {
		idx := strings.Index(query, prefix)
		if idx != -1 {
			fromIdx = idx + len(prefix)
			break
		}
	}
	// Fallback: query starts with "FROM "
	if fromIdx == -1 && strings.HasPrefix(query, "FROM ") {
		fromIdx = 5
	}
	if fromIdx == -1 {
		return d.callNext(query)
	}

	// Extract the table name (first whitespace-delimited token after FROM)
	rest := query[fromIdx:]
	spaceIdx := strings.IndexAny(rest, " \n\t")
	var tableName string
	if spaceIdx == -1 {
		tableName = rest
	} else {
		tableName = rest[:spaceIdx]
	}
	if tableName == "" {
		return d.callNext(query)
	}

	sampled := fmt.Sprintf("%s SAMPLE %.4f", tableName, d.ratio)
	query = query[:fromIdx] + sampled + query[fromIdx+len(tableName):]

	return d.callNext(query)
}

// ============================================================================
// OPTIMIZATION DECORATOR
// ============================================================================

// OptimizationDecorator adds query optimization hints
type OptimizationDecorator struct {
	BaseDecorator
	hints []string
}

// NewOptimizationDecorator creates a new optimization decorator
func NewOptimizationDecorator(hints ...string) *OptimizationDecorator {
	return &OptimizationDecorator{
		hints: hints,
	}
}

// Decorate adds optimization hints to the query
func (d *OptimizationDecorator) Decorate(query string) string {
	if len(d.hints) == 0 {
		return d.callNext(query)
	}
	
	// Add hints as comments at the beginning
	hintComments := make([]string, len(d.hints))
	for i, hint := range d.hints {
		hintComments[i] = fmt.Sprintf("-- HINT: %s", hint)
	}
	
	decorated := strings.Join(hintComments, "\n") + "\n" + query
	
	return d.callNext(decorated)
}

// ============================================================================
// CACHING DECORATOR
// ============================================================================

// CachingDecorator adds query result cache settings
type CachingDecorator struct {
	BaseDecorator
	ttlSeconds int
}

// NewCachingDecorator creates a new caching decorator
func NewCachingDecorator(ttlSeconds int) *CachingDecorator {
	return &CachingDecorator{
		ttlSeconds: ttlSeconds,
	}
}

// Decorate adds caching settings to the query
func (d *CachingDecorator) Decorate(query string) string {
	if d.ttlSeconds > 0 {
		query = appendSetting(query, "use_query_cache = 1")
		query = appendSetting(query, fmt.Sprintf("query_cache_ttl = %d", d.ttlSeconds))
	}

	return d.callNext(query)
}

// ============================================================================
// PREWHERE DECORATOR
// ============================================================================

// PrewhereDecorator converts WHERE to PREWHERE for better performance
type PrewhereDecorator struct {
	BaseDecorator
	columns []string
}

// NewPrewhereDecorator creates a new PREWHERE decorator
func NewPrewhereDecorator(columns ...string) *PrewhereDecorator {
	return &PrewhereDecorator{
		columns: columns,
	}
}

// Decorate converts WHERE conditions on specified columns to PREWHERE.
// This is a legacy decorator — prefer using EngineOptimizer's deterministic
// PREWHERE promotion for new code.
func (d *PrewhereDecorator) Decorate(query string) string {
	if len(d.columns) == 0 || strings.Contains(query, "PREWHERE") {
		return d.callNext(query)
	}

	// Only promote when the WHERE clause actually references one of our columns
	whereIdx := strings.Index(query, "\nWHERE ")
	if whereIdx == -1 && strings.HasPrefix(query, "WHERE ") {
		whereIdx = 0
	}
	if whereIdx == -1 {
		// Also try " WHERE "
		whereIdx = strings.Index(query, " WHERE ")
		if whereIdx != -1 {
			whereIdx++ // point at 'W'
		}
	}
	if whereIdx == -1 {
		return d.callNext(query)
	}

	// Advance past any leading newline
	if query[whereIdx] == '\n' {
		whereIdx++
	}

	for _, col := range d.columns {
		afterWhere := query[whereIdx+6:] // skip "WHERE "
		if strings.Contains(afterWhere, col) {
			query = query[:whereIdx] + "PREWHERE" + query[whereIdx+5:] // replace "WHERE" (5 chars) with "PREWHERE" (8 chars)
			break
		}
	}

	return d.callNext(query)
}

// ============================================================================
// LIMIT DECORATOR
// ============================================================================

// LimitDecorator adds or modifies LIMIT clause
type LimitDecorator struct {
	BaseDecorator
	limit int
}

// NewLimitDecorator creates a new limit decorator
func NewLimitDecorator(limit int) *LimitDecorator {
	return &LimitDecorator{
		limit: limit,
	}
}

// Decorate adds or modifies LIMIT clause
func (d *LimitDecorator) Decorate(query string) string {
	if d.limit <= 0 {
		return d.callNext(query)
	}
	
	// Check if LIMIT already exists
	if strings.Contains(query, "LIMIT") {
		// Don't override existing LIMIT
		return d.callNext(query)
	}
	
	// Add LIMIT at the end
	query = query + fmt.Sprintf("\nLIMIT %d", d.limit)
	
	return d.callNext(query)
}

// ============================================================================
// FORMAT DECORATOR
// ============================================================================

// FormatDecorator adds output format specification
type FormatDecorator struct {
	BaseDecorator
	format string
}

// NewFormatDecorator creates a new format decorator
func NewFormatDecorator(format string) *FormatDecorator {
	return &FormatDecorator{
		format: format,
	}
}

// Decorate adds FORMAT clause
func (d *FormatDecorator) Decorate(query string) string {
	if d.format == "" {
		return d.callNext(query)
	}
	
	// Add FORMAT at the end
	query = query + fmt.Sprintf("\nFORMAT %s", d.format)
	
	return d.callNext(query)
}

// ============================================================================
// TIMEOUT DECORATOR
// ============================================================================

// TimeoutDecorator adds query timeout settings
type TimeoutDecorator struct {
	BaseDecorator
	timeoutSeconds int
}

// NewTimeoutDecorator creates a new timeout decorator
func NewTimeoutDecorator(timeoutSeconds int) *TimeoutDecorator {
	return &TimeoutDecorator{
		timeoutSeconds: timeoutSeconds,
	}
}

// Decorate adds timeout settings
func (d *TimeoutDecorator) Decorate(query string) string {
	if d.timeoutSeconds <= 0 {
		return d.callNext(query)
	}

	setting := fmt.Sprintf("max_execution_time = %d", d.timeoutSeconds)
	query = appendSetting(query, setting)

	return d.callNext(query)
}

// ============================================================================
// COMMENT DECORATOR
// ============================================================================

// CommentDecorator adds descriptive comments to queries
type CommentDecorator struct {
	BaseDecorator
	comment string
}

// NewCommentDecorator creates a new comment decorator
func NewCommentDecorator(comment string) *CommentDecorator {
	return &CommentDecorator{
		comment: comment,
	}
}

// Decorate adds a comment to the query
func (d *CommentDecorator) Decorate(query string) string {
	if d.comment == "" {
		return d.callNext(query)
	}
	
	comment := fmt.Sprintf("/* %s */\n", d.comment)
	query = comment + query
	
	return d.callNext(query)
}

// ============================================================================
// SETTINGS HELPER
// ============================================================================

// appendSetting safely appends a key=value setting to the query's SETTINGS
// clause. If SETTINGS already exists, the setting is appended with a comma.
// If no SETTINGS clause exists, a new one is added at the end.
func appendSetting(query, setting string) string {
	settingsIdx := strings.Index(query, "SETTINGS ")
	if settingsIdx != -1 {
		// Check if this setting key is already present
		settingKey := strings.SplitN(setting, " = ", 2)[0]
		if strings.Contains(query[settingsIdx:], settingKey) {
			return query
		}
		// Insert after "SETTINGS " prefix
		insertPos := settingsIdx + 9 // len("SETTINGS ")
		return query[:insertPos] + setting + ", " + query[insertPos:]
	}
	return strings.TrimRight(query, " \n\t") + "\nSETTINGS " + setting
}

// ============================================================================
// BUILDER INTEGRATION
// ============================================================================

// DecoratedSQLBuilder wraps a SQLBuilder with decorators
type DecoratedSQLBuilder struct {
	builder    builder.SQLBuilder
	decorators []QueryDecorator
}

// NewDecoratedSQLBuilder creates a new decorated SQL builder
func NewDecoratedSQLBuilder(builder builder.SQLBuilder) *DecoratedSQLBuilder {
	return &DecoratedSQLBuilder{
		builder:    builder,
		decorators: make([]QueryDecorator, 0),
	}
}

// AddDecorator adds a decorator to the chain
func (b *DecoratedSQLBuilder) AddDecorator(decorator QueryDecorator) *DecoratedSQLBuilder {
	b.decorators = append(b.decorators, decorator)
	return b
}

// Build builds the query and applies all decorators
func (b *DecoratedSQLBuilder) Build() string {
	query := b.builder.Build()
	
	// Apply decorators in order
	for _, decorator := range b.decorators {
		query = decorator.Decorate(query)
	}
	
	return query
}

// Delegate all other methods to the wrapped builder
func (b *DecoratedSQLBuilder) Select(columns ...string) builder.SQLBuilder {
	b.builder.Select(columns...)
	return b
}

func (b *DecoratedSQLBuilder) From(table string) builder.SQLBuilder {
	b.builder.From(table)
	return b
}

func (b *DecoratedSQLBuilder) Where(condition string) builder.SQLBuilder {
	b.builder.Where(condition)
	return b
}

func (b *DecoratedSQLBuilder) And(condition string) builder.SQLBuilder {
	b.builder.And(condition)
	return b
}

func (b *DecoratedSQLBuilder) Or(condition string) builder.SQLBuilder {
	b.builder.Or(condition)
	return b
}

func (b *DecoratedSQLBuilder) WhereIn(column string, values ...string) builder.SQLBuilder {
	b.builder.WhereIn(column, values...)
	return b
}

func (b *DecoratedSQLBuilder) SelectDistinct(columns ...string) builder.SQLBuilder {
	b.builder.SelectDistinct(columns...)
	return b
}

func (b *DecoratedSQLBuilder) SelectExpr(expr, alias string) builder.SQLBuilder {
	b.builder.SelectExpr(expr, alias)
	return b
}

func (b *DecoratedSQLBuilder) FromSubquery(subquery, alias string) builder.SQLBuilder {
	b.builder.FromSubquery(subquery, alias)
	return b
}

func (b *DecoratedSQLBuilder) InnerJoin(table, condition string) builder.SQLBuilder {
	b.builder.InnerJoin(table, condition)
	return b
}

func (b *DecoratedSQLBuilder) LeftJoin(table, condition string) builder.SQLBuilder {
	b.builder.LeftJoin(table, condition)
	return b
}

func (b *DecoratedSQLBuilder) RightJoin(table, condition string) builder.SQLBuilder {
	b.builder.RightJoin(table, condition)
	return b
}

func (b *DecoratedSQLBuilder) GroupBy(columns ...string) builder.SQLBuilder {
	b.builder.GroupBy(columns...)
	return b
}

func (b *DecoratedSQLBuilder) Having(condition string) builder.SQLBuilder {
	b.builder.Having(condition)
	return b
}

func (b *DecoratedSQLBuilder) OrderBy(columns ...string) builder.SQLBuilder {
	b.builder.OrderBy(columns...)
	return b
}

func (b *DecoratedSQLBuilder) OrderByDesc(columns ...string) builder.SQLBuilder {
	b.builder.OrderByDesc(columns...)
	return b
}

func (b *DecoratedSQLBuilder) Limit(limit int) builder.SQLBuilder {
	b.builder.Limit(limit)
	return b
}

func (b *DecoratedSQLBuilder) Offset(offset int) builder.SQLBuilder {
	b.builder.Offset(offset)
	return b
}

func (b *DecoratedSQLBuilder) Window(name, partition, orderBy string) builder.SQLBuilder {
	b.builder.Window(name, partition, orderBy)
	return b
}

func (b *DecoratedSQLBuilder) With(name, query string) builder.SQLBuilder {
	b.builder.With(name, query)
	return b
}

func (b *DecoratedSQLBuilder) Sample(ratio float64) builder.SQLBuilder {
	b.builder.Sample(ratio)
	return b
}

func (b *DecoratedSQLBuilder) Prewhere(condition string) builder.SQLBuilder {
	b.builder.Prewhere(condition)
	return b
}

func (b *DecoratedSQLBuilder) Reset() builder.SQLBuilder {
	b.builder.Reset()
	return b
}

func (b *DecoratedSQLBuilder) Clone() builder.SQLBuilder {
	return &DecoratedSQLBuilder{
		builder:    b.builder.Clone(),
		decorators: append([]QueryDecorator{}, b.decorators...),
	}
}

// ============================================================================
// DECORATOR CHAIN BUILDER
// ============================================================================

// DecoratorChain builds a chain of decorators
type DecoratorChain struct {
	first QueryDecorator
	last  QueryDecorator
}

// NewDecoratorChain creates a new decorator chain
func NewDecoratorChain() *DecoratorChain {
	return &DecoratorChain{}
}

// Add adds a decorator to the chain
func (c *DecoratorChain) Add(decorator QueryDecorator) *DecoratorChain {
	if c.first == nil {
		c.first = decorator
		c.last = decorator
	} else {
		c.last.SetNext(decorator)
		c.last = decorator
	}
	return c
}

// Decorate applies all decorators in the chain
func (c *DecoratorChain) Decorate(query string) string {
	if c.first == nil {
		return query
	}
	return c.first.Decorate(query)
}

// ============================================================================
// USAGE EXAMPLES
// ============================================================================

// Example usage:
// 
// chain := NewDecoratorChain().
//     Add(NewCommentDecorator("Generated by PromQL transpiler")).
//     Add(NewPrewhereDecorator("timestamp")).
//     Add(NewSamplingDecorator(0.1)).
//     Add(NewCachingDecorator(300)).
//     Add(NewTimeoutDecorator(30)).
//     Add(NewLimitDecorator(10000))
// 
// query := builder.Build()
// decoratedQuery := chain.Decorate(query)
