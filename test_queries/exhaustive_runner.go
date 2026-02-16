//go:build ignore

package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shinro/promql-transpiler/pkg/clickhouse"
	"github.com/shinro/promql-transpiler/pkg/transpiler"
)

// exhaustive test covering every PromQL feature
func main() {
	queries := []struct {
		name   string
		promql string
	}{
		// ── SIMPLE SELECTORS ──────────────────────────────────────────
		{"simple metric", `up`},
		{"metric with all matchers", `http_requests_total{job="api",status!="500",method=~"GET|POST",handler!~"/health.*"}`},
		{"label-only selector", `{__name__="up",job="node"}`},

		// ── RANGE VECTORS ────────────────────────────────────────────
		{"range vector", `http_requests_total[5m]`},
		{"range vector with offset", `http_requests_total[5m] offset 1h`},
		{"range vector with @ modifier", `http_requests_total[1h] @ 1700000000`},
		{"range vector offset and @", `http_requests_total[5m] offset 1d @ 1700000000`},

		// ── RATE FAMILY ──────────────────────────────────────────────
		{"rate", `rate(http_requests_total[5m])`},
		{"irate", `irate(http_requests_total[5m])`},
		{"increase", `increase(http_requests_total[1h])`},
		{"delta", `delta(temperature[1h])`},
		{"idelta", `idelta(temperature[5m])`},
		{"rate with labels and offset", `rate(http_requests_total{job="api",status=~"2.."}[5m] offset 1h)`},

		// ── AGGREGATIONS ─────────────────────────────────────────────
		{"sum by", `sum by (job) (rate(http_requests_total[5m]))`},
		{"sum without", `sum without (instance, pod) (rate(http_requests_total[5m]))`},
		{"avg by", `avg by (cluster) (node_cpu_seconds_total)`},
		{"max", `max(up)`},
		{"min by", `min by (job) (up)`},
		{"count by", `count by (status) (http_requests_total)`},
		{"stddev", `stddev(rate(http_requests_total[5m]))`},
		{"stdvar", `stdvar(rate(http_requests_total[5m]))`},
		{"topk", `topk(10, rate(http_requests_total[5m]))`},
		{"bottomk", `bottomk(5, up)`},
		{"quantile", `quantile(0.95, rate(http_requests_total[5m]))`},
		{"count_values", `count_values("version", build_info)`},
		{"group by", `group by (job) (up)`},
		{"pre-grouping syntax", `sum by (job, status) (rate(http_requests_total[5m]))`},
		{"post-grouping syntax", `sum(rate(http_requests_total[5m])) by (job, status)`},

		// ── OVER-TIME FUNCTIONS ──────────────────────────────────────
		{"avg_over_time", `avg_over_time(up[5m])`},
		{"sum_over_time", `sum_over_time(http_requests_total[1h])`},
		{"min_over_time", `min_over_time(temperature[30m])`},
		{"max_over_time", `max_over_time(temperature[30m])`},
		{"count_over_time", `count_over_time(up[5m])`},
		{"stddev_over_time", `stddev_over_time(temperature[1h])`},
		{"stdvar_over_time", `stdvar_over_time(temperature[1h])`},
		{"quantile_over_time", `quantile_over_time(0.9, temperature[1h])`},
		{"last_over_time", `last_over_time(up[5m])`},
		{"present_over_time", `present_over_time(up[5m])`},
		{"absent_over_time", `absent_over_time(up[5m])`},

		// ── MATH FUNCTIONS ───────────────────────────────────────────
		{"abs", `abs(delta(temperature[1h]))`},
		{"ceil", `ceil(rate(http_requests_total[5m]))`},
		{"floor", `floor(rate(http_requests_total[5m]))`},
		{"round", `round(rate(http_requests_total[5m]))`},
		{"round with precision", `round(rate(http_requests_total[5m]), 0.01)`},
		{"sqrt", `sqrt(rate(http_requests_total[5m]))`},
		{"exp", `exp(rate(http_requests_total[5m]))`},
		{"ln", `ln(rate(http_requests_total[5m]))`},
		{"log2", `log2(rate(http_requests_total[5m]))`},
		{"log10", `log10(rate(http_requests_total[5m]))`},
		{"sgn", `sgn(delta(temperature[1h]))`},
		{"clamp", `clamp(rate(http_requests_total[5m]), 0, 100)`},
		{"clamp_min", `clamp_min(rate(http_requests_total[5m]), 0)`},
		{"clamp_max", `clamp_max(rate(http_requests_total[5m]), 100)`},

		// ── TRIGONOMETRIC FUNCTIONS ──────────────────────────────────
		{"sin", `sin(up)`},
		{"cos", `cos(up)`},
		{"tan", `tan(up)`},
		{"asin", `asin(up)`},
		{"acos", `acos(up)`},
		{"atan", `atan(up)`},
		{"sinh", `sinh(up)`},
		{"cosh", `cosh(up)`},
		{"tanh", `tanh(up)`},
		{"asinh", `asinh(up)`},
		{"acosh", `acosh(up)`},
		{"atanh", `atanh(up)`},
		{"deg", `deg(up)`},
		{"rad", `rad(up)`},
		{"pi", `pi()`},

		// ── DATE/TIME FUNCTIONS ──────────────────────────────────────
		{"time()", `time()`},
		{"timestamp", `timestamp(up)`},
		{"day_of_month()", `day_of_month()`},
		{"day_of_week()", `day_of_week()`},
		{"day_of_year()", `day_of_year()`},
		{"days_in_month()", `days_in_month()`},
		{"hour()", `hour()`},
		{"minute()", `minute()`},
		{"month()", `month()`},
		{"year()", `year()`},
		{"day_of_month with arg", `day_of_month(process_start_time_seconds)`},

		// ── HISTOGRAM FUNCTIONS ──────────────────────────────────────
		{"histogram_quantile", `histogram_quantile(0.95, sum by (le) (rate(http_request_duration_seconds_bucket[5m])))`},

		// ── LABEL FUNCTIONS ──────────────────────────────────────────
		{"label_replace", `label_replace(up, "host", "$1", "instance", "(.*):.*")`},
		{"label_join", `label_join(up, "full_id", "-", "job", "instance")`},

		// ── SORT FUNCTIONS ───────────────────────────────────────────
		{"sort", `sort(rate(http_requests_total[5m]))`},
		{"sort_desc", `sort_desc(rate(http_requests_total[5m]))`},

		// ── OTHER FUNCTIONS ──────────────────────────────────────────
		{"absent", `absent(up{job="missing"})`},
		{"changes", `changes(process_start_time_seconds[15m])`},
		{"resets", `resets(process_start_time_seconds[15m])`},
		{"deriv", `deriv(process_cpu_seconds_total[1h])`},
		{"predict_linear", `predict_linear(node_filesystem_free_bytes[1h], 3600)`},
		{"scalar", `scalar(up)`},
		{"vector", `vector(1)`},

		// ── BINARY OPERATIONS ────────────────────────────────────────
		{"addition", `http_requests_total + 10`},
		{"subtraction", `http_requests_total - 5`},
		{"multiplication", `rate(http_requests_total[5m]) * 100`},
		{"division", `http_requests_total / http_requests_failed`},
		{"modulo", `http_requests_total % 10`},
		{"power", `http_requests_total ^ 2`},

		// ── COMPARISON OPERATORS ─────────────────────────────────────
		{"greater than", `http_requests_total > 100`},
		{"less than", `http_requests_total < 1000`},
		{"equal", `http_requests_total == 0`},
		{"not equal", `http_requests_total != 0`},
		{"gte", `rate(http_requests_total[5m]) >= 0.5`},
		{"lte", `rate(http_requests_total[5m]) <= 10`},
		{"bool modifier", `http_requests_total > bool 100`},

		// ── LOGICAL OPERATORS ────────────────────────────────────────
		{"and", `up == 1 and rate(http_requests_total[5m]) > 0`},
		{"or", `up == 0 or absent(up)`},
		{"unless", `rate(http_requests_total[5m]) unless rate(http_errors_total[5m])`},

		// ── VECTOR MATCHING ──────────────────────────────────────────
		{"on()", `rate(http_requests_total[5m]) / on (job) rate(http_errors_total[5m])`},
		{"ignoring()", `rate(http_requests_total[5m]) / ignoring (instance) rate(http_errors_total[5m])`},
		{"group_left", `rate(http_requests_total[5m]) / on (job) group_left (version) build_info`},
		{"group_right", `rate(http_requests_total[5m]) * on (job) group_right (version) build_info`},

		// ── UNARY OPERATORS ──────────────────────────────────────────
		{"unary minus", `-rate(http_requests_total[5m])`},
		{"unary plus", `+rate(http_requests_total[5m])`},

		// ── PARENTHESIZED EXPRESSIONS ────────────────────────────────
		{"parens", `(rate(http_requests_total[5m]) + 1) * 100`},
		{"nested parens", `((up == 1) or (absent(up)))`},

		// ── SUBQUERIES ───────────────────────────────────────────────
		{"subquery", `max_over_time(rate(http_requests_total[5m])[30m:1m])`},
		{"subquery no step", `max_over_time(rate(http_requests_total[5m])[30m:])`},
		{"subquery with offset", `avg_over_time(rate(http_requests_total[5m])[1h:5m] offset 2h)`},

		// ── COMPLEX NESTED QUERIES ───────────────────────────────────
		{"nested agg + rate", `sum by (status) (rate(http_requests_total{job="api"}[5m]))`},
		{"agg of agg", `max(sum by (job) (rate(http_requests_total[5m])))`},
		{"histogram p99", `histogram_quantile(0.99, sum by (le) (rate(http_request_duration_seconds_bucket{job="api"}[5m])))`},
		{"error ratio", `sum(rate(http_requests_total{status=~"5.."}[5m])) / sum(rate(http_requests_total[5m]))`},
		{"multi-level nesting", `topk(5, sum by (job) (rate(http_requests_total{status!="200"}[5m]) / rate(http_requests_total[5m])))`},

		// ── MEGA QUERIES ─────────────────────────────────────────────
		{"mega query 1", `label_replace(histogram_quantile(0.95, sum by (le, path, method) (rate(http_request_duration_seconds_bucket{job=~"core-.*", status!~"5.."}[5m] offset 1h))) / on (path) group_left (version) max_over_time(sum without (pod) (rate(http_requests_total{env="prod"}[1h]) * 100)[30m:1m]), "service", "$1", "job", "(.*)-.*") > 1.5`},
		{"mega query 2", `label_join(label_replace(sort_desc((histogram_quantile(0.99,sum by(le,cluster)(rate(http_request_duration_seconds_bucket{method=~"PO.*|PU.*",status!="200"}[5m]offset 1d)))^(max_over_time(deriv(process_cpu_seconds_total[1h]@1700000000)[2h:1m])/on(cluster)group_left(version,build)count_values("version",build_info)))or(clamp((abs(idelta(kafka_topic_partition_current_offset[5m]))+scalar(round(vector(10.5))))%2,0,10)>bool 1)unless ignoring(instance,pod)((sin(time())*day_of_month())>0 and(changes(process_start_time_seconds[15m])+predict_linear(node_filesystem_free_bytes[1h],3600))>=0)),"source","$1","job","(.*)"),"final_label",",","source","version")`},
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
			failures = append(failures, fmt.Sprintf("  [%d] %-30s  FAIL: %v", i+1, q.name, err))
		} else {
			passed++
			if !strings.Contains(sql, "SELECT") && !strings.Contains(sql, "value") {
				failed++
				failures = append(failures, fmt.Sprintf("  [%d] %-30s  FAIL: SQL looks empty/invalid", i+1, q.name))
			}
		}
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("  PromQL Exhaustive Test Results\n")
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
	fmt.Println("\n  ALL TESTS PASSED!")
}
