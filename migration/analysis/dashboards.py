"""
Dashboard normaliser — converts raw Grafana dashboards into the
migration intermediate representation, extracting PromQL targets,
variable references, and visualization metadata.
"""

import logging
import re
from typing import Dict, List, Optional, Set

from migration.discovery.grafana import (
    GrafanaDashboard,
    GrafanaDiscoveryResult,
    GrafanaPanel,
    GrafanaVariable,
)

log = logging.getLogger("migration.analysis.dashboards")

# Regex that catches $var, ${var}, ${var:format}
_VAR_REF_RE = re.compile(r'\$\{?([a-zA-Z_]\w*)(?::[^}]*)?\}?')

# Standard Grafana built-in variables (should NOT be treated as custom vars)
_BUILTIN_VARS: Set[str] = {
    "__interval", "__interval_ms", "__rate_interval",
    "__range", "__range_s", "__range_ms",
    "__from", "__to", "__name", "__org",
    "__dashboard", "__panel", "__user",
    "timeFilter", "timeFrom", "timeTo", "timeGroup",
    "unixEpochFilter", "unixEpochFrom", "unixEpochTo",
}

# ─── Normalised output ───────────────────────────────────────


class NormalisedPanelQuery:
    """One PromQL target inside a panel."""

    __slots__ = ("ref_id", "promql", "legend_format", "instant",
                 "variable_refs")

    def __init__(
        self,
        ref_id: str,
        promql: str,
        legend_format: str = "",
        instant: bool = False,
    ):
        self.ref_id = ref_id
        self.promql = promql
        self.legend_format = legend_format
        self.instant = instant
        self.variable_refs: List[str] = []


class NormalisedPanel:
    """Panel in migration IR."""

    __slots__ = ("id", "dashboard_uid", "title", "viz_type",
                 "queries", "thresholds", "unit", "description")

    def __init__(
        self,
        panel_id: int,
        dashboard_uid: str,
        title: str,
        viz_type: str,
        queries: Optional[List[NormalisedPanelQuery]] = None,
        thresholds: Optional[List[Dict]] = None,
        unit: str = "",
        description: str = "",
    ):
        self.id = panel_id
        self.dashboard_uid = dashboard_uid
        self.title = title
        self.viz_type = viz_type
        self.queries = queries or []
        self.thresholds = thresholds or []
        self.unit = unit
        self.description = description


class NormalisedVariable:
    """Template variable in migration IR."""

    __slots__ = ("name", "var_type", "promql", "default_value",
                 "options", "label")

    def __init__(
        self,
        name: str,
        var_type: str,
        promql: str = "",
        default_value: str = "",
        options: Optional[List[str]] = None,
        label: str = "",
    ):
        self.name = name
        self.var_type = var_type
        self.promql = promql
        self.default_value = default_value
        self.options = options or []
        self.label = label


class NormalisedDashboard:
    """Dashboard in migration IR."""

    __slots__ = ("uid", "title", "tags", "panels", "variables")

    def __init__(
        self,
        uid: str,
        title: str,
        tags: Optional[List[str]] = None,
        panels: Optional[List[NormalisedPanel]] = None,
        variables: Optional[List[NormalisedVariable]] = None,
    ):
        self.uid = uid
        self.title = title
        self.tags = tags or []
        self.panels = panels or []
        self.variables = variables or []


# ─── Normaliser ───────────────────────────────────────────────


def normalise_dashboards(
    discovery: GrafanaDiscoveryResult,
) -> List[NormalisedDashboard]:
    """
    Convert GrafanaDashboard objects into NormalisedDashboards,
    keeping only panels that have at least one PromQL target.
    """
    prom_ds_uids = _prometheus_datasource_uids(discovery)

    normalised: List[NormalisedDashboard] = []
    for dash in discovery.dashboards:
        nd = _normalise_one(dash, prom_ds_uids)
        if nd.panels:  # skip dashboards with zero Prom panels
            normalised.append(nd)

    total_panels = sum(len(d.panels) for d in normalised)
    total_queries = sum(
        len(p.queries) for d in normalised for p in d.panels
    )
    log.info(
        "Normalised %d dashboards, %d panels, %d PromQL queries",
        len(normalised), total_panels, total_queries,
    )
    return normalised


def _normalise_one(
    dash: GrafanaDashboard,
    prom_ds_uids: Set[str],
) -> NormalisedDashboard:
    nd = NormalisedDashboard(
        uid=dash.uid,
        title=dash.title,
        tags=dash.tags,
    )
    # Variables
    declared_vars: Set[str] = set()
    for v in dash.variables:
        nv = NormalisedVariable(
            name=v.name,
            var_type=v.var_type,
            promql=v.query if v.var_type == "query" else "",
            default_value=v.current,
            options=v.options,
            label=v.label or v.name,
        )
        nd.variables.append(nv)
        declared_vars.add(v.name)

    # Panels
    for panel in dash.panels:
        np = _normalise_panel(panel, dash.uid, prom_ds_uids, declared_vars)
        if np is not None:
            nd.panels.append(np)

    return nd


def _normalise_panel(
    panel: GrafanaPanel,
    dashboard_uid: str,
    prom_ds_uids: Set[str],
    declared_vars: Set[str],
) -> Optional[NormalisedPanel]:
    queries: List[NormalisedPanelQuery] = []

    for t in panel.targets:
        # Skip targets that don't point at a Prometheus datasource
        if t.datasource_uid and t.datasource_uid not in prom_ds_uids:
            # If no datasource map (anonymous), keep all targets with expr
            if prom_ds_uids:
                continue

        if not t.expr.strip():
            continue

        nq = NormalisedPanelQuery(
            ref_id=t.ref_id,
            promql=t.expr,
            legend_format=t.legend_format,
            instant=t.instant,
        )
        nq.variable_refs = _detect_variable_refs(t.expr, declared_vars)
        queries.append(nq)

    if not queries:
        return None

    # Extract unit from fieldConfig
    unit = ""
    defaults = panel.field_config.get("defaults", {})
    unit = defaults.get("unit", "")

    return NormalisedPanel(
        panel_id=panel.id,
        dashboard_uid=dashboard_uid,
        title=panel.title,
        viz_type=panel.panel_type,
        queries=queries,
        thresholds=panel.thresholds,
        unit=unit,
        description=panel.description,
    )


def _detect_variable_refs(
    promql: str,
    declared_vars: Set[str],
) -> List[str]:
    """Find $var / ${var} references in a PromQL string."""
    found: List[str] = []
    for match in _VAR_REF_RE.finditer(promql):
        name = match.group(1)
        if name in _BUILTIN_VARS:
            continue
        if name in declared_vars:
            found.append(name)
    return list(dict.fromkeys(found))   # dedup, preserve order


def _prometheus_datasource_uids(
    discovery: GrafanaDiscoveryResult,
) -> Set[str]:
    """Return UIDs of all Prometheus-type datasources."""
    uids: Set[str] = set()
    for uid, ds_type in discovery.datasource_map.items():
        if "prometheus" in ds_type.lower():
            uids.add(uid)
    return uids
