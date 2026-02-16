package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/transpiler"
	"github.com/spf13/cobra"
)

var (
	query      string
	inputFile  string
	outputFile string
	startTime  string
	endTime    string
	step       string
	verbose    bool
)

func main() {
	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		fmt.Fprintln(os.Stderr, "\nReceived interrupt signal, exiting...")
		os.Exit(130) // 128 + SIGINT
	}()
	
	var rootCmd = &cobra.Command{
		Use:   "promql-transpiler",
		Short: "Transpile PromQL queries to ClickHouse SQL",
		Long: `A high-performance transpiler that converts Prometheus Query Language (PromQL) 
queries into ClickHouse SQL, with full support for cardinality handling, 
time-series operations, and metric aggregations.`,
		Run: runTranspiler,
	}

	rootCmd.Flags().StringVarP(&query, "query", "q", "", "PromQL query to transpile")
	rootCmd.Flags().StringVarP(&inputFile, "file", "f", "", "Read PromQL query from file")
	rootCmd.Flags().StringVarP(&outputFile, "output", "o", "", "Output file for SQL (default: stdout)")
	rootCmd.Flags().StringVarP(&startTime, "start", "s", "", "Start time (RFC3339 format)")
	rootCmd.Flags().StringVarP(&endTime, "end", "e", "", "End time (RFC3339 format)")
	rootCmd.Flags().StringVar(&step, "step", "15s", "Query step duration")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Verbose output")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTranspiler(cmd *cobra.Command, args []string) {
	// Read query from file if specified
	if inputFile != "" {
		data, err := os.ReadFile(inputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading file: %v\n", err)
			os.Exit(1)
		}
		query = strings.TrimSpace(string(data))
	} else if query == "" {
		// Try reading from stdin
		fi, err := os.Stdin.Stat()
		if err == nil && (fi.Mode()&os.ModeCharDevice) == 0 {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading stdin: %v\n", err)
				os.Exit(1)
			}
			query = strings.TrimSpace(string(data))
		}
	}

	if query == "" {
		fmt.Fprintf(os.Stderr, "Error: query is required. Use -q, -f, or pipe to stdin\n")
		os.Exit(1)
	}

	// Create transpiler
	config := &transpiler.Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
		EnableSampling:     true,
		DefaultTimeRange:   time.Hour,
	}

	t := transpiler.New(config)

	// Parse time range if provided
	if startTime != "" || endTime != "" {
		var start, end time.Time
		var err error

		if startTime != "" {
			start, err = time.Parse(time.RFC3339, startTime)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing start time: %v\n", err)
				os.Exit(1)
			}
		} else {
			start = time.Now().Add(-time.Hour)
		}

		if endTime != "" {
			end, err = time.Parse(time.RFC3339, endTime)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing end time: %v\n", err)
				os.Exit(1)
			}
		} else {
			end = time.Now()
		}

		stepDuration, err := time.ParseDuration(step)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing step duration: %v\n", err)
			os.Exit(1)
		}

		t.SetTimeRange(start, end, stepDuration)
	}

	// Transpile the query with timeout
	if verbose {
		fmt.Fprintf(os.Stderr, "Transpiling PromQL query: %s\n", query)
	}

	// Add 30 second timeout to prevent hanging on pathological input
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	type result struct {
		sql string
		err error
	}
	resultChan := make(chan result, 1)
	
	go func() {
		sql, err := t.Transpile(query)
		resultChan <- result{sql, err}
	}()
	
	var sql string
	var err error
	select {
	case <-ctx.Done():
		fmt.Fprintf(os.Stderr, "Error: transpilation timeout after 30 seconds\n")
		os.Exit(1)
	case res := <-resultChan:
		sql = res.sql
		err = res.err
	}
	
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error transpiling query: %v\n", err)
		os.Exit(1)
	}

	// Output the result
	if outputFile != "" {
		err := os.WriteFile(outputFile, []byte(sql), 0644)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error writing to file: %v\n", err)
			os.Exit(1)
		}
		if verbose {
			fmt.Fprintf(os.Stderr, "SQL written to %s\n", outputFile)
		}
	} else {
		fmt.Println(sql)
	}
}
