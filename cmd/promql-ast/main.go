package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"github.com/shinro/promql-transpiler/pkg/promapi"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <promql-query>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Example: %s 'rate(http_requests_total[5m])'\n", os.Args[0])
		os.Exit(1)
	}

	query := os.Args[1]
	prometheusURL := "http://localhost:9090"
	if url := os.Getenv("PROMETHEUS_URL"); url != "" {
		prometheusURL = url
	}

	fmt.Printf("Parsing query: %s\n", query)
	fmt.Printf("Prometheus URL: %s\n\n", prometheusURL)

	// Get AST from Prometheus API
	client := promapi.NewClient(prometheusURL)
	astNode, err := client.ParseQuery(query)
	if err != nil {
		log.Fatalf("Failed to parse query: %v", err)
	}

	// Print JSON AST from Prometheus
	fmt.Println("=== Prometheus JSON AST ===")
	jsonBytes, _ := json.MarshalIndent(astNode, "", "  ")
	fmt.Println(string(jsonBytes))
	fmt.Println()

	// Show node type
	fmt.Printf("Node Type: %s\n", astNode.Type)
	if astNode.Name != "" {
		fmt.Printf("Metric Name: %s\n", astNode.Name)
	}
	if astNode.Op != "" {
		fmt.Printf("Operator: %s\n", astNode.Op)
	}
	if astNode.Func != nil {
		fmt.Printf("Function: %s\n", astNode.Func.Name)
	}
}
