//go:build ignore

package main

import (
	"fmt"
	"strings"

	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
	trans := transpiler.New(nil)

	// Test query with on()
	promql1 := `rate(a[5m]) / on(job) rate(b[5m])`
	sql1, err := trans.Transpile(promql1)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("=== TEST 1: on(job) ===\n")
	fmt.Printf("PromQL: %s\n\n", promql1)
	fmt.Printf("SQL JOIN condition:\n")
	// Extract just the JOIN condition
	if idx := strings.Index(sql1, "ON l.timestamp"); idx != -1 {
		endIdx := strings.Index(sql1[idx:], "\n")
		if endIdx == -1 {
			endIdx = len(sql1[idx:])
		}
		fmt.Printf("%s\n\n", sql1[idx:idx+endIdx])
	}

	// Test query with ignoring()
	promql2 := `rate(a[5m]) / ignoring(instance) rate(b[5m])`
	sql2, err := trans.Transpile(promql2)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("=== TEST 2: ignoring(instance) ===\n")
	fmt.Printf("PromQL: %s\n\n", promql2)
	fmt.Printf("SQL JOIN condition:\n")
	if idx := strings.Index(sql2, "ON l.timestamp"); idx != -1 {
		endIdx := strings.Index(sql2[idx:], "\n")
		if endIdx == -1 {
			endIdx = len(sql2[idx:])
		}
		fmt.Printf("%s\n\n", sql2[idx:idx+endIdx])
	}

	// Test with multiple labels
	promql3 := `rate(a[5m]) / on(job, instance) rate(b[5m])`
	sql3, err := trans.Transpile(promql3)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("=== TEST 3: on(job, instance) ===\n")
	fmt.Printf("PromQL: %s\n\n", promql3)
	fmt.Printf("SQL JOIN condition:\n")
	if idx := strings.Index(sql3, "ON l.timestamp"); idx != -1 {
		endIdx := strings.Index(sql3[idx:], "\n")
		if endIdx == -1 {
			endIdx = len(sql3[idx:])
		}
		fmt.Printf("%s\n\n", sql3[idx:idx+endIdx])
	}

	// Test with group_left
	promql4 := `rate(a[5m]) / on(job) group_left(version) build_info`
	sql4, err := trans.Transpile(promql4)

	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("=== TEST 4: on(job) group_left(version) ===\n")
	fmt.Printf("PromQL: %s\n\n", promql4)
	fmt.Printf("SQL SELECT and JOIN:\n")
	if idx := strings.Index(sql4, "SELECT\n"); idx != -1 {
		endIdx := strings.Index(sql4[idx:], "FROM left_query")
		if endIdx != -1 {
			fmt.Printf("%s\n", sql4[idx:idx+endIdx])
		}
	}
	if idx := strings.Index(sql4, "ON l.timestamp"); idx != -1 {
		endIdx := strings.Index(sql4[idx:], "\n")
		if endIdx == -1 {
			endIdx = len(sql4[idx:])
		}
		fmt.Printf("JOIN: %s\n", sql4[idx:idx+endIdx])
	}
}
