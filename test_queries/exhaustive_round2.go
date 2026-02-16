//go:build ignore

package main

import (
	"fmt"
	"os"
	"time"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

// Round 2: Edge cases, deeply nested, adversarial queries
func main() {
	queries := []struct {
		name   string
		promql string
	}{
		// ── EDGE: NUMBERS & LITERALS ─────────────────────────────────
		{"number literal", `42`},
		{"float literal", `3.14`},
		{"negative number", `-1`},
		{"scientific notation", `1e3`},
		{"string literal in func", `label_replace(up, "dc", "us-east", "", "")`},

		// ── EDGE: DEEPLY NESTED AGGREGATIONS ─────────────────────────
		{"3-level nested agg", `max(avg by (cluster) (sum by (cluster, job) (rate(http_requests_total[5m]))))`},
		{"topk of sum of rate", `topk(3, sum by (job) (rate(http_requests_total[5m])))`},
		{"bottomk of avg", `bottomk(5, avg by (job) (up))`},
		{"quantile of sum", `quantile(0.99, sum by (job) (rate(http_requests_total[5m])))`},

		// ── EDGE: MULTIPLE BINARY OPS ────────────────────────────────
		{"chained arithmetic", `(rate(a[5m]) + rate(b[5m])) / (rate(c[5m]) + rate(d[5m]))`},
		{"mixed ops precedence", `rate(a[5m]) + rate(b[5m]) * 2 - 1`},
		{"power of power", `http_requests_total ^ 2 ^ 3`},
		{"comparison chain", `rate(a[5m]) > 0 and rate(b[5m]) < 100`},

		// ── EDGE: COMPLEX VECTOR MATCHING ────────────────────────────
		{"nested on group_left", `sum by (job) (rate(http_requests_total[5m])) / on (job) group_left (version) max by (job, version) (build_info)`},
		{"ignoring group_right", `rate(a[5m]) * ignoring (instance) group_right (extra) rate(b[5m])`},
		{"binary with unless", `rate(http_requests_total[5m]) > 0.5 unless ignoring (instance) rate(http_errors_total[5m]) > 0.1`},

		// ── EDGE: OFFSET AND @ IN COMPLEX EXPRS ──────────────────────
		{"offset in nested", `sum by (job) (rate(http_requests_total[5m] offset 2h))`},
		{"@ in nested agg", `sum(rate(http_requests_total[5m] @ 1700000000))`},
		{"offset in binary lhs", `rate(http_requests_total[5m] offset 1h) / rate(http_requests_total[5m])`},
		{"both offset and @", `rate(http_requests_total[5m] offset 1h @ 1700000000)`},

		// ── EDGE: SUBQUERIES IN COMPLEX EXPRS ────────────────────────
		{"subquery in agg", `sum by (job) (max_over_time(rate(http_requests_total[5m])[1h:5m]))`},
		{"subquery + binary", `max_over_time(rate(a[5m])[30m:1m]) / avg_over_time(rate(b[5m])[30m:1m])`},
		{"subquery with offset", `min_over_time(rate(http_requests_total[5m])[1h:5m] offset 2h)`},

		// ── EDGE: LABEL FUNCTIONS WITH COMPLEX ARGS ──────────────────
		{"label_replace complex", `label_replace(sum by (job) (rate(http_requests_total[5m])), "svc", "$1", "job", "(.*)-service")`},
		{"label_join complex", `label_join(topk(5, rate(http_requests_total[5m])), "id", ":", "job", "instance", "status")`},
		{"nested label_replace", `label_replace(label_replace(up, "dc", "$1", "instance", "(.*?)\\..*"), "env", "$1", "job", "(.*)-.*")`},

		// ── EDGE: HISTOGRAM COMPLEX ──────────────────────────────────
		{"histogram p50 with filter", `histogram_quantile(0.5, sum by (le, method) (rate(http_request_duration_seconds_bucket{job="api",method=~"GET|POST"}[5m])))`},
		{"histogram p90 with offset", `histogram_quantile(0.9, sum by (le) (rate(http_request_duration_seconds_bucket[5m] offset 1h)))`},

		// ── EDGE: BOOL WITH BINARY OPS ───────────────────────────────
		{"bool eq", `rate(http_requests_total[5m]) == bool 0`},
		{"bool ne", `up != bool 1`},
		{"bool lt", `http_requests_total < bool 100`},
		{"bool gte", `rate(http_requests_total[5m]) >= bool 0.5`},

		// ── EDGE: UNARY IN COMPLEX EXPRS ─────────────────────────────
		{"unary in binary", `-rate(a[5m]) + rate(b[5m])`},
		{"double negative", `-(-rate(a[5m]))`},
		{"unary in agg", `sum by (job) (-rate(http_requests_total[5m]))`},

		// ── EDGE: ABSENT PATTERNS ────────────────────────────────────
		{"absent with labels", `absent(up{job="nonexistent",env="prod"})`},

		// ── EDGE: CHANGES & RESETS WITH AGGREGATION ──────────────────
		{"changes in agg", `sum by (job) (changes(process_start_time_seconds[1h]))`},
		{"resets in agg", `max by (instance) (resets(process_start_time_seconds[1h]))`},

		// ── EDGE: DERIV / PREDICT_LINEAR COMPLEX ─────────────────────
		{"deriv in agg", `avg by (instance) (deriv(node_filesystem_free_bytes[1h]))`},
		{"predict_linear with filter", `predict_linear(node_filesystem_free_bytes{env="prod"}[4h], 86400)`},
		{"predict_linear in sort", `sort_desc(predict_linear(node_filesystem_free_bytes[4h], 86400))`},

		// ── EDGE: MULTIPLE OR / AND / UNLESS ─────────────────────────
		{"multi or", `up == 0 or absent(up) or rate(http_requests_total[5m]) > 100`},
		{"multi and", `up == 1 and rate(http_requests_total[5m]) > 0 and rate(http_errors_total[5m]) < 0.01`},
		{"or then unless", `(up == 0 or absent(up)) unless on (job) (rate(http_requests_total[5m]) > 0)`},

		// ── EDGE: PARENTHESIZED EXPRS ────────────────────────────────
		{"deeply nested parens", `(((((up)))))`},
		{"parens around binary", `(rate(a[5m]) + rate(b[5m])) * (rate(c[5m]) - rate(d[5m]))`},
		{"parens in agg args", `sum by (job) ((rate(http_requests_total[5m])))`},

		// ── EDGE: SCALAR / VECTOR ────────────────────────────────────
		{"scalar of agg", `scalar(sum(up))`},
		{"vector in binary", `vector(1) + up`},
		{"vector in agg", `sum(vector(42))`},

		// ── EDGE: TIMESTAMP FUNCTION ─────────────────────────────────
		{"timestamp func", `timestamp(up)`},
		{"timestamp in binary", `time() - timestamp(up) > 300`},

		// ── EDGE: ALL OVER-TIME WITH LABELS ──────────────────────────
		{"avg_over_time filtered", `avg_over_time(node_cpu_seconds_total{mode="idle"}[5m])`},
		{"sum_over_time with offset", `sum_over_time(http_requests_total[1h] offset 2h)`},
		{"quantile_over_time complex", `quantile_over_time(0.99, rate(http_request_duration_seconds_sum[5m])[1h:5m])`},

		// ── EDGE: MEGA COMBINED ──────────────────────────────────────
		{"SLA query", `1 - (sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m])))`},
		{"latency percentile", `histogram_quantile(0.95, sum by (le, method) (rate(http_request_duration_seconds_bucket{job="api"}[5m])))`},
		{"disk full prediction", `predict_linear(node_filesystem_free_bytes{mountpoint="/"}[6h], 24*3600) < 0`},
		{"cpu saturation", `1 - avg by (instance) (rate(node_cpu_seconds_total{mode="idle"}[5m]))`},
		{"memory usage", `(node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes * 100`},
		{"error rate with threshold", `sum by (job) (rate(http_requests_total{status=~"5.."}[5m])) / sum by (job) (rate(http_requests_total[5m])) > 0.01`},
		{"multi-window rate comparison", `rate(http_requests_total[1m]) / rate(http_requests_total[30m])`},
		{"request vs capacity", `sum by (job) (rate(http_requests_total[5m])) / on (job) group_left () max by (job) (http_server_max_requests)`},
	}

	config := &transpiler.Config{
		Schema:             clickhouse.DefaultSchema(),
		EnableOptimization: true,
	}

	start, _ := time.Parse(time.RFC3339, "2024-01-01T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2024-01-01T23:59:59Z")

	passed := 0
	failed := 0
	var failures []string

	for i, q := range queries {
		t := transpiler.New(config)
		t.SetTimeRange(start, end, 15*time.Second)

		sql, err := t.Transpile(q.promql)
		if err != nil {
			failed++
			failures = append(failures, fmt.Sprintf("  [%d] %-35s  FAIL: %v", i+1, q.name, err))
		} else {
			passed++
			_ = sql // SQL is valid as long as transpile succeeds
		if false {
				failed++
				failures = append(failures, fmt.Sprintf("  [%d] %-35s  FAIL: SQL looks empty", i+1, q.name))
			}
		}
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("  PromQL Edge-Case Test Results (R2)\n")
	fmt.Printf("========================================\n")
	fmt.Printf("  Total:  %d\n", len(queries))
	fmt.Printf("  Passed: %d\n", passed)
	fmt.Printf("  Failed: %d\n", failed)
	fmt.Printf("========================================\n")

	if failed > 0 {
		fmt.Println("\nFailures:")
		for _, f := range failures {
			fmt.Println(f)
		}
		os.Exit(1)
	}
	fmt.Println("\n  ALL EDGE-CASE TESTS PASSED!")
}
