"""
Grafana API fetcher — retrieves dashboards, panels, variables, and alerts.

Endpoints used:
  GET /api/search?type=dash-db        → list all dashboards
  GET /api/dashboards/uid/:uid        → full dashboard JSON
  GET /api/datasources                → datasource map
  GET /api/v1/provisioning/alert-rules → unified alerting rules (Grafana ≥9)
"""

import logging
from typing import Any, Dict, List, Optional

from migration import config
from migration.http_client import fetch_json

log = logging.getLogger("migration.discovery.grafana")

# ─── Data structures ──────────────────────────────────────────


class GrafanaTarget:
    """One query target inside a panel."""

    __slots__ = ("ref_id", "expr", "legend_format", "instant", "interval", "datasource_uid")

    def __init__(
        self,
        ref_id: str = "A",
        expr: str = "",
        legend_format: str = "",
        instant: bool = False,
        interval: str = "",
        datasource_uid: str = "",
    ):
        self.ref_id = ref_id
        self.expr = expr
        self.legend_format = legend_format
        self.instant = instant
        self.interval = interval
        self.datasource_uid = datasource_uid


class GrafanaPanel:
    """One panel inside a dashboard."""

    __slots__ = (
        "id", "title", "panel_type", "targets", "thresholds",
        "field_config", "description",
    )

    def __init__(
        self,
        panel_id: int = 0,
        title: str = "",
        panel_type: str = "timeseries",
        targets: Optional[List[GrafanaTarget]] = None,
        thresholds: Optional[List[Dict]] = None,
        field_config: Optional[Dict] = None,
        description: str = "",
    ):
        self.id = panel_id
        self.title = title
        self.panel_type = panel_type
        self.targets = targets or []
        self.thresholds = thresholds or []
        self.field_config = field_config or {}
        self.description = description


class GrafanaVariable:
    """One template variable."""

    __slots__ = ("name", "var_type", "query", "current", "options", "datasource_uid", "label")

    def __init__(
        self,
        name: str = "",
        var_type: str = "custom",
        query: str = "",
        current: str = "",
        options: Optional[List[str]] = None,
        datasource_uid: str = "",
        label: str = "",
    ):
        self.name = name
        self.var_type = var_type      # query | custom | interval | datasource | textbox
        self.query = query
        self.current = current
        self.options = options or []
        self.datasource_uid = datasource_uid
        self.label = label


class GrafanaDashboard:
    """Normalised dashboard."""

    __slots__ = ("uid", "title", "tags", "panels", "variables", "raw")

    def __init__(
        self,
        uid: str = "",
        title: str = "",
        tags: Optional[List[str]] = None,
        panels: Optional[List[GrafanaPanel]] = None,
        variables: Optional[List[GrafanaVariable]] = None,
        raw: Optional[Dict] = None,
    ):
        self.uid = uid
        self.title = title
        self.tags = tags or []
        self.panels = panels or []
        self.variables = variables or []
        self.raw = raw or {}


class GrafanaAlertRule:
    """One Grafana Unified Alerting rule."""

    __slots__ = ("uid", "title", "condition", "queries", "for_duration",
                 "labels", "annotations", "folder_title", "rule_group")

    def __init__(self, **kw):
        self.uid = kw.get("uid", "")
        self.title = kw.get("title", "")
        self.condition = kw.get("condition", "")
        self.queries = kw.get("queries", [])     # list of {refId, model{expr}}
        self.for_duration = kw.get("for_duration", "")
        self.labels = kw.get("labels", {})
        self.annotations = kw.get("annotations", {})
        self.folder_title = kw.get("folder_title", "")
        self.rule_group = kw.get("rule_group", "")


class GrafanaDiscoveryResult:
    """Aggregated result from the Grafana API."""

    def __init__(self):
        self.dashboards: List[GrafanaDashboard] = []
        self.alert_rules: List[GrafanaAlertRule] = []
        self.datasource_map: Dict[str, str] = {}   # uid → type


# ─── Fetcher ──────────────────────────────────────────────────


class GrafanaFetcher:
    """Fetches dashboards, panels, variables, and alerts from Grafana."""

    def __init__(
        self,
        base_url: Optional[str] = None,
        api_token: Optional[str] = None,
    ):
        self.base = (base_url or config.GRAFANA_URL).rstrip("/")
        self.token = api_token or config.GRAFANA_API_TOKEN

    # ── public ────────────────────────────────────────────────

    def fetch_all(self) -> GrafanaDiscoveryResult:
        result = GrafanaDiscoveryResult()
        result.datasource_map = self._fetch_datasources()
        result.dashboards = self._fetch_all_dashboards()
        result.alert_rules = self._fetch_alert_rules()
        log.info(
            "Grafana discovery complete: %d dashboards (%d total panels), "
            "%d alert rules, %d datasources",
            len(result.dashboards),
            sum(len(d.panels) for d in result.dashboards),
            len(result.alert_rules),
            len(result.datasource_map),
        )
        return result

    # ── internal ──────────────────────────────────────────────

    def _api(self, path: str, **kw) -> Any:
        return fetch_json(
            f"{self.base}{path}",
            token=self.token,
            bearer=True,
            **kw,
        )

    def _fetch_datasources(self) -> Dict[str, str]:
        try:
            ds_list = self._api("/api/datasources")
        except Exception as exc:
            log.warning("Could not fetch datasources: %s", exc)
            return {}
        mapping: Dict[str, str] = {}
        for ds in ds_list:
            uid = ds.get("uid", "")
            ds_type = ds.get("type", "")
            if uid:
                mapping[uid] = ds_type
        return mapping

    def _fetch_all_dashboards(self) -> List[GrafanaDashboard]:
        # List all dashboard UIDs
        try:
            search = self._api("/api/search", params={"type": "dash-db", "limit": 5000})
        except Exception as exc:
            log.warning("Could not list dashboards: %s", exc)
            return []

        dashboards: List[GrafanaDashboard] = []
        for item in search:
            uid = item.get("uid", "")
            if not uid:
                continue
            try:
                raw = self._api(f"/api/dashboards/uid/{uid}")
            except Exception as exc:
                log.warning("Skipping dashboard %s: %s", uid, exc)
                continue
            dash = self._parse_dashboard(raw)
            dashboards.append(dash)
        return dashboards

    def _parse_dashboard(self, raw: Dict) -> GrafanaDashboard:
        d = raw.get("dashboard", {})
        dash = GrafanaDashboard(
            uid=d.get("uid", ""),
            title=d.get("title", "Untitled"),
            tags=d.get("tags", []),
            raw=d,
        )
        # Variables
        for tpl in d.get("templating", {}).get("list", []):
            var = GrafanaVariable(
                name=tpl.get("name", ""),
                var_type=tpl.get("type", "custom"),
                query=tpl.get("query", "") if isinstance(tpl.get("query"), str)
                       else str(tpl.get("query", "")),
                current=self._extract_current(tpl.get("current", {})),
                options=[o.get("value", "") for o in tpl.get("options", [])
                         if isinstance(o, dict)],
                datasource_uid=self._extract_ds_uid(tpl.get("datasource")),
                label=tpl.get("label", ""),
            )
            dash.variables.append(var)

        # Panels (including nested row-panels)
        dash.panels = self._extract_panels(d.get("panels", []))
        return dash

    def _extract_panels(self, raw_panels: List[Dict]) -> List[GrafanaPanel]:
        panels: List[GrafanaPanel] = []
        for p in raw_panels:
            # Row panels may nest child panels
            if p.get("type") == "row":
                panels.extend(self._extract_panels(p.get("panels", [])))
                continue

            targets: List[GrafanaTarget] = []
            for t in p.get("targets", []):
                expr = t.get("expr", "")
                if not expr:
                    continue
                targets.append(GrafanaTarget(
                    ref_id=t.get("refId", "A"),
                    expr=expr,
                    legend_format=t.get("legendFormat", ""),
                    instant=t.get("instant", False),
                    interval=t.get("interval", ""),
                    datasource_uid=self._extract_ds_uid(t.get("datasource")),
                ))

            thresholds = []
            fc = p.get("fieldConfig", {})
            defaults = fc.get("defaults", {})
            thresh = defaults.get("thresholds", {})
            if thresh:
                thresholds = thresh.get("steps", [])

            panel = GrafanaPanel(
                panel_id=p.get("id", 0),
                title=p.get("title", ""),
                panel_type=p.get("type", "timeseries"),
                targets=targets,
                thresholds=thresholds,
                field_config=fc,
                description=p.get("description", ""),
            )
            panels.append(panel)
        return panels

    def _fetch_alert_rules(self) -> List[GrafanaAlertRule]:
        try:
            rules = self._api("/api/v1/provisioning/alert-rules")
        except Exception:
            # Grafana <9 or unified alerting disabled
            try:
                rules = self._api("/api/ruler/grafana/api/v1/rules")
                return self._parse_ruler_rules(rules)
            except Exception as exc2:
                log.warning("Could not fetch Grafana alert rules: %s", exc2)
                return []

        if not isinstance(rules, list):
            return []
        result: List[GrafanaAlertRule] = []
        for r in rules:
            queries = []
            for d in r.get("data", []):
                model = d.get("model", {})
                queries.append({
                    "refId": d.get("refId", ""),
                    "expr": model.get("expr", ""),
                    "datasource_uid": self._extract_ds_uid(model.get("datasource")),
                })
            result.append(GrafanaAlertRule(
                uid=r.get("uid", ""),
                title=r.get("title", ""),
                condition=r.get("condition", ""),
                queries=queries,
                for_duration=r.get("for", ""),
                labels=r.get("labels", {}),
                annotations=r.get("annotations", {}),
                folder_title=r.get("folderTitle", ""),
                rule_group=r.get("ruleGroup", ""),
            ))
        return result

    def _parse_ruler_rules(self, ruler_data: Dict) -> List[GrafanaAlertRule]:
        """Parse the legacy ruler response format."""
        result: List[GrafanaAlertRule] = []
        if isinstance(ruler_data, dict):
            for _ns, groups in ruler_data.items():
                if not isinstance(groups, list):
                    continue
                for group in groups:
                    for rule in group.get("rules", []):
                        expr = rule.get("expr", rule.get("query", ""))
                        result.append(GrafanaAlertRule(
                            uid=rule.get("uid", rule.get("name", "")),
                            title=rule.get("alert", rule.get("name", "")),
                            queries=[{"refId": "A", "expr": expr}],
                            for_duration=str(rule.get("for", "")),
                            labels=rule.get("labels", {}),
                            annotations=rule.get("annotations", {}),
                            rule_group=group.get("name", ""),
                        ))
        return result

    # ── helpers ───────────────────────────────────────────────

    @staticmethod
    def _extract_current(current: Any) -> str:
        if isinstance(current, dict):
            v = current.get("value", current.get("text", ""))
            return v if isinstance(v, str) else str(v)
        return str(current) if current else ""

    @staticmethod
    def _extract_ds_uid(ds: Any) -> str:
        if isinstance(ds, dict):
            return ds.get("uid", "")
        if isinstance(ds, str):
            return ds
        return ""
