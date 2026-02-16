"""
Rule normaliser — converts raw Prometheus rules into normalised IR,
extracting referenced metrics for dependency resolution.
"""

import logging
import re
from typing import Dict, List, Optional, Set

from migration.discovery.prometheus import PromDiscoveryResult, PromRule

log = logging.getLogger("migration.analysis.rules")

# Regex to find metric selectors inside PromQL.
# Matches identifiers that look like metric names (word chars, colons, dots)
# followed optionally by label matchers in {}.
_METRIC_RE = re.compile(
    r'(?<![a-zA-Z0-9_:\.])'          # negative lookbehind
    r'([a-zA-Z_:][a-zA-Z0-9_:.]*)'   # metric name
    r'(?:\s*\{[^}]*\})?'             # optional label matchers
    r'(?:\s*\[[^\]]+\])?'            # optional range selector
)

# Known PromQL function / keyword names that are NOT metrics
_PROMQL_KEYWORDS: Set[str] = {
    "rate", "irate", "increase", "delta", "deriv", "idelta",
    "sum", "avg", "min", "max", "count", "stddev", "stdvar",
    "topk", "bottomk", "count_values", "quantile", "group",
    "sum_over_time", "avg_over_time", "min_over_time", "max_over_time",
    "count_over_time", "stddev_over_time", "stdvar_over_time",
    "last_over_time", "present_over_time", "quantile_over_time",
    "absent", "absent_over_time", "scalar", "vector",
    "sort", "sort_desc", "sort_by_label", "sort_by_label_desc",
    "histogram_quantile", "histogram_count", "histogram_sum",
    "histogram_fraction", "histogram_avg",
    "label_join", "label_replace",
    "clamp", "clamp_max", "clamp_min",
    "round", "ceil", "floor", "exp", "ln", "log2", "log10", "sqrt",
    "abs", "sgn",
    "time", "timestamp", "day_of_month", "day_of_week", "day_of_year",
    "days_in_month", "hour", "minute", "month", "year",
    "changes", "resets", "predict_linear", "holt_winters",
    "by", "without", "on", "ignoring", "group_left", "group_right",
    "bool", "offset", "and", "or", "unless", "inf", "nan",
    "pi", "deg", "rad", "acos", "asin", "atan", "cos", "sin", "tan",
    "acosh", "asinh", "atanh", "cosh", "sinh", "tanh",
}


# ─── Normalised output ───────────────────────────────────────


class NormalisedRule:
    """Rule in the migration intermediate representation."""

    __slots__ = (
        "id", "group_name", "rule_type", "name", "promql",
        "for_duration", "labels", "annotations",
        "referenced_metrics", "referenced_rules",
    )

    def __init__(
        self,
        rule_id: str,
        group_name: str,
        rule_type: str,
        name: str,
        promql: str,
        for_duration: str = "",
        labels: Optional[Dict[str, str]] = None,
        annotations: Optional[Dict[str, str]] = None,
    ):
        self.id = rule_id
        self.group_name = group_name
        self.rule_type = rule_type
        self.name = name
        self.promql = promql
        self.for_duration = for_duration
        self.labels = labels or {}
        self.annotations = annotations or {}
        self.referenced_metrics: List[str] = []
        self.referenced_rules: List[str] = []

    def __repr__(self) -> str:
        return f"<NormalisedRule {self.rule_type}:{self.id}>"


# ─── Normaliser ───────────────────────────────────────────────


def normalise_rules(discovery: PromDiscoveryResult) -> List[NormalisedRule]:
    """
    Convert raw PromRule objects into NormalisedRules with
    metric-reference extraction and cross-rule dependency detection.
    """
    # First pass: collect all recording-rule output names
    recording_names: Set[str] = set()
    for r in discovery.rules:
        if r.rule_type == "recording":
            recording_names.add(r.name)

    # Second pass: normalise each rule
    normalised: List[NormalisedRule] = []
    for r in discovery.rules:
        rule_id = f"{r.group_name}:{r.name}"
        nr = NormalisedRule(
            rule_id=rule_id,
            group_name=r.group_name,
            rule_type=r.rule_type,
            name=r.name,
            promql=r.query,
            for_duration=r.duration,
            labels=r.labels,
            annotations=r.annotations,
        )
        # Extract metric references from PromQL
        nr.referenced_metrics = _extract_metrics(r.query)
        # Identify which of those are recording rules (= dependencies)
        nr.referenced_rules = [
            m for m in nr.referenced_metrics if m in recording_names
        ]
        normalised.append(nr)

    log.info(
        "Normalised %d rules (%d recording, %d alerting)",
        len(normalised),
        sum(1 for n in normalised if n.rule_type == "recording"),
        sum(1 for n in normalised if n.rule_type == "alerting"),
    )
    return normalised


def _extract_metrics(promql: str) -> List[str]:
    """
    Extract metric names from a PromQL expression using regex heuristics.
    Not a full parser — but fast and correct for the vast majority of
    real-world queries.  The Go transpiler handles authoritative parsing.
    """
    candidates: List[str] = []
    for match in _METRIC_RE.finditer(promql):
        name = match.group(1)
        if name.lower() not in _PROMQL_KEYWORDS:
            candidates.append(name)
    # Deduplicate while preserving order
    seen: Set[str] = set()
    unique: List[str] = []
    for c in candidates:
        if c not in seen:
            seen.add(c)
            unique.append(c)
    return unique
