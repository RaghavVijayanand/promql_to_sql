"""
GAP Stack Raw File Extractor
Pulls raw config and data files directly from running Grafana, Alertmanager,
and Prometheus instances. No local file copying, no path heuristics.
"""

import os
import json
import shutil
import requests

# ---------------------------------------------------------------------------
# Configuration — edit these to match your environment
# ---------------------------------------------------------------------------
PROMETHEUS_URL   = "http://localhost:9090"
ALERTMANAGER_URL = "http://localhost:9093"
GRAFANA_URL      = "http://localhost:3000"

# Grafana auth — use one or the other, token takes priority if both are set.
GRAFANA_TOKEN    = "glsa_xxxxxxxxxxxxxxxxx"          # Service account token or API key


OUTPUT_DIR = "gap_data"
# ---------------------------------------------------------------------------


def _mkdir(path: str) -> None:
    os.makedirs(path, exist_ok=True)


def _save_json(data: dict, path: str) -> None:
    with open(path, "w") as f:
        json.dump(data, f, indent=2)
    print(f"  saved {path}")


def _save_text(text: str, path: str) -> None:
    with open(path, "w") as f:
        f.write(text)
    print(f"  saved {path}")


def _get_json(url: str, auth=None, headers: dict | None = None) -> dict | None:
    try:
        r = requests.get(url, auth=auth, headers=headers, timeout=10)
        r.raise_for_status()
        return r.json()
    except requests.RequestException as e:
        print(f"  ERROR {url}: {e}")
        return None


def _grafana_auth() -> tuple[dict, tuple | None]:
    """
    Returns (headers, auth) for Grafana requests.
    Bearer token is used when GRAFANA_TOKEN is set; falls back to basic auth.
    """
    if GRAFANA_TOKEN:
        return {"Authorization": f"Bearer {GRAFANA_TOKEN}"}, None
    return {}, (GRAFANA_USER, GRAFANA_PASSWORD)


# ---------------------------------------------------------------------------
# Prometheus
# ---------------------------------------------------------------------------

def extract_prometheus() -> None:
    print("\n=== Prometheus ===")
    base = os.path.join(OUTPUT_DIR, "prometheus")
    _mkdir(base)

    # Raw YAML config
    data = _get_json(f"{PROMETHEUS_URL}/api/v1/status/config")
    if data:
        _save_json(data, os.path.join(base, "config.json"))
        yaml = data.get("data", {}).get("yaml", "")
        if yaml:
            _save_text(yaml, os.path.join(base, "prometheus.yml"))

    # Alerting / recording rules
    data = _get_json(f"{PROMETHEUS_URL}/api/v1/rules")
    if data:
        _save_json(data, os.path.join(base, "rules.json"))

    # Scrape targets and their state
    data = _get_json(f"{PROMETHEUS_URL}/api/v1/targets")
    if data:
        _save_json(data, os.path.join(base, "targets.json"))

    # Metric metadata
    data = _get_json(f"{PROMETHEUS_URL}/api/v1/metadata")
    if data:
        _save_json(data, os.path.join(base, "metadata.json"))

    # TSDB cardinality / storage stats
    data = _get_json(f"{PROMETHEUS_URL}/api/v1/status/tsdb")
    if data:
        _save_json(data, os.path.join(base, "tsdb_stats.json"))


# ---------------------------------------------------------------------------
# Alertmanager
# ---------------------------------------------------------------------------

def extract_alertmanager() -> None:
    print("\n=== Alertmanager ===")
    base = os.path.join(OUTPUT_DIR, "alertmanager")
    _mkdir(base)

    # Status + raw YAML config
    data = _get_json(f"{ALERTMANAGER_URL}/api/v2/status")
    if data:
        _save_json(data, os.path.join(base, "status.json"))
        yaml = data.get("config", {}).get("original", "")
        if yaml:
            _save_text(yaml, os.path.join(base, "alertmanager.yml"))

    # Active alerts
    data = _get_json(f"{ALERTMANAGER_URL}/api/v2/alerts")
    if data is not None:
        _save_json(data, os.path.join(base, "alerts.json"))

    # Configured receivers
    data = _get_json(f"{ALERTMANAGER_URL}/api/v2/receivers")
    if data is not None:
        _save_json(data, os.path.join(base, "receivers.json"))

    # Active silences
    data = _get_json(f"{ALERTMANAGER_URL}/api/v2/silences")
    if data is not None:
        _save_json(data, os.path.join(base, "silences.json"))


# ---------------------------------------------------------------------------
# Grafana
# ---------------------------------------------------------------------------

def extract_grafana() -> None:
    print("\n=== Grafana ===")
    base = os.path.join(OUTPUT_DIR, "grafana")
    _mkdir(base)
    _mkdir(os.path.join(base, "dashboards"))

    headers, auth = _grafana_auth()

    # Verify credentials
    r = requests.get(f"{GRAFANA_URL}/api/user", auth=auth, headers=headers, timeout=10)
    if r.status_code == 401:
        hint = "token" if GRAFANA_TOKEN else f"user '{GRAFANA_USER}'"
        print(f"  ERROR: authentication failed for {hint}")
        return

    # Datasources
    data = _get_json(f"{GRAFANA_URL}/api/datasources", auth=auth, headers=headers)
    if data is not None:
        _save_json(data, os.path.join(base, "datasources.json"))

    # Installed + enabled plugins
    data = _get_json(f"{GRAFANA_URL}/api/plugins?enabled=1", auth=auth, headers=headers)
    if data is not None:
        _save_json(data, os.path.join(base, "plugins.json"))

    # Dashboard list, then fetch each full definition
    dashboard_list = _get_json(f"{GRAFANA_URL}/api/search?type=dash-db", auth=auth, headers=headers)
    if not dashboard_list:
        return

    print(f"  fetching {len(dashboard_list)} dashboards...")
    for entry in dashboard_list:
        uid   = entry.get("uid")
        title = entry.get("title", "untitled")
        if not uid:
            continue
        data = _get_json(f"{GRAFANA_URL}/api/dashboards/uid/{uid}", auth=auth, headers=headers)
        if data:
            safe_title = "".join(c for c in title if c.isalnum() or c in " _-").strip()
            filename   = f"{safe_title}_{uid}.json"
            _save_json(data, os.path.join(base, "dashboards", filename))


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    _mkdir(OUTPUT_DIR)

    extract_prometheus()
    extract_alertmanager()
    extract_grafana()

    archive = "gap_data_export"
    shutil.make_archive(archive, "zip", OUTPUT_DIR)
    print(f"\nDone. Archive: {os.path.abspath(archive + '.zip')}")