"""
Transpilation dispatcher — calls the Go PromQL→ClickHouse transpiler
binary as a subprocess, handles schema configuration, and collects
results.

The transpiler binary API (from cmd/promql-transpiler/main.go):
  promql-transpiler -q '<promql>' -s <start> -e <end> --step <step>
"""

import hashlib
import logging
import os
import subprocess
from pathlib import Path
from typing import Dict, List, Optional, Tuple

from migration import config

log = logging.getLogger("migration.transpile.dispatch")

# ─── Result ───────────────────────────────────────────────────


class TranspileResult:
    """Outcome of transpiling a single PromQL expression."""

    __slots__ = (
        "promql", "sql", "error", "source_id",
        "content_hash", "metric_table",
    )

    def __init__(
        self,
        promql: str,
        sql: str = "",
        error: str = "",
        source_id: str = "",
        metric_table: str = "",
    ):
        self.promql = promql
        self.sql = sql
        self.error = error
        self.source_id = source_id
        self.metric_table = metric_table
        self.content_hash = hashlib.sha256(
            f"{promql}|{sql}".encode()
        ).hexdigest()

    @property
    def ok(self) -> bool:
        return bool(self.sql) and not self.error


# ─── Dispatcher ───────────────────────────────────────────────


class TranspileDispatcher:
    """
    Calls the compiled Go transpiler binary for each PromQL expression.

    The binary is expected at the path configured by TRANSPILER_BINARY
    (resolved relative to the migration/ directory).

    When a ``TableSchema`` is provided (via ``set_schema``), the dispatcher
    appends ``--table``, ``--metric-col``, etc. flags so the Go binary
    generates SQL that targets the real OTel column names.
    """

    def __init__(
        self,
        binary_path: Optional[str] = None,
        start_time: Optional[str] = None,
        end_time: Optional[str] = None,
        step: Optional[str] = None,
    ):
        raw = binary_path or config.TRANSPILER_BINARY
        # Resolve relative to transpiler root (migration/transpile/dispatch.py → ../../)
        self.binary = str(
            (Path(__file__).resolve().parent.parent.parent / raw).resolve()
        )
        self.start = start_time or config.DEFAULT_START_TIME
        self.end = end_time or config.DEFAULT_END_TIME
        self.step = step or config.DEFAULT_STEP
        self._schema_args: List[str] = []  # extra CLI flags from TableSchema

        if not os.path.isfile(self.binary):
            log.warning("Transpiler binary not found at %s (resolved from %s)",
                        self.binary, raw)

    # ── schema wiring ─────────────────────────────────────────

    def set_schema(self, ts) -> None:
        """
        Accept a TableSchema (or any object with the right attrs) and
        build the list of CLI flags once, reused for every subprocess call.
        """
        args: List[str] = []
        if getattr(ts, "table_name", ""):
            args += ["--table", ts.table_name]
        if getattr(ts, "metric_name_col", ""):
            args += ["--metric-col", ts.metric_name_col]
        if getattr(ts, "labels_col", ""):
            args += ["--labels-col", ts.labels_col]
        if getattr(ts, "timestamp_col", ""):
            args += ["--timestamp-col", ts.timestamp_col]
        if getattr(ts, "value_col", ""):
            args += ["--value-col", ts.value_col]
        if getattr(ts, "engine", ""):
            args += ["--engine", ts.engine]
        if getattr(ts, "order_key", ""):
            args += ["--order-by", ts.order_key]
        if getattr(ts, "partition_key", ""):
            args += ["--partition-by", ts.partition_key]
        self._schema_args = args
        log.info("Schema CLI args: %s", " ".join(args))

    # ── public ────────────────────────────────────────────────

    def transpile(
        self,
        promql: str,
        source_id: str = "",
    ) -> TranspileResult:
        """
        Transpile a single PromQL expression.

        Returns a TranspileResult with either .sql populated (success)
        or .error populated (failure).
        """
        promql = promql.strip()
        if not promql:
            return TranspileResult(promql, error="empty expression", source_id=source_id)

        cmd = [
            self.binary,
            "-q", promql,
            "-s", self.start,
            "-e", self.end,
            "--step", self.step,
        ] + self._schema_args

        try:
            proc = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=30,
            )
        except FileNotFoundError:
            return TranspileResult(
                promql,
                error=f"transpiler binary not found: {self.binary}",
                source_id=source_id,
            )
        except subprocess.TimeoutExpired:
            return TranspileResult(
                promql,
                error="transpilation timed out after 30s",
                source_id=source_id,
            )

        if proc.returncode != 0:
            err = proc.stderr.strip() or f"exit code {proc.returncode}"
            log.debug("Transpile failed for %s: %s", source_id, err)
            return TranspileResult(promql, error=err, source_id=source_id)

        sql = proc.stdout.strip()
        return TranspileResult(promql, sql=sql, source_id=source_id)

    def transpile_batch(
        self,
        items: List[Tuple[str, str]],
    ) -> Dict[str, TranspileResult]:
        """
        Transpile a list of (source_id, promql) tuples.
        Returns {source_id: TranspileResult}.
        """
        results: Dict[str, TranspileResult] = {}
        total = len(items)
        for idx, (sid, promql) in enumerate(items, 1):
            result = self.transpile(promql, source_id=sid)
            results[sid] = result
            if result.ok:
                log.debug("[%d/%d] OK  %s", idx, total, sid)
            else:
                log.warning("[%d/%d] FAIL %s — %s", idx, total, sid, result.error)
        successes = sum(1 for r in results.values() if r.ok)
        log.info(
            "Batch transpilation: %d/%d succeeded, %d failed",
            successes, total, total - successes,
        )
        return results
