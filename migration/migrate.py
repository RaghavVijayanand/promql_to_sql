"""
GAP → ClickStack Migration Orchestrator

Main entry point that wires all five layers together:
  1. Discovery   — fetch config from Prometheus, Grafana, Alertmanager
  2. Analysis    — normalise, build dependency graph, toposort
  3. Transpile   — resolve variables, call PromQL→ClickHouse transpiler
  4. Adapt       — generate views, HyperDX dashboards, HyperDX alerts
  5. Deploy      — execute DDL, push to HyperDX, produce report

Usage:
  python migrate.py                    # full migration
  python migrate.py --dry-run          # plan only, no side effects
  python migrate.py --source prom      # only migrate Prometheus rules
  python migrate.py --source grafana   # only migrate Grafana dashboards
"""

import argparse
import logging
import sys
from collections import Counter
from typing import Dict, List, Optional, Tuple

# ── Foundation ────────────────────────────────────────────────
from migration import config
from migration.deploy.report import MigrationReport

# ── Layer 1: Discovery ────────────────────────────────────────
from migration.discovery.prometheus import PrometheusFetcher, PromDiscoveryResult
from migration.discovery.grafana import GrafanaFetcher, GrafanaDiscoveryResult
from migration.discovery.alertmanager import AlertmanagerFetcher, AlertmanagerDiscoveryResult

# ── Layer 2: Semantic Analysis ────────────────────────────────
from migration.analysis.rules import normalise_rules, NormalisedRule, _extract_metrics
from migration.analysis.dashboards import normalise_dashboards, NormalisedDashboard
from migration.analysis.alerts import normalise_alerts, NormalisedAlert
from migration.analysis.graph import build_deployment_plan, DeploymentPlan, NODE_RECORDING

# ── Layer 3: Transpilation ────────────────────────────────────
from migration.transpile.variables import VariablePreProcessor
from migration.transpile.dispatch import TranspileDispatcher, TranspileResult

# ── Layer 4: Adaptation ──────────────────────────────────────
from migration.adapt.schema import SchemaResolver, SchemaMap
from migration.adapt.views import ViewGenerator
from migration.adapt.dashboards import DashboardBuilder
from migration.adapt.alerts import AlertBuilder

# ── Layer 5: Deployment ──────────────────────────────────────
from migration.deploy.clickhouse_exec import ClickHouseExecutor
from migration.deploy.hyperdx import HyperDXClient


log: logging.Logger = logging.getLogger("migration")


# ═══════════════════════════════════════════════════════════════
#  ORCHESTRATOR
# ═══════════════════════════════════════════════════════════════


class Orchestrator:
    """End-to-end migration pipeline."""

    def __init__(
        self,
        dry_run: bool = False,
        sources: Optional[List[str]] = None,
    ):
        self.dry_run = dry_run
        self.sources = sources or ["prom", "grafana", "alertmanager"]
        self.report = MigrationReport()
        self.report.dry_run = dry_run

    def run(self) -> MigrationReport:
        """Execute the full migration and return a report."""
        log.info("Starting %smigration…",
                 "DRY-RUN " if self.dry_run else "")

        # ── 1. Discovery ──────────────────────────────────────
        prom_result, grafana_result, am_result = self._discover()

        # ── 2. Semantic Analysis ──────────────────────────────
        norm_rules, norm_dashboards, norm_alerts, plan = (
            self._analyse(prom_result, grafana_result, am_result)
        )

        # ── 3. Schema Resolution ─────────────────────────────
        schema_map = self._resolve_schema()

        # ── 4. Transpilation ──────────────────────────────────
        transpile_results = self._transpile_all(
            norm_rules, norm_dashboards, norm_alerts, plan, schema_map,
        )

        # ── 5. Adaptation ────────────────────────────────────
        views, hdx_dashboards, hdx_alerts = self._adapt(
            norm_rules, norm_dashboards, norm_alerts,
            transpile_results, schema_map,
        )

        # ── 6. Deployment ────────────────────────────────────
        self._deploy(views, hdx_dashboards, hdx_alerts)

        # ── 7. Report ────────────────────────────────────────
        self.report.finalise()
        report_text = self.report.to_text()
        log.info("\n%s", report_text)
        self.report.save("migration_report.txt")

        return self.report

    # ──────────────────────────────────────────────────────────
    #  LAYER 1: DISCOVERY
    # ──────────────────────────────────────────────────────────

    def _discover(self) -> Tuple[
        PromDiscoveryResult,
        GrafanaDiscoveryResult,
        AlertmanagerDiscoveryResult,
    ]:
        prom_result = PromDiscoveryResult()
        grafana_result = GrafanaDiscoveryResult()
        am_result = AlertmanagerDiscoveryResult()

        if "prom" in self.sources:
            log.info("Discovering Prometheus at %s…", config.PROMETHEUS_URL)
            try:
                prom_result = PrometheusFetcher().fetch_all()
            except Exception as exc:
                log.error("Prometheus discovery failed: %s", exc)

        if "grafana" in self.sources:
            log.info("Discovering Grafana at %s…", config.GRAFANA_URL)
            try:
                grafana_result = GrafanaFetcher().fetch_all()
            except Exception as exc:
                log.error("Grafana discovery failed: %s", exc)

        if "alertmanager" in self.sources:
            log.info("Discovering Alertmanager at %s…", config.ALERTMANAGER_URL)
            try:
                am_result = AlertmanagerFetcher().fetch_all()
            except Exception as exc:
                log.error("Alertmanager discovery failed: %s", exc)

        # Populate report
        self.report.prom_rules = len(prom_result.rules)
        self.report.prom_recording = sum(
            1 for r in prom_result.rules if r.rule_type == "recording"
        )
        self.report.prom_alerting = sum(
            1 for r in prom_result.rules if r.rule_type == "alerting"
        )
        self.report.grafana_dashboards = len(grafana_result.dashboards)
        self.report.grafana_panels = sum(
            len(d.panels) for d in grafana_result.dashboards
        )
        self.report.alertmanager_receivers = len(am_result.receivers)
        self.report.alertmanager_silences = len(am_result.silences)

        return prom_result, grafana_result, am_result

    # ──────────────────────────────────────────────────────────
    #  LAYER 2: SEMANTIC ANALYSIS
    # ──────────────────────────────────────────────────────────

    def _analyse(
        self,
        prom: PromDiscoveryResult,
        grafana: GrafanaDiscoveryResult,
        am: AlertmanagerDiscoveryResult,
    ) -> Tuple[
        List[NormalisedRule],
        List[NormalisedDashboard],
        List[NormalisedAlert],
        DeploymentPlan,
    ]:
        log.info("Normalising artefacts…")

        norm_rules = normalise_rules(prom)
        norm_dashboards = normalise_dashboards(grafana)
        norm_alerts = normalise_alerts(norm_rules, am)

        # Populate report
        self.report.grafana_queries = sum(
            len(p.queries) for d in norm_dashboards for p in d.panels
        )
        self.report.grafana_variables = sum(
            len(d.variables) for d in norm_dashboards
        )

        log.info("Building dependency graph…")
        plan = build_deployment_plan(norm_rules, norm_dashboards, norm_alerts)

        self.report.dep_nodes = len(plan.ordered_nodes)
        self.report.dep_edges = len(plan.edges)
        self.report.dep_cycles = len(plan.cycles)

        if plan.has_cycles:
            log.warning("Dependency cycles detected — deployment "
                        "may require manual review")

        return norm_rules, norm_dashboards, norm_alerts, plan

    # ──────────────────────────────────────────────────────────
    #  SCHEMA RESOLUTION
    # ──────────────────────────────────────────────────────────

    def _resolve_schema(self) -> SchemaMap:
        log.info("Resolving ClickHouse schema…")
        try:
            return SchemaResolver().resolve()
        except Exception as exc:
            log.warning("Schema resolution failed (using defaults): %s", exc)
            return SchemaMap()

    # ──────────────────────────────────────────────────────────
    #  LAYER 3: TRANSPILATION
    # ──────────────────────────────────────────────────────────

    def _transpile_all(
        self,
        rules: List[NormalisedRule],
        dashboards: List[NormalisedDashboard],
        alerts: List[NormalisedAlert],
        plan: DeploymentPlan,
        schema_map: SchemaMap,
    ) -> Dict[str, TranspileResult]:
        log.info("Transpiling %d artefacts…", len(plan.ordered_nodes))

        dispatcher = TranspileDispatcher()

        # Wire schema so transpiler binary produces OTel column names
        if schema_map.default_table:
            dispatcher.set_schema(schema_map.default_table)

        results: Dict[str, TranspileResult] = {}

        # Collect all (source_id, promql) pairs for batch transpilation
        batch: List[Tuple[str, str]] = []

        # Build lookup dicts for dependency-ordered iteration
        rules_by_id = {r.id: r for r in rules}
        alerts_by_id = {a.id: a for a in alerts}

        # Iterate in topological order so recording rules depended on
        # by later rules/alerts get transpiled first.
        for node in plan.ordered_nodes:
            node_id = node.key.split(":", 1)[-1]  # "rule:id" → "id"
            if node_id in rules_by_id:
                rule = rules_by_id[node_id]
                if rule.rule_type == "recording":
                    batch.append((rule.id, rule.promql))
                elif rule.rule_type == "alerting":
                    alert_obj = alerts_by_id.get(rule.id)
                    expr = alert_obj.metric_expr if alert_obj else rule.promql
                    batch.append((f"alert:{rule.id}", expr))

        # Add any rules not in the graph (fallback for disconnected nodes)
        seen = {sid for sid, _ in batch}
        for rule in rules:
            if rule.id not in seen and f"alert:{rule.id}" not in seen:
                if rule.rule_type == "recording":
                    batch.append((rule.id, rule.promql))
                elif rule.rule_type == "alerting":
                    alert_obj = alerts_by_id.get(rule.id)
                    expr = alert_obj.metric_expr if alert_obj else rule.promql
                    batch.append((f"alert:{rule.id}", expr))

        # Dashboard panels
        for dash in dashboards:
            var_defaults = {v.name: v.default_value for v in dash.variables}
            preprocessor = VariablePreProcessor(var_defaults)

            for panel in dash.panels:
                for q in panel.queries:
                    key = f"panel:{dash.uid}:{panel.id}:{q.ref_id}"
                    cleaned = preprocessor.preprocess(q.promql)
                    batch.append((key, cleaned))

        # Execute batch
        results = dispatcher.transpile_batch(batch)

        # Report
        self.report.transpile_total = len(results)
        self.report.transpile_success = sum(
            1 for r in results.values() if r.ok
        )
        self.report.transpile_failed = sum(
            1 for r in results.values() if not r.ok
        )
        for sid, r in results.items():
            if not r.ok:
                self.report.record_transpile_error(sid, r.promql, r.error)

        return results

    # ──────────────────────────────────────────────────────────
    #  LAYER 4: ADAPTATION
    # ──────────────────────────────────────────────────────────

    def _adapt(
        self,
        rules: List[NormalisedRule],
        dashboards: List[NormalisedDashboard],
        alerts: List[NormalisedAlert],
        transpile_results: Dict[str, TranspileResult],
        schema_map: SchemaMap,
    ):
        log.info("Generating ClickStack artefacts…")

        # Count how many times each recording rule is referenced
        ref_counts: Counter = Counter()
        for rule in rules:
            for dep in rule.referenced_rules:
                ref_counts[dep] += 1
        for a in alerts:
            for m in _extract_metrics(a.promql):
                if m in {r.name for r in rules if r.rule_type == "recording"}:
                    ref_counts[m] += 1

        # Views
        vg = ViewGenerator(
            database=config.CLICKHOUSE_DATABASE,
            reference_counts=dict(ref_counts),
        )
        views = vg.generate_all(rules, transpile_results)

        # Dashboards
        default_table = (
            schema_map.default_table.full_name
            if schema_map.default_table
            else f"{config.CLICKHOUSE_DATABASE}.otel_metrics_gauge"
        )
        label_col = (
            schema_map.default_table.labels_col
            if schema_map.default_table
            else "Attributes"
        )
        metric_col = (
            schema_map.default_table.metric_name_col
            if schema_map.default_table
            else "MetricName"
        )
        db = DashboardBuilder(
            default_table=default_table,
            label_col=label_col,
            metric_col=metric_col,
        )
        hdx_dashboards = db.build_all(dashboards, transpile_results)

        # Alerts
        ab = AlertBuilder()
        hdx_alerts = ab.build_all(alerts, transpile_results)

        return views, hdx_dashboards, hdx_alerts

    # ──────────────────────────────────────────────────────────
    #  LAYER 5: DEPLOYMENT
    # ──────────────────────────────────────────────────────────

    def _deploy(self, views, hdx_dashboards, hdx_alerts):
        log.info("Deploying to ClickStack…")

        # ClickHouse views
        ch = ClickHouseExecutor(dry_run=self.dry_run)
        self.report.views_stats = ch.deploy_views(views)

        # HyperDX dashboards + alerts
        hdx = HyperDXClient(dry_run=self.dry_run)
        self.report.dashboard_stats = hdx.deploy_dashboards(hdx_dashboards)
        self.report.alert_stats = hdx.deploy_alerts(hdx_alerts)


# ═══════════════════════════════════════════════════════════════
#  CLI
# ═══════════════════════════════════════════════════════════════


def main():
    parser = argparse.ArgumentParser(
        description="GAP → ClickStack Migration Orchestrator",
    )
    parser.add_argument(
        "--dry-run", action="store_true",
        help="Plan only — do not create any resources.",
    )
    parser.add_argument(
        "--source", nargs="*",
        choices=["prom", "grafana", "alertmanager"],
        default=["prom", "grafana", "alertmanager"],
        help="Which source systems to migrate (default: all).",
    )
    parser.add_argument(
        "--log-level", default=None,
        help="Override log level (DEBUG, INFO, WARNING, ERROR).",
    )
    args = parser.parse_args()

    # Logging
    if args.log_level:
        config.LOG_LEVEL = args.log_level
    config.setup_logging()

    orch = Orchestrator(
        dry_run=args.dry_run or config.DRY_RUN,
        sources=args.source,
    )

    try:
        report = orch.run()
        # Exit with non-zero if there were failures
        total_failures = (
            report.transpile_failed
            + report.views_stats.get("failed", 0)
            + report.dashboard_stats.get("failed", 0)
            + report.alert_stats.get("failed", 0)
        )
        sys.exit(1 if total_failures > 0 else 0)
    except KeyboardInterrupt:
        log.info("Migration interrupted by user.")
        sys.exit(130)
    except Exception as exc:
        log.exception("Migration failed: %s", exc)
        sys.exit(2)


if __name__ == "__main__":
    main()
