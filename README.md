# PromQL to ClickHouse SQL Transpiler

A high-performance transpiler that converts Prometheus Query Language (PromQL) queries into ClickHouse SQL, with full support for cardinality handling, time-series operations, and metric aggregations.

## Features

- **Complete PromQL Support**: Handles metric selectors, range vectors, instant vectors, and scalar operations
- **Cardinality Management**: Intelligent cardinality estimation and optimization for high-performance queries
- **Time-Series Functions**: Full support for rate(), increase(), delta(), and other PromQL functions
- **Aggregation Operations**: Sum, avg, max, min, count, topk, bottomk, and more
- **Label Matching**: Supports all PromQL label matching operators (=, !=, =~, !~)
- **Subquery Support**: Handles nested queries and complex PromQL expressions
- **Optimized SQL Generation**: Generates efficient ClickHouse SQL with proper indexing hints

## Architecture

```
transpiler/
├── cmd/promql-transpiler/    # CLI application
├── pkg/
│   ├── promapi/             # Prometheus API client for parsing
│   ├── transpiler/          # Core transpilation logic
│   └── clickhouse/          # ClickHouse schema and query builder
├── internal/
│   └── cardinality/         # Cardinality estimation and handling
├── test/                    # Integration tests
└── examples/                # Example usage

```

## Installation

```bash
# Clone and build
cd transpiler
make deps
make build

# Install globally
make install
```

## Usage

### Command Line

```bash
# Transpile a PromQL query
promql-transpiler -query 'rate(http_requests_total[5m])'


# Specify time range
promql-transpiler -query 'sum(rate(http_requests_total[5m])) by (status)' -start 2024-01-01T00:00:00Z -end 2024-01-01T23:59:59Z

# Output to file
promql-transpiler -query 'up' -output query.sql
```

### As a Library

```go
package main

import (
    "fmt"
    "github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
    t := transpiler.New()
    
    promql := `rate(http_requests_total{job="api"}[5m])`
    sql, err := t.Transpile(promql)
    if err != nil {
        panic(err)
    }
    
    fmt.Println(sql)
}
```

## ClickHouse Schema

The transpiler expects a ClickHouse schema designed for time-series metrics:

```sql
CREATE TABLE metrics (
    metric_name String,
    labels Map(String, String),
    timestamp DateTime64(3),
    value Float64
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp);

-- For better cardinality handling
CREATE TABLE metrics_cardinality (
    metric_name String,
    label_key String,
    label_value String,
    cardinality UInt64
) ENGINE = SummingMergeTree()
ORDER BY (metric_name, label_key, label_value);
```

## Examples

### Basic Metric Selection

**PromQL:**
```promql
http_requests_total
```

**ClickHouse SQL:**
```sql
SELECT timestamp, value, labels
FROM metrics
WHERE metric_name = 'http_requests_total'
```

### Rate Calculation

**PromQL:**
```promql
rate(http_requests_total[5m])
```

**ClickHouse SQL:**
```sql
SELECT 
    timestamp,
    labels,
    (value - lagInFrame(value) OVER w) / 
    (toUnixTimestamp64Milli(timestamp) - toUnixTimestamp64Milli(lagInFrame(timestamp) OVER w)) AS rate_value
FROM metrics
WHERE metric_name = 'http_requests_total'
  AND timestamp >= now() - INTERVAL 5 MINUTE
WINDOW w AS (PARTITION BY labels ORDER BY timestamp)
```

### Aggregation with Grouping

**PromQL:**
```promql
sum(rate(http_requests_total[5m])) by (status)
```

**ClickHouse SQL:**
```sql
WITH rates AS (
    SELECT 
        labels['status'] AS status,
        timestamp,
        (value - lagInFrame(value) OVER w) / 
        (toUnixTimestamp64Milli(timestamp) - toUnixTimestamp64Milli(lagInFrame(timestamp) OVER w)) AS rate_value
    FROM metrics
    WHERE metric_name = 'http_requests_total'
      AND timestamp >= now() - INTERVAL 5 MINUTE
    WINDOW w AS (PARTITION BY labels ORDER BY timestamp)
)
SELECT 
    status,
    timestamp,
    sum(rate_value) AS value
FROM rates
GROUP BY status, timestamp
ORDER BY timestamp
```

## Cardinality Handling

The transpiler implements intelligent cardinality management to prevent query explosions:

1. **Pre-query Analysis**: Estimates result cardinality before execution
2. **Automatic Sampling**: Switches to sampling for high-cardinality queries
3. **Label Pruning**: Removes unnecessary labels to reduce cardinality
4. **Aggregation Hints**: Suggests optimal aggregation strategies

## Testing

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run benchmarks
make benchmark
```

## Performance

- Handles queries with millions of time-series points
- Optimized for ClickHouse's columnar storage
- Supports streaming results for large datasets
- Intelligent query planning based on cardinality estimates

## Supported PromQL Features

### Metrics & Selectors
- [x] Instant vectors
- [x] Range vectors
- [x] Label matchers (=, !=, =~, !~)
- [x] Offset modifier

### Aggregation Operators
- [x] sum, min, max, avg, stddev, stdvar
- [x] count, count_values
- [x] bottomk, topk
- [x] quantile

### Functions
- [x] rate(), irate()
- [x] increase(), delta(), idelta()
- [x] histogram_quantile()
- [x] abs(), ceil(), floor(), round()
- [x] clamp_max(), clamp_min()
- [x] changes(), resets()

### Binary Operators
- [x] Arithmetic: +, -, *, /, %, ^
- [x] Comparison: ==, !=, >, <, >=, <=
- [x] Logical: and, or, unless
- [x] Vector matching (on, ignoring)

## License

MIT License - see LICENSE file for details

## Contributing

Contributions welcome! Please read CONTRIBUTING.md for guidelines.
