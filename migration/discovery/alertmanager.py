"""
Alertmanager API v2 fetcher — retrieves routing tree, receivers, silences.

Endpoints used:
  GET /api/v2/status    → config (routing tree + receivers + global settings)
  GET /api/v2/alerts    → currently firing alerts (for validation)
  GET /api/v2/silences  → active silences
"""

import logging
import re
from typing import Any, Dict, List, Optional

from migration import config
from migration.http_client import fetch_json

log = logging.getLogger("migration.discovery.alertmanager")

# ─── Data structures ──────────────────────────────────────────


class Receiver:
    """A notification receiver (email, Slack, webhook, PagerDuty, etc.)."""

    __slots__ = ("name", "email_configs", "slack_configs",
                 "webhook_configs", "pagerduty_configs", "other_configs")

    def __init__(self, name: str = "", **kw):
        self.name = name
        self.email_configs: List[Dict] = kw.get("email_configs", [])
        self.slack_configs: List[Dict] = kw.get("slack_configs", [])
        self.webhook_configs: List[Dict] = kw.get("webhook_configs", [])
        self.pagerduty_configs: List[Dict] = kw.get("pagerduty_configs", [])
        self.other_configs: List[Dict] = kw.get("other_configs", [])


class RouteNode:
    """One node in the Alertmanager routing tree."""

    __slots__ = (
        "receiver", "group_by", "group_wait", "group_interval",
        "repeat_interval", "matchers", "match", "match_re",
        "continue_routing", "children",
    )

    def __init__(self, **kw):
        self.receiver: str = kw.get("receiver", "")
        self.group_by: List[str] = kw.get("group_by", [])
        self.group_wait: str = kw.get("group_wait", "")
        self.group_interval: str = kw.get("group_interval", "")
        self.repeat_interval: str = kw.get("repeat_interval", "")
        self.matchers: List[str] = kw.get("matchers", [])
        self.match: Dict[str, str] = kw.get("match", {})
        self.match_re: Dict[str, str] = kw.get("match_re", {})
        self.continue_routing: bool = kw.get("continue", False)
        self.children: List["RouteNode"] = []


class Silence:
    """An active or pending silence."""

    __slots__ = ("id", "matchers", "starts_at", "ends_at",
                 "created_by", "comment", "status")

    def __init__(self, **kw):
        self.id: str = kw.get("id", "")
        self.matchers: List[Dict] = kw.get("matchers", [])
        self.starts_at: str = kw.get("startsAt", "")
        self.ends_at: str = kw.get("endsAt", "")
        self.created_by: str = kw.get("createdBy", "")
        self.comment: str = kw.get("comment", "")
        self.status: str = kw.get("status", {}).get("state", "active") \
            if isinstance(kw.get("status"), dict) else str(kw.get("status", ""))


class AlertmanagerDiscoveryResult:
    """Aggregated result from the Alertmanager API."""

    def __init__(self):
        self.global_config: Dict[str, Any] = {}
        self.route: Optional[RouteNode] = None
        self.receivers: List[Receiver] = []
        self.silences: List[Silence] = []
        self.inhibit_rules: List[Dict] = []
        self.active_alerts: List[Dict] = []


# ─── Fetcher ──────────────────────────────────────────────────


class AlertmanagerFetcher:
    """Fetches config and state from Alertmanager v2 API."""

    def __init__(self, base_url: Optional[str] = None):
        self.base = (base_url or config.ALERTMANAGER_URL).rstrip("/")

    # ── public ────────────────────────────────────────────────

    def fetch_all(self) -> AlertmanagerDiscoveryResult:
        result = AlertmanagerDiscoveryResult()

        status = self._fetch_status()
        cfg = self._extract_config(status)
        result.global_config = cfg.get("global", {})
        result.route = self._parse_route(cfg.get("route", {}))
        result.receivers = self._parse_receivers(cfg.get("receivers", []))
        result.inhibit_rules = cfg.get("inhibit_rules", [])
        result.silences = self._fetch_silences()
        result.active_alerts = self._fetch_alerts()

        log.info(
            "Alertmanager discovery complete: %d receivers, %d silences, "
            "%d active alerts",
            len(result.receivers),
            len(result.silences),
            len(result.active_alerts),
        )
        return result

    # ── internal ──────────────────────────────────────────────

    def _fetch_status(self) -> Dict[str, Any]:
        url = f"{self.base}/api/v2/status"
        try:
            return fetch_json(url)
        except Exception as exc:
            log.warning("Could not fetch Alertmanager status: %s", exc)
            return {}

    def _extract_config(self, status: Dict) -> Dict[str, Any]:
        """
        The v2 status response nests config inside either
        'config.original' (YAML string) or 'config' (parsed dict).
        Handle both formats.
        """
        config_block = status.get("config", {})

        # If it's already a parsed dict with 'route', use it directly.
        if isinstance(config_block, dict) and "route" in config_block:
            return config_block

        # Some versions return {'original': '<yaml>'} — attempt YAML parse.
        original = config_block.get("original", "") if isinstance(config_block, dict) else ""
        if original:
            try:
                import yaml  # optional dependency
                return yaml.safe_load(original) or {}
            except Exception:
                log.debug("YAML parsing failed for Alertmanager config; "
                          "falling back to empty config")
        return {}

    def _parse_route(self, raw: Dict) -> RouteNode:
        node = RouteNode(
            receiver=raw.get("receiver", ""),
            group_by=raw.get("group_by", []),
            group_wait=raw.get("group_wait", ""),
            group_interval=raw.get("group_interval", ""),
            repeat_interval=raw.get("repeat_interval", ""),
            matchers=raw.get("matchers", []),
            match=raw.get("match", {}),
            match_re=raw.get("match_re", {}),
            **{"continue": raw.get("continue", False)},
        )
        for child in raw.get("routes", []):
            node.children.append(self._parse_route(child))
        return node

    def _parse_receivers(self, raw_list: List[Dict]) -> List[Receiver]:
        receivers: List[Receiver] = []
        for r in raw_list:
            receivers.append(Receiver(
                name=r.get("name", ""),
                email_configs=r.get("email_configs", []),
                slack_configs=r.get("slack_configs", []),
                webhook_configs=r.get("webhook_configs", []),
                pagerduty_configs=r.get("pagerduty_configs", []),
                other_configs=[
                    c for key in r if key.endswith("_configs")
                    and key not in ("email_configs", "slack_configs",
                                    "webhook_configs", "pagerduty_configs")
                    for c in (r[key] if isinstance(r[key], list) else [])
                ],
            ))
        return receivers

    def _fetch_silences(self) -> List[Silence]:
        url = f"{self.base}/api/v2/silences"
        try:
            data = fetch_json(url)
        except Exception as exc:
            log.warning("Could not fetch silences: %s", exc)
            return []
        return [Silence(**s) for s in data] if isinstance(data, list) else []

    def _fetch_alerts(self) -> List[Dict]:
        url = f"{self.base}/api/v2/alerts"
        try:
            data = fetch_json(url)
        except Exception as exc:
            log.warning("Could not fetch active alerts: %s", exc)
            return []
        return data if isinstance(data, list) else []


# ─── Routing helper ───────────────────────────────────────────


def resolve_receiver(
    route: Optional[RouteNode],
    alert_labels: Dict[str, str],
) -> str:
    """
    Walk the routing tree and return the name of the receiver that
    would handle an alert with the given labels.  Follows Alertmanager
    semantics: first-match wins unless ``continue_routing`` is set,
    in which case matching continues to the next sibling.
    """
    if route is None:
        return ""

    receivers = _collect_receivers(route, alert_labels)
    return receivers[0] if receivers else route.receiver


def _collect_receivers(
    route: RouteNode,
    alert_labels: Dict[str, str],
) -> List[str]:
    """
    Recursively collect all matching receivers, honoring ``continue_routing``.
    Returns a list of receiver names in match order.
    """
    matched: List[str] = []

    for child in route.children:
        if _matches_node(child, alert_labels):
            # Recurse into the child to check deeper matches
            deeper = _collect_receivers(child, alert_labels)
            if deeper:
                matched.extend(deeper)
            else:
                matched.append(child.receiver)

            # Unless 'continue' is set, stop at first match
            if not child.continue_routing:
                break

    return matched


def _matches_node(node: RouteNode, alert_labels: Dict[str, str]) -> bool:
    """Check if a RouteNode matches the given alert labels."""
    # 'match' → exact label matches
    for k, v in node.match.items():
        if alert_labels.get(k) != v:
            return False
    # 'match_re' → regex matches
    for k, pattern in node.match_re.items():
        val = alert_labels.get(k, "")
        try:
            if not re.fullmatch(pattern, val):
                return False
        except re.error:
            if pattern not in val:
                return False
    # 'matchers' → new-style matchers (alertname="X", severity=~"crit.*")
    for m in node.matchers:
        if "=~" in m:
            lbl, pat = m.split("=~", 1)
            if not re.fullmatch(pat.strip().strip('"'),
                                alert_labels.get(lbl.strip(), "")):
                return False
        elif "!=" in m:
            lbl, val = m.split("!=", 1)
            if alert_labels.get(lbl.strip()) == val.strip().strip('"'):
                return False
        elif "=" in m:
            lbl, val = m.split("=", 1)
            if alert_labels.get(lbl.strip()) != val.strip().strip('"'):
                return False
    return True
