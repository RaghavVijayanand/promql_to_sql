"""
HyperDX dashboard builder — converts normalised dashboard + transpiled
SQL into HyperDX-compatible dashboard JSON (saved searches).

Each Grafana panel → one or more HyperDX chart entries within a dashboard.

Panel type mapping:
  timeseries → line chart
  stat       → number chart
  gauge      → number chart with thresholds
  table      → table chart
  heatmap    → histogram chart
  bar_gauge  → bar chart
"""

import hashlib
import json
import logging
from typing import Any, Dict, List, Optional, Tuple

from migration.analysis.dashboards import (
    NormalisedDashboard,
    NormalisedPanel,
    NormalisedPanelQuery,
    NormalisedVariable,
)
from migration.transpile.dispatch import TranspileResult
from migration.transpile.variables import (
    VariablePreProcessor,
    variable_query_to_sql,
)

log = logging.getLogger("migration.adapt.dashboards")

# ─── Grafana → HyperDX visualisation mapping ─────────────────

_VIZ_MAP: Dict[str, str] = {
    "timeseries": "line",
    "graph":      "line",
    "stat":       "number",
    "singlestat": "number",
    "gauge":      "number",
    "table":      "table",
    "heatmap":    "histogram",
    "histogram":  "histogram",
    "barchart":   "bar",
    "bargauge":   "bar",
    "piechart":   "pie",
    "text":       "markdown",
}

# ─── Output ───────────────────────────────────────────────────


class HyperDXChart:
    """One chart inside a HyperDX dashboard."""

    __slots__ = (
        "id", "title", "chart_type", "sql_queries",
        "filters", "thresholds", "unit", "legend_formats",
        "description",
    )

    def __init__(self, **kw):
        self.id: str = kw.get("id", "")
        self.title: str = kw.get("title", "")
        self.chart_type: str = kw.get("chart_type", "line")
        self.sql_queries: List[str] = kw.get("sql_queries", [])
        self.filters: List[Dict[str, str]] = kw.get("filters", [])
        self.thresholds: List[Dict] = kw.get("thresholds", [])
        self.unit: str = kw.get("unit", "")
        self.legend_formats: List[str] = kw.get("legend_formats", [])
        self.description: str = kw.get("description", "")


class HyperDXDashboard:
    """Complete HyperDX dashboard definition."""

    __slots__ = (
        "external_id", "title", "tags",
        "charts", "variables", "content_hash",
    )

    def __init__(self, **kw):
        self.external_id: str = kw.get("external_id", "")
        self.title: str = kw.get("title", "")
        self.tags: List[str] = kw.get("tags", [])
        self.charts: List[HyperDXChart] = kw.get("charts", [])
        self.variables: List[Dict[str, Any]] = kw.get("variables", [])
        self.content_hash: str = ""

    def compute_hash(self) -> str:
        blob = json.dumps(self.to_dict(), sort_keys=True)
        self.content_hash = hashlib.sha256(blob.encode()).hexdigest()
        return self.content_hash

    def to_dict(self) -> Dict[str, Any]:
        return {
            "externalId": self.external_id,
            "name": self.title,
            "tags": self.tags,
            "charts": [
                {
                    "id": c.id,
                    "name": c.title,
                    "type": c.chart_type,
                    "series": [
                        {"sql": sq, "legend": lf}
                        for sq, lf in zip(
                            c.sql_queries,
                            c.legend_formats + [""] * len(c.sql_queries),
                        )
                    ],
                    "thresholds": c.thresholds,
                    "unit": c.unit,
                    "description": c.description,
                }
                for c in self.charts
            ],
            "filters": self.variables,
        }


# ─── Builder ──────────────────────────────────────────────────


class DashboardBuilder:
    """Builds HyperDX dashboards from normalised Grafana data + SQL."""

    def __init__(
        self,
        default_table: str = "otel.otel_metrics_gauge",
        label_col: str = "Attributes",
        metric_col: str = "MetricName",
    ):
        self.default_table = default_table
        self.label_col = label_col
        self.metric_col = metric_col

    def build(
        self,
        dashboard: NormalisedDashboard,
        transpile_results: Dict[str, TranspileResult],
    ) -> HyperDXDashboard:
        """
        Convert a NormalisedDashboard into a HyperDXDashboard.

        `transpile_results` is keyed by panel query id:
          "panel:{dashboard_uid}:{panel_id}:{ref_id}" → TranspileResult
        """
        hdx = HyperDXDashboard(
            external_id=f"grafana:{dashboard.uid}",
            title=f"[Migrated] {dashboard.title}",
            tags=["migrated", "grafana"] + dashboard.tags,
        )

        # Build variable filters
        for var in dashboard.variables:
            hdx.variables.append(self._build_variable_filter(var))

        # Create a preprocessor with the dashboard's variable defaults
        # so we can annotate generated SQL with parameter placeholders.
        var_defaults = {v.name: v.default_value for v in dashboard.variables}
        preprocessor = VariablePreProcessor(var_defaults)
        # Pre-run preprocessing to populate _substituted mapping
        # (the actual preprocessing happened before transpilation;
        #  we just need the same variable mapping for annotation)
        for panel in dashboard.panels:
            for q in panel.queries:
                preprocessor.preprocess(q.promql)

        # Build charts from panels
        for panel in dashboard.panels:
            chart = self._build_chart(
                panel, dashboard.uid, transpile_results,
                preprocessor=preprocessor,
            )
            if chart is not None:
                hdx.charts.append(chart)

        hdx.compute_hash()
        return hdx

    def build_all(
        self,
        dashboards: List[NormalisedDashboard],
        transpile_results: Dict[str, TranspileResult],
    ) -> List[HyperDXDashboard]:
        result = [self.build(d, transpile_results) for d in dashboards]
        log.info(
            "Built %d HyperDX dashboards with %d total charts",
            len(result),
            sum(len(d.charts) for d in result),
        )
        return result

    # ── internal ──────────────────────────────────────────────

    def _build_chart(
        self,
        panel: NormalisedPanel,
        dash_uid: str,
        results: Dict[str, TranspileResult],
        preprocessor: Optional[VariablePreProcessor] = None,
    ) -> Optional[HyperDXChart]:
        chart_type = _VIZ_MAP.get(panel.viz_type, "line")
        sql_queries: List[str] = []
        legends: List[str] = []
        all_filters: List[Dict[str, str]] = []

        for q in panel.queries:
            key = f"panel:{dash_uid}:{panel.id}:{q.ref_id}"
            tr = results.get(key)
            if tr is None or not tr.ok:
                continue

            sql = tr.sql
            # Annotate SQL: replace sentinel variables with HyperDX
            # parameter placeholders and collect filter metadata
            if preprocessor is not None:
                sql, filters = preprocessor.annotate_sql(sql)
                for f in filters:
                    # Deduplicate by name
                    if not any(ef["name"] == f["name"] for ef in all_filters):
                        all_filters.append(f)

            sql_queries.append(sql)
            legends.append(q.legend_format)

        if not sql_queries:
            return None

        chart_id = f"grafana_{dash_uid}_panel_{panel.id}"
        return HyperDXChart(
            id=chart_id,
            title=panel.title,
            chart_type=chart_type,
            sql_queries=sql_queries,
            filters=all_filters,
            thresholds=panel.thresholds,
            unit=panel.unit,
            legend_formats=legends,
            description=panel.description,
        )

    def _build_variable_filter(
        self,
        var: NormalisedVariable,
    ) -> Dict[str, Any]:
        filter_def: Dict[str, Any] = {
            "name": var.name,
            "label": var.label or var.name,
            "type": var.var_type,
            "default": var.default_value,
        }

        if var.var_type == "query" and var.promql:
            sql = variable_query_to_sql(
                var.promql,
                table=self.default_table,
                label_col=self.label_col,
                metric_col=self.metric_col,
            )
            if sql:
                filter_def["sql"] = sql

        if var.var_type == "custom" and var.options:
            filter_def["options"] = var.options

        if var.var_type == "interval":
            filter_def["options"] = var.options or [
                "1m", "5m", "15m", "30m", "1h", "6h", "12h", "1d",
            ]

        return filter_def
