"""
Dependency graph builder — constructs a DAG across recording rules,
alert rules, and dashboard panels, then produces a topological
deployment order.

Nodes: recording rules, alerting rules, dashboard panels
Edges: artefact A depends on recording rule B  (A references B's output metric)
"""

import logging
import re
from collections import defaultdict, deque
from typing import Dict, List, Set, Tuple

from migration.analysis.rules import NormalisedRule
from migration.analysis.dashboards import NormalisedDashboard
from migration.analysis.alerts import NormalisedAlert

log = logging.getLogger("migration.analysis.graph")

# ─── Node types ───────────────────────────────────────────────

NODE_RECORDING = "recording"
NODE_ALERTING = "alerting"
NODE_PANEL = "panel"


def _is_metric_ref(rule_name: str, expr: str) -> bool:
    """
    Check whether *rule_name* appears as a standalone metric reference
    in *expr* using word-boundary matching.  This avoids false positives
    from substring matching (e.g. ``job:requests`` inside
    ``job:requests_total:rate5m``).
    """
    # PromQL metric names can contain [a-zA-Z_:0-9], so we use
    # negative look-behind/ahead for those characters.
    pattern = r'(?<![a-zA-Z0-9_:])' + re.escape(rule_name) + r'(?![a-zA-Z0-9_:])'
    return bool(re.search(pattern, expr))


class DependencyNode:
    """A node in the dependency DAG."""

    __slots__ = ("key", "node_type", "payload")

    def __init__(self, key: str, node_type: str, payload):
        self.key = key              # deterministic id
        self.node_type = node_type
        self.payload = payload      # NormalisedRule | NormalisedAlert | NormalisedPanel

    def __repr__(self) -> str:
        return f"<Node {self.node_type}:{self.key}>"


class DeploymentPlan:
    """Ordered list of nodes + edge metadata."""

    def __init__(self):
        self.ordered_nodes: List[DependencyNode] = []
        self.edges: List[Tuple[str, str]] = []      # (from, to)
        self.cycles: List[List[str]] = []

    @property
    def has_cycles(self) -> bool:
        return len(self.cycles) > 0


# ─── Graph builder ────────────────────────────────────────────


def build_deployment_plan(
    rules: List[NormalisedRule],
    dashboards: List[NormalisedDashboard],
    alerts: List[NormalisedAlert],
) -> DeploymentPlan:
    """
    Build a dependency DAG and return a topologically-sorted deployment plan.

    Recording rules are deployed first (leaf dependencies), then alerts
    and dashboard panels that reference them.
    """
    # Build lookup: recording-rule output name → node key
    recording_key_by_name: Dict[str, str] = {}
    for r in rules:
        if r.rule_type == "recording":
            recording_key_by_name[r.name] = r.id

    # --- Nodes ---
    nodes: Dict[str, DependencyNode] = {}
    adjacency: Dict[str, List[str]] = defaultdict(list)   # node → depends-on
    in_degree: Dict[str, int] = {}

    # Recording rules
    for r in rules:
        if r.rule_type != "recording":
            continue
        key = f"rule:{r.id}"
        nodes[key] = DependencyNode(key, NODE_RECORDING, r)
        in_degree.setdefault(key, 0)
        for dep_name in r.referenced_rules:
            dep_key = f"rule:{recording_key_by_name[dep_name]}"
            adjacency[key].append(dep_key)

    # Alerting rules
    for a in alerts:
        key = f"alert:{a.id}"
        nodes[key] = DependencyNode(key, NODE_ALERTING, a)
        in_degree.setdefault(key, 0)
        # Find recording-rule dependencies inside the alert PromQL
        # (use the metric_expr if threshold was extracted, else full promql)
        expr = a.metric_expr or a.promql
        for rname, rkey in recording_key_by_name.items():
            if _is_metric_ref(rname, expr):
                dep = f"rule:{rkey}"
                adjacency[key].append(dep)

    # Dashboard panels
    for dash in dashboards:
        for panel in dash.panels:
            pkey = f"panel:{dash.uid}:{panel.id}"
            nodes[pkey] = DependencyNode(pkey, NODE_PANEL, panel)
            in_degree.setdefault(pkey, 0)
            for q in panel.queries:
                for rname, rkey in recording_key_by_name.items():
                    if _is_metric_ref(rname, q.promql):
                        dep = f"rule:{rkey}"
                        adjacency[pkey].append(dep)

    # --- Compute in-degrees ---
    plan = DeploymentPlan()
    for node_key, deps in adjacency.items():
        for dep in deps:
            in_degree.setdefault(dep, 0)
            in_degree.setdefault(node_key, 0)
            in_degree[node_key] += 1
            plan.edges.append((node_key, dep))

    # Ensure every node has an in-degree entry
    for key in nodes:
        in_degree.setdefault(key, 0)

    # --- Kahn's topological sort ---
    queue: deque[str] = deque()
    for key, deg in in_degree.items():
        if deg == 0:
            queue.append(key)

    sorted_keys: List[str] = []
    while queue:
        current = queue.popleft()
        sorted_keys.append(current)
        # Find nodes that depend on current
        for node_key, deps in adjacency.items():
            if current in deps:
                in_degree[node_key] -= 1
                if in_degree[node_key] == 0:
                    queue.append(node_key)

    # Cycle detection
    if len(sorted_keys) < len(nodes):
        remaining = set(nodes.keys()) - set(sorted_keys)
        plan.cycles.append(list(remaining))
        log.warning("Dependency cycle detected among: %s", remaining)
        # Add remaining nodes at the end so migration can still proceed
        sorted_keys.extend(remaining)

    # Populate plan — deploy dependencies first, so we reverse
    # (Kahn's gives "ready first" = leaves first, which is correct)
    plan.ordered_nodes = [nodes[k] for k in sorted_keys if k in nodes]

    log.info(
        "Deployment plan: %d nodes, %d edges, %d cycles",
        len(plan.ordered_nodes),
        len(plan.edges),
        len(plan.cycles),
    )
    return plan
