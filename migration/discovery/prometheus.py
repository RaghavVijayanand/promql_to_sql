"""
Prometheus API fetcher — retrieves rules, metadata, and target labels.

Endpoints used:
  GET /api/v1/rules            → recording + alert rules
  GET /api/v1/metadata         → metric type metadata
  GET /api/v1/label/__name__/values → all metric names
  GET /api/v1/targets          → scrape-target labels
"""

import logging
from typing import Any, Dict, List, Optional

from migration import config
from migration.http_client import fetch_json

log = logging.getLogger("migration.discovery.prometheus")

# ─── Data structures ──────────────────────────────────────────


class PromRule:
    """One recording or alerting rule."""

    __slots__ = (
        "group_name", "rule_type", "name", "query",
        "duration", "labels", "annotations", "state",
    )

    def __init__(
        self,
        group_name: str,
        rule_type: str,
        name: str,
        query: str,
        duration: str = "",
        labels: Optional[Dict[str, str]] = None,
        annotations: Optional[Dict[str, str]] = None,
        state: str = "inactive",
    ):
        self.group_name = group_name
        self.rule_type = rule_type      # "recording" | "alerting"
        self.name = name
        self.query = query
        self.duration = duration         # e.g. "1m", "5m"
        self.labels = labels or {}
        self.annotations = annotations or {}
        self.state = state

    def __repr__(self) -> str:
        return f"<PromRule {self.rule_type}:{self.group_name}/{self.name}>"


class PromDiscoveryResult:
    """Aggregated result from the Prometheus API."""

    def __init__(self):
        self.rules: List[PromRule] = []
        self.metric_metadata: Dict[str, str] = {}   # metric_name → type
        self.metric_names: List[str] = []
        self.targets: List[Dict[str, Any]] = []


# ─── Fetcher ──────────────────────────────────────────────────


class PrometheusFetcher:
    """Fetches observability configuration from a Prometheus instance."""

    def __init__(self, base_url: Optional[str] = None):
        self.base = (base_url or config.PROMETHEUS_URL).rstrip("/")

    # ── public ────────────────────────────────────────────────

    def fetch_all(self) -> PromDiscoveryResult:
        result = PromDiscoveryResult()
        result.rules = self._fetch_rules()
        result.metric_metadata = self._fetch_metadata()
        result.metric_names = self._fetch_metric_names()
        result.targets = self._fetch_targets()
        log.info(
            "Prometheus discovery complete: %d rules, %d metric types, "
            "%d metric names, %d targets",
            len(result.rules),
            len(result.metric_metadata),
            len(result.metric_names),
            len(result.targets),
        )
        return result

    # ── internal ──────────────────────────────────────────────

    def _fetch_rules(self) -> List[PromRule]:
        url = f"{self.base}/api/v1/rules"
        data = fetch_json(url)
        if data.get("status") != "success":
            log.warning("Prometheus /api/v1/rules returned non-success: %s",
                        data.get("status"))
            return []

        rules: List[PromRule] = []
        for group in data.get("data", {}).get("groups", []):
            gname = group.get("name", "default")
            for r in group.get("rules", []):
                rtype = r.get("type", "unknown")
                name = r.get("name", "")
                query = r.get("query", r.get("expr", ""))
                duration = r.get("duration", 0)
                # Prometheus returns duration as float seconds
                if isinstance(duration, (int, float)) and duration > 0:
                    dur_str = f"{int(duration)}s"
                else:
                    dur_str = str(duration) if duration else ""
                rules.append(PromRule(
                    group_name=gname,
                    rule_type=rtype,
                    name=name,
                    query=query,
                    duration=dur_str,
                    labels=r.get("labels", {}),
                    annotations=r.get("annotations", {}),
                    state=r.get("state", "inactive"),
                ))
        log.debug("Fetched %d rules from Prometheus", len(rules))
        return rules

    def _fetch_metadata(self) -> Dict[str, str]:
        url = f"{self.base}/api/v1/metadata"
        try:
            data = fetch_json(url)
        except Exception as exc:
            log.warning("Could not fetch /api/v1/metadata: %s", exc)
            return {}
        if data.get("status") != "success":
            return {}
        meta: Dict[str, str] = {}
        for metric_name, entries in data.get("data", {}).items():
            if entries:
                meta[metric_name] = entries[0].get("type", "unknown")
        return meta

    def _fetch_metric_names(self) -> List[str]:
        url = f"{self.base}/api/v1/label/__name__/values"
        try:
            data = fetch_json(url)
        except Exception as exc:
            log.warning("Could not fetch metric names: %s", exc)
            return []
        if data.get("status") != "success":
            return []
        return sorted(data.get("data", []))

    def _fetch_targets(self) -> List[Dict[str, Any]]:
        url = f"{self.base}/api/v1/targets"
        try:
            data = fetch_json(url)
        except Exception as exc:
            log.warning("Could not fetch targets: %s", exc)
            return []
        if data.get("status") != "success":
            return []
        return data.get("data", {}).get("activeTargets", [])
