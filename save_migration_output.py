"""
Complete Migration Output Generator
====================================
Runs the full GAP → ClickStack migration pipeline and saves ALL output
to migration_output/. Covers everything except actual deployment.

Output structure:
  migration_output/
  ├── REPORT.md                    — Complete human-readable migration report
  ├── discovery/
  │   ├── prometheus_rules.json    — Raw Prometheus rules
  │   ├── prometheus_metrics.json  — Discovered metric names
  │   ├── grafana_dashboards.json  — Raw Grafana dashboard JSON
  │   ├── alertmanager_config.json — Alertmanager receivers & routes
  ├── analysis/
  │   ├── normalised_rules.json    — Normalised recording/alerting rules
  │   ├── normalised_dashboards.json — Normalised dashboard panels
  │   ├── normalised_alerts.json   — Normalised alert definitions
  │   ├── dependency_graph.json    — Deployment dependency graph
  ├── schema/
  │   ├── clickhouse_tables.json   — All OTel tables and columns
  ├── transpilation/
  │   ├── transpiled_sql.json      — All PromQL → SQL conversions
  ├── adaptation/
  │   ├── views.sql                — ClickHouse VIEW/MV DDL
  │   ├── dashboards.json          — HyperDX dashboard definitions
  │   ├── alerts.json              — HyperDX alert definitions
  ├── data_migration/
  │   ├── migrate_prometheus_data.py — Script to migrate historical data
  │   ├── migrate_prometheus_data.sql — Raw SQL for data migration

Usage:
  python save_migration_output.py
"""

import json
import logging
import os
import subprocess
import sys
import textwrap
from datetime import datetime, timezone

sys.path.insert(0, os.path.join(os.path.dirname(__file__), "migration"))
sys.path.insert(0, os.path.dirname(__file__))

from migration import config
from migration.migrate import Orchestrator

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)-7s] %(name)s — %(message)s",
    datefmt="%Y-%m-%dT%H:%M:%S",
)
log = logging.getLogger("save_output")

BASE_DIR = os.path.join(os.path.dirname(__file__), "migration_output")

# Subdirectories
DIRS = {
    "discovery": os.path.join(BASE_DIR, "discovery"),
    "analysis": os.path.join(BASE_DIR, "analysis"),
    "schema": os.path.join(BASE_DIR, "schema"),
    "transpilation": os.path.join(BASE_DIR, "transpilation"),
    "adaptation": os.path.join(BASE_DIR, "adaptation"),
    "data_migration": os.path.join(BASE_DIR, "data_migration"),
}


def _write_json(path, data):
    with open(path, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, default=str)
    log.info("  ✅ %s", os.path.relpath(path, BASE_DIR))


def _write_text(path, text):
    with open(path, "w", encoding="utf-8") as f:
        f.write(text)
    log.info("  ✅ %s", os.path.relpath(path, BASE_DIR))


def _ch_query(sql):
    """Run a ClickHouse query via docker exec and return output."""
    try:
        result = subprocess.run(
            ["docker", "exec", "clickstack", "clickhouse-client", "--query", sql],
            capture_output=True, text=True, timeout=15,
        )
        return result.stdout.strip()
    except Exception as e:
        return f"ERROR: {e}"


def main():
    for d in DIRS.values():
        os.makedirs(d, exist_ok=True)

    timestamp = datetime.now(timezone.utc).isoformat()
    orch = Orchestrator(dry_run=True)

    # ═══════════════════════════════════════════════════════════
    #  1) DISCOVERY
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 1: Discovery ═══")
    prom_result, grafana_result, am_result = orch._discover()

    # Prometheus rules
    prom_rules_data = []
    for r in prom_result.rules:
        prom_rules_data.append({
            "name": r.name,
            "group": r.group_name,
            "type": r.rule_type,
            "query": r.query,
            "duration": getattr(r, "duration", ""),
            "labels": getattr(r, "labels", {}),
            "annotations": getattr(r, "annotations", {}),
            "state": getattr(r, "state", ""),
        })
    _write_json(os.path.join(DIRS["discovery"], "prometheus_rules.json"), prom_rules_data)

    # Prometheus metrics
    _write_json(
        os.path.join(DIRS["discovery"], "prometheus_metrics.json"),
        sorted(list(prom_result.metric_names)),
    )

    # Grafana dashboards (raw)
    grafana_data = []
    for d in grafana_result.dashboards:
        dash_info = {
            "uid": d.uid,
            "title": d.title,
            "panels": [],
        }
        for p in d.panels:
            panel_info = {
                "id": p.id,
                "title": p.title,
                "type": getattr(p, "panel_type", getattr(p, "type", "")),
                "targets": [],
            }
            for t in getattr(p, "targets", []):
                panel_info["targets"].append({
                    "expr": getattr(t, "expr", ""),
                    "refId": getattr(t, "refId", getattr(t, "ref_id", "")),
                    "legendFormat": getattr(t, "legendFormat", getattr(t, "legend_format", "")),
                })
            dash_info["panels"].append(panel_info)
        grafana_data.append(dash_info)
    _write_json(os.path.join(DIRS["discovery"], "grafana_dashboards.json"), grafana_data)

    # Alertmanager config
    def _safe_dict(obj):
        if isinstance(obj, dict):
            return obj
        if hasattr(obj, "__slots__"):
            return {s: getattr(obj, s, None) for s in obj.__slots__}
        if hasattr(obj, "__dict__"):
            return obj.__dict__
        return str(obj)

    am_data = {
        "receivers": [],
        "silences": [_safe_dict(s) for s in am_result.silences],
        "route": _safe_dict(am_result.route) if am_result.route else None,
    }
    for recv in am_result.receivers:
        am_data["receivers"].append({
            "name": recv.name,
            "email_configs": getattr(recv, "email_configs", []),
            "slack_configs": getattr(recv, "slack_configs", []),
            "webhook_configs": getattr(recv, "webhook_configs", []),
            "pagerduty_configs": getattr(recv, "pagerduty_configs", []),
            "other_configs": getattr(recv, "other_configs", []),
        })
    _write_json(os.path.join(DIRS["discovery"], "alertmanager_config.json"), am_data)

    # ═══════════════════════════════════════════════════════════
    #  2) ANALYSIS
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 2: Analysis ═══")
    norm_rules, norm_dashboards, norm_alerts, plan = orch._analyse(
        prom_result, grafana_result, am_result
    )

    # Normalised rules
    rules_data = []
    for r in norm_rules:
        rules_data.append({
            "id": r.id,
            "name": r.name,
            "group_name": r.group_name,
            "rule_type": r.rule_type,
            "promql": r.promql,
            "referenced_rules": getattr(r, "referenced_rules", []),
        })
    _write_json(os.path.join(DIRS["analysis"], "normalised_rules.json"), rules_data)

    # Normalised dashboards
    dash_data = []
    for d in norm_dashboards:
        panels = []
        for p in d.panels:
            queries = []
            for q in p.queries:
                queries.append({
                    "ref_id": q.ref_id,
                    "promql": q.promql,
                    "legend": getattr(q, "legend_format", getattr(q, "legend", "")),
                })
            panels.append({
                "id": p.id,
                "title": p.title,
                "viz_type": p.viz_type,
                "queries": queries,
                "thresholds": getattr(p, "thresholds", []),
                "unit": getattr(p, "unit", ""),
            })
        dash_data.append({
            "uid": d.uid,
            "title": d.title,
            "panels": panels,
            "variables": [
                {"name": v.name, "default_value": v.default_value}
                for v in d.variables
            ],
        })
    _write_json(os.path.join(DIRS["analysis"], "normalised_dashboards.json"), dash_data)

    # Normalised alerts
    alerts_data = []
    for a in norm_alerts:
        alerts_data.append({
            "id": a.id,
            "name": a.name,
            "promql": a.promql,
            "metric_expr": getattr(a, "metric_expr", ""),
            "threshold_op": a.threshold_op,
            "threshold_value": a.threshold_value,
            "for_duration": a.for_duration,
            "severity": a.severity,
            "receiver_name": a.receiver_name,
            "annotations": a.annotations,
        })
    _write_json(os.path.join(DIRS["analysis"], "normalised_alerts.json"), alerts_data)

    # Dependency graph
    graph_data = {
        "nodes": [
            {"key": n.key, "type": getattr(n, "node_type", "")}
            for n in plan.ordered_nodes
        ],
        "edges": [(str(e[0]), str(e[1])) for e in plan.edges],
        "cycles": plan.cycles,
        "has_cycles": plan.has_cycles,
    }
    _write_json(os.path.join(DIRS["analysis"], "dependency_graph.json"), graph_data)

    # ═══════════════════════════════════════════════════════════
    #  3) CLICKHOUSE SCHEMA
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 3: ClickHouse Schema ═══")

    tables_to_inspect = [
        "otel_metrics_gauge", "otel_metrics_sum", "otel_metrics_histogram",
        "otel_metrics_exponential_histogram", "otel_metrics_summary",
        "otel_traces", "otel_logs", "hyperdx_sessions",
    ]

    schema_data = {"database": config.CLICKHOUSE_DATABASE, "tables": {}}
    for tbl in tables_to_inspect:
        cols_raw = _ch_query(
            f"SELECT name, type FROM system.columns "
            f"WHERE database='{config.CLICKHOUSE_DATABASE}' AND table='{tbl}' "
            f"FORMAT JSONEachRow"
        )
        columns = []
        if cols_raw and not cols_raw.startswith("ERROR"):
            for line in cols_raw.strip().split("\n"):
                try:
                    columns.append(json.loads(line))
                except json.JSONDecodeError:
                    pass

        rows_raw = _ch_query(
            f"SELECT count() FROM {config.CLICKHOUSE_DATABASE}.{tbl}"
        )
        row_count = int(rows_raw) if rows_raw.isdigit() else 0

        if columns:
            schema_data["tables"][tbl] = {
                "columns": columns,
                "row_count": row_count,
            }

    # Also get distinct metric names
    for tbl in ["otel_metrics_gauge", "otel_metrics_sum"]:
        raw = _ch_query(
            f"SELECT DISTINCT MetricName FROM {config.CLICKHOUSE_DATABASE}.{tbl} "
            f"FORMAT JSONEachRow"
        )
        metrics = []
        if raw and not raw.startswith("ERROR"):
            for line in raw.strip().split("\n"):
                try:
                    metrics.append(json.loads(line).get("MetricName", ""))
                except json.JSONDecodeError:
                    pass
        if tbl in schema_data["tables"]:
            schema_data["tables"][tbl]["distinct_metrics"] = sorted(metrics)

    _write_json(os.path.join(DIRS["schema"], "clickhouse_tables.json"), schema_data)

    # ═══════════════════════════════════════════════════════════
    #  4) TRANSPILATION
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 4: Transpilation ═══")
    schema_map = orch._resolve_schema()
    transpile_results = orch._transpile_all(
        norm_rules, norm_dashboards, norm_alerts, plan, schema_map,
    )

    sql_data = {}
    for key, tr in transpile_results.items():
        sql_data[key] = {
            "ok": tr.ok,
            "promql": tr.promql,
            "sql": tr.sql if tr.ok else None,
            "error": tr.error if not tr.ok else None,
        }
    _write_json(os.path.join(DIRS["transpilation"], "transpiled_sql.json"), sql_data)

    succeeded = sum(1 for r in transpile_results.values() if r.ok)
    failed = sum(1 for r in transpile_results.values() if not r.ok)

    # ═══════════════════════════════════════════════════════════
    #  5) ADAPTATION
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 5: Adaptation ═══")
    views, hdx_dashboards, hdx_alerts = orch._adapt(
        norm_rules, norm_dashboards, norm_alerts,
        transpile_results, schema_map,
    )

    # Views SQL
    views_sql = "-- ClickHouse Views generated from Prometheus recording rules\n"
    views_sql += "-- Generated by GAP → ClickStack migration tool\n\n"
    for v in views:
        views_sql += f"-- Source: {v.source_rule_id}\n"
        views_sql += f"-- Type: {'MATERIALIZED VIEW' if v.is_materialized else 'VIEW'}\n"
        views_sql += f"-- Hash: {v.content_hash}\n"
        views_sql += f"{v.ddl};\n\n"
    _write_text(os.path.join(DIRS["adaptation"], "views.sql"), views_sql)

    # Dashboards JSON
    _write_json(
        os.path.join(DIRS["adaptation"], "dashboards.json"),
        [d.to_dict() for d in hdx_dashboards],
    )

    # Alerts JSON
    _write_json(
        os.path.join(DIRS["adaptation"], "alerts.json"),
        [a.to_dict() for a in hdx_alerts],
    )

    # ═══════════════════════════════════════════════════════════
    #  6) DATA MIGRATION
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 6: Data Migration Artifacts ═══")

    # The metrics list from the manual migration reference
    app_metrics = [
        "http_requests_total",
        "network_bytes_sent_total",
        "http_request_duration_milliseconds_bucket",
        "http_request_duration_milliseconds_count",
        "http_request_duration_milliseconds_sum",
        "http_response_size_bytes_bucket",
        "http_response_size_bytes_count",
        "http_response_size_bytes_sum",
        "system_cpu_usage_percent",
        "system_memory_usage_bytes",
    ]

    # OTel-format metric names (what the OTel SDK actually sends)
    otel_metrics = [
        "http.server.request.count",
        "http.server.request.duration",
        "http.server.response.size",
        "system.cpu.utilization",
        "system.memory.usage",
        "system.network.io",
    ]

    # Generate the Python data migration script
    migrate_py = textwrap.dedent(f'''\
        """
        Migrate historical Prometheus metric data into ClickHouse.

        This script queries Prometheus for historical metric data and inserts
        it into the ClickHouse OTel tables so that HyperDX can visualise
        both old (Prometheus) and new (OTel) data in a single timeline.

        Prerequisites:
          pip install requests clickhouse-connect

        Usage:
          python migrate_prometheus_data.py
        """

        import requests
        import datetime
        import time
        import clickhouse_connect

        # ── Config ────────────────────────────────────────────────
        PROM_URL = "{config.PROMETHEUS_URL}"
        CLICKHOUSE_HOST = "{config.CLICKHOUSE_HOST}"
        CLICKHOUSE_PORT = {config.CLICKHOUSE_PORT}
        CLICKHOUSE_DB = "{config.CLICKHOUSE_DATABASE}"

        # Metrics to migrate from Prometheus
        # These are the Prometheus-format metric names from the GAP stack
        METRICS = {json.dumps(app_metrics, indent=4)}

        # Mapping: Prometheus metric name → ClickHouse target table
        # Gauges (point-in-time) → otel_metrics_gauge
        # Counters (cumulative) → otel_metrics_sum
        METRIC_TABLE_MAP = {{
            "http_requests_total": "otel_metrics_sum",
            "network_bytes_sent_total": "otel_metrics_sum",
            "http_request_duration_milliseconds_bucket": "otel_metrics_histogram",
            "http_request_duration_milliseconds_count": "otel_metrics_sum",
            "http_request_duration_milliseconds_sum": "otel_metrics_sum",
            "http_response_size_bytes_bucket": "otel_metrics_histogram",
            "http_response_size_bytes_count": "otel_metrics_sum",
            "http_response_size_bytes_sum": "otel_metrics_sum",
            "system_cpu_usage_percent": "otel_metrics_gauge",
            "system_memory_usage_bytes": "otel_metrics_gauge",
        }}

        # Time range: last 3 days
        END = int(time.time())
        START = END - (3 * 24 * 60 * 60)
        STEP = 60  # 1-minute resolution

        # ── Connect ───────────────────────────────────────────────
        client = clickhouse_connect.get_client(
            host=CLICKHOUSE_HOST,
            port=CLICKHOUSE_PORT,
        )
        print("✅ Connected to ClickHouse")

        # ── Migration Loop ────────────────────────────────────────
        total_rows = 0

        for metric in METRICS:
            print(f"\\n🔄 Migrating: {{metric}}")

            response = requests.get(
                f"{{PROM_URL}}/api/v1/query_range",
                params={{
                    "query": metric,
                    "start": START,
                    "end": END,
                    "step": STEP,
                }},
            )

            if response.status_code != 200:
                print(f"  ❌ Prometheus query failed: {{response.status_code}}")
                continue

            data = response.json().get("data", {{}}).get("result", [])
            if not data:
                print("  ⚠ No data found")
                continue

            target_table = METRIC_TABLE_MAP.get(metric, "otel_metrics_gauge")

            rows = []
            for series in data:
                labels = series["metric"]
                labels.pop("__name__", None)

                for ts, value in series["values"]:
                    rows.append({{
                        "MetricName": metric,
                        "TimeUnix": datetime.datetime.fromtimestamp(float(ts)),
                        "Value": float(value),
                        "Attributes": labels,
                    }})

            if rows:
                try:
                    client.insert(
                        f"{{CLICKHOUSE_DB}}.{{target_table}}",
                        data=[[r["MetricName"], r["TimeUnix"], r["Value"], r["Attributes"]]
                              for r in rows],
                        column_names=["MetricName", "TimeUnix", "Value", "Attributes"],
                    )
                    print(f"  ✅ Inserted {{len(rows)}} rows → {{target_table}}")
                    total_rows += len(rows)
                except Exception as e:
                    print(f"  ❌ Insert failed: {{e}}")
            else:
                print("  ⚠ No data points")

        print(f"\\n🎯 Migration complete. Total rows inserted: {{total_rows}}")
    ''')
    _write_text(os.path.join(DIRS["data_migration"], "migrate_prometheus_data.py"), migrate_py)

    # Generate raw SQL for reference
    migrate_sql_lines = [
        "-- Historical Data Migration: Prometheus → ClickHouse",
        "-- These queries show the target table structure for migrated data",
        "",
        "-- ═══════════════════════════════════════════",
        "-- Target tables (already created by OTel Collector)",
        "-- ═══════════════════════════════════════════",
        "",
        f"-- Database: {config.CLICKHOUSE_DATABASE}",
        "-- Gauge metrics → otel_metrics_gauge",
        "--   system_cpu_usage_percent",
        "--   system_memory_usage_bytes",
        "",
        "-- Sum/Counter metrics → otel_metrics_sum",
        "--   http_requests_total",
        "--   network_bytes_sent_total",
        "--   http_request_duration_milliseconds_count",
        "--   http_request_duration_milliseconds_sum",
        "--   http_response_size_bytes_count",
        "--   http_response_size_bytes_sum",
        "",
        "-- Histogram metrics → otel_metrics_histogram",
        "--   http_request_duration_milliseconds_bucket",
        "--   http_response_size_bytes_bucket",
        "",
        "-- ═══════════════════════════════════════════",
        "-- Sample ClickHouse queries for migrated data",
        "-- ═══════════════════════════════════════════",
        "",
        "-- Memory usage (MB) over time",
        f"SELECT",
        f"    toStartOfMinute(TimeUnix) AS timestamp,",
        f"    avg(Value) / 1024 / 1024 AS memory_mb",
        f"FROM {config.CLICKHOUSE_DATABASE}.otel_metrics_gauge",
        f"WHERE MetricName = 'system.memory.usage'",
        f"    AND TimeUnix >= now() - INTERVAL 15 MINUTE",
        f"GROUP BY timestamp",
        f"ORDER BY timestamp;",
        "",
        "-- CPU utilization (%) over time",
        f"SELECT",
        f"    toStartOfMinute(TimeUnix) AS timestamp,",
        f"    avg(Value) * 100 AS cpu_percent",
        f"FROM {config.CLICKHOUSE_DATABASE}.otel_metrics_gauge",
        f"WHERE MetricName = 'system.cpu.utilization'",
        f"    AND TimeUnix >= now() - INTERVAL 15 MINUTE",
        f"GROUP BY timestamp",
        f"ORDER BY timestamp;",
        "",
        "-- Request throughput by status code",
        f"SELECT",
        f"    toStartOfMinute(TimeUnix) AS timestamp,",
        f"    Attributes['http.response.status_code'] AS status_code,",
        f"    (max(Value) - min(Value)) / 60 AS rps",
        f"FROM {config.CLICKHOUSE_DATABASE}.otel_metrics_sum",
        f"WHERE MetricName = 'http.server.request.count'",
        f"    AND TimeUnix >= now() - INTERVAL 15 MINUTE",
        f"GROUP BY timestamp, status_code",
        f"ORDER BY timestamp;",
        "",
        "-- ═══════════════════════════════════════════",
        "-- OTel-equivalent metrics (what the OTel SDK sends)",
        "-- ═══════════════════════════════════════════",
        "-- Prometheus name              → OTel name",
        "-- system_cpu_usage_percent     → system.cpu.utilization",
        "-- system_memory_usage_bytes    → system.memory.usage",
        "-- http_requests_total          → http.server.request.count",
        "-- http_request_duration_*      → http.server.request.duration",
        "-- http_response_size_bytes_*   → http.server.response.size",
        "-- network_bytes_sent_total     → system.network.io",
    ]
    _write_text(
        os.path.join(DIRS["data_migration"], "migrate_prometheus_data.sql"),
        "\n".join(migrate_sql_lines) + "\n",
    )

    # ═══════════════════════════════════════════════════════════
    #  7) COMPLETE REPORT
    # ═══════════════════════════════════════════════════════════
    log.info("═══ PHASE 7: Report ═══")

    # Build schema summary for report
    schema_summary = ""
    for tbl, info in schema_data.get("tables", {}).items():
        cols = info.get("columns", [])
        rows = info.get("row_count", 0)
        metrics_list = info.get("distinct_metrics", [])
        schema_summary += f"\n### `{tbl}` ({rows} rows, {len(cols)} columns)\n"
        schema_summary += "| Column | Type |\n|--------|------|\n"
        for c in cols:
            schema_summary += f"| `{c['name']}` | `{c['type']}` |\n"
        if metrics_list:
            schema_summary += f"\n**Distinct metrics ({len(metrics_list)}):** "
            schema_summary += ", ".join(f"`{m}`" for m in metrics_list[:15])
            if len(metrics_list) > 15:
                schema_summary += f" ... (+{len(metrics_list)-15} more)"
            schema_summary += "\n"

    # Transpilation summary
    transpile_summary = ""
    for key, info in sql_data.items():
        status = "✅" if info["ok"] else "❌"
        transpile_summary += f"\n#### {status} `{key}`\n"
        transpile_summary += f"**PromQL:** `{info['promql']}`\n"
        if info["ok"]:
            transpile_summary += f"```sql\n{info['sql']}\n```\n"
        else:
            transpile_summary += f"**Error:** {info['error']}\n"

    report = textwrap.dedent(f"""\
    # GAP → ClickStack Migration Report

    **Generated:** {timestamp}
    **Status:** All phases complete (deployment skipped — output saved to files)

    ---

    ## Summary

    | Phase | Items | Status |
    |-------|-------|--------|
    | Discovery | {len(prom_result.rules)} Prom rules, {len(grafana_result.dashboards)} Grafana dashboards, {len(am_result.receivers)} AM receivers | ✅ |
    | Analysis | {len(norm_rules)} normalised rules, {len(norm_dashboards)} dashboards, {len(norm_alerts)} alerts | ✅ |
    | Schema | {len(schema_data.get('tables', {}))} ClickHouse tables | ✅ |
    | Transpilation | {succeeded}/{len(transpile_results)} succeeded | {"✅" if failed == 0 else "⚠️"} |
    | Adaptation | {len(views)} views, {len(hdx_dashboards)} dashboards, {len(hdx_alerts)} alerts | ✅ |
    | Data Migration | Scripts generated | 📄 |

    ---

    ## 1. Discovery

    ### Prometheus
    - **URL:** `{config.PROMETHEUS_URL}`
    - **Rules found:** {len(prom_result.rules)}
      - Alerting: {sum(1 for r in prom_result.rules if r.rule_type == 'alerting')}
      - Recording: {sum(1 for r in prom_result.rules if r.rule_type == 'recording')}
    - **Metrics discovered:** {len(prom_result.metric_names)}

    ### Grafana
    - **URL:** `{config.GRAFANA_URL}`
    - **Dashboards:** {len(grafana_result.dashboards)}
    - **Total panels:** {sum(len(d.panels) for d in grafana_result.dashboards)}

    ### Alertmanager
    - **URL:** `{config.ALERTMANAGER_URL}`
    - **Receivers:** {len(am_result.receivers)}
    - **Active silences:** {len(am_result.silences)}

    ---

    ## 2. Normalised Alerts

    | Alert | PromQL | Threshold | Severity |
    |-------|--------|-----------|----------|
    """)

    for a in norm_alerts:
        report += f"| **{a.name}** | `{a.promql[:60]}{'...' if len(a.promql)>60 else ''}` | {a.threshold_op} {a.threshold_value} | {a.severity} |\n"

    report += f"""
---

## 3. ClickHouse Schema (OTel Tables)

The OpenTelemetry Collector inside ClickStack stores telemetry in these tables:

{schema_summary}

---

## 4. Transpilation Results

**{succeeded} succeeded, {failed} failed** out of {len(transpile_results)} total.

{transpile_summary}

---

## 5. Adapted Output

### Views ({len(views)})
{"No recording rules → no views generated." if not views else "See `adaptation/views.sql` for DDL."}

### Dashboards ({len(hdx_dashboards)})
"""
    for d in hdx_dashboards:
        dd = d.to_dict()
        report += f"- **{dd['name']}** — {len(dd.get('charts',[]))} charts\n"

    report += f"""
### Alerts ({len(hdx_alerts)})
"""
    for a in hdx_alerts:
        ad = a.to_dict()
        report += f"- **{ad['name']}** — threshold: {ad.get('thresholdType','')} {ad.get('threshold','')}, interval: {ad.get('interval','')}\n"

    report += f"""
---

## 6. Data Migration

### Prometheus → ClickHouse Metric Mapping

| Prometheus Metric | OTel Equivalent | Target Table |
|-------------------|-----------------|--------------|
| `system_cpu_usage_percent` | `system.cpu.utilization` | `otel_metrics_gauge` |
| `system_memory_usage_bytes` | `system.memory.usage` | `otel_metrics_gauge` |
| `http_requests_total` | `http.server.request.count` | `otel_metrics_sum` |
| `network_bytes_sent_total` | `system.network.io` | `otel_metrics_sum` |
| `http_request_duration_ms_*` | `http.server.request.duration` | `otel_metrics_histogram` |
| `http_response_size_bytes_*` | `http.server.response.size` | `otel_metrics_histogram` |

### Scripts
- `data_migration/migrate_prometheus_data.py` — Python script to pull historical data from Prometheus and insert into ClickHouse
- `data_migration/migrate_prometheus_data.sql` — Reference SQL queries for the migrated data

---

## File Index

```
migration_output/
├── REPORT.md
├── discovery/
│   ├── prometheus_rules.json
│   ├── prometheus_metrics.json
│   ├── grafana_dashboards.json
│   └── alertmanager_config.json
├── analysis/
│   ├── normalised_rules.json
│   ├── normalised_dashboards.json
│   ├── normalised_alerts.json
│   └── dependency_graph.json
├── schema/
│   └── clickhouse_tables.json
├── transpilation/
│   └── transpiled_sql.json
├── adaptation/
│   ├── views.sql
│   ├── dashboards.json
│   └── alerts.json
└── data_migration/
    ├── migrate_prometheus_data.py
    └── migrate_prometheus_data.sql
```
"""

    _write_text(os.path.join(BASE_DIR, "REPORT.md"), report)

    # Final summary
    log.info("\n" + "=" * 60)
    log.info("COMPLETE MIGRATION OUTPUT SAVED")
    log.info("=" * 60)
    log.info("  📁 %s", BASE_DIR)
    log.info("  📊 Discovery:     %d rules, %d dashboards, %d receivers",
             len(prom_result.rules), len(grafana_result.dashboards), len(am_result.receivers))
    log.info("  🔍 Analysis:      %d rules, %d dashboards, %d alerts",
             len(norm_rules), len(norm_dashboards), len(norm_alerts))
    log.info("  🗄️  Schema:       %d tables", len(schema_data.get("tables", {})))
    log.info("  🔄 Transpiled:    %d/%d OK", succeeded, len(transpile_results))
    log.info("  📦 Adapted:       %d views, %d dashboards, %d alerts",
             len(views), len(hdx_dashboards), len(hdx_alerts))
    log.info("  📜 Data Migration: scripts ready")
    log.info("=" * 60)


if __name__ == "__main__":
    main()
