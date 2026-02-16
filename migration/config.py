"""
Configuration loader — reads .env and exposes typed settings.

Every value has a default so the orchestrator runs out-of-the-box against
a local ClickStack + GAP stack; clients override only what differs.
"""

import os
import logging
from pathlib import Path
from dotenv import load_dotenv

_ENV_PATH = Path(__file__).resolve().parent / ".env"
load_dotenv(_ENV_PATH)


def _get(key: str, default: str = "") -> str:
    return os.getenv(key, default).strip()


def _bool(key: str, default: bool = False) -> bool:
    return _get(key, str(default)).lower() in ("true", "1", "yes")


def _int(key: str, default: int = 0) -> int:
    try:
        return int(_get(key, str(default)))
    except ValueError:
        return default


# ── Source endpoints ──────────────────────────────────────────

PROMETHEUS_URL: str = _get("PROMETHEUS_URL", "http://localhost:9090")
GRAFANA_URL: str = _get("GRAFANA_URL", "http://localhost:3000")
GRAFANA_API_TOKEN: str = _get("GRAFANA_API_TOKEN")
ALERTMANAGER_URL: str = _get("ALERTMANAGER_URL", "http://localhost:9093")

# ── Target: ClickHouse ────────────────────────────────────────

CLICKHOUSE_HOST: str = _get("CLICKHOUSE_HOST", "localhost")
CLICKHOUSE_PORT: int = _int("CLICKHOUSE_PORT", 8123)
CLICKHOUSE_USER: str = _get("CLICKHOUSE_USER", "default")
CLICKHOUSE_PASSWORD: str = _get("CLICKHOUSE_PASSWORD")
CLICKHOUSE_DATABASE: str = _get("CLICKHOUSE_DATABASE", "otel")

# ── Target: HyperDX ──────────────────────────────────────────

HYPERDX_URL: str = _get("HYPERDX_URL", "http://localhost:8080")
HYPERDX_API_KEY: str = _get("HYPERDX_API_KEY")

# ── Transpiler ────────────────────────────────────────────────

# Platform-aware default binary
import platform as _platform
_default_binary = (
    "bin/promql-transpiler.exe"
    if _platform.system() == "Windows"
    else "bin/promql-transpiler"
)
TRANSPILER_BINARY: str = _get("TRANSPILER_BINARY", _default_binary)

# ── OTel table names (empty → auto-detect) ───────────────────

OTEL_METRICS_TABLE_GAUGE: str = _get("OTEL_METRICS_TABLE_GAUGE")
OTEL_METRICS_TABLE_SUM: str = _get("OTEL_METRICS_TABLE_SUM")
OTEL_METRICS_TABLE_HISTOGRAM: str = _get("OTEL_METRICS_TABLE_HISTOGRAM")
OTEL_METRICS_TABLE_SUMMARY: str = _get("OTEL_METRICS_TABLE_SUMMARY")

# ── Metric name convention ────────────────────────────────────

METRIC_NAME_STYLE: str = _get("METRIC_NAME_STYLE", "auto")

# ── Migration behaviour ──────────────────────────────────────

DRY_RUN: bool = _bool("MIGRATION_DRY_RUN", False)
PRUNE_ORPHANS: bool = _bool("MIGRATION_PRUNE_ORPHANS", False)
LOG_LEVEL: str = _get("MIGRATION_LOG_LEVEL", "INFO")

# ── ClickHouse optimisation ──────────────────────────────────

PARTITION_RETENTION_DAYS: int = _int("PARTITION_RETENTION_DAYS", 90)
VIEW_PARTITION_BY: str = _get("VIEW_PARTITION_BY", "")  # e.g. toYYYYMM(TimeUnix)
DEFAULT_ENGINE: str = _get("DEFAULT_CH_ENGINE", "MergeTree")

# ── Time range defaults ──────────────────────────────────────

DEFAULT_START_TIME: str = _get("DEFAULT_START_TIME", "2024-01-01T00:00:00Z")
DEFAULT_END_TIME: str = _get("DEFAULT_END_TIME", "2024-01-01T23:59:59Z")
DEFAULT_STEP: str = _get("DEFAULT_STEP", "60s")

# ── Network ───────────────────────────────────────────────────

HTTP_TIMEOUT: int = _int("HTTP_TIMEOUT_SECONDS", 30)
HTTP_RETRIES: int = _int("HTTP_RETRY_COUNT", 3)
HTTP_RETRY_DELAY: int = _int("HTTP_RETRY_DELAY_SECONDS", 2)


# ── Logging bootstrap ────────────────────────────────────────

def setup_logging() -> logging.Logger:
    """Return the root migration logger, configured from .env."""
    level = getattr(logging, LOG_LEVEL.upper(), logging.INFO)
    logging.basicConfig(
        level=level,
        format="%(asctime)s [%(levelname)-7s] %(name)s — %(message)s",
        datefmt="%Y-%m-%dT%H:%M:%S",
    )
    return logging.getLogger("migration")
