"""
ClickHouse DDL executor — runs CREATE VIEW / MATERIALIZED VIEW statements
and manages the migration state table for idempotency.
"""

import logging
from typing import List, Optional

from migration import config
from migration.adapt.views import ViewDefinition

log = logging.getLogger("migration.deploy.clickhouse_exec")


class ClickHouseExecutor:
    """
    Executes DDL against ClickHouse and tracks migration state.
    Supports dry-run mode where DDL is logged but not executed.
    """

    def __init__(self, ch_client=None, dry_run: Optional[bool] = None):
        self._client = ch_client
        self.dry_run = dry_run if dry_run is not None else config.DRY_RUN
        self._ensure_state_table()

    # ── public ────────────────────────────────────────────────

    def deploy_views(
        self,
        views: List[ViewDefinition],
    ) -> dict:
        """
        Deploy all view definitions.
        Returns {created: int, updated: int, unchanged: int, failed: int, errors: []}
        """
        stats = {"created": 0, "updated": 0, "unchanged": 0, "failed": 0, "errors": []}

        for vd in views:
            try:
                status = self._deploy_one_view(vd)
                stats[status] += 1
            except Exception as exc:
                stats["failed"] += 1
                stats["errors"].append({"name": vd.name, "error": str(exc)})
                log.error("Failed to deploy view %s: %s", vd.name, exc)

        log.info(
            "View deployment: %d created, %d updated, %d unchanged, %d failed",
            stats["created"], stats["updated"], stats["unchanged"], stats["failed"],
        )
        return stats

    def execute_sql(self, sql: str, description: str = "") -> bool:
        """Execute arbitrary SQL (for ad-hoc queries). Returns success."""
        if self.dry_run:
            log.info("[DRY-RUN] Would execute: %s — %s", description, sql[:200])
            return True
        try:
            self._get_client().command(sql)
            return True
        except Exception as exc:
            log.error("SQL execution failed (%s): %s", description, exc)
            return False

    # ── internal ──────────────────────────────────────────────

    def _deploy_one_view(self, vd: ViewDefinition) -> str:
        """Deploy a single view. Returns 'created', 'updated', or 'unchanged'."""
        existing_hash = self._get_deployed_hash(vd.name)

        if existing_hash == vd.content_hash:
            log.debug("Unchanged: %s", vd.name)
            return "unchanged"

        if existing_hash:
            # View exists but content changed — drop + recreate
            log.info("Updating view %s (hash changed)", vd.name)
            if not self.dry_run:
                self._get_client().command(vd.drop_ddl)
            status = "updated"
        else:
            status = "created"

        if self.dry_run:
            log.info("[DRY-RUN] Would create: %s", vd.name)
        else:
            # Execute DDL (may contain multiple statements separated by ;)
            for stmt in vd.ddl.split(";"):
                stmt = stmt.strip()
                if stmt and not stmt.startswith("--"):
                    self._get_client().command(stmt)
            log.info("Created view: %s", vd.name)

        self._save_state(vd.name, "view", vd.source_rule_id, vd.content_hash)
        return status

    # ── state table ───────────────────────────────────────────

    def _ensure_state_table(self):
        ddl = """
        CREATE TABLE IF NOT EXISTS {db}._migration_state (
            artefact_id   String,
            artefact_type String,
            source_id     String,
            content_hash  String,
            deployed_at   DateTime DEFAULT now()
        )
        ENGINE = ReplacingMergeTree(deployed_at)
        ORDER BY (artefact_id)
        """.format(db=config.CLICKHOUSE_DATABASE)

        if self.dry_run:
            return
        try:
            self._get_client().command(ddl)
        except Exception as exc:
            log.warning("Could not create _migration_state table: %s", exc)

    def _get_deployed_hash(self, artefact_id: str) -> Optional[str]:
        if self.dry_run:
            return None
        try:
            result = self._get_client().query(
                "SELECT content_hash FROM {db}._migration_state FINAL "
                "WHERE artefact_id = '{aid}'".format(
                    db=config.CLICKHOUSE_DATABASE,
                    aid=artefact_id.replace("'", "''"),
                )
            )
            if result.result_rows:
                return result.result_rows[0][0]
        except Exception:
            pass
        return None

    def _save_state(
        self,
        artefact_id: str,
        artefact_type: str,
        source_id: str,
        content_hash: str,
    ):
        if self.dry_run:
            return
        try:
            self._get_client().command(
                "INSERT INTO {db}._migration_state "
                "(artefact_id, artefact_type, source_id, content_hash) "
                "VALUES ('{aid}', '{atype}', '{sid}', '{hash}')".format(
                    db=config.CLICKHOUSE_DATABASE,
                    aid=artefact_id.replace("'", "''"),
                    atype=artefact_type,
                    sid=source_id.replace("'", "''"),
                    hash=content_hash,
                )
            )
        except Exception as exc:
            log.warning("Could not save migration state for %s: %s",
                        artefact_id, exc)

    def get_all_deployed(self) -> dict:
        """Return {artefact_id: content_hash} for all deployed artefacts."""
        if self.dry_run:
            return {}
        try:
            result = self._get_client().query(
                "SELECT artefact_id, content_hash "
                "FROM {db}._migration_state FINAL".format(
                    db=config.CLICKHOUSE_DATABASE,
                )
            )
            return {row[0]: row[1] for row in result.result_rows}
        except Exception:
            return {}

    # ── client ────────────────────────────────────────────────

    def _get_client(self):
        if self._client is None:
            import clickhouse_connect
            self._client = clickhouse_connect.get_client(
                host=config.CLICKHOUSE_HOST,
                port=config.CLICKHOUSE_PORT,
                username=config.CLICKHOUSE_USER,
                password=config.CLICKHOUSE_PASSWORD,
            )
        return self._client
