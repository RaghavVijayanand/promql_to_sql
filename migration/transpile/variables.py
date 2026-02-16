"""
Variable pre-processor — resolves Grafana template variables and
macros into concrete values *before* the PromQL reaches the
transpiler, then annotates the resulting SQL with placeholder
markers for HyperDX query-time parameterisation.

Strategy:
  1. Replace built-in macros ($__interval, $__timeFilter, etc.) with
     concrete defaults so the PromQL parses cleanly.
  2. Replace custom variables ($status, $instance) with a sentinel
     value for transpilation, then swap the sentinel in the SQL output
     with a HyperDX filter parameter.
"""

import logging
import re
from typing import Dict, List, Optional, Set, Tuple

log = logging.getLogger("migration.transpile.variables")

# ─── Constants ────────────────────────────────────────────────

_SENTINEL_PREFIX = "__HDX_VAR_"
_SENTINEL_SUFFIX = "__"

# Built-in Grafana variables and their concrete substitutions
_BUILTIN_SUBS: Dict[str, str] = {
    "$__interval":       "1m",
    "$__interval_ms":    "60000",
    "$__rate_interval":  "1m",
    "$__range":          "1h",
    "$__range_s":        "3600",
    "$__range_ms":       "3600000",
}

# Regex for Grafana macros that take column arguments
_MACRO_TIME_FILTER = re.compile(r'\$__timeFilter\(([^)]+)\)')
_MACRO_TIME_FROM = re.compile(r'\$__timeFrom\(\)')
_MACRO_TIME_TO = re.compile(r'\$__timeTo\(\)')
_MACRO_TIME_GROUP = re.compile(r'\$__timeGroup\(([^,]+),\s*([^)]+)\)')
_MACRO_EPOCH_FILTER = re.compile(r'\$__unixEpochFilter\(([^)]+)\)')

# Regex for user-defined variables: $var or ${var} or ${var:format}
_USER_VAR_RE = re.compile(r'\$\{?([a-zA-Z_]\w*)(?::\w+)?\}?')

# Known built-in variable names (to avoid treating as user vars)
_BUILTIN_NAMES: Set[str] = {
    "__interval", "__interval_ms", "__rate_interval",
    "__range", "__range_s", "__range_ms",
    "__from", "__to", "__name", "__dashboard",
    "__panel", "__user", "__org",
}


# ─── Public API ───────────────────────────────────────────────


class VariablePreProcessor:
    """
    Resolves variables in a PromQL string before transpilation.

    After transpilation, call `annotate_sql()` to convert sentinel
    values in the SQL into HyperDX parameter bindings.
    """

    def __init__(
        self,
        declared_variables: Optional[Dict[str, str]] = None,
    ):
        # name → default value  (from Grafana template var "current")
        self.declared: Dict[str, str] = declared_variables or {}
        # Track which variables were substituted (for HyperDX filter generation)
        self._substituted: Dict[str, str] = {}

    def preprocess(self, promql: str) -> str:
        """
        Returns PromQL with all variables replaced so the transpiler
        receives syntactically valid input.
        """
        self._substituted.clear()
        result = promql

        # 1. Replace built-in scalar variables
        for var, value in _BUILTIN_SUBS.items():
            result = result.replace(var, value)

        # 2. Strip Grafana SQL macros that are NOT valid PromQL.
        #    These appear in ClickHouse/SQL data-source panels but
        #    are meaningless for PromQL transpilation.  We remove them
        #    so the transpiler receives clean PromQL.
        result = _MACRO_TIME_FILTER.sub('', result)
        result = _MACRO_TIME_FROM.sub('0', result)
        result = _MACRO_TIME_TO.sub('0', result)
        result = _MACRO_TIME_GROUP.sub(r'\1', result)
        result = _MACRO_EPOCH_FILTER.sub('', result)

        # Clean up any leftover empty matchers, e.g. `metric_name{,}`
        result = re.sub(r'\{\s*,\s*\}', '{}', result)
        result = re.sub(r'\{\s*,', '{', result)
        result = re.sub(r',\s*\}', '}', result)

        # 3. Replace user-defined variables with either:
        #    - their default value (if known and looks like a label value)
        #    - a sentinel value (if no default or default is empty)
        result = self._replace_user_vars(result)

        return result

    def get_substituted_variables(self) -> Dict[str, str]:
        """
        Return {var_name: sentinel_or_default} for all user variables
        that were substituted during the last `preprocess()` call.
        """
        return dict(self._substituted)

    def annotate_sql(
        self,
        sql: str,
    ) -> Tuple[str, List[Dict[str, str]]]:
        """
        Replace sentinel values in *sql* with HyperDX-compatible
        parameter placeholders and return the SQL + filter metadata.
        """
        filters: List[Dict[str, str]] = []
        annotated = sql

        for var_name, sub_value in self._substituted.items():
            sentinel = f"{_SENTINEL_PREFIX}{var_name}{_SENTINEL_SUFFIX}"
            if sentinel in annotated:
                placeholder = f":{var_name}"
                annotated = annotated.replace(f"'{sentinel}'", placeholder)
                annotated = annotated.replace(sentinel, placeholder)
                default = self.declared.get(var_name, "")
                filters.append({
                    "name": var_name,
                    "type": "string",
                    "default": default,
                })
            elif sub_value in annotated and sub_value != sentinel:
                # Default value was used — record for documentation
                filters.append({
                    "name": var_name,
                    "type": "string",
                    "default": sub_value,
                    "note": "hardcoded_default",
                })

        return annotated, filters

    # ── internal ──────────────────────────────────────────────

    def _replace_user_vars(self, promql: str) -> str:
        def _replacer(m: re.Match) -> str:
            full = m.group(0)
            name = m.group(1)

            # Skip built-ins
            if name in _BUILTIN_NAMES:
                return full

            # Skip if not a declared dashboard variable
            if name not in self.declared:
                return full

            default = self.declared.get(name, "")
            if default and default != "All" and default != "$__all":
                # Use the default value directly — it will appear in SQL
                self._substituted[name] = default
                return default
            else:
                # Use sentinel — will be swapped later
                sentinel = f"{_SENTINEL_PREFIX}{name}{_SENTINEL_SUFFIX}"
                self._substituted[name] = sentinel
                return sentinel

        return _USER_VAR_RE.sub(_replacer, promql)


# ─── Variable-query transpilation ─────────────────────────────

# For Grafana query-type variables like label_values(metric, label)
_LABEL_VALUES_RE = re.compile(
    r'label_values\(\s*([^,)]+?)(?:\s*,\s*([^)]+))?\s*\)'
)


def variable_query_to_sql(
    var_query: str,
    table: str = "otel.otel_metrics_gauge",
    label_col: str = "Attributes",
    metric_col: str = "MetricName",
) -> Optional[str]:
    """
    Convert a Grafana variable query (e.g. label_values(up, instance))
    into a ClickHouse SQL query that returns the distinct values.

    Returns None if the query format is not recognised.
    """
    m = _LABEL_VALUES_RE.match(var_query.strip())
    if not m:
        return None

    if m.group(2):
        # label_values(metric, label)
        metric = m.group(1).strip()
        label = m.group(2).strip()
        return (
            f"SELECT DISTINCT {label_col}['{label}'] AS value "
            f"FROM {table} "
            f"WHERE {metric_col} = '{metric}' "
            f"AND {label_col}['{label}'] != '' "
            f"ORDER BY value"
        )
    else:
        # label_values(label)  — across all metrics
        label = m.group(1).strip()
        return (
            f"SELECT DISTINCT {label_col}['{label}'] AS value "
            f"FROM {table} "
            f"WHERE {label_col}['{label}'] != '' "
            f"ORDER BY value"
        )
