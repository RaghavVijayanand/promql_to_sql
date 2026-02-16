# Integration Test Suite

This directory contains integration tests for the PromQL transpiler.

## Running Integration Tests

```bash
cd transpiler
go test ./test/... -v
```

## Test Categories

1. **End-to-End Tests**: Full transpilation workflow
2. **ClickHouse Integration**: Tests against actual ClickHouse instance
3. **Performance Tests**: Benchmark various query patterns
4. **Cardinality Tests**: High-cardinality scenario testing

## Prerequisites for Integration Tests

- Running ClickHouse instance (localhost:9000)
- Test data loaded in ClickHouse

## Setup Test Environment

```bash
# Start ClickHouse with Docker
docker run -d --name clickhouse-test -p 9000:9000 clickhouse/clickhouse-server

# Load test schema
cat ../docs/SCHEMA.md | grep -A 100 "CREATE TABLE" | docker exec -i clickhouse-test clickhouse-client

# Load test data
docker exec -i clickhouse-test clickhouse-client < test_data.sql
```

## Writing Integration Tests

Example test structure:

```go
func TestIntegration_BasicQuery(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }
    
    // Test implementation
}
```

## Running Specific Tests

```bash
# Run only integration tests
go test ./test/... -v

# Skip integration tests
go test ./... -short

# Run specific test
go test ./test/... -run TestIntegration_BasicQuery -v
```
