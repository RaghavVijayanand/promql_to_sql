"""
Alert normaliser — merges Prometheus alert rules with Alertmanager
routing to produce a unified alert model including receiver bindings.
"""

import logging
import re
from typing import Dict, List, Optional

from migration.discovery.prometheus import PromDiscoveryResult
from migration.discovery.alertmanager import (
    AlertmanagerDiscoveryResult,
    Receiver,
    RouteNode,
    resolve_receiver,
)
from migration.analysis.rules import NormalisedRule

log = logging.getLogger("migration.analysis.alerts")

# ─── Normalised output ───────────────────────────────────────


class NormalisedAlert:
    """Unified alert with routing and receiver information."""

    __slots__ = (
        "id", "name", "promql", "for_duration",
        "severity", "labels", "annotations",
        "receiver_name", "receiver",
        "threshold_value", "threshold_op",
        "metric_expr",
    )

    def __init__(
        self,
        alert_id: str,
        name: str,
        promql: str,
        for_duration: str = "",
        severity: str = "",
        labels: Optional[Dict[str, str]] = None,
        annotations: Optional[Dict[str, str]] = None,
        receiver_name: str = "",
        receiver: Optional[Receiver] = None,
    ):
        self.id = alert_id
        self.name = name
        self.promql = promql
        self.for_duration = for_duration
        self.severity = severity
        self.labels = labels or {}
        self.annotations = annotations or {}
        self.receiver_name = receiver_name
        self.receiver = receiver
        # Populated by threshold extractor
        self.threshold_value: Optional[float] = None
        self.threshold_op: str = ""
        self.metric_expr: str = ""   # PromQL without the comparison


# ─── Normaliser ───────────────────────────────────────────────


def normalise_alerts(
    normalised_rules: List[NormalisedRule],
    am_result: AlertmanagerDiscoveryResult,
) -> List[NormalisedAlert]:
    """
    Take alerting rules from the normalised rule set and merge them
    with Alertmanager routing to produce NormalisedAlert objects.
    """
    receiver_map = {r.name: r for r in am_result.receivers}

    alerts: List[NormalisedAlert] = []
    for rule in normalised_rules:
        if rule.rule_type != "alerting":
            continue

        severity = rule.labels.get("severity", "")
        all_labels = {**rule.labels, "alertname": rule.name}
        rcv_name = resolve_receiver(am_result.route, all_labels)
        rcv = receiver_map.get(rcv_name)

        na = NormalisedAlert(
            alert_id=rule.id,
            name=rule.name,
            promql=rule.promql,
            for_duration=rule.for_duration,
            severity=severity,
            labels=rule.labels,
            annotations=rule.annotations,
            receiver_name=rcv_name,
            receiver=rcv,
        )

        # Extract threshold from the PromQL expression
        _extract_threshold(na)
        alerts.append(na)

    log.info("Normalised %d alert rules with receiver bindings", len(alerts))
    return alerts


# ─── Threshold extraction ────────────────────────────────────

# Matches trailing comparison operator + number at the root level.
# Examples:  ") * 100 > 20"  or  "some_metric > 0.5"
_THRESHOLD_RE = re.compile(
    r'^(.*?)\s*(>|<|>=|<=|==|!=)\s*([0-9]+(?:\.[0-9]+)?)\s*$',
    re.DOTALL,
)

# Also handle the reverse:  "0.5 < some_metric"
_THRESHOLD_REV_RE = re.compile(
    r'^\s*([0-9]+(?:\.[0-9]+)?)\s*(>|<|>=|<=|==|!=)\s*(.*?)\s*$',
    re.DOTALL,
)

_FLIP_OP = {">": "<", "<": ">", ">=": "<=", "<=": ">=", "==": "==", "!=": "!="}


def _extract_threshold(alert: NormalisedAlert) -> None:
    """
    Attempt to split the alert PromQL into (metric_expr) <op> <threshold>.
    On success, populates alert.metric_expr, threshold_op, threshold_value.
    On failure, metric_expr is set to the full PromQL (threshold stays None).
    """
    expr = alert.promql.strip()

    # Try normal order: expr > N
    m = _THRESHOLD_RE.match(expr)
    if m:
        alert.metric_expr = m.group(1).strip()
        alert.threshold_op = m.group(2)
        alert.threshold_value = float(m.group(3))
        return

    # Try reversed order: N < expr
    m = _THRESHOLD_REV_RE.match(expr)
    if m:
        alert.metric_expr = m.group(3).strip()
        alert.threshold_op = _FLIP_OP.get(m.group(2), m.group(2))
        alert.threshold_value = float(m.group(1))
        return

    # Could not extract — use full expression
    alert.metric_expr = expr
    log.debug("No threshold extracted from alert %s: %s", alert.name, expr)
