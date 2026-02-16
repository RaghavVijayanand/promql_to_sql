//go:build ignore

package main

import (
	"fmt"

	"github.com/shinro/promql-transpiler/internal/cardinality"
)

func main() {
	fmt.Println("Cardinality Estimation Examples")
	fmt.Println("================================\n")
	
	// Create a cardinality estimator
	estimator := cardinality.NewEstimator()
	
	// Example 1: Low cardinality query
	fmt.Println("Example 1: Low cardinality metric")
	result1 := estimator.Estimate("cpu_usage", 2, 3600) // 2 labels, 1 hour
	printEstimateResult(result1)
	
	// Example 2: Medium cardinality query
	fmt.Println("\nExample 2: Medium cardinality metric")
	result2 := estimator.Estimate("http_requests", 5, 3600) // 5 labels, 1 hour
	printEstimateResult(result2)
	
	// Example 3: High cardinality query
	fmt.Println("\nExample 3: High cardinality metric")
	result3 := estimator.Estimate("user_actions", 8, 86400) // 8 labels, 1 day
	printEstimateResult(result3)
	
	// Example 4: Very high cardinality query
	fmt.Println("\nExample 4: Very high cardinality metric")
	result4 := estimator.Estimate("trace_spans", 12, 86400) // 12 labels, 1 day
	printEstimateResult(result4)
	
	// Example 5: Using the optimizer
	fmt.Println("\n\nOptimization Hints Example")
	fmt.Println("==========================\n")
	
	optimizer := cardinality.NewOptimizer()
	hints := optimizer.Optimize("http_requests", []string{"method", "path", "status", "host", "user_id", "session_id"}, 86400)
	
	fmt.Println("Query: http_requests with 6 labels over 1 day")
	fmt.Println("\nOptimization hints:")
	for i, hint := range hints {
		fmt.Printf("%d. [%s] %s (Impact: %s)\n", i+1, hint.Type, hint.Description, hint.Impact)
	}
	
	// Example 6: Calculate sample size
	fmt.Println("\n\nSample Size Calculation")
	fmt.Println("=======================\n")
	
	totalCard := uint64(10000000) // 10M data points
	precision := 0.01              // 1% error margin
	
	sampleSize := cardinality.CalculateSampleSize(totalCard, precision)
	fmt.Printf("Total cardinality: %d\n", totalCard)
	fmt.Printf("Desired precision: %.2f%%\n", precision*100)
	fmt.Printf("Recommended sample size: %d\n", sampleSize)
	fmt.Printf("Sample ratio: %.4f (%.2f%%)\n", float64(sampleSize)/float64(totalCard), float64(sampleSize)/float64(totalCard)*100)
	
	// Example 7: Estimate query cost
	fmt.Println("\n\nQuery Cost Estimation")
	fmt.Println("=====================\n")
	
	cardinality1 := uint64(1000)
	operations1 := 5
	complexity1 := 1.0
	cost1 := cardinality.EstimateQueryCost(cardinality1, operations1, complexity1)
	
	cardinality2 := uint64(1000000)
	operations2 := 10
	complexity2 := 2.5
	cost2 := cardinality.EstimateQueryCost(cardinality2, operations2, complexity2)
	
	fmt.Printf("Query 1 - Cardinality: %d, Operations: %d, Complexity: %.1f\n", cardinality1, operations1, complexity1)
	fmt.Printf("Estimated cost: %.0f\n\n", cost1)
	
	fmt.Printf("Query 2 - Cardinality: %d, Operations: %d, Complexity: %.1f\n", cardinality2, operations2, complexity2)
	fmt.Printf("Estimated cost: %.0f\n", cost2)
	fmt.Printf("Cost ratio: %.2fx more expensive\n", cost2/cost1)
}

func printEstimateResult(result *cardinality.EstimateResult) {
	fmt.Printf("Estimated cardinality: %d\n", result.EstimatedCardinality)
	fmt.Printf("High cardinality: %v\n", result.IsHighCardinality)
	fmt.Printf("Suggested strategy: %s\n", result.SuggestedStrategy)
	fmt.Printf("Suggested sample ratio: %.2f\n", result.SuggestedSampleRatio)
	
	if len(result.Warnings) > 0 {
		fmt.Println("Warnings:")
		for _, warning := range result.Warnings {
			fmt.Printf("  - %s\n", warning)
		}
	}
}
