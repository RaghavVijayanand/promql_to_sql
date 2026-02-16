"""
Schema resolver — introspects ClickHouse to discover OTel table layouts
and builds the metric-name mapping between Prometheus and OTel conventions.

At startup the orchestrator calls `resolve()` which:
  1. Queries system.tables for otel_metrics_* tables
  2. Queries system.columns for each table's column types
  3. Queries each table for distinct MetricNames
  4. Builds a name-mapping dictionary (prometheus name → otel name)
  5. Returns a SchemaMap ready for the transpiler to use
"""

import logging
import re
from typing import Any, Dict, List, Optional, Tuple

from migration import config

log = logging.getLogger("migration.adapt.schema")

# ─── Data structures ──────────────────────────────────────────


class TableSchema:
    """Physical schema of one OTel metrics table."""

    __slots__ = (
        "database", "table_name", "full_name", "engine",
        "metric_name_col", "labels_col", "timestamp_col", "value_col",
        "order_key", "partition_key",
    )

    def __init__(self, **kw):
        self.database: str = kw.get("database", "")
        self.table_name: str = kw.get("table_name", "")
        self.full_name: str = kw.get("full_name", "")
        self.engine: str = kw.get("engine", "MergeTree")
        self.metric_name_col: str = kw.get("metric_name_col", "MetricName")
        self.labels_col: str = kw.get("labels_col", "Attributes")
        self.timestamp_col: str = kw.get("timestamp_col", "TimeUnix")
        self.value_col: str = kw.get("value_col", "Value")
        self.order_key: str = kw.get("order_key", "")
        self.partition_key: str = kw.get("partition_key", "")


class SchemaMap:
    """
    Resolved schema containing:
      - table schemas by metric type (gauge, sum, histogram, summary)
      - metric name mapping (prom name → OTel name)
      - default fallback table
    """

    def __init__(self):
        self.tables: Dict[str, TableSchema] = {}       # type_key → TableSchema
        self.metric_names: Dict[str, str] = {}          # prom_name → otel_name
        self.otel_names: Dict[str, str] = {}            # otel_name → table type_key
        self.default_table: Optional[TableSchema] = None

    def table_for_metric(self, metric_name: str) -> Optional[TableSchema]:
        """Return the table that stores the given metric, or default."""
        otel_name = self.metric_names.get(metric_name, metric_name)
        type_key = self.otel_names.get(otel_name)
        if type_key and type_key in self.tables:
            return self.tables[type_key]
        return self.default_table


# ─── Resolver ─────────────────────────────────────────────────


class SchemaResolver:
    """Introspects ClickHouse to build a SchemaMap."""

    def __init__(self, ch_client=None):
        self._client = ch_client

    def resolve(self) -> SchemaMap:
        """Execute introspection and return a populated SchemaMap."""
        if self._client is None:
            self._client = _get_client()

        schema_map = SchemaMap()

        # 1. Discover tables
        tables = self._discover_tables()
        for type_key, ts in tables.items():
            # 2. Introspect columns for each table
            ts = self._introspect_columns(ts)
            schema_map.tables[type_key] = ts

        # 3. Pick default table (prefer gauge, then sum)
        for pref in ("gauge", "sum", "histogram"):
            if pref in schema_map.tables:
                schema_map.default_table = schema_map.tables[pref]
                break
        if not schema_map.default_table and schema_map.tables:
            schema_map.default_table = next(iter(schema_map.tables.values()))

        # 4. Discover metric names and build mapping
        schema_map.metric_names, schema_map.otel_names = (
            self._build_metric_mapping(schema_map.tables)
        )

        log.info(
            "Schema resolved: %d tables, %d metric name mappings, "
            "default table = %s",
            len(schema_map.tables),
            len(schema_map.metric_names),
            schema_map.default_table.full_name if schema_map.default_table else "none",
        )
        return schema_map

    # ── internal ──────────────────────────────────────────────

    def _discover_tables(self) -> Dict[str, TableSchema]:
        """Find otel_metrics_* tables in system.tables."""
        db = config.CLICKHOUSE_DATABASE
        tables: Dict[str, TableSchema] = {}

        # Check config overrides first
        overrides = {
            "gauge":     config.OTEL_METRICS_TABLE_GAUGE,
            "sum":       config.OTEL_METRICS_TABLE_SUM,
            "histogram": config.OTEL_METRICS_TABLE_HISTOGRAM,
            "summary":   config.OTEL_METRICS_TABLE_SUMMARY,
        }
        for type_key, override in overrides.items():
            if override:
                tables[type_key] = TableSchema(
                    database=db,
                    table_name=override,
                    full_name=f"{db}.{override}",
                )

        if tables:
            return tables

        # Auto-detect
        try:
            rows = self._client.query(
                "SELECT name, engine, sorting_key, partition_key "
                "FROM system.tables "
                f"WHERE database = '{db}' "
                "AND name LIKE '%metrics%' "
                "AND engine LIKE '%MergeTree%'"
            )
            for row in rows.result_rows:
                name = row[0]
                engine = row[1]
                order_key = row[2] if len(row) > 2 else ""
                part_key = row[3] if len(row) > 3 else ""
                type_key = self._infer_type_key(name)
                tables[type_key] = TableSchema(
                    database=db,
                    table_name=name,
                    full_name=f"{db}.{name}",
                    engine=engine,
                    order_key=order_key,
                    partition_key=part_key,
                )
        except Exception as exc:
            log.warning("Could not auto-detect OTel tables: %s", exc)
            # Fallback to common defaults
            for tk, tn in [
                ("gauge", "otel_metrics_gauge"),
                ("sum", "otel_metrics_sum"),
                ("histogram", "otel_metrics_histogram"),
            ]:
                tables[tk] = TableSchema(
                    database=db, table_name=tn, full_name=f"{db}.{tn}"
                )

        return tables

    def _introspect_columns(self, ts: TableSchema) -> TableSchema:
        """Determine which columns serve as timestamp, value, labels, etc."""
        try:
            rows = self._client.query(
                "SELECT name, type "
                "FROM system.columns "
                f"WHERE database = '{ts.database}' AND table = '{ts.table_name}'"
            )
        except Exception:
            return ts  # use defaults

        for name, col_type in rows.result_rows:
            low = name.lower()
            tlow = col_type.lower()
            if "datetime" in tlow and ("time" in low or "timestamp" in low):
                ts.timestamp_col = name
            elif "float" in tlow and ("value" in low or "gauge" in low or "sum" in low):
                ts.value_col = name
            elif "map" in tlow and ("attr" in low or "label" in low or "resource" in low):
                if "resource" not in low:  # prefer non-resource attributes
                    ts.labels_col = name
            elif "string" in tlow and ("metric" in low or "name" in low):
                if "metric" in low:
                    ts.metric_name_col = name
        return ts

    def _build_metric_mapping(
        self,
        tables: Dict[str, TableSchema],
    ) -> Tuple[Dict[str, str], Dict[str, str]]:
        """
        Query distinct metric names from ClickHouse and build
        prometheus_name → otel_name mapping.
        """
        all_otel_names: Dict[str, str] = {}  # otel_name → type_key
        prom_to_otel: Dict[str, str] = {}

        for type_key, ts in tables.items():
            try:
                rows = self._client.query(
                    f"SELECT DISTINCT {ts.metric_name_col} "
                    f"FROM {ts.full_name} "
                    f"LIMIT 5000"
                )
                for (otel_name,) in rows.result_rows:
                    all_otel_names[otel_name] = type_key
            except Exception as exc:
                log.debug(
                    "Could not query metric names from %s: %s",
                    ts.full_name, exc,
                )

        style = config.METRIC_NAME_STYLE.lower()

        for otel_name in all_otel_names:
            # Always map the name to itself
            prom_to_otel[otel_name] = otel_name

            if style in ("auto", "prometheus"):
                # Generate prom-style equivalent and map it
                prom_name = _to_prom_name(otel_name)
                if prom_name != otel_name:
                    prom_to_otel[prom_name] = otel_name

            if style in ("auto", "otel"):
                # Generate otel-style equivalent and map it
                otel_alt = _to_otel_name(otel_name)
                if otel_alt != otel_name:
                    prom_to_otel[otel_alt] = otel_name

        return prom_to_otel, all_otel_names

    @staticmethod
    def _infer_type_key(table_name: str) -> str:
        low = table_name.lower()
        if "gauge" in low:
            return "gauge"
        if "sum" in low:
            return "sum"
        if "histogram" in low or "histo" in low:
            return "histogram"
        if "summary" in low:
            return "summary"
        if "exponential" in low:
            return "exp_histogram"
        return low.split("_")[-1] if "_" in low else low


# ─── Name mapping helpers ─────────────────────────────────────


def _to_prom_name(otel_name: str) -> str:
    """Convert OTel dot-style name to Prometheus underscore-style."""
    return otel_name.replace(".", "_")


def _to_otel_name(prom_name: str) -> str:
    """
    Convert Prometheus underscore-style name to OTel dot-style.

    Only convert underscores that are namespace separators — not every
    underscore.  Known namespace prefixes (http_, rpc_, db_, net_, etc.)
    are converted to dot-separated form.  Everything else is left as-is
    because Prometheus names like ``scrape_duration_seconds`` have no OTel
    dot-style equivalent.
    """
    if "_" not in prom_name:
        return prom_name

    # Known OTel semantic convention namespace prefixes
    _OTEL_PREFIXES = (
        "http_", "rpc_", "db_", "net_", "messaging_",
        "faas_", "cloud_", "container_", "host_", "os_",
        "process_", "k8s_", "aws_", "gcp_", "azure_",
        "system_", "service_",
    )
    for prefix in _OTEL_PREFIXES:
        if prom_name.startswith(prefix):
            # Convert only the namespace prefix part to dots, keep
            # the rest with underscores replaced by dots.
            return prom_name.replace("_", ".")

    # Not a recognized OTel namespace — return unchanged.
    return prom_name


# ─── ClickHouse client factory ────────────────────────────────


def _get_client():
    """Create a clickhouse-connect client from .env configuration."""
    import clickhouse_connect
    return clickhouse_connect.get_client(
        host=config.CLICKHOUSE_HOST,
        port=config.CLICKHOUSE_PORT,
        username=config.CLICKHOUSE_USER,
        password=config.CLICKHOUSE_PASSWORD,
    )
