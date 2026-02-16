# GAP → ClickStack Migration Orchestrator

Automated migration of observability logic (queries, dashboards, alerts)
from a **Prometheus + Grafana + Alertmanager** stack into **ClickStack**
(ClickHouse + HyperDX + OpenTelemetry).

---

## Quick Start

```bash
cd migration/

# 1. Copy and configure environment
cp .env.example .env
#    Edit .env with your endpoint URLs, API tokens, etc.

# 2. Install Python dependencies
pip install -r requirements.txt

# 3. Dry-run (plan only, no side-effects)
python migrate.py --dry-run

# 4. Full migration
python migrate.py
```

---

## What It Does

| Source | Artefact | Target in ClickStack |
|--------|----------|----------------------|
| Prometheus recording rules | PromQL → ClickHouse SQL | `CREATE VIEW` / `MATERIALIZED VIEW` in ClickHouse |
| Prometheus alert rules | PromQL → ClickHouse SQL | HyperDX alert with SQL condition |
| Grafana dashboards + panels | PromQL → ClickHouse SQL | HyperDX dashboard with SQL-backed charts |
| Grafana template variables | `label_values()` → SQL | HyperDX filter dropdowns |
| Alertmanager routing/receivers | YAML → channel config | HyperDX notification channels |

---

## Configuration (.env)

All client-specific values live in a single `.env` file. See
[.env.example](.env.example) for the full reference. Key settings:

| Variable | Purpose |
|----------|---------|
| `PROMETHEUS_URL` | Prometheus HTTP API base URL |
| `GRAFANA_URL` | Grafana HTTP API base URL |
| `GRAFANA_API_TOKEN` | Grafana service-account or API key |
| `ALERTMANAGER_URL` | Alertmanager v2 API base URL |
| `CLICKHOUSE_HOST/PORT/USER/PASSWORD` | ClickHouse connection inside ClickStack |
| `CLICKHOUSE_DATABASE` | OTel database name (default: `otel`) |
| `HYPERDX_URL` | HyperDX API base URL |
| `HYPERDX_API_KEY` | HyperDX authentication token |
| `TRANSPILER_BINARY` | Path to the compiled PromQL→ClickHouse transpiler |
| `METRIC_NAME_STYLE` | `auto` / `prometheus` / `otel` |
| `MIGRATION_DRY_RUN` | `true` to plan without deploying |
| `MIGRATION_PRUNE_ORPHANS` | `true` to remove stale artefacts |

---

## Architecture

```
migration/
  migrate.py              ← main entry point (CLI)
  config.py               ← loads .env, exposes typed settings
  http_client.py          ← shared HTTP with retry + auth

  discovery/              ← Layer 1: API fetchers
    prometheus.py            GET /api/v1/rules, /metadata, /targets
    grafana.py               GET /api/search, /api/dashboards/uid/:uid
    alertmanager.py          GET /api/v2/status, /alerts, /silences

  analysis/               ← Layer 2: normalisation + dependency graph
    rules.py                 Prometheus rules → NormalisedRule
    dashboards.py            Grafana panels → NormalisedDashboard
    alerts.py                Alert + routing merge → NormalisedAlert
    graph.py                 DAG builder + topological sort

  transpile/              ← Layer 3: PromQL → ClickHouse SQL
    variables.py             Grafana variable pre-processing
    dispatch.py              Calls the Go transpiler binary

  adapt/                  ← Layer 4: generate ClickStack artefacts
    schema.py                Introspect ClickHouse OTel tables
    views.py                 Recording rules → VIEW / MV DDL
    dashboards.py            Panels → HyperDX dashboard JSON
    alerts.py                Alerts → HyperDX alert JSON

  deploy/                 ← Layer 5: execute + track
    clickhouse_exec.py       Run DDL, manage _migration_state table
    hyperdx.py               Push dashboards + alerts via REST
    report.py                Human-readable + JSON migration report
```

---

## CLI Usage

```bash
# Full migration (all sources)
python migrate.py

# Dry-run mode
python migrate.py --dry-run

# Migrate only Prometheus rules
python migrate.py --source prom

# Migrate only Grafana dashboards
python migrate.py --source grafana

# Migrate only Alertmanager config
python migrate.py --source alertmanager

# Combine sources
python migrate.py --source prom grafana

# Override log level
python migrate.py --log-level DEBUG
```

---

## Idempotency

Every migration artefact is tracked in a ClickHouse table
(`_migration_state`) by its content hash (SHA-256). Re-running the
migration after no changes produces zero mutations. Changed artefacts
are updated in place; new artefacts are created; stale artefacts are
left untouched unless `--prune` is enabled.

---

## Edge Cases Handled

- **Nested recording rules** — dependency graph resolves deployment order
- **Multi-query Grafana panels** — each target transpiled independently
- **Dashboard variables** — substituted with defaults before transpilation,
  parameterised in HyperDX after
- **Histogram queries** — routed to the histogram OTel table
- **Alert thresholds** — extracted from PromQL and mapped to HyperDX conditions
- **FOR durations** — preserved in HyperDX alert configuration
- **Metric name conventions** — auto-mapped between `http_requests_total`
  (Prometheus) and `http.requests.total` (OTel)
