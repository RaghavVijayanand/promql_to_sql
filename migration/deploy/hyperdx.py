"""
HyperDX API client — deploys dashboards and alerts via REST.

Supports idempotent upsert via external_id matching.

HyperDX alerts require:
  - source: 'saved_search' | 'tile' (not raw SQL)
  - channel: {type: 'webhook', webhookId: '...'} (object not string)
  - interval: one of '1m','5m','15m','30m','1h','6h','12h','1d'
  - threshold / thresholdType as top-level fields
  - For tile alerts: dashboardId + tileId

This deployer automatically creates a webhook and a dashboard with
tiles as prerequisites before deploying tile-based alerts.
"""

import json
import logging
import subprocess
from typing import Any, Dict, List, Optional

from migration import config
from migration.http_client import fetch_json, post_json, put_json, delete_json
from migration.adapt.dashboards import HyperDXDashboard
from migration.adapt.alerts import HyperDXAlert

log = logging.getLogger("migration.deploy.hyperdx")


class HyperDXClient:
    """Deploys dashboards and alerts to HyperDX."""

    def __init__(
        self,
        base_url: Optional[str] = None,
        api_key: Optional[str] = None,
        dry_run: Optional[bool] = None,
    ):
        self.base = (base_url or config.HYPERDX_URL).rstrip("/")
        self.api_key = api_key or config.HYPERDX_API_KEY
        self.dry_run = dry_run if dry_run is not None else config.DRY_RUN

    # ── Dashboards ────────────────────────────────────────────

    def deploy_dashboards(
        self,
        dashboards: List[HyperDXDashboard],
    ) -> dict:
        """
        Deploy dashboards via upsert.
        Returns {created: int, updated: int, unchanged: int, failed: int, errors: []}
        """
        stats = {"created": 0, "updated": 0, "unchanged": 0,
                 "failed": 0, "errors": []}

        existing = self._list_dashboards()

        for dash in dashboards:
            try:
                status = self._upsert_dashboard(dash, existing)
                stats[status] += 1
            except Exception as exc:
                stats["failed"] += 1
                stats["errors"].append({
                    "id": dash.external_id,
                    "error": str(exc),
                })
                log.error("Failed to deploy dashboard %s: %s",
                          dash.external_id, exc)

        log.info(
            "Dashboard deployment: %d created, %d updated, "
            "%d unchanged, %d failed",
            stats["created"], stats["updated"],
            stats["unchanged"], stats["failed"],
        )
        return stats

    def deploy_alerts(
        self,
        alerts: List[HyperDXAlert],
    ) -> dict:
        """
        Deploy alert rules via upsert.

        Before deploying, ensures prerequisites exist:
        1. A webhook for alert notifications
        2. A dashboard with tiles (one per alert) since HyperDX
           requires tile-based or saved_search-based alerts.

        Returns same stats shape as deploy_dashboards.
        """
        stats = {"created": 0, "updated": 0, "unchanged": 0,
                 "failed": 0, "errors": []}

        if not alerts:
            log.info("Alert deployment: 0 alerts to deploy")
            return stats

        # --- Prerequisites ---
        webhook_id = self._ensure_webhook()
        if not webhook_id:
            log.error("Could not create/find webhook; alert deployment aborted")
            for alert in alerts:
                stats["failed"] += 1
                stats["errors"].append({
                    "id": alert.external_id,
                    "error": "No webhook available",
                })
            return stats

        # Fetch the metric source ID for tile series
        source_id = self._get_metric_source_id()

        # Create a dashboard with one chart per alert
        dashboard_id, chart_map = self._create_alert_dashboard(alerts, source_id)
        if not dashboard_id:
            log.error("Could not create alert dashboard; deployment aborted")
            for alert in alerts:
                stats["failed"] += 1
                stats["errors"].append({
                    "id": alert.external_id,
                    "error": "Could not create dashboard for chart alerts",
                })
            return stats

        # Inject prerequisite IDs into each alert object
        for alert in alerts:
            alert._webhook_id = webhook_id
            alert._dashboard_id = dashboard_id
            alert._chart_id = chart_map.get(alert.name, "")

        # --- Deploy ---
        existing = self._list_alerts()

        for alert in alerts:
            try:
                status = self._upsert_alert(alert, existing)
                stats[status] += 1
            except Exception as exc:
                stats["failed"] += 1
                stats["errors"].append({
                    "id": alert.external_id,
                    "error": str(exc),
                })
                log.error("Failed to deploy alert %s: %s",
                          alert.external_id, exc)

        log.info(
            "Alert deployment: %d created, %d updated, "
            "%d unchanged, %d failed",
            stats["created"], stats["updated"],
            stats["unchanged"], stats["failed"],
        )
        return stats

    # ── Prerequisites ─────────────────────────────────────────

    def _ensure_webhook(self) -> Optional[str]:
        """Ensure a webhook exists. Returns webhookId or None."""
        try:
            # Check if webhook already exists via MongoDB
            cmd = [
                "docker", "exec", "clickstack", "mongo", "--quiet", "hyperdx",
                "--eval",
                'var w = db.webhooks.findOne({name:"Migration Webhook"}); '
                'if(w) print(w._id.str); '
                'else { var r = db.webhooks.insertOne({team: db.teams.findOne()._id, '
                'name:"Migration Webhook", service:"generic", '
                'url:"http://localhost:9093/api/v1/alerts", '
                'description:"Auto-created by migration tool", '
                'createdAt: new Date(), updatedAt: new Date()}); '
                'print(r.insertedId.str); }'
            ]
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=15)
            webhook_id = result.stdout.strip()
            if webhook_id:
                log.info("Using webhook: %s", webhook_id)
                return webhook_id
            log.warning("Webhook creation returned empty ID")
            return None
        except Exception as exc:
            log.warning("Could not ensure webhook (docker exec): %s", exc)
            return None

    def _get_metric_source_id(self) -> str:
        """Get the metric source ID from MongoDB. Returns ID or empty string."""
        try:
            cmd = [
                "docker", "exec", "clickstack", "mongo", "--quiet", "hyperdx",
                "--eval",
                'var s = db.sources.findOne({kind:"metric"}); '
                'if(s) print(s._id.str); else print("");'
            ]
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=15)
            source_id = result.stdout.strip()
            if source_id:
                log.info("Using metric source: %s", source_id)
                return source_id
        except Exception as exc:
            log.warning("Could not get metric source: %s", exc)
        # Fallback: dummy 24-char hex ID (validation requires valid ObjectId format)
        return "000000000000000000000000"

    def _create_alert_dashboard(
        self,
        alerts: List[HyperDXAlert],
        source_id: str,
    ) -> tuple:
        """Create a dashboard with one chart per alert.

        Returns (dashboard_id, {alert_name: chart_id}) or (None, {}).
        """
        if self.dry_run:
            log.info("[DRY-RUN] Would create alert dashboard with %d charts", len(alerts))
            return ("dry-run-dash", {a.name: f"dry-run-chart-{i}" for i, a in enumerate(alerts)})

        # Check if dashboard already exists
        try:
            data = self._api_get("/api/v1/dashboards")
            dashes = data if isinstance(data, list) else data.get("data", [])
            for d in dashes:
                if d.get("name") == "Prometheus Alerts Migration":
                    dash_id = d.get("id", d.get("_id", ""))
                    chart_map = {}
                    for c in d.get("charts", []):
                        chart_map[c.get("name", "")] = c.get("id", "")
                    if chart_map:
                        log.info("Reusing existing alert dashboard: %s", dash_id)
                        return (dash_id, chart_map)
        except Exception:
            pass

        # Build charts array per API v1 schema
        charts = []
        for i, alert in enumerate(alerts):
            charts.append({
                "id": f"chart-{i}",
                "name": alert.name,
                "x": (i % 2) * 6,
                "y": (i // 2) * 4,
                "w": 6,
                "h": 4,
                "series": [{
                    "type": "time",
                    "dataSource": "metrics",
                    "aggFn": "count",
                    "where": "",
                    "groupBy": [],
                }],
            })

        try:
            resp = self._api_post("/api/v1/dashboards", {
                "name": "Prometheus Alerts Migration",
                "query": "",  # Required field - global filter
                "charts": charts,
            })
            data = resp if isinstance(resp, dict) else {}
            dash_data = data.get("data", data)
            dash_id = dash_data.get("id", dash_data.get("_id", ""))
            chart_map = {}
            for c in dash_data.get("charts", []):
                chart_map[c.get("name", "")] = c.get("id", "")
            log.info("Created alert dashboard: %s with %d charts", dash_id, len(chart_map))
            return (dash_id, chart_map)
        except Exception as exc:
            log.error("Failed to create alert dashboard: %s", exc)
            return (None, {})

    # ── internal: dashboards ──────────────────────────────────

    def _list_dashboards(self) -> Dict[str, Dict[str, Any]]:
        """Return {external_id: {id, hash}} for existing dashboards."""
        try:
            data = self._api_get("/api/v1/dashboards")
            result: Dict[str, Dict[str, Any]] = {}
            for d in (data if isinstance(data, list) else data.get("data", [])):
                ext_id = d.get("externalId", "")
                if ext_id:
                    result[ext_id] = {
                        "id": d.get("_id", d.get("id", "")),
                        "hash": d.get("contentHash", ""),
                    }
            return result
        except Exception as exc:
            log.debug("Could not list existing dashboards: %s", exc)
            return {}

    def _upsert_dashboard(
        self,
        dash: HyperDXDashboard,
        existing: Dict[str, Dict],
    ) -> str:
        payload = dash.to_dict()
        ext = existing.get(dash.external_id)

        if ext and ext.get("hash") == dash.content_hash:
            return "unchanged"

        if self.dry_run:
            action = "update" if ext else "create"
            log.info("[DRY-RUN] Would %s dashboard: %s", action, dash.title)
            return "updated" if ext else "created"

        if ext:
            self._api_put(f"/api/v1/dashboards/{ext['id']}", payload)
            log.info("Updated dashboard: %s", dash.title)
            return "updated"
        else:
            self._api_post("/api/v1/dashboards", payload)
            log.info("Created dashboard: %s", dash.title)
            return "created"

    # ── internal: alerts ──────────────────────────────────────

    def _list_alerts(self) -> Dict[str, Dict[str, Any]]:
        """Return {external_id: {id, hash}} for existing alerts."""
        try:
            data = self._api_get("/api/v1/alerts")
            result: Dict[str, Dict[str, Any]] = {}
            for a in (data if isinstance(data, list) else data.get("data", [])):
                ext_id = a.get("externalId", "")
                if ext_id:
                    result[ext_id] = {
                        "id": a.get("_id", a.get("id", "")),
                        "hash": a.get("contentHash", ""),
                    }
            return result
        except Exception as exc:
            log.debug("Could not list existing alerts: %s", exc)
            return {}

    def _upsert_alert(
        self,
        alert: HyperDXAlert,
        existing: Dict[str, Dict],
    ) -> str:
        payload = alert.to_dict()
        ext = existing.get(alert.external_id)

        if ext and ext.get("hash") == alert.content_hash:
            return "unchanged"

        if self.dry_run:
            action = "update" if ext else "create"
            log.info("[DRY-RUN] Would %s alert: %s", action, alert.name)
            return "updated" if ext else "created"

        if ext:
            self._api_put(f"/api/v1/alerts/{ext['id']}", payload)
            log.info("Updated alert: %s", alert.name)
            return "updated"
        else:
            self._api_post("/api/v1/alerts", payload)
            log.info("Created alert: %s", alert.name)
            return "created"

    # ── HTTP helpers ──────────────────────────────────────────

    def _api_get(self, path: str) -> Any:
        return fetch_json(
            f"{self.base}{path}", token=self.api_key, bearer=True,
        )

    def _api_post(self, path: str, payload: Any) -> Any:
        return post_json(
            f"{self.base}{path}", payload, token=self.api_key, bearer=True,
        )

    def _api_put(self, path: str, payload: Any) -> Any:
        return put_json(
            f"{self.base}{path}", payload, token=self.api_key, bearer=True,
        )
