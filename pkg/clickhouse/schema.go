package clickhouse

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Schema represents the ClickHouse schema for metrics.
type Schema struct {
	TableName        string
	MetricNameColumn string
	LabelsColumn     string
	TimestampColumn  string
	ValueColumn      string
	CardinalityTable string

	// TableMeta describes the physical layout of the ClickHouse table
	// (engine type, ORDER BY key, partition key, skip indexes).
	// The engine-aware optimizer uses this for deterministic query
	// transformations. If nil, DefaultTableMeta() is used.
	TableMeta *TableMeta
}

// GetTableMeta returns the schema's TableMeta, falling back to the
// default time-series layout if none was explicitly configured.
func (s *Schema) GetTableMeta() *TableMeta {
	if s.TableMeta != nil {
		return s.TableMeta
	}
	return DefaultTableMeta()
}

// DefaultSchema returns the default schema configuration for a standard
// Prometheus-compatible ClickHouse metrics table.
func DefaultSchema() *Schema {
	return &Schema{
		TableName:        "metrics",
		MetricNameColumn: "metric_name",
		LabelsColumn:     "labels",
		TimestampColumn:  "timestamp",
		ValueColumn:      "value",
		CardinalityTable: "metrics_cardinality",
		TableMeta:        DefaultTableMeta(),
	}
}

// QueryBuilder builds ClickHouse SQL queries
type QueryBuilder struct {
	schema    *Schema
	timeRange *TimeRange
}

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// NewQueryBuilder creates a new QueryBuilder
func NewQueryBuilder(schema *Schema) *QueryBuilder {
	return &QueryBuilder{
		schema: schema,
	}
}

// SetTimeRange sets the time range for queries
func (qb *QueryBuilder) SetTimeRange(start, end time.Time, step time.Duration) {
	qb.timeRange = &TimeRange{
		Start: start,
		End:   end,
		Step:  step,
	}
}

// BuildSelect builds a SELECT statement
func (qb *QueryBuilder) BuildSelect(columns ...string) string {
	if len(columns) == 0 {
		columns = []string{"*"}
	}
	return fmt.Sprintf("SELECT %s", strings.Join(columns, ", "))
}

// BuildFrom builds a FROM clause
func (qb *QueryBuilder) BuildFrom(table string) string {
	if table == "" {
		table = qb.schema.TableName
	}
	return fmt.Sprintf("FROM %s", table)
}

// BuildWhere builds a WHERE clause
func (qb *QueryBuilder) BuildWhere(conditions ...string) string {
	if len(conditions) == 0 {
		return ""
	}
	
	validConditions := make([]string, 0, len(conditions))
	for _, cond := range conditions {
		if cond != "" {
			validConditions = append(validConditions, cond)
		}
	}
	
	if len(validConditions) == 0 {
		return ""
	}
	
	return fmt.Sprintf("WHERE %s", strings.Join(validConditions, " AND "))
}

// BuildMetricNameCondition builds a condition for metric name
func (qb *QueryBuilder) BuildMetricNameCondition(metricName string) string {
	if metricName == "" {
		return ""
	}
	return fmt.Sprintf("%s = '%s'", qb.schema.MetricNameColumn, escapeSQLString(metricName))
}

// BuildLabelCondition builds a condition for label matching
func (qb *QueryBuilder) BuildLabelCondition(labelName, operator, labelValue string) string {
	escapedValue := escapeSQLString(labelValue)
	
	switch operator {
	case "=":
		return fmt.Sprintf("%s['%s'] = '%s'", qb.schema.LabelsColumn, labelName, escapedValue)
	case "!=":
		return fmt.Sprintf("%s['%s'] != '%s'", qb.schema.LabelsColumn, labelName, escapedValue)
	case "=~":
		return fmt.Sprintf("match(%s['%s'], '%s')", qb.schema.LabelsColumn, labelName, escapedValue)
	case "!~":
		return fmt.Sprintf("NOT match(%s['%s'], '%s')", qb.schema.LabelsColumn, labelName, escapedValue)
	default:
		return ""
	}
}

// BuildTimeRangeCondition builds a time range condition.
// Uses ClickHouse's toDateTime() for explicit type casting, which ensures
// correct comparison behavior with both DateTime and DateTime64 columns.
func (qb *QueryBuilder) BuildTimeRangeCondition(start, end time.Time) string {
	if start.IsZero() && end.IsZero() {
		return ""
	}

	var conditions []string

	if !start.IsZero() {
		conditions = append(conditions, fmt.Sprintf("%s >= toDateTime('%s')",
			qb.schema.TimestampColumn, start.Format("2006-01-02 15:04:05")))
	}

	if !end.IsZero() {
		conditions = append(conditions, fmt.Sprintf("%s <= toDateTime('%s')",
			qb.schema.TimestampColumn, end.Format("2006-01-02 15:04:05")))
	}

	return strings.Join(conditions, " AND ")
}

// BuildGroupBy builds a GROUP BY clause
func (qb *QueryBuilder) BuildGroupBy(columns ...string) string {
	if len(columns) == 0 {
		return ""
	}
	return fmt.Sprintf("GROUP BY %s", strings.Join(columns, ", "))
}

// BuildOrderBy builds an ORDER BY clause
func (qb *QueryBuilder) BuildOrderBy(columns ...string) string {
	if len(columns) == 0 {
		return ""
	}
	return fmt.Sprintf("ORDER BY %s", strings.Join(columns, ", "))
}

// BuildLimit builds a LIMIT clause
func (qb *QueryBuilder) BuildLimit(limit int) string {
	if limit <= 0 {
		return ""
	}
	return fmt.Sprintf("LIMIT %d", limit)
}

// BuildRateCalculation builds a rate calculation expression
func (qb *QueryBuilder) BuildRateCalculation(rangeSeconds float64) string {
	return fmt.Sprintf(`
		(%s - lagInFrame(%s) OVER w) / 
		(toUnixTimestamp64Milli(%s) - toUnixTimestamp64Milli(lagInFrame(%s) OVER w)) * 1000
	`, qb.schema.ValueColumn, qb.schema.ValueColumn,
	   qb.schema.TimestampColumn, qb.schema.TimestampColumn)
}

// BuildIncreaseCalculation builds an increase calculation expression
func (qb *QueryBuilder) BuildIncreaseCalculation() string {
	return fmt.Sprintf(`
		(%s - lagInFrame(%s) OVER w)
	`, qb.schema.ValueColumn, qb.schema.ValueColumn)
}

// BuildDeltaCalculation builds a delta calculation expression
func (qb *QueryBuilder) BuildDeltaCalculation() string {
	return qb.BuildIncreaseCalculation()
}

// BuildWindow builds a WINDOW clause
func (qb *QueryBuilder) BuildWindow(name, partitionBy, orderBy string) string {
	parts := []string{name, "AS ("}
	
	if partitionBy != "" {
		parts = append(parts, fmt.Sprintf("PARTITION BY %s", partitionBy))
	}
	
	if orderBy != "" {
		if partitionBy != "" {
			parts = append(parts, "")
		}
		parts = append(parts, fmt.Sprintf("ORDER BY %s", orderBy))
	}
	
	parts = append(parts, ")")
	return fmt.Sprintf("WINDOW %s", strings.Join(parts, " "))
}

// BuildAggregation builds an aggregation expression
func (qb *QueryBuilder) BuildAggregation(aggFunc string, expr string) string {
	return fmt.Sprintf("%s(%s)", aggFunc, expr)
}

// BuildCTE builds a Common Table Expression
func (qb *QueryBuilder) BuildCTE(name, query string) string {
	return fmt.Sprintf("%s AS (\n%s\n)", name, query)
}

// BuildWithClause builds a WITH clause
func (qb *QueryBuilder) BuildWithClause(ctes ...string) string {
	if len(ctes) == 0 {
		return ""
	}
	return fmt.Sprintf("WITH %s", strings.Join(ctes, ",\n"))
}

// BuildLabelExtraction builds an expression to extract a label value
func (qb *QueryBuilder) BuildLabelExtraction(labelName string) string {
	return fmt.Sprintf("%s['%s']", qb.schema.LabelsColumn, labelName)
}

// BuildQuantileCalculation builds a quantile calculation
func (qb *QueryBuilder) BuildQuantileCalculation(quantile float64, expr string) string {
	return fmt.Sprintf("quantile(%g)(%s)", quantile, expr)
}

// BuildTopKCalculation builds a topk calculation
func (qb *QueryBuilder) BuildTopKCalculation(k int, expr string) string {
	return fmt.Sprintf("topK(%d)(%s)", k, expr)
}

// BuildHistogramQuantile builds a histogram_quantile calculation
func (qb *QueryBuilder) BuildHistogramQuantile(phi float64, bucketColumn, valueColumn string) string {
	return fmt.Sprintf(`
		quantile(%g)(
			arrayJoin(
				arrayMap(
					(bucket, count) -> (bucket, count),
					%s,
					%s
				)
			).2
		)
	`, phi, bucketColumn, valueColumn)
}

// BuildCardinalityCheck builds a query to check cardinality
func (qb *QueryBuilder) BuildCardinalityCheck(metricName string, labels []string) string {
	var conditions []string
	
	if metricName != "" {
		conditions = append(conditions, fmt.Sprintf("metric_name = '%s'", escapeSQLString(metricName)))
	}
	
	if len(labels) > 0 {
		labelList := make([]string, len(labels))
		for i, label := range labels {
			labelList[i] = fmt.Sprintf("'%s'", escapeSQLString(label))
		}
		conditions = append(conditions, fmt.Sprintf("label_key IN (%s)", strings.Join(labelList, ", ")))
	}
	
	whereClause := ""
	if len(conditions) > 0 {
		whereClause = fmt.Sprintf("WHERE %s", strings.Join(conditions, " AND "))
	}
	
	return fmt.Sprintf(`
		SELECT 
			metric_name,
			label_key,
			sum(cardinality) as total_cardinality
		FROM %s
		%s
		GROUP BY metric_name, label_key
		ORDER BY total_cardinality DESC
	`, qb.schema.CardinalityTable, whereClause)
}

// BuildSamplingQuery builds a query with sampling for high cardinality
func (qb *QueryBuilder) BuildSamplingQuery(baseQuery string, sampleRatio float64) string {
	if sampleRatio >= 1.0 || sampleRatio <= 0 {
		return baseQuery
	}
	
	// Add SAMPLE clause to the base query
	// This assumes the base query is a SELECT statement
	parts := strings.SplitN(baseQuery, "FROM", 2)
	if len(parts) != 2 {
		return baseQuery
	}
	
	return fmt.Sprintf("%s FROM %s SAMPLE %g", 
		parts[0], 
		strings.TrimSpace(parts[1]), 
		sampleRatio)
}

// BuildLabelGrouping builds label grouping expression
func (qb *QueryBuilder) BuildLabelGrouping(labels []string) []string {
	groupBy := make([]string, len(labels))
	for i, label := range labels {
		groupBy[i] = qb.BuildLabelExtraction(label)
	}
	return groupBy
}

// Helper functions

func escapeSQLString(s string) string {
	// Escape single quotes by doubling them
	return strings.ReplaceAll(s, "'", "''")
}

// FormatTimestamp formats a timestamp for ClickHouse
func FormatTimestamp(t time.Time) string {
	return t.Format("2006-01-02 15:04:05")
}

// BuildSimpleQuery builds a simple query for metric selection.
// Label conditions are sorted by name for deterministic SQL output.
func (qb *QueryBuilder) BuildSimpleQuery(metricName string, labels map[string]string, start, end time.Time) string {
	conditions := []string{qb.BuildMetricNameCondition(metricName)}

	// Sort label names for deterministic output
	sortedLabels := make([]string, 0, len(labels))
	for labelName := range labels {
		sortedLabels = append(sortedLabels, labelName)
	}
	sort.Strings(sortedLabels)
	for _, labelName := range sortedLabels {
		conditions = append(conditions, qb.BuildLabelCondition(labelName, "=", labels[labelName]))
	}
	
	if !start.IsZero() || !end.IsZero() {
		conditions = append(conditions, qb.BuildTimeRangeCondition(start, end))
	}
	
	parts := []string{
		qb.BuildSelect(qb.schema.TimestampColumn, qb.schema.ValueColumn, qb.schema.LabelsColumn),
		qb.BuildFrom(""),
		qb.BuildWhere(conditions...),
		qb.BuildOrderBy(qb.schema.TimestampColumn),
	}
	
	var validParts []string
	for _, part := range parts {
		if part != "" {
			validParts = append(validParts, part)
		}
	}
	
	return strings.Join(validParts, "\n")
}
