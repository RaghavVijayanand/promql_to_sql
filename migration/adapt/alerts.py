"""
HyperDX alert builder — converts normalised alerts + transpiled SQL
into HyperDX alert configuration JSON.

Maps:
  - Prometheus `for` duration → HyperDX evaluation window
  - Threshold operator + value → HyperDX condition
  - Alertmanager receiver → HyperDX notification channel
"""

import hashlib
import json
import logging
from typing import Any, Dict, List, Optional

from migration.analysis.alerts import NormalisedAlert
from migration.discovery.alertmanager import Receiver
from migration.transpile.dispatch import TranspileResult

log = logging.getLogger("migration.adapt.alerts")

# ─── Output ───────────────────────────────────────────────────


class HyperDXAlert:
    """One HyperDX alert rule configuration."""

    __slots__ = (
        "external_id", "name", "sql",
        "condition_column", "condition_op", "condition_threshold",
        "interval", "for_duration",
        "severity", "channel", "channel_config",
        "annotations", "content_hash",
        "_webhook_id", "_dashboard_id", "_tile_id",
    )

    def __init__(self, **kw):
        self.external_id: str = kw.get("external_id", "")
        self.name: str = kw.get("name", "")
        self.sql: str = kw.get("sql", "")
        self.condition_column: str = kw.get("condition_column", "value")
        self.condition_op: str = kw.get("condition_op", ">")
        self.condition_threshold: Optional[float] = kw.get("condition_threshold")
        self.interval: str = kw.get("interval", "1m")
        self.for_duration: str = kw.get("for_duration", "")
        self.severity: str = kw.get("severity", "warning")
        self.channel: str = kw.get("channel", "")
        self.channel_config: Dict[str, Any] = kw.get("channel_config", {})
        self.annotations: Dict[str, str] = kw.get("annotations", {})
        self.content_hash: str = ""
        self._webhook_id: str = ""
        self._dashboard_id: str = ""
        self._tile_id: str = ""

    def compute_hash(self) -> str:
        blob = json.dumps(self.to_dict(), sort_keys=True)
        self.content_hash = hashlib.sha256(blob.encode()).hexdigest()
        return self.content_hash

    def to_dict(self) -> Dict[str, Any]:
        """Build HyperDX-compatible alert payload.

        HyperDX requires alerts to be either ``saved_search`` or ``tile``
        based.  For metrics alerts migrated from Prometheus, we use the
        ``tile`` source which requires ``dashboardId`` and ``tileId`` to be
        injected by the deployer before calling this method.
        """
        # Map our operator to HyperDX thresholdType
        threshold_type = "above"
        if self.condition_op in ("<", "<="):
            threshold_type = "below"

        d: Dict[str, Any] = {
            "name": self.name,
            "source": "tile",
            "interval": self.interval,
            "threshold": self.condition_threshold if self.condition_threshold is not None else 1,
            "thresholdType": threshold_type,
        }

        # Channel must be {type: 'webhook', webhookId: '...'}
        if isinstance(self.channel_config, dict) and self.channel_config.get("webhookId"):
            d["channel"] = {
                "type": "webhook",
                "webhookId": self.channel_config["webhookId"],
            }
        elif hasattr(self, '_webhook_id') and self._webhook_id:
            d["channel"] = {
                "type": "webhook",
                "webhookId": self._webhook_id,
            }

        # Tile source fields (injected by deployer)
        if hasattr(self, '_dashboard_id') and self._dashboard_id:
            d["dashboardId"] = self._dashboard_id
        if hasattr(self, '_tile_id') and self._tile_id:
            d["tileId"] = self._tile_id

        # Message with migration context
        message_parts = [f"Migrated from Prometheus alert: {self.name}"]
        if self.annotations:
            if self.annotations.get("description"):
                message_parts.append(self.annotations["description"])
            elif self.annotations.get("summary"):
                message_parts.append(self.annotations["summary"])
        d["message"] = " | ".join(message_parts)

        return d


# ─── Builder ──────────────────────────────────────────────────


class AlertBuilder:
    """Builds HyperDX alert definitions from normalised alerts + SQL."""

    def build(
        self,
        alert: NormalisedAlert,
        transpiled: TranspileResult,
    ) -> Optional[HyperDXAlert]:
        """Build a single HyperDX alert. Returns None if SQL is missing."""
        if not transpiled.ok:
            log.warning(
                "Skipping alert %s — transpile error: %s",
                alert.name, transpiled.error,
            )
            return None

        channel, channel_config = self._map_receiver(
            alert.receiver_name, alert.receiver
        )

        ha = HyperDXAlert(
            external_id=f"prom_alert:{alert.id}",
            name=alert.name,
            sql=transpiled.sql,
            condition_op=alert.threshold_op or ">",
            condition_threshold=alert.threshold_value,
            interval=self._infer_interval(alert),
            for_duration=alert.for_duration,
            severity=alert.severity or "warning",
            channel=channel,
            channel_config=channel_config,
            annotations=alert.annotations,
        )
        ha.compute_hash()
        return ha

    def build_all(
        self,
        alerts: List[NormalisedAlert],
        transpile_results: Dict[str, TranspileResult],
    ) -> List[HyperDXAlert]:
        """Build alerts for all normalised alert rules."""
        result: List[HyperDXAlert] = []
        for a in alerts:
            key = f"alert:{a.id}"
            tr = transpile_results.get(key)
            if tr is None:
                continue
            ha = self.build(a, tr)
            if ha is not None:
                result.append(ha)
        log.info("Built %d HyperDX alert definitions", len(result))
        return result

    # ── internal ──────────────────────────────────────────────

    @staticmethod
    def _map_receiver(
        name: str,
        receiver: Optional[Receiver],
    ) -> tuple:
        """Map Alertmanager receiver → HyperDX channel type + config."""
        if receiver is None:
            return (name or "default", {})

        if receiver.email_configs:
            cfg = receiver.email_configs[0]
            return ("email", {
                "to": cfg.get("to", ""),
                "send_resolved": cfg.get("send_resolved", True),
            })

        if receiver.slack_configs:
            cfg = receiver.slack_configs[0]
            return ("slack", {
                "channel": cfg.get("channel", ""),
                "api_url": cfg.get("api_url", ""),
            })

        if receiver.webhook_configs:
            cfg = receiver.webhook_configs[0]
            return ("webhook", {
                "url": cfg.get("url", ""),
            })

        if receiver.pagerduty_configs:
            cfg = receiver.pagerduty_configs[0]
            return ("pagerduty", {
                "routing_key": cfg.get("routing_key", cfg.get("service_key", "")),
            })

        return (name or "default", {})

    @staticmethod
    def _infer_interval(alert: NormalisedAlert) -> str:
        """
        Infer a reasonable evaluation interval from the alert's FOR duration.
        Must return a valid HyperDX interval:
        '1m', '5m', '15m', '30m', '1h', '6h', '12h', '1d'
        """
        dur = alert.for_duration
        if not dur:
            return "5m"
        try:
            if dur.endswith("s"):
                secs = int(dur[:-1])
                return "1m" if secs <= 120 else "5m"
            if dur.endswith("m"):
                mins = int(dur[:-1])
                if mins <= 5:
                    return "1m"
                elif mins <= 15:
                    return "5m"
                elif mins <= 30:
                    return "15m"
                else:
                    return "30m"
            if dur.endswith("h"):
                return "1h"
            if dur.endswith("d"):
                return "6h"
        except ValueError:
            pass
        return "5m"
