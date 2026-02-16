package main

import (
	"fmt"
	"log"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

// Integration test to verify the transpiler works end-to-end
func main() {
	fmt.Println("========================================")
	fmt.Println("PromQL Transpiler - Integration Test")
	fmt.Println("========================================")

	// Create transpiler
	t := transpiler.New(nil)

	// Test cases
	tests := []struct {
		name   string
		promql string
	}{
		{"Simple metric", "up"},
		{"Metric with labels", `http_requests_total{job="api",status="200"}`},
		{"Range selector", "http_requests_total[5m]"},
		{"Rate function", "rate(http_requests_total[5m])"},
		{"Increase function", "increase(http_requests_total[1h])"},
		{"Delta function", "delta(temperature[30m])"},
		{"Sum aggregation", "sum(http_requests_total) by (status)"},
		{"Avg aggregation", "avg(cpu_usage) by (host)"},
		{"Max aggregation", "max(memory_usage)"},
		{"Complex query", `sum(rate(http_requests_total{job="api"}[5m])) by (status)`},
		{"Math operation", "http_requests_total + 100"},
		{"Division", "http_requests_total / 1000"},
		{"Abs function", "abs(temperature - 20)"},
		{"Multiple aggregations", `sum(http_requests_total) by (status, method)`},
		{"Regex matcher", `http_requests_total{path=~"/api/.*"}`},
	}

	passedTests := 0
	failedTests := 0

	for i, test := range tests {
		fmt.Printf("[%d/%d] Testing: %s\n", i+1, len(tests), test.name)
		fmt.Printf("PromQL: %s\n", test.promql)

		sql, err := t.Transpile(test.promql)
		if err != nil {
			fmt.Printf("❌ FAILED: %v\n\n", err)
			failedTests++
			continue
		}

		if sql == "" {
			fmt.Printf("❌ FAILED: Empty SQL generated\n\n")
			failedTests++
			continue
		}

		// Basic validation
		if !containsBasicSQL(sql) {
			fmt.Printf("❌ FAILED: Invalid SQL structure\n\n")
			failedTests++
			continue
		}

		fmt.Printf("✅ PASSED\n")
		fmt.Printf("Generated SQL (preview):\n%s\n\n", truncateSQL(sql, 200))
		passedTests++
	}

	// Custom schema test
	fmt.Println("\n--- Custom Schema Test ---")
	customSchema := &clickhouse.Schema{
		TableName:        "custom_metrics",
		MetricNameColumn: "name",
		LabelsColumn:     "tags",
		TimestampColumn:  "ts",
		ValueColumn:      "val",
	}

	config := &transpiler.Config{
		Schema:             customSchema,
		EnableOptimization: true,
	}

	customT := transpiler.New(config)
	sql, err := customT.Transpile("up")
	if err != nil {
		log.Printf("❌ Custom schema test FAILED: %v", err)
		failedTests++
	} else if containsString(sql, "custom_metrics") && containsString(sql, "name") {
		fmt.Println("✅ Custom schema test PASSED")
		passedTests++
	} else {
		fmt.Println("❌ Custom schema test FAILED: Schema not applied correctly")
		failedTests++
	}

	// Summary
	fmt.Println("\n========================================")
	fmt.Println("Test Summary")
	fmt.Println("========================================")
	fmt.Printf("Total Tests: %d\n", passedTests+failedTests)
	fmt.Printf("Passed: %d\n", passedTests)
	fmt.Printf("Failed: %d\n", failedTests)

	if failedTests == 0 {
		fmt.Println("\n✅ All tests passed! The transpiler is working correctly.")
	} else {
		fmt.Printf("\n❌ %d test(s) failed. Please check the implementation.\n", failedTests)
	}

	fmt.Println("\nNext steps:")
	fmt.Println("  1. Try the CLI: bin/promql-transpiler -q \"your_query\"")
	fmt.Println("  2. Read the documentation: README.md")
	fmt.Println("  3. Run examples: go run examples/basic_usage.go")
}

func containsBasicSQL(sql string) bool {
	return containsString(sql, "SELECT") || containsString(sql, "FROM")
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && findString(s, substr)
}

func findString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func truncateSQL(sql string, maxLen int) string {
	if len(sql) <= maxLen {
		return sql
	}
	return sql[:maxLen] + "..."
}
