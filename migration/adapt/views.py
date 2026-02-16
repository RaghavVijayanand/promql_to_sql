"""
View / Materialized View generator — converts transpiled recording-rule
SQL into ClickHouse DDL (CREATE VIEW / CREATE MATERIALIZED VIEW).

Decision matrix:
  - Referenced by ≥2 artefacts OR involves rate()/increase() → MATERIALIZED VIEW
  - Otherwise → VIEW (lazy evaluation)

ClickHouse optimization:
  - AggregatingMergeTree for pre-aggregated MVs
  - PARTITION BY toYYYYMM() for efficient range pruning
  - ORDER BY aligned with GROUP BY for optimize_read_in_order
  - TTL for automatic data lifecycle management
  - minmax skip indexes on timestamp columns
"""

import hashlib
import logging
import re
from typing import Dict, List, Optional

from migration import config
from migration.analysis.rules import NormalisedRule
from migration.transpile.dispatch import TranspileResult

log = logging.getLogger("migration.adapt.views")

# ─── Output ───────────────────────────────────────────────────


class ViewDefinition:
    """ClickHouse DDL for one recording rule."""

    __slots__ = (
        "name", "source_rule_id", "ddl", "is_materialized",
        "content_hash", "drop_ddl",
    )

    def __init__(
        self,
        name: str,
        source_rule_id: str,
        ddl: str,
        is_materialized: bool = False,
    ):
        self.name = name
        self.source_rule_id = source_rule_id
        self.ddl = ddl
        self.is_materialized = is_materialized
        self.content_hash = hashlib.sha256(ddl.encode()).hexdigest()
        kind = "MATERIALIZED VIEW" if is_materialized else "VIEW"
        self.drop_ddl = f"DROP {kind} IF EXISTS {name}"


# ─── Generator ────────────────────────────────────────────────


class ViewGenerator:
    """
    Generates ClickHouse VIEW / MATERIALIZED VIEW DDL for recording rules.

    Engine selection:
      AggregatingMergeTree — when GROUP BY is present (stores intermediate
      aggregate states, enables incremental merging).

    Layout optimizations:
      - ORDER BY aligned with GROUP BY columns for optimize_read_in_order
      - PARTITION BY toYYYYMM(timestamp_col) for month-granularity pruning
      - TTL based on config.PARTITION_RETENTION_DAYS
      - minmax skip index on the timestamp column for sub-partition pruning
    """

    def __init__(
        self,
        database: str = "otel",
        reference_counts: Optional[Dict[str, int]] = None,
        timestamp_col: str = "TimeUnix",
        partition_expr: str = "",
        ttl_days: int = 0,
    ):
        self.database = database
        self._ref_counts = reference_counts or {}
        self.timestamp_col = timestamp_col
        self.partition_expr = (
            partition_expr
            or getattr(config, "VIEW_PARTITION_BY", "")
            or f"toYYYYMM({self.timestamp_col})"
        )
        self.ttl_days = (
            ttl_days
            or getattr(config, "PARTITION_RETENTION_DAYS", 90)
        )

    def generate(
        self,
        rule: NormalisedRule,
        transpiled: TranspileResult,
    ) -> Optional[ViewDefinition]:
        """
        Generate DDL for a single recording rule.
        Returns None if transpilation failed.
        """
        if not transpiled.ok:
            log.warning("Skipping view for %s — transpile error: %s",
                        rule.id, transpiled.error)
            return None

        view_name = self._view_name(rule)
        should_mat = self._should_materialize(rule, transpiled.sql)

        if should_mat:
            ddl = self._materialized_view(view_name, transpiled.sql, rule)
        else:
            ddl = self._regular_view(view_name, transpiled.sql)

        vd = ViewDefinition(
            name=view_name,
            source_rule_id=rule.id,
            ddl=ddl,
            is_materialized=should_mat,
        )
        log.debug("Generated %s %s for %s",
                  "MV" if should_mat else "VIEW", view_name, rule.id)
        return vd

    def generate_all(
        self,
        rules: List[NormalisedRule],
        results: Dict[str, TranspileResult],
    ) -> List[ViewDefinition]:
        """Generate views for all recording rules."""
        views: List[ViewDefinition] = []
        for rule in rules:
            if rule.rule_type != "recording":
                continue
            result = results.get(rule.id)
            if result is None:
                continue
            vd = self.generate(rule, result)
            if vd is not None:
                views.append(vd)
        log.info("Generated %d view definitions for recording rules", len(views))
        return views

    # ── internal ──────────────────────────────────────────────

    def _view_name(self, rule: NormalisedRule) -> str:
        """Deterministic, SQL-safe view name."""
        safe_group = re.sub(r'[^a-zA-Z0-9]', '_', rule.group_name).lower()
        safe_name = re.sub(r'[^a-zA-Z0-9_:]', '_', rule.name).lower()
        safe_name = safe_name.replace(':', '_')
        return f"{self.database}.mv_rec_{safe_group}_{safe_name}"

    def _should_materialize(self, rule: NormalisedRule, sql: str) -> bool:
        """Decide between VIEW and MATERIALIZED VIEW."""
        # Rule of thumb: materialise if referenced multiple times or expensive
        refs = self._ref_counts.get(rule.name, 0)
        if refs >= 2:
            return True
        # If the query involves rate / increase / window functions → expensive
        sql_lower = sql.lower()
        expensive_patterns = [
            "laginframe", "over w", "over (", "window",
            # rate-like patterns the transpiler generates
            "toUnixTimestamp64Milli".lower(),
        ]
        for pat in expensive_patterns:
            if pat in sql_lower:
                return True
        return False

    def _regular_view(self, name: str, sql: str) -> str:
        return f"CREATE OR REPLACE VIEW {name} AS\n{sql}"

    def _materialized_view(
        self,
        name: str,
        sql: str,
        rule: NormalisedRule,
    ) -> str:
        """
        Wrap the transpiled SQL in a MATERIALIZED VIEW with
        AggregatingMergeTree engine.  Falls back to a regular VIEW
        if the SQL structure doesn't lend itself to MV.

        ClickHouse optimizations applied:
          - ORDER BY = GROUP BY columns → enables optimize_read_in_order
          - PARTITION BY toYYYYMM(ts) → month-level partition pruning
          - TTL → automatic expiry matching source retention
          - INDEX ts_minmax → skip granules based on time range
        """
        # Extract GROUP BY columns (heuristic)
        group_cols = self._extract_group_by(sql)
        if not group_cols:
            # Cannot determine ORDER BY for MV — use regular VIEW
            return self._regular_view(name, sql)

        target_table = f"{name}_data"
        order_key = ", ".join(group_cols)

        # Detect which column in GROUP BY is timestamp-like for PARTITION/TTL
        ts_col = self._detect_timestamp_col(group_cols)

        ddl_lines = [
            f"-- Recording rule: {rule.name}",
            f"-- Group: {rule.group_name}",
            f"-- PromQL: {rule.promql}",
            f"",
            f"CREATE TABLE IF NOT EXISTS {target_table} (",
        ]

        # Column definitions — inferred from SELECT clause
        select_cols = self._extract_select_columns(sql)
        for col in select_cols:
            ddl_lines.append(f"    {col},")
        # Remove trailing comma from last column
        if ddl_lines and ddl_lines[-1].endswith(","):
            ddl_lines[-1] = ddl_lines[-1][:-1]

        # Skip index on timestamp column for sub-partition pruning
        if ts_col:
            ddl_lines.append(f"    , INDEX idx_{ts_col}_minmax {ts_col} TYPE minmax GRANULARITY 8192")

        ddl_lines.append(f")")
        ddl_lines.append(f"ENGINE = AggregatingMergeTree()")

        # PARTITION BY — month granularity keeps partition count manageable
        if ts_col:
            ddl_lines.append(f"PARTITION BY toYYYYMM({ts_col})")

        ddl_lines.append(f"ORDER BY ({order_key})")

        # TTL — automatic lifecycle management
        if ts_col and self.ttl_days > 0:
            ddl_lines.append(f"TTL {ts_col} + INTERVAL {self.ttl_days} DAY DELETE")

        ddl_lines.append(f"SETTINGS index_granularity = 8192;")
        ddl_lines.append(f"")
        ddl_lines.append(f"CREATE MATERIALIZED VIEW IF NOT EXISTS {name}")
        ddl_lines.append(f"TO {target_table}")
        ddl_lines.append(f"AS {sql}")

        return "\n".join(ddl_lines)

    def _detect_timestamp_col(self, cols: List[str]) -> str:
        """Identify which GROUP BY column is the timestamp."""
        ts_hints = {"timeunix", "timestamp", "time", "ts", "created_at", "event_time"}
        for col in cols:
            col_clean = col.strip().lower()
            # Handle expressions like toStartOfMinute(TimeUnix)
            inner = re.search(r'\(([^)]+)\)', col_clean)
            check = inner.group(1) if inner else col_clean
            if check in ts_hints:
                return col.strip()
        return ""

    @staticmethod
    def _extract_select_columns(sql: str) -> List[str]:
        """
        Best-effort extraction of column definitions from the SELECT clause.
        Returns placeholder type definitions for the target table.
        """
        select_match = re.search(
            r'SELECT\s+(.*?)\s+FROM\b', sql,
            re.IGNORECASE | re.DOTALL,
        )
        if not select_match:
            return []

        raw = select_match.group(1)
        # Split on top-level commas (skip commas inside parens)
        depth = 0
        parts: List[str] = []
        current: List[str] = []
        for ch in raw:
            if ch == '(':
                depth += 1
            elif ch == ')':
                depth -= 1
            elif ch == ',' and depth == 0:
                parts.append("".join(current).strip())
                current = []
                continue
            current.append(ch)
        if current:
            parts.append("".join(current).strip())

        defs: List[str] = []
        for expr in parts:
            # Determine alias: "expr AS alias" or last word
            alias_match = re.search(r'\bAS\s+(\w+)\s*$', expr, re.IGNORECASE)
            if alias_match:
                alias = alias_match.group(1)
            else:
                alias = expr.split()[-1] if expr.split() else "col"
                alias = re.sub(r'[^a-zA-Z0-9_]', '', alias)
            if not alias:
                continue
            # Infer type heuristically
            dtype = _infer_column_type(expr)
            defs.append(f"{alias} {dtype}")
        return defs

    @staticmethod
    def _extract_group_by(sql: str) -> List[str]:
        """Extract column names from a GROUP BY clause."""
        match = re.search(r'GROUP\s+BY\s+(.*?)(?:ORDER|LIMIT|HAVING|$)',
                          sql, re.IGNORECASE | re.DOTALL)
        if not match:
            return []
        cols = match.group(1).strip().rstrip(";")
        return [c.strip() for c in cols.split(",") if c.strip()]


def _infer_column_type(expr: str) -> str:
    """Heuristic column type inference from a SQL expression."""
    e = expr.lower()
    if "state(" in e or "merge(" in e:
        return "AggregateFunction(avg, Float64)"
    if any(kw in e for kw in ("count", "sum", "avg", "min(", "max(")):
        return "Float64"
    if any(kw in e for kw in ("tostartofsecond", "tostartofminute",
                               "tostartofhour", "tostartoffivemin")):
        return "DateTime"
    if "timeunix" in e or "timestamp" in e:
        return "DateTime64(3)"
    if "metricname" in e or "metric_name" in e:
        return "LowCardinality(String)"
    if "attributes" in e or "labels" in e:
        return "Map(LowCardinality(String), String)"
    return "Float64"
