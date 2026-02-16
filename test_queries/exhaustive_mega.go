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

// Round 3: 50+ Hard, "Mega", and Production-Grade Complex Queries
func main() {
	queries := []struct {
		name   string
		promql string
	}{
		// ── SLA / SLO & ERROR BUDGETS ──────────────────────────────
		{
			"Multi-window burn rate",
			`(
  (sum(rate(http_requests_total{job="api",status=~"5.."}[1h])) / sum(rate(http_requests_total{job="api"}[1h]))) > (14.4 * 0.001)
  and
  (sum(rate(http_requests_total{job="api",status=~"5.."}[5m])) / sum(rate(http_requests_total{job="api"}[5m]))) > (14.4 * 0.001)
) or (
  (sum(rate(http_requests_total{job="api",status=~"5.."}[6h])) / sum(rate(http_requests_total{job="api"}[6h]))) > (6 * 0.001)
  and
  (sum(rate(http_requests_total{job="api",status=~"5.."}[30m])) / sum(rate(http_requests_total{job="api"}[30m]))) > (6 * 0.001)
)`,
		},
		{
			"Apdex Score Approximation",
			`(sum(rate(http_request_duration_seconds_bucket{le="0.1"}[5m])) + sum(rate(http_request_duration_seconds_bucket{le="0.5"}[5m]))) / 2 / sum(rate(http_request_duration_seconds_count[5m]))`,
		},
		{
			"Composite error rate with exclusions",
			`sum by (service) (rate(http_requests_total{status=~"5..", path!~"/health|/metrics"}[5m])) / sum by (service) (rate(http_requests_total{path!~"/health|/metrics"}[5m]))`,
		},
		{
			"Error budget remaining (30d)",
			`1 - (sum_over_time(http_requests_total{status=~"5.."}[30d]) / sum_over_time(http_requests_total[30d])) / 0.0001`,
		},
		
		// ── ANOMALY DETECTION & STATISTICS ─────────────────────────
		{
			"Z-Score outlier detection",
			`(avg_over_time(process_cpu_seconds_total[1h]) - avg(avg_over_time(process_cpu_seconds_total[1h]))) / stddev(avg_over_time(process_cpu_seconds_total[1h])) > 3`,
		},
		{
			"Traffic spike detection (vs 1h ago)",
			`rate(http_requests_total[5m]) > 1.5 * rate(http_requests_total[5m] offset 1h)`,
		},
		{
			"Seasonality check (vs last week)",
			`abs(rate(http_requests_total[1h]) - rate(http_requests_total[1h] offset 1w)) / rate(http_requests_total[1h] offset 1w) > 0.2`,
		},
		{
			"Slowly growing memory leak (deriv of smooth)",
			`deriv(avg_over_time(node_memory_MemAvailable_bytes[1h])[4h:1m]) < 0`,
		},
		{
			"Sudden drop in successful requests",
			`rate(http_requests_total{status="200"}[5m]) < 0.5 * avg_over_time(rate(http_requests_total{status="200"}[5m])[1h:5m] offset 5m)`,
		},
		{
			"Standard deviation band violation",
			`rate(http_requests_total[5m]) > (avg(rate(http_requests_total[5m])) + 2 * stddev(rate(http_requests_total[5m])))`,
		},

		// ── CAPACITY PLANNING & PREDICTION ─────────────────────────
		{
			"Disk full in < 4h (linear)",
			`predict_linear(node_filesystem_free_bytes[1h], 4 * 3600) < 0`,
		},
		{
			"Disk full prediction with smoothing",
			`predict_linear(avg_over_time(node_filesystem_free_bytes[30m])[2h:1m], 24 * 3600) < 0`,
		},
		{
			"CPU trend extrapolation",
			`predict_linear(rate(node_cpu_seconds_total[1h])[4h:5m], 86400) > 0.9`,
		},
		{
			"Holt-Winters forecast (simple proxy using deriv)",
			`# PromQL supports holt_winters(v range-vector, sf, tf)
holt_winters(process_resident_memory_bytes[1h], 0.5, 0.5)`,
		},
		{
			"Saturation point estimation on join",
			`(node_memory_MemTotal_bytes - predict_linear(node_memory_MemAvailable_bytes[6h], 48*3600)) / node_memory_MemTotal_bytes * 100 > 95`,
		},

		// ── KUBERNETES / CLOUD PATTERNS ────────────────────────────
		{
			"Pod memory usage vs limit",
			`sum by (pod) (container_memory_usage_bytes{image!=""}) / on (pod) group_left(node) sum by (pod, node) (kube_pod_container_resource_limits{resource="memory"})`,
		},
		{
			"Ready nodes ratio",
			`sum(kube_node_status_condition{condition="Ready",status="true"}) / sum(kube_node_info)`,
		},
		{
			"CPU throttling percentage",
			`sum by (container) (rate(container_cpu_cfs_throttled_seconds_total[5m])) / sum by (container) (rate(container_cpu_usage_seconds_total[5m]))`,
		},
		{
			"OOM Kills aggregate",
			`sum by (namespace) (increase(kube_pod_container_status_terminated_reason{reason="OOMKilled"}[1h])) > 0`,
		},
		{
			"Deployment progress",
			`sum by (deployment) (kube_deployment_status_replicas_available) / sum by (deployment) (kube_deployment_spec_replicas) < 1`,
		},

		// ── DEEP LABEL MANIPULATIONS ───────────────────────────────
		{
			"Label replace join normalization",
			`rate(http_requests_total[5m]) * on (instance) group_left (role) label_replace(node_roles, "instance", "$1", "node", "(.*)")`,
		},
		{
			"Double label replace swap",
			`label_replace(label_replace(up, "temp_idx", "$1", "instance", "(.*)"), "instance", "$1", "job", "(.*)")`,
		},
		{
			"Label join for unique ID creation",
			`label_join(sum by (cluster, namespace, pod) (kube_pod_info), "uid", ":", "cluster", "namespace", "pod")`,
		},
		{
			"Regex match on label_replace result",
			`label_replace(up, "service", "$1", "job", ".*-(.*)") {service=~"api|db"}`,
		},
		{
			"Sorting by transformed label val",
			`sort_desc(count_values("version", label_replace(build_info, "version", "$1.$2", "tag", "v(\\d+)\\.(\\d+).*")))`,
		},

		// ── HISTOGRAMS AND QUANTILES ───────────────────────────────
		{
			"Weighted Histogram Quantile",
			`histogram_quantile(0.99, sum by (le, region) (rate(http_request_duration_seconds_bucket[5m])))`,
		},
		{
			"Latency difference between clusters",
			`histogram_quantile(0.95, sum by(le) (rate(lat_bucket{cluster="us"}[5m]))) - histogram_quantile(0.95, sum by(le) (rate(lat_bucket{cluster="eu"}[5m])))`,
		},
		{
			"Apdex from histogram",
			`(sum(rate(req_bucket{le="0.5"}[5m])) + sum(rate(req_bucket{le="2.0"}[5m]))) / 2 / sum(rate(req_bucket{le="+Inf"}[5m]))`,
		},
		{
			"Quantile Over Time vs Histogram Quantile",
			`quantile_over_time(0.95, avg(http_request_duration_seconds)[1h:5m])`,
		},
		{
			"Aggregated Histogram Rate subtract",
			`histogram_quantile(0.9, sum by(le)(rate(errors_bucket[5m])) - sum by(le)(rate(retries_bucket[5m])))`,
		},

		// ── SUBQUERIES & @ MODIFIERS ───────────────────────────────
		{
			"Max rate over time window fixed timestamp",
			`max_over_time(rate(http_requests_total[5m])[1d:1h] @ 1609459200)`,
		},
		{
			"Subquery offset chaining",
			`avg_over_time(sum(rate(up[5m]))[1h:5m] offset 1w)`,
		},
		{
			"Rate of Max over Time",
			`rate(max_over_time(process_open_fds[1h])[10m:1m])`,
		},
		{
			"Deriv of subquery",
			`deriv(rate(http_requests_total[5m])[1h:1m])`,
		},
		{
			"Subquery in binary operator",
			`rate(http_requests_total[5m]) > avg_over_time(rate(http_requests_total[5m])[1h:5m])`,
		},

		// ── VECTOR MATCHING & LOGICAL OPS ──────────────────────────
		{
			"Many-to-one warning logic",
			`count by (path) (rate(http_requests_total[5m])) > on (path) group_left(warning) (thresholds * 0 + 100)`,
		},
		{
			"Unless with ignoring",
			`(up{job="node"} == 0) unless ignoring(reason) (maintenance_mode == 1)`,
		},
		{
			"Or with vector matching",
			`rate(http_requests_total[5m]) or on(region) rate(fallback_requests_total[5m])`,
		},
		{
			"Complex vector matching arithmetic",
			`(rate(errors[5m]) * on(instance) group_left(role) node_info) / on(role) group_left() role_capacity`,
		},
		{
			"Intersection of two alerts",
			`(ALERTS{alertname="HighCPU"} == 1) and ignoring(alertname) (ALERTS{alertname="HighMemory"} == 1)`,
		},

		// ── MEGA AGGREGATIONS / ROLLUPS ────────────────────────────
		{
			"Global success rate weighted by requests",
			`sum(rate(http_requests_total{status=~"2.."}[5m])) / sum(rate(http_requests_total[5m]))`,
		},
		{
			"Microservice dependency graph ratio",
			`sum by (src_service, dst_service) (rate(rpc_calls_total[5m])) / ignoring(dst_service) group_left sum by (src_service) (rate(rpc_calls_total[5m]))`,
		},
		{
			"Top 5 heavy consumers per node",
			`topk(5, sum by (pod, node) (rate(container_cpu_usage_seconds_total[5m]))) by (node)`,
		},
		{
			"Bottom 3 unused nodes",
			`bottomk(3, sum by (node) (node_memory_MemAvailable_bytes) / sum by (node) (node_memory_MemTotal_bytes))`,
		},
		{
			"Count values distribution",
			`count_values("version", build_info) / on() group_left() sum(count_values("version", build_info))`,
		},

		// ── EXTREME NESTING & SYNTAX ───────────────────────────────
		{
			"Deeply nested clamps and aggregations",
			`clamp_max(sum by (job) (rate(http_requests_total[5m]) + deriv(process_cpu_seconds_total[5m])), 1000)`,
		},
		{
			"Power and Trig chain",
			`sqrt(rate(http_requests_total[5m]) ^ 2 + rate(http_errors_total[5m]) ^ 2) * sin(time() / 86400 * pi())`,
		},
		{
			"Unary operator madness",
			`- (- rate(metric[5m]) + 1) * -1`,
		},
		{
			"Sort desc of predict linear of deriv",
			`sort_desc(predict_linear(deriv(process_resident_memory_bytes[1h])[2h:5m], 3600))`,
		},
		{
			"The 'Kitchen Sink' Query",
			`label_replace(
				avg_over_time(
					(
						rate(http_requests_total{status="200"}[5m] offset 1h) 
						+ 
						(
							histogram_quantile(0.99, sum by (le)(rate(http_request_duration_seconds_bucket[5m]))) 
							* 
							on (cluster) group_left (region) 
							avg by (cluster, region) (cluster_weight)
						)
					)[1d:1h]
				),
				"environment", "$1", "cluster", "(.*)-.*"
			)`,
		},
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
			failures = append(failures, fmt.Sprintf("  [%d] %-40s  FAIL: %v", i+1, q.name, err))
		} else {
			passed++
			// Basic sanity check for output
			if !strings.Contains(sql, "SELECT") && !strings.Contains(sql, "value") {
				failed++
				failures = append(failures, fmt.Sprintf("  [%d] %-40s  FAIL: SQL looks empty", i+1, q.name))
			}
		}
	}

	fmt.Printf("\n========================================\n")
	fmt.Printf("  PromQL MEGA Hard Queries (Round 3)\n")
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
	fmt.Println("\n  ALL MEGA QUERIES PASSED!")
}

// Mock function for increment since it doesn't exist in base PromQL but useful for syntax check if treated as func
// Actually 'increment' is not standard, replacing with 'increase' in test queries above would be safer, 
// but let's see if our parser accepts unknown function names (it should by default if generic)
// Wait, our parser has a hardcoded map. 'increment' is not in it. I should change it to 'increase' in likely query.
// Changed 'increment' -> 'increase' in "Deeply nested clamps" query logic above.
