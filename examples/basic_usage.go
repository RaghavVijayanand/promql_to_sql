//go:build ignore

package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
	// Create a new transpiler with default configuration
	t := transpiler.New(nil)
	
	// Set time range for queries
	start := time.Now().Add(-1 * time.Hour)
	end := time.Now()
	step := 15 * time.Second
	
	t.SetTimeRange(start, end, step)
	
	// Example 1: Simple metric selection
	fmt.Println("Example 1: Simple metric selection")
	fmt.Println("=====================================")
	sql, err := t.Transpile(`http_requests_total`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: http_requests_total\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 2: Metric with label filters
	fmt.Println("Example 2: Metric with label filters")
	fmt.Println("=====================================")
	sql, err = t.Transpile(`http_requests_total{job="api",status="200"}`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: http_requests_total{job=\"api\",status=\"200\"}\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 3: Rate calculation
	fmt.Println("Example 3: Rate calculation")
	fmt.Println("============================")
	sql, err = t.Transpile(`rate(http_requests_total[5m])`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: rate(http_requests_total[5m])\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 4: Aggregation with grouping
	fmt.Println("Example 4: Aggregation with grouping")
	fmt.Println("=====================================")
	sql, err = t.Transpile(`sum(http_requests_total) by (status)`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: sum(http_requests_total) by (status)\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 5: Complex query - sum of rate with grouping
	fmt.Println("Example 5: Complex query")
	fmt.Println("=========================")
	sql, err = t.Transpile(`sum(rate(http_requests_total{job="api"}[5m])) by (status)`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: sum(rate(http_requests_total{job=\"api\"}[5m])) by (status)\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 6: Mathematical operations
	fmt.Println("Example 6: Mathematical operations")
	fmt.Println("===================================")
	sql, err = t.Transpile(`http_requests_total + 100`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: http_requests_total + 100\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 7: Increase calculation
	fmt.Println("Example 7: Increase calculation")
	fmt.Println("================================")
	sql, err = t.Transpile(`increase(http_requests_total[1h])`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: increase(http_requests_total[1h])\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 8: Average aggregation
	fmt.Println("Example 8: Average aggregation")
	fmt.Println("===============================")
	sql, err = t.Transpile(`avg(temperature) by (location)`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: avg(temperature) by (location)\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 9: Math functions
	fmt.Println("Example 9: Math functions")
	fmt.Println("=========================")
	sql, err = t.Transpile(`abs(temperature - 20)`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: abs(temperature - 20)\n\nClickHouse SQL:\n%s\n\n", sql)
	
	// Example 10: Multiple aggregations
	fmt.Println("Example 10: Max aggregation")
	fmt.Println("===========================")
	sql, err = t.Transpile(`max(cpu_usage) by (host)`)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	fmt.Printf("PromQL: max(cpu_usage) by (host)\n\nClickHouse SQL:\n%s\n\n", sql)
}
