# Prometheus API Parser

This transpiler uses Prometheus' `/api/v1/parse_query` endpoint to parse PromQL queries and works **directly with the JSON AST** returned by Prometheus.

## Why Direct JSON AST?

- **Accuracy**: Uses Prometheus' official parser
- **Zero Maintenance**: No custom parser or converter to maintain
- **Simplicity**: One less conversion layer
- **Trust**: Guaranteed compatibility with Prometheus
- **Performance**: Skips AST conversion overhead

## How It Works

**Simplified Architecture:**

```
PromQL Query → Prometheus API → JSON AST → Transpiler → ClickHouse SQL
```

1. **API Call**: Send PromQL to Prometheus `/api/v1/parse_query`
2. **JSON AST**: Prometheus returns parsed query as JSON structure
3. **Direct Transpilation**: Transpiler works directly with JSON nodes
4. **SQL Generation**: Generates optimized ClickHouse SQL

**No intermediate AST conversion** - the transpiler operates on Prometheus' JSON format directly.

## Usage

### Configuration

Simply provide the Prometheus URL when creating the transpiler:

```go
config := &transpiler.Config{
    Schema:        clickhouse.DefaultSchema(),
    PrometheusURL: "http://localhost:9090", // Default if not specified
}
trans := transpiler.New(config)
```

### Example

```go
package main

import (
    "fmt"
    "time"
    
    "github.com/shinro/promql-transpiler/pkg/clickhouse"
    "github.com/shinro/promql-transpiler/pkg/transpiler"
)

func main() {
    config := &transpiler.Config{
        Schema:        clickhouse.DefaultSchema(),
        PrometheusURL: "http://localhost:9090",
    }
    
    trans := transpiler.New(config)
    trans.SetTimeRange(time.Now().Add(-1*time.Hour), time.Now(), time.Minute)
    
    sql, err := trans.Transpile("rate(http_requests_total[5m])")
    if err != nil {
        panic(err)
    }
    
    fmt.Println(sql)
}
```

## Architecture

### Packages

#### `pkg/promapi/client.go`
- HTTP client for Prometheus API
- JSON structures for API responses
- Handles POST requests with form-encoded bodies
- Returns `*ASTNode` (JSON structure)

#### `pkg/promapi/parser.go`
- High-level parser interface
- Wraps the client
- Provides `Parse(query string) (*ASTNode, error)` method

#### `pkg/transpiler/transpiler_jsonast.go`
- Transpiles JSON AST nodes directly to SQL
- Node type dispatch: `vectorSelector`, `matrixSelector`, `call`, `aggregation`, `binaryExpr`, etc.
- No conversion layer - works with Prometheus JSON format

### Modified Packages

#### `pkg/transpiler/transpiler.go`
- Uses `apiParser` to get JSON AST
- Calls `transpileNode()` with `*promapi.ASTNode`
- Removed internal AST conversion

## JSON AST Node Types

The transpiler works directly with these Prometheus JSON node types:

| Prometheus Type | Description | Example |
|----------------|-------------|---------|
| `vectorSelector` | Instant vector selector | `up`, `http_requests{job="api"}` |
| `matrixSelector` | Range vector selector | `requests[5m]` |
| `numberLiteral` | Numeric constant | `100`, `0.95` |
| `binaryExpr` | Binary operations | `+`, `-`, `*`, `/`, `==`, `>` |
| `call` | Function calls | `rate()`, `sum_over_time()` |
| `aggregation` | Aggregations | `sum`, `avg`, `max` with `by`/`without` |
| `unaryExpr` | Unary operations | `+`, `-` |
| `parenExpr` | Parenthesized expressions | `(...)` |

Each node type is transpiled directly to ClickHouse SQL without intermediate representation.

## Performance Considerations

- **Network overhead**: Each query requires an HTTP request to Prometheus (~5-20ms)
- **Caching recommendation**: Implement query caching for frequently used queries
- **Local Prometheus**: Run Prometheus locally to minimize latency (~1ms vs 10-50ms remote)

## Testing

Run the example:

```bash
# Make sure Prometheus is running on localhost:9090
go run examples/api_parser_example.go
```

## Requirements

- **Prometheus instance** must be running and accessible
- Default URL: `http://localhost:9090`
- Can be overridden via `PrometheusURL` config

## Future Improvements

1. Query caching to reduce API calls
2. Connection pooling for better performance
3. Fallback to cached parser for common queries
4. Metrics for API call success/failure rates
