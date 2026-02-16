# migration/check_health_standalone.py
"""Standalone connection checker - no migration imports needed."""
import os
import sys
import requests
from clickhouse_driver import Client as CHClient
from dotenv import load_dotenv

# Load .env
load_dotenv()

def get_env(key, default=""):
    return os.getenv(key, default)

PROMETHEUS_URL = get_env("PROMETHEUS_URL", "http://localhost:9090")
GRAFANA_URL = get_env("GRAFANA_URL", "http://localhost:3000")
GRAFANA_API_TOKEN = get_env("GRAFANA_API_TOKEN", "")
ALERTMANAGER_URL = get_env("ALERTMANAGER_URL", "http://localhost:9093")
CLICKHOUSE_HOST = get_env("CLICKHOUSE_HOST", "localhost")
CLICKHOUSE_USER = get_env("CLICKHOUSE_USER", "default")
CLICKHOUSE_PASSWORD = get_env("CLICKHOUSE_PASSWORD", "")
HYPERDX_URL = get_env("HYPERDX_URL", "http://localhost:8080")
HYPERDX_API_KEY = get_env("HYPERDX_API_KEY", "")

def check_prometheus():
    try:
        resp = requests.get(f"{PROMETHEUS_URL}/api/v1/status/buildinfo", timeout=5)
        resp.raise_for_status()
        data = resp.json()
        print("✓ Prometheus API:", data.get('data', {}).get('version', 'OK'))
        return True
    except Exception as e:
        print(f"✗ Prometheus: {e}")
        return False

def check_grafana():
    try:
        headers = {}
        if GRAFANA_API_TOKEN:
            headers["Authorization"] = f"Bearer {GRAFANA_API_TOKEN}"
        resp = requests.get(f"{GRAFANA_URL}/api/health", headers=headers, timeout=5)
        resp.raise_for_status()
        data = resp.json()
        print(f"✓ Grafana API: {data.get('database', 'OK')}")
        return True
    except Exception as e:
        print(f"✗ Grafana: {e}")
        return False

def check_alertmanager():
    try:
        resp = requests.get(f"{ALERTMANAGER_URL}/api/v2/status", timeout=5)
        resp.raise_for_status()
        data = resp.json()
        print(f"✓ Alertmanager API: {data.get('cluster', {}).get('status', 'OK')}")
        return True
    except Exception as e:
        print(f"✗ Alertmanager: {e}")
        return False

def check_clickhouse():
    try:
        ch = CHClient(
            host=CLICKHOUSE_HOST,
            port=9000,
            user=CLICKHOUSE_USER,
            password=CLICKHOUSE_PASSWORD
        )
        result = ch.execute("SELECT 1")
        assert result == [(1,)]
        
        # Check for OTel tables
        tables = ch.execute("""
            SELECT name FROM system.tables 
            WHERE database IN ('default', 'otel') AND name LIKE 'otel_%'
        """)
        print(f"✓ ClickHouse: {len(tables)} OTel tables found")
        if len(tables) == 0:
            print("  ⚠ Warning: No OTel tables yet (normal if just started)")
        return True
    except Exception as e:
        print(f"✗ ClickHouse: {e}")
        return False

def check_hyperdx():
    try:
        # HyperDX health endpoint - simpler and more reliable than /api/v2/me
        resp = requests.get(f"{HYPERDX_URL}/api/health", timeout=5)
        resp.raise_for_status()
        data = resp.json()
        status = data.get('status', 'unknown')
        print(f"✓ HyperDX API: {status}")
        return status == 'ok'
    except Exception as e:
        print(f"✗ HyperDX: {e}")
        return False

if __name__ == "__main__":
    print("=== GAP → ClickStack Connection Health Check ===\n")
    
    checks = [
        ("Prometheus", check_prometheus),
        ("Grafana", check_grafana),
        ("Alertmanager", check_alertmanager),
        ("ClickHouse", check_clickhouse),
        ("HyperDX", check_hyperdx),
    ]
    
    results = {}
    for name, fn in checks:
        print(f"[{name}]", end=" ")
        results[name] = fn()
        print()
    
    print("="*50)
    passed = sum(results.values())
    total = len(results)
    
    if passed == total:
        print(f"✓ All {total} checks passed - ready for migration!")
        sys.exit(0)
    else:
        print(f"⚠ {passed}/{total} checks passed - fix failures before migrating")
        sys.exit(1)