"""
Migration report generator — produces a human-readable summary of the
migration run including counts, failures, and actionable next steps.
"""

import json
import logging
from datetime import datetime, timezone
from typing import Any, Dict, List, Optional

log = logging.getLogger("migration.deploy.report")


class MigrationReport:
    """Accumulates migration statistics and produces a report."""

    def __init__(self):
        self.started_at: str = datetime.now(timezone.utc).isoformat()
        self.finished_at: str = ""

        # Source counts
        self.prom_rules: int = 0
        self.prom_recording: int = 0
        self.prom_alerting: int = 0
        self.grafana_dashboards: int = 0
        self.grafana_panels: int = 0
        self.grafana_queries: int = 0
        self.grafana_variables: int = 0
        self.alertmanager_receivers: int = 0
        self.alertmanager_silences: int = 0

        # Transpilation
        self.transpile_total: int = 0
        self.transpile_success: int = 0
        self.transpile_failed: int = 0
        self.transpile_errors: List[Dict[str, str]] = []

        # Deployment
        self.views_stats: Dict[str, int] = {}
        self.dashboard_stats: Dict[str, int] = {}
        self.alert_stats: Dict[str, int] = {}

        # Dependency graph
        self.dep_nodes: int = 0
        self.dep_edges: int = 0
        self.dep_cycles: int = 0

        # Dry-run flag
        self.dry_run: bool = False

    def finalise(self):
        self.finished_at = datetime.now(timezone.utc).isoformat()

    def record_transpile_error(
        self, source_id: str, promql: str, error: str
    ):
        self.transpile_errors.append({
            "source_id": source_id,
            "promql": promql[:200],
            "error": error[:300],
        })

    def to_text(self) -> str:
        """Generate a human-readable text report."""
        lines = []
        bar = "=" * 70

        if self.dry_run:
            lines.append("DRY-RUN MIGRATION REPORT")
        else:
            lines.append("MIGRATION REPORT")
        lines.append(bar)
        lines.append(f"Started:   {self.started_at}")
        lines.append(f"Finished:  {self.finished_at}")
        lines.append("")

        # ── Discovery ──
        lines.append("DISCOVERY")
        lines.append("-" * 40)
        lines.append(f"  Prometheus rules:      {self.prom_rules}")
        lines.append(f"    Recording:           {self.prom_recording}")
        lines.append(f"    Alerting:            {self.prom_alerting}")
        lines.append(f"  Grafana dashboards:    {self.grafana_dashboards}")
        lines.append(f"    Panels:              {self.grafana_panels}")
        lines.append(f"    PromQL queries:      {self.grafana_queries}")
        lines.append(f"    Variables:           {self.grafana_variables}")
        lines.append(f"  Alertmanager receivers:{self.alertmanager_receivers}")
        lines.append(f"  Alertmanager silences: {self.alertmanager_silences}")
        lines.append("")

        # ── Dependency Graph ──
        lines.append("DEPENDENCY GRAPH")
        lines.append("-" * 40)
        lines.append(f"  Nodes: {self.dep_nodes}   Edges: {self.dep_edges}   Cycles: {self.dep_cycles}")
        lines.append("")

        # ── Transpilation ──
        lines.append("TRANSPILATION")
        lines.append("-" * 40)
        lines.append(f"  Total:     {self.transpile_total}")
        lines.append(f"  Succeeded: {self.transpile_success}")
        lines.append(f"  Failed:    {self.transpile_failed}")

        if self.transpile_errors:
            lines.append("")
            lines.append("  Failed expressions:")
            for err in self.transpile_errors[:20]:
                lines.append(f"    [{err['source_id']}]")
                lines.append(f"      PromQL: {err['promql']}")
                lines.append(f"      Error:  {err['error']}")
            if len(self.transpile_errors) > 20:
                lines.append(f"    ... and {len(self.transpile_errors) - 20} more")
        lines.append("")

        # ── Deployment ──
        lines.append("DEPLOYMENT")
        lines.append("-" * 40)
        for label, stats in [
            ("Views",      self.views_stats),
            ("Dashboards", self.dashboard_stats),
            ("Alerts",     self.alert_stats),
        ]:
            if stats:
                parts = ", ".join(f"{k}={v}" for k, v in stats.items()
                                 if k != "errors")
                lines.append(f"  {label}: {parts}")
                for e in stats.get("errors", []):
                    if isinstance(e, dict):
                        lines.append(f"    ERROR: {e.get('name', e.get('id', '?'))} — {e.get('error', '?')}")
        lines.append("")
        lines.append(bar)

        return "\n".join(lines)

    def to_dict(self) -> Dict[str, Any]:
        """Structured JSON-serialisable report."""
        return {
            "started_at": self.started_at,
            "finished_at": self.finished_at,
            "dry_run": self.dry_run,
            "discovery": {
                "prometheus_rules": self.prom_rules,
                "recording_rules": self.prom_recording,
                "alert_rules": self.prom_alerting,
                "grafana_dashboards": self.grafana_dashboards,
                "grafana_panels": self.grafana_panels,
                "grafana_queries": self.grafana_queries,
                "grafana_variables": self.grafana_variables,
                "alertmanager_receivers": self.alertmanager_receivers,
                "alertmanager_silences": self.alertmanager_silences,
            },
            "dependency_graph": {
                "nodes": self.dep_nodes,
                "edges": self.dep_edges,
                "cycles": self.dep_cycles,
            },
            "transpilation": {
                "total": self.transpile_total,
                "succeeded": self.transpile_success,
                "failed": self.transpile_failed,
                "errors": self.transpile_errors,
            },
            "deployment": {
                "views": self.views_stats,
                "dashboards": self.dashboard_stats,
                "alerts": self.alert_stats,
            },
        }

    def save(self, path: str):
        """Save both text and JSON reports."""
        text = self.to_text()
        with open(path, "w", encoding="utf-8") as f:
            f.write(text)
        with open(path.replace(".txt", ".json"), "w", encoding="utf-8") as f:
            json.dump(self.to_dict(), f, indent=2)
        log.info("Report saved to %s", path)
