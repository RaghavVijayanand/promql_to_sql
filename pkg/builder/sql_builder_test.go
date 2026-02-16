package builder

import (
	"strings"
	"testing"
)

// TestSQLBuilder tests the Builder Pattern implementation
func TestSQLBuilder(t *testing.T) {
	tests := []struct {
		name     string
		buildFn  func(SQLBuilder) string
		expected string
	}{
		{
			name: "Simple SELECT",
			buildFn: func(b SQLBuilder) string {
				return b.Select("column1", "column2").
					From("table1").
					Build()
			},
			expected: "SELECT column1, column2\nFROM table1",
		},
		{
			name: "SELECT with WHERE",
			buildFn: func(b SQLBuilder) string {
				return b.Select("value").
					From("metrics").
					Where("timestamp >= 1234567890").
					And("job = 'api'").
					Build()
			},
			expected: "SELECT value\nFROM metrics\nWHERE timestamp >= 1234567890 AND job = 'api'",
		},
		{
			name: "SELECT with GROUP BY and HAVING",
			buildFn: func(b SQLBuilder) string {
				return b.Select("job", "AVG(value) as avg_value").
					From("metrics").
					Where("timestamp >= 1234567890").
					GroupBy("job").
					Having("avg_value > 100").
					Build()
			},
			expected: "SELECT job, AVG(value) as avg_value\nFROM metrics\nWHERE timestamp >= 1234567890\nGROUP BY job\nHAVING avg_value > 100",
		},
		{
			name: "SELECT with ORDER BY and LIMIT",
			buildFn: func(b SQLBuilder) string {
				return b.Select("*").
					From("metrics").
					OrderBy("timestamp").
					Limit(100).
					Build()
			},
			expected: "SELECT *\nFROM metrics\nORDER BY timestamp\nLIMIT 100",
		},
		{
			name: "SELECT with JOINs",
			buildFn: func(b SQLBuilder) string {
				return b.Select("m.value", "l.label_value").
					From("metrics m").
					InnerJoin("labels l", "m.label_id = l.id").
					Where("m.timestamp >= 1234567890").
					Build()
			},
			expected: "SELECT m.value, l.label_value\nFROM metrics m\nINNER JOIN labels l ON m.label_id = l.id\nWHERE m.timestamp >= 1234567890",
		},
		{
			name: "SELECT DISTINCT",
			buildFn: func(b SQLBuilder) string {
				return b.SelectDistinct("job").
					From("metrics").
					Build()
			},
			expected: "SELECT DISTINCT job\nFROM metrics",
		},
		{
			name: "SELECT with CTE",
			buildFn: func(b SQLBuilder) string {
				return b.With("recent_metrics", "SELECT * FROM metrics WHERE timestamp >= 1234567890").
					Select("AVG(value)").
					From("recent_metrics").
					Build()
			},
			expected: "WITH recent_metrics AS (\nSELECT * FROM metrics WHERE timestamp >= 1234567890\n)\nSELECT AVG(value)\nFROM recent_metrics",
		},
		{
			name: "ClickHouse PREWHERE",
			buildFn: func(b SQLBuilder) string {
				return b.Select("value", "timestamp").
					From("metrics").
					Prewhere("timestamp >= 1234567890").
					Where("job = 'api'").
					Build()
			},
			expected: "SELECT value, timestamp\nFROM metrics\nPREWHERE timestamp >= 1234567890\nWHERE job = 'api'",
		},
		{
			name: "ClickHouse SAMPLE",
			buildFn: func(b SQLBuilder) string {
				return b.Select("*").
					From("metrics").
					Sample(0.1).
					Build()
			},
			expected: "SELECT *\nFROM metrics SAMPLE 0.1000",
		},
		{
			name: "Complex query with multiple features",
			buildFn: func(b SQLBuilder) string {
				return b.
					With("filtered_metrics", "SELECT * FROM metrics WHERE timestamp >= 1234567890").
					SelectExpr("job", "").
					SelectExpr("AVG(value)", "avg_value").
					SelectExpr("COUNT(*)", "count").
					FromSubquery("SELECT * FROM filtered_metrics WHERE value > 0", "fm").
					Where("job != ''").
					GroupBy("job").
					Having("count > 100").
					OrderByDesc("avg_value").
					Limit(10).
					Build()
			},
			expected: "WITH filtered_metrics AS (\nSELECT * FROM metrics WHERE timestamp >= 1234567890\n)\nSELECT job, AVG(value) AS avg_value, COUNT(*) AS count\nFROM (\nSELECT * FROM filtered_metrics WHERE value > 0\n) AS fm\nWHERE job != ''\nGROUP BY job\nHAVING count > 100\nORDER BY avg_value DESC\nLIMIT 10",
		},
		{
			name: "WHERE IN clause",
			buildFn: func(b SQLBuilder) string {
				return b.Select("*").
					From("metrics").
					WhereIn("job", "api", "web", "backend").
					Build()
			},
			expected: "SELECT *\nFROM metrics\nWHERE job IN ('api', 'web', 'backend')",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := NewClickHouseSQLBuilder()
			result := tt.buildFn(builder)
			
			// Normalize whitespace for comparison
			result = normalizeWhitespace(result)
			expected := normalizeWhitespace(tt.expected)
			
			if result != expected {
				t.Errorf("Expected:\n%s\n\nGot:\n%s", expected, result)
			}
		})
	}
}

// TestBuilderReset tests the Reset functionality
func TestBuilderReset(t *testing.T) {
	builder := NewClickHouseSQLBuilder()
	
	// Build first query
	sql1 := builder.Select("col1").From("table1").Build()
	
	// Reset and build second query
	builder.Reset()
	sql2 := builder.Select("col2").From("table2").Build()
	
	expected1 := "SELECT col1\nFROM table1"
	expected2 := "SELECT col2\nFROM table2"
	
	if normalizeWhitespace(sql1) != normalizeWhitespace(expected1) {
		t.Errorf("First query failed. Expected:\n%s\n\nGot:\n%s", expected1, sql1)
	}
	
	if normalizeWhitespace(sql2) != normalizeWhitespace(expected2) {
		t.Errorf("Second query after reset failed. Expected:\n%s\n\nGot:\n%s", expected2, sql2)
	}
}

// TestBuilderClone tests the Clone functionality
func TestBuilderClone(t *testing.T) {
	original := NewClickHouseSQLBuilder()
	original.Select("col1").From("table1").Where("id > 0")
	
	// Clone the builder
	cloned := original.Clone()
	
	// Modify clone
	clonedSQL := cloned.And("status = 'active'").Build()
	
	// Original should be unchanged
	originalSQL := original.Build()
	
	expectedOriginal := "SELECT col1\nFROM table1\nWHERE id > 0"
	expectedCloned := "SELECT col1\nFROM table1\nWHERE id > 0 AND status = 'active'"
	
	if normalizeWhitespace(originalSQL) != normalizeWhitespace(expectedOriginal) {
		t.Errorf("Original builder was modified. Expected:\n%s\n\nGot:\n%s", expectedOriginal, originalSQL)
	}
	
	if normalizeWhitespace(clonedSQL) != normalizeWhitespace(expectedCloned) {
		t.Errorf("Cloned builder incorrect. Expected:\n%s\n\nGot:\n%s", expectedCloned, clonedSQL)
	}
}

// TestFluentInterface tests method chaining
func TestFluentInterface(t *testing.T) {
	builder := NewClickHouseSQLBuilder()
	
	// All methods should return SQLBuilder for chaining
	result := builder.
		Select("col1").
		Select("col2").
		From("table1").
		Where("a = 1").
		And("b = 2").
		Or("c = 3").
		GroupBy("col1").
		Having("COUNT(*) > 5").
		OrderBy("col1").
		Limit(100).
		Offset(50)
	
	// Should be able to continue chaining
	sql := result.Build()
	
	if sql == "" {
		t.Error("Fluent interface broken - Build() returned empty string")
	}
}

// TestORCondition tests OR logic in WHERE clause
func TestORCondition(t *testing.T) {
	builder := NewClickHouseSQLBuilder()
	
	sql := builder.
		Select("*").
		From("metrics").
		Where("job = 'api'").
		Or("job = 'web'").
		Build()
	
	if !strings.Contains(sql, "OR") {
		t.Error("OR condition not properly added to WHERE clause")
	}
}

// TestWindow tests window functions
func TestWindow(t *testing.T) {
	builder := NewClickHouseSQLBuilder()
	
	sql := builder.
		Select("value").
		SelectExpr("ROW_NUMBER() OVER w", "row_num").
		From("metrics").
		Window("w", "labels", "timestamp").
		Build()
	
	if !strings.Contains(sql, "WINDOW") {
		t.Error("WINDOW clause not added")
	}
	
	if !strings.Contains(sql, "PARTITION BY labels") {
		t.Error("PARTITION BY not added to window")
	}
	
	if !strings.Contains(sql, "ORDER BY timestamp") {
		t.Error("ORDER BY not added to window")
	}
}

// Helper function to normalize whitespace for comparison
func normalizeWhitespace(s string) string {
	// Replace multiple spaces with single space
	s = strings.Join(strings.Fields(s), " ")
	// Replace newlines with spaces
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// Benchmark tests
func BenchmarkSimpleQuery(b *testing.B) {
	for i := 0; i < b.N; i++ {
		builder := NewClickHouseSQLBuilder()
		_ = builder.Select("value").From("metrics").Where("timestamp >= 1234567890").Build()
	}
}

func BenchmarkComplexQuery(b *testing.B) {
	for i := 0; i < b.N; i++ {
		builder := NewClickHouseSQLBuilder()
		_ = builder.
			With("cte1", "SELECT * FROM table1").
			Select("col1", "col2", "col3").
			From("metrics").
			InnerJoin("labels", "metrics.id = labels.id").
			Where("timestamp >= 1234567890").
			And("value > 0").
			GroupBy("col1", "col2").
			Having("COUNT(*) > 10").
			OrderBy("col1").
			Limit(1000).
			Build()
	}
}

func BenchmarkBuilderReuse(b *testing.B) {
	builder := NewClickHouseSQLBuilder()
	
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		builder.Reset()
		_ = builder.Select("value").From("metrics").Build()
	}
}
