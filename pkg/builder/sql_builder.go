package builder

import (
	"fmt"
	"strings"
)

// SQLBuilder implements the Builder Pattern for SQL query construction
// Following Single Responsibility Principle - only builds SQL
// Following Fluent Interface pattern for ease of use
type SQLBuilder interface {
	// Select operations
	Select(columns ...string) SQLBuilder
	SelectDistinct(columns ...string) SQLBuilder
	SelectExpr(expr, alias string) SQLBuilder
	
	// From operations
	From(table string) SQLBuilder
	FromSubquery(subquery, alias string) SQLBuilder
	
	// Join operations
	InnerJoin(table, condition string) SQLBuilder
	LeftJoin(table, condition string) SQLBuilder
	RightJoin(table, condition string) SQLBuilder
	
	// Where operations
	Where(condition string) SQLBuilder
	And(condition string) SQLBuilder
	Or(condition string) SQLBuilder
	WhereIn(column string, values ...string) SQLBuilder
	
	// Group and Having
	GroupBy(columns ...string) SQLBuilder
	Having(condition string) SQLBuilder
	
	// Order and Limit
	OrderBy(columns ...string) SQLBuilder
	OrderByDesc(columns ...string) SQLBuilder
	Limit(limit int) SQLBuilder
	Offset(offset int) SQLBuilder
	
	// Window functions
	Window(name, partition, orderBy string) SQLBuilder
	
	// CTE (Common Table Expression)
	With(name, query string) SQLBuilder
	
	// ClickHouse specific
	Sample(ratio float64) SQLBuilder
	Prewhere(condition string) SQLBuilder
	
	// Build operations
	Build() string
	Reset() SQLBuilder
	Clone() SQLBuilder
}

// ClickHouseSQLBuilder implements SQLBuilder for ClickHouse
type ClickHouseSQLBuilder struct {
	ctes       []cte
	selectCols []string
	distinct   bool
	fromTable  string
	joins      []join
	whereConds []string
	groupByCols []string
	havingCond  string
	orderByCols []orderBy
	limitVal    int
	offsetVal   int
	windows     []window
	sampleRatio float64
	prewhereCond string
}

type cte struct {
	name  string
	query string
}

type join struct {
	joinType  string
	table     string
	condition string
}

type orderBy struct {
	column string
	desc   bool
}

type window struct {
	name      string
	partition string
	orderBy   string
}

// NewClickHouseSQLBuilder creates a new ClickHouse SQL builder
func NewClickHouseSQLBuilder() SQLBuilder {
	return &ClickHouseSQLBuilder{
		ctes:        make([]cte, 0, 2),
		selectCols:  make([]string, 0, 8),
		joins:       make([]join, 0, 2),
		whereConds:  make([]string, 0, 4),
		groupByCols: make([]string, 0, 4),
		orderByCols: make([]orderBy, 0, 2),
		windows:     make([]window, 0, 1),
		limitVal:    0,
		offsetVal:   0,
		sampleRatio: 0,
	}
}

// Select adds columns to SELECT clause
func (b *ClickHouseSQLBuilder) Select(columns ...string) SQLBuilder {
	b.selectCols = append(b.selectCols, columns...)
	return b
}

// SelectDistinct adds DISTINCT to SELECT
func (b *ClickHouseSQLBuilder) SelectDistinct(columns ...string) SQLBuilder {
	b.distinct = true
	b.selectCols = append(b.selectCols, columns...)
	return b
}

// SelectExpr adds an expression with alias
func (b *ClickHouseSQLBuilder) SelectExpr(expr, alias string) SQLBuilder {
	if alias != "" {
		b.selectCols = append(b.selectCols, fmt.Sprintf("%s AS %s", expr, alias))
	} else {
		b.selectCols = append(b.selectCols, expr)
	}
	return b
}

// From sets the FROM table
func (b *ClickHouseSQLBuilder) From(table string) SQLBuilder {
	b.fromTable = table
	return b
}

// FromSubquery sets a subquery as the FROM source
func (b *ClickHouseSQLBuilder) FromSubquery(subquery, alias string) SQLBuilder {
	b.fromTable = fmt.Sprintf("(\n%s\n) AS %s", subquery, alias)
	return b
}

// InnerJoin adds an INNER JOIN
func (b *ClickHouseSQLBuilder) InnerJoin(table, condition string) SQLBuilder {
	b.joins = append(b.joins, join{"INNER JOIN", table, condition})
	return b
}

// LeftJoin adds a LEFT JOIN
func (b *ClickHouseSQLBuilder) LeftJoin(table, condition string) SQLBuilder {
	b.joins = append(b.joins, join{"LEFT JOIN", table, condition})
	return b
}

// RightJoin adds a RIGHT JOIN
func (b *ClickHouseSQLBuilder) RightJoin(table, condition string) SQLBuilder {
	b.joins = append(b.joins, join{"RIGHT JOIN", table, condition})
	return b
}

// Where adds a WHERE condition
func (b *ClickHouseSQLBuilder) Where(condition string) SQLBuilder {
	if condition != "" {
		b.whereConds = append(b.whereConds, condition)
	}
	return b
}

// And adds an AND condition
func (b *ClickHouseSQLBuilder) And(condition string) SQLBuilder {
	if condition != "" {
		b.whereConds = append(b.whereConds, condition)
	}
	return b
}

// Or adds an OR condition
func (b *ClickHouseSQLBuilder) Or(condition string) SQLBuilder {
	if condition != "" && len(b.whereConds) > 0 {
		lastIdx := len(b.whereConds) - 1
		b.whereConds[lastIdx] = fmt.Sprintf("(%s) OR (%s)", b.whereConds[lastIdx], condition)
	} else if condition != "" {
		b.whereConds = append(b.whereConds, condition)
	}
	return b
}

// WhereIn adds a WHERE IN condition
func (b *ClickHouseSQLBuilder) WhereIn(column string, values ...string) SQLBuilder {
	if len(values) > 0 {
		quotedValues := make([]string, len(values))
		for i, v := range values {
			quotedValues[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
		}
		condition := fmt.Sprintf("%s IN (%s)", column, strings.Join(quotedValues, ", "))
		b.whereConds = append(b.whereConds, condition)
	}
	return b
}

// GroupBy adds GROUP BY columns
func (b *ClickHouseSQLBuilder) GroupBy(columns ...string) SQLBuilder {
	b.groupByCols = append(b.groupByCols, columns...)
	return b
}

// Having adds a HAVING condition
func (b *ClickHouseSQLBuilder) Having(condition string) SQLBuilder {
	b.havingCond = condition
	return b
}

// OrderBy adds ORDER BY columns
func (b *ClickHouseSQLBuilder) OrderBy(columns ...string) SQLBuilder {
	for _, col := range columns {
		b.orderByCols = append(b.orderByCols, orderBy{col, false})
	}
	return b
}

// OrderByDesc adds ORDER BY DESC columns
func (b *ClickHouseSQLBuilder) OrderByDesc(columns ...string) SQLBuilder {
	for _, col := range columns {
		b.orderByCols = append(b.orderByCols, orderBy{col, true})
	}
	return b
}

// Limit sets the LIMIT
func (b *ClickHouseSQLBuilder) Limit(limit int) SQLBuilder {
	b.limitVal = limit
	return b
}

// Offset sets the OFFSET
func (b *ClickHouseSQLBuilder) Offset(offset int) SQLBuilder {
	b.offsetVal = offset
	return b
}

// Window adds a window function definition
func (b *ClickHouseSQLBuilder) Window(name, partition, orderBy string) SQLBuilder {
	b.windows = append(b.windows, window{name, partition, orderBy})
	return b
}

// With adds a CTE (Common Table Expression)
func (b *ClickHouseSQLBuilder) With(name, query string) SQLBuilder {
	b.ctes = append(b.ctes, cte{name, query})
	return b
}

// Sample adds SAMPLE clause (ClickHouse specific)
func (b *ClickHouseSQLBuilder) Sample(ratio float64) SQLBuilder {
	b.sampleRatio = ratio
	return b
}

// Prewhere adds PREWHERE clause (ClickHouse specific)
func (b *ClickHouseSQLBuilder) Prewhere(condition string) SQLBuilder {
	b.prewhereCond = condition
	return b
}

// Build constructs the final SQL query
func (b *ClickHouseSQLBuilder) Build() string {
	var sb strings.Builder
	sb.Grow(512) // Pre-allocate reasonable buffer size
	
	// CTEs
	if len(b.ctes) > 0 {
		sb.WriteString("WITH ")
		for i, c := range b.ctes {
			if i > 0 {
				sb.WriteString(",\n")
			}
			sb.WriteString(c.name)
			sb.WriteString(" AS (\n")
			sb.WriteString(c.query)
			sb.WriteString("\n)")
		}
		sb.WriteString("\n")
	}
	
	// SELECT
	if b.distinct {
		sb.WriteString("SELECT DISTINCT ")
	} else {
		sb.WriteString("SELECT ")
	}
	if len(b.selectCols) > 0 {
		sb.WriteString(strings.Join(b.selectCols, ", "))
	} else {
		sb.WriteString("*")
	}
	sb.WriteString("\n")
	
	// FROM
	if b.fromTable != "" {
		sb.WriteString("FROM ")
		sb.WriteString(b.fromTable)
		if b.sampleRatio > 0 && b.sampleRatio < 1 {
			sb.WriteString(fmt.Sprintf(" SAMPLE %.4f", b.sampleRatio))
		}
		sb.WriteString("\n")
	}
	
	// JOINs
	for _, j := range b.joins {
		sb.WriteString(j.joinType)
		sb.WriteString(" ")
		sb.WriteString(j.table)
		sb.WriteString(" ON ")
		sb.WriteString(j.condition)
		sb.WriteString("\n")
	}
	
	// PREWHERE (ClickHouse optimization)
	if b.prewhereCond != "" {
		sb.WriteString("PREWHERE ")
		sb.WriteString(b.prewhereCond)
		sb.WriteString("\n")
	}
	
	// WHERE
	if len(b.whereConds) > 0 {
		sb.WriteString("WHERE ")
		sb.WriteString(strings.Join(b.whereConds, " AND "))
		sb.WriteString("\n")
	}
	
	// GROUP BY
	if len(b.groupByCols) > 0 {
		sb.WriteString("GROUP BY ")
		sb.WriteString(strings.Join(b.groupByCols, ", "))
		sb.WriteString("\n")
	}
	
	// HAVING
	if b.havingCond != "" {
		sb.WriteString("HAVING ")
		sb.WriteString(b.havingCond)
		sb.WriteString("\n")
	}
	
	// WINDOW
	if len(b.windows) > 0 {
		sb.WriteString("WINDOW ")
		for i, w := range b.windows {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(w.name)
			sb.WriteString(" AS (")
			parts := make([]string, 0, 2)
			if w.partition != "" {
				parts = append(parts, "PARTITION BY "+w.partition)
			}
			if w.orderBy != "" {
				parts = append(parts, "ORDER BY "+w.orderBy)
			}
			sb.WriteString(strings.Join(parts, " "))
			sb.WriteString(")")
		}
		sb.WriteString("\n")
	}
	
	// ORDER BY
	if len(b.orderByCols) > 0 {
		sb.WriteString("ORDER BY ")
		for i, o := range b.orderByCols {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(o.column)
			if o.desc {
				sb.WriteString(" DESC")
			}
		}
		sb.WriteString("\n")
	}
	
	// LIMIT and OFFSET
	if b.limitVal > 0 {
		sb.WriteString(fmt.Sprintf("LIMIT %d", b.limitVal))
		if b.offsetVal > 0 {
			sb.WriteString(fmt.Sprintf(" OFFSET %d", b.offsetVal))
		}
	}
	
	result := sb.String()
	return strings.TrimSpace(result)
}

// Reset clears all builder state
func (b *ClickHouseSQLBuilder) Reset() SQLBuilder {
	b.ctes = make([]cte, 0)
	b.selectCols = make([]string, 0)
	b.distinct = false
	b.fromTable = ""
	b.joins = make([]join, 0)
	b.whereConds = make([]string, 0)
	b.groupByCols = make([]string, 0)
	b.havingCond = ""
	b.orderByCols = make([]orderBy, 0)
	b.limitVal = 0
	b.offsetVal = 0
	b.windows = make([]window, 0)
	b.sampleRatio = 0
	b.prewhereCond = ""
	return b
}

// Clone creates a copy of the builder
func (b *ClickHouseSQLBuilder) Clone() SQLBuilder {
	clone := &ClickHouseSQLBuilder{
		ctes:         make([]cte, len(b.ctes)),
		selectCols:   make([]string, len(b.selectCols)),
		distinct:     b.distinct,
		fromTable:    b.fromTable,
		joins:        make([]join, len(b.joins)),
		whereConds:   make([]string, len(b.whereConds)),
		groupByCols:  make([]string, len(b.groupByCols)),
		havingCond:   b.havingCond,
		orderByCols:  make([]orderBy, len(b.orderByCols)),
		limitVal:     b.limitVal,
		offsetVal:    b.offsetVal,
		windows:      make([]window, len(b.windows)),
		sampleRatio:  b.sampleRatio,
		prewhereCond: b.prewhereCond,
	}
	
	copy(clone.ctes, b.ctes)
	copy(clone.selectCols, b.selectCols)
	copy(clone.joins, b.joins)
	copy(clone.whereConds, b.whereConds)
	copy(clone.groupByCols, b.groupByCols)
	copy(clone.orderByCols, b.orderByCols)
	copy(clone.windows, b.windows)
	
	return clone
}
