package main

import (
	"fmt"
	"log"
	"time"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
	// Create transpiler with Prometheus API parser
	config := &transpiler.Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
		PrometheusURL:      "http://localhost:9090",
	}
	trans := transpiler.New(config)
	trans.SetTimeRange(time.Now().Add(-1*time.Hour), time.Now(), time.Minute)

	// Test multiple queries using Prometheus API parser
	queries := []string{
		"foo/bar",
		"rate(http_requests_total[5m])",
		"sum by (instance) (up)",
		"node_cpu_seconds_total{mode='idle'} > 100",
		"histogram_quantile(0.95, rate(http_request_duration_seconds_bucket[5m]))",
	}

	for _, q := range queries {
		fmt.Printf("Query: %s\n", q)
		sql, err := trans.Transpile(q)
		if err != nil {
			log.Printf("Error transpiling %q: %v", q, err)
			continue
		}
		fmt.Printf("SQL:\n%s\n\n", sql)
	}
}
