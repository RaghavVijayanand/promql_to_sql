# Quick Start Guide

Get started with the PromQL to ClickHouse SQL Transpiler in minutes!

## Installation

### Option 1: Build from Source

```bash
cd transpiler
make deps
make build
```

The binary will be available at `bin/promql-transpiler`.

### Option 2: Using Go Install

```bash
go install github.com/shinro/promql-transpiler/cmd/promql-transpiler@latest
```

### Option 3: Using Docker

```bash
cd transpiler
docker build -t promql-transpiler .
docker run promql-transpiler -q "up"
```

## Basic Usage

### Command Line

```bash
# Simple query
promql-transpiler -q "http_requests_total"

# Query with time range
promql-transpiler -q "rate(http_requests_total[5m])" \
  -s "2024-01-01T00:00:00Z" \
  -e "2024-01-01T23:59:59Z"

# Save to file
promql-transpiler -q "sum(http_requests_total) by (status)" -o query.sql

# Verbose output
promql-transpiler -q "up" -v
```

### As a Library

Create a new Go file:

```go
package main

import (
    "fmt"
    "log"
    
    "github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
    // Create transpiler
    t := transpiler.New(nil)
    
    // Transpile a query
    sql, err := t.Transpile(`rate(http_requests_total[5m])`)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(sql)
}
```

## Common Query Examples

### 1. Basic Metric Selection

**PromQL:**
```promql
http_requests_total
```

**Command:**
```bash
promql-transpiler -q "http_requests_total"
```

### 2. With Label Filters

**PromQL:**
```promql
http_requests_total{job="api", status="200"}
```

**Command:**
```bash
promql-transpiler -q 'http_requests_total{job="api", status="200"}'
```

### 3. Rate Calculation

**PromQL:**
```promql
rate(http_requests_total[5m])
```

**Command:**
```bash
promql-transpiler -q "rate(http_requests_total[5m])"
```

### 4. Aggregation

**PromQL:**
```promql
sum(http_requests_total) by (status)
```

**Command:**
```bash
promql-transpiler -q "sum(http_requests_total) by (status)"
```

### 5. Complex Query

**PromQL:**
```promql
sum(rate(http_requests_total{job="api"}[5m])) by (status)
```

**Command:**
```bash
promql-transpiler -q 'sum(rate(http_requests_total{job="api"}[5m])) by (status)'
```

### 6. Mathematical Operations

**PromQL:**
```promql
(http_requests_total - http_request_errors_total) / http_requests_total * 100
```

**Command:**
```bash
promql-transpiler -q "(http_requests_total - http_request_errors_total) / http_requests_total * 100"
```

## Setting Up ClickHouse

### 1. Start ClickHouse

Using Docker:

```bash
docker run -d \
  --name clickhouse \
  -p 8123:8123 \
  -p 9000:9000 \
  clickhouse/clickhouse-server
```

### 2. Create Schema

```bash
# Connect to ClickHouse
docker exec -it clickhouse clickhouse-client

# Create metrics table
CREATE TABLE metrics (
    metric_name String,
    labels Map(String, String),
    timestamp DateTime64(3),
    value Float64
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp);

# Create cardinality table
CREATE TABLE metrics_cardinality (
    metric_name String,
    label_key String,
    label_value String,
    cardinality UInt64
) ENGINE = SummingMergeTree()
ORDER BY (metric_name, label_key, label_value);
```

### 3. Insert Sample Data

```sql
INSERT INTO metrics (metric_name, labels, timestamp, value) VALUES
('http_requests_total', {'job': 'api', 'status': '200', 'method': 'GET'}, now() - INTERVAL 5 MINUTE, 1000),
('http_requests_total', {'job': 'api', 'status': '200', 'method': 'GET'}, now() - INTERVAL 4 MINUTE, 1050),
('http_requests_total', {'job': 'api', 'status': '200', 'method': 'GET'}, now() - INTERVAL 3 MINUTE, 1100),
('http_requests_total', {'job': 'api', 'status': '404', 'method': 'GET'}, now() - INTERVAL 5 MINUTE, 10),
('http_requests_total', {'job': 'api', 'status': '404', 'method': 'GET'}, now() - INTERVAL 4 MINUTE, 12),
('http_requests_total', {'job': 'web', 'status': '200', 'method': 'POST'}, now() - INTERVAL 5 MINUTE, 500);
```

## Running Your First Query

### 1. Generate SQL

```bash
promql-transpiler -q "rate(http_requests_total[5m])" -o query.sql
```

### 2. Execute in ClickHouse

```bash
# Copy the SQL and execute
docker exec -it clickhouse clickhouse-client

# Or pipe directly
cat query.sql | docker exec -i clickhouse clickhouse-client
```

### 3. View Results

The results will show the rate of change for your metrics!

## Testing the Examples

Run the provided example programs:

```bash
# Basic usage examples
go run examples/basic_usage.go

# Custom schema example
go run examples/custom_schema.go

# Cardinality estimation examples
go run examples/cardinality_examples.go
```

## Next Steps

1. **Read the full documentation**: Check out `README.md` for comprehensive features
2. **Explore the schema**: See `docs/SCHEMA.md` for advanced schema configurations
3. **Development**: Read `docs/DEVELOPMENT.md` if you want to contribute
4. **Custom configuration**: Learn about custom schemas and optimization settings

## Troubleshooting

### Issue: "command not found: promql-transpiler"

Make sure the binary is in your PATH:

```bash
export PATH=$PATH:$(pwd)/bin
```

Or use the full path:

```bash
./bin/promql-transpiler -q "up"
```

### Issue: Parse errors

Check your PromQL syntax. Use verbose mode for details:

```bash
promql-transpiler -q "your_query" -v
```

### Issue: Generated SQL doesn't work in ClickHouse

1. Verify your ClickHouse schema matches the expected structure
2. Check that you have data in the time range you're querying
3. Try a simpler query first to isolate the issue

## Support

For issues, questions, or contributions:
- Check the documentation in the `docs/` folder
- Review the examples in `examples/`
- Run the tests to ensure everything works: `make test`

## Quick Reference

| PromQL Pattern | Command |
|---------------|---------|
| Simple metric | `promql-transpiler -q "metric_name"` |
| With labels | `promql-transpiler -q 'metric{label="value"}'` |
| Rate | `promql-transpiler -q "rate(metric[5m])"` |
| Aggregation | `promql-transpiler -q "sum(metric) by (label)"` |
| Time range | Add `-s START -e END` |
| Save to file | Add `-o output.sql` |
| Verbose | Add `-v` |

Happy querying! 🚀
