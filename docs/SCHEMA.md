# ClickHouse Schema Setup

This document describes the ClickHouse schema required for the PromQL transpiler.

## Required Tables

### 1. Metrics Table

This is the main table that stores time-series metric data.

```sql
CREATE TABLE metrics (
    metric_name String,
    labels Map(String, String),
    timestamp DateTime64(3),
    value Float64,
    INDEX metric_idx metric_name TYPE bloom_filter GRANULARITY 1,
    INDEX labels_idx mapKeys(labels) TYPE bloom_filter GRANULARITY 1
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp)
SETTINGS index_granularity = 8192;
```

**Column Descriptions:**
- `metric_name`: The name of the metric (e.g., "http_requests_total")
- `labels`: A map containing label key-value pairs (e.g., {"job": "api", "status": "200"})
- `timestamp`: The timestamp of the measurement with millisecond precision
- `value`: The numeric value of the metric

**Optimizations:**
- Partitioned by month for efficient data management
- Ordered by metric name and timestamp for fast queries
- Bloom filter indexes on metric name and label keys for faster filtering

### 2. Cardinality Tracking Table

This table tracks the cardinality of labels for query optimization.

```sql
CREATE TABLE metrics_cardinality (
    metric_name String,
    label_key String,
    label_value String,
    cardinality UInt64
) ENGINE = SummingMergeTree()
ORDER BY (metric_name, label_key, label_value)
SETTINGS index_granularity = 8192;
```

**Column Descriptions:**
- `metric_name`: The metric name
- `label_key`: The label key (e.g., "status")
- `label_value`: The label value (e.g., "200")
- `cardinality`: The count of unique time series with this combination

**Purpose:**
- Helps estimate query cardinality before execution
- Enables intelligent sampling for high-cardinality queries
- Supports query optimization decisions

## Sample Data Insertion

### Insert Metric Data

```sql
INSERT INTO metrics (metric_name, labels, timestamp, value) VALUES
('http_requests_total', {'job': 'api', 'status': '200'}, now(), 1500),
('http_requests_total', {'job': 'api', 'status': '404'}, now(), 42),
('http_requests_total', {'job': 'web', 'status': '200'}, now(), 3200),
('cpu_usage', {'host': 'server1', 'cpu': '0'}, now(), 45.2),
('cpu_usage', {'host': 'server1', 'cpu': '1'}, now(), 38.7);
```

### Insert Cardinality Data

```sql
INSERT INTO metrics_cardinality (metric_name, label_key, label_value, cardinality) VALUES
('http_requests_total', 'job', 'api', 100),
('http_requests_total', 'job', 'web', 80),
('http_requests_total', 'status', '200', 150),
('http_requests_total', 'status', '404', 30),
('cpu_usage', 'host', 'server1', 200),
('cpu_usage', 'cpu', '0', 100);
```

## Alternative Schemas

### Time-Series Optimized Schema

For ultra-high performance with time-series data:

```sql
CREATE TABLE metrics_timeseries (
    metric_name LowCardinality(String),
    labels_hash UInt64,
    labels Map(String, String),
    timestamp DateTime64(3),
    value Float64,
    PROJECTION timeseries_projection (
        SELECT 
            metric_name,
            labels_hash,
            timestamp,
            value
        ORDER BY (labels_hash, timestamp)
    )
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp)
SETTINGS index_granularity = 8192;
```

### Separate Table Per Metric Type

For extremely high-volume metrics, consider separating by type:

```sql
-- For counter metrics
CREATE TABLE metrics_counter (
    metric_name String,
    labels Map(String, String),
    timestamp DateTime64(3),
    value UInt64,  -- Counters are always positive integers
    reset_count UInt32  -- Track counter resets
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp);

-- For gauge metrics
CREATE TABLE metrics_gauge (
    metric_name String,
    labels Map(String, String),
    timestamp DateTime64(3),
    value Float64
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp);

-- For histogram metrics
CREATE TABLE metrics_histogram (
    metric_name String,
    labels Map(String, String),
    timestamp DateTime64(3),
    buckets Array(Float64),
    counts Array(UInt64)
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(timestamp)
ORDER BY (metric_name, timestamp);
```

## Materialized Views for Pre-Aggregation

For frequently queried aggregations, create materialized views:

```sql
-- Pre-aggregated hourly rates
CREATE MATERIALIZED VIEW metrics_hourly_rate
ENGINE = SummingMergeTree()
PARTITION BY toYYYYMM(hour)
ORDER BY (metric_name, labels, hour)
AS SELECT
    metric_name,
    labels,
    toStartOfHour(timestamp) AS hour,
    sum(value) AS total_value,
    count() AS count
FROM metrics
WHERE metric_name LIKE '%_total'  -- Typically counters
GROUP BY metric_name, labels, hour;
```

## Data Retention Policy

Set up TTL for automatic data cleanup:

```sql
ALTER TABLE metrics
MODIFY TTL timestamp + INTERVAL 90 DAY;  -- Keep 90 days of raw data

ALTER TABLE metrics_cardinality
MODIFY TTL toDateTime(0) + INTERVAL 365 DAY;  -- Keep cardinality data for 1 year
```

## Query Examples

### Query Metrics

```sql
-- Get latest values for a metric
SELECT 
    timestamp,
    labels,
    value
FROM metrics
WHERE metric_name = 'http_requests_total'
  AND timestamp >= now() - INTERVAL 1 HOUR
ORDER BY timestamp DESC;

-- Get rate calculation
SELECT 
    timestamp,
    labels,
    (value - lagInFrame(value) OVER w) / 
    (toUnixTimestamp64Milli(timestamp) - toUnixTimestamp64Milli(lagInFrame(timestamp) OVER w)) * 1000 AS rate
FROM metrics
WHERE metric_name = 'http_requests_total'
  AND timestamp >= now() - INTERVAL 5 MINUTE
WINDOW w AS (PARTITION BY labels ORDER BY timestamp);
```

### Query Cardinality

```sql
-- Check cardinality for a metric
SELECT 
    label_key,
    sum(cardinality) AS total_cardinality
FROM metrics_cardinality
WHERE metric_name = 'http_requests_total'
GROUP BY label_key
ORDER BY total_cardinality DESC;
```

## Performance Tips

1. **Use LowCardinality** for columns with limited distinct values
2. **Partition wisely** - monthly partitions work well for most use cases
3. **Create appropriate indexes** - bloom filters for high-cardinality columns
4. **Use projections** for common query patterns
5. **Pre-aggregate** frequently queried data using materialized views
6. **Monitor cardinality** to prevent query explosion
7. **Set appropriate TTL** to manage storage costs

## Troubleshooting

### High Memory Usage

If queries are using too much memory:
- Reduce time range
- Add more specific label filters
- Use sampling: `SAMPLE 0.1` for 10% sampling
- Increase aggregation before filtering

### Slow Queries

If queries are slow:
- Check if indexes are being used: `EXPLAIN` query
- Verify partition pruning is working
- Consider creating projections for specific query patterns
- Pre-aggregate data using materialized views

### High Cardinality Issues

If experiencing high cardinality:
- Review label usage - avoid high-cardinality labels like user IDs
- Use the cardinality tracking table
- Implement label pruning
- Consider sampling strategies
