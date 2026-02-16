# PromQL to ClickHouse SQL Transpiler

A comprehensive, production-ready transpiler that converts Prometheus Query Language (PromQL) to ClickHouse SQL, with full support for Prometheus, Alertmanager, and Grafana.

## 🏗️ Architecture & Design Patterns

This project follows **SOLID principles** and implements multiple **design patterns** for maintainability, extensibility, and clean code.

### Design Patterns Implemented

#### 1. **Strategy Pattern** (`pkg/strategy/`)
Different transpilation strategies for various PromQL contexts:
- **TranspilationStrategy**: Main interface for transpilation
- **FunctionStrategy**: Handles PromQL functions
- **AggregationStrategy**: Handles aggregation operations
- **BinaryOpStrategy**: Handles binary operations

```go
type TranspilationStrategy interface {
    Transpile(expr ast.Expr, ctx *Context) (string, error)
    CanHandle(expr ast.Expr) bool
    GetPriority() int
}
```

#### 2. **Builder Pattern** (`pkg/builder/`)
Fluent SQL query construction:

```go
sql := builder.NewClickHouseSQLBuilder().
    Select("value", "timestamp").
    From("metrics").
    Where("timestamp >= 1234567890").
    GroupBy("labels").
    OrderBy("timestamp").
    Limit(1000).
    Build()
```

Features:
- Fluent interface for readability
- ClickHouse-specific optimizations (PREWHERE, SAMPLE)
- CTE (Common Table Expression) support
- Window functions
- Method chaining

#### 3. **Visitor Pattern** (`pkg/visitor/`)
Clean AST traversal and SQL generation:

```go
type Visitor interface {
    VisitVectorSelector(vs *ast.VectorSelector) (string, error)
    VisitMatrixSelector(ms *ast.MatrixSelector) (string, error)
    VisitAggregateExpr(ae *ast.AggregateExpr) (string, error)
    VisitCall(call *ast.Call) (string, error)
    VisitBinaryExpr(be *ast.BinaryExpr) (string, error)
}
```

Benefits:
- Separation of concerns (AST structure vs. SQL generation)
- Easy to add new SQL generation strategies
- Context tracking during traversal

#### 4. **Factory Pattern** (`pkg/factory/`)
Creates query components and handlers:

```go
factory := NewClickHouseComponentFactory()
handler, _ := factory.CreateFunctionHandler("rate")
aggHandler, _ := factory.CreateAggregationHandler("sum")
sqlBuilder := factory.CreateSQLBuilder()
```

Registered handlers:
- 60+ PromQL functions
- 11+ aggregation operators
- Expression handlers for all AST node types

#### 5. **Chain of Responsibility** (`pkg/strategy/registry.go`)
Priority-based strategy selection:

```go
registry := NewStrategyRegistry()
registry.Register(strategy1, 10)  // High priority
registry.Register(strategy2, 5)   // Lower priority

strategy, _ := registry.GetStrategy(expr, ctx)
result, _ := strategy.Transpile(expr, ctx)
```

#### 6. **Template Method** (`pkg/strategy/registry.go`)
Common patterns in base strategy:

```go
type BaseStrategy struct {
    // Common fields
}

func (s *BaseStrategy) PreProcess(expr ast.Expr) error {
    // Common preprocessing
}

func (s *BaseStrategy) PostProcess(sql string) string {
    // Common post-processing
}
```

### SOLID Principles

#### Single Responsibility Principle ✅
Each component has one clear purpose:
- `Lexer`: Only tokenization
- `Parser`: Only AST generation
- `Transpiler`: Only orchestration
- `Builder`: Only SQL construction
- `Factory`: Only component creation

#### Open/Closed Principle ✅
- **Open for extension**: Register new strategies, handlers, and functions
- **Closed for modification**: Core interfaces don't change

```go
// Extend without modifying existing code
registry.Register(NewCustomStrategy(), priority)
factory.RegisterFunctionHandler("custom_func", handler)
```

#### Liskov Substitution Principle ✅
All implementations can be substituted:

```go
var builder builder.SQLBuilder
builder = builder.NewClickHouseSQLBuilder()  // Can substitute
builder = builder.NewPostgreSQLBuilder()     // Can substitute
```

#### Interface Segregation Principle ✅
Small, focused interfaces:

```go
type SchemaProvider interface {
    GetTableName() string
    GetValueColumn() string
}

type TimeRangeProvider interface {
    GetStartTime() int64
    GetEndTime() int64
}
```

#### Dependency Inversion Principle ✅
Depend on abstractions:

```go
type Transpiler struct {
    builder    builder.SQLBuilder      // Interface, not concrete
    factory    factory.ComponentFactory // Interface, not concrete
    visitor    visitor.Visitor          // Interface, not concrete
}
```

## 📦 Project Structure

```
transpiler/
├── cmd/
│   └── promql-transpiler/
│       └── main.go              # CLI entry point
├── pkg/
│   ├── lexer/
│   │   └── lexer.go            # Tokenization
│   ├── parser/
│   │   └── parser.go           # AST generation
│   ├── ast/
│   │   └── ast.go              # AST node definitions
│   ├── builder/
│   │   └── sql_builder.go      # Builder pattern (SQL construction)
│   ├── factory/
│   │   ├── component_factory.go # Factory pattern (component creation)
│   │   └── handlers.go          # Function & aggregation handlers
│   ├── strategy/
│   │   ├── interfaces.go        # Strategy pattern interfaces
│   │   └── registry.go          # Chain of Responsibility
│   ├── visitor/
│   │   └── visitor.go           # Visitor pattern (AST traversal)
│   ├── grafana/
│   │   └── variables.go         # Grafana template variables & macros
│   ├── alertmanager/
│   │   └── alerts.go            # Alertmanager alert rules & state
│   ├── transpiler/
│   │   └── transpiler.go        # Main transpiler logic
│   ├── clickhouse/
│   │   └── schema.go            # ClickHouse schema utilities
│   └── cardinality/
│       └── estimator.go         # Cardinality estimation
├── internal/
│   └── config/
│       └── config.go            # Configuration
├── test/
│   ├── lexer_test.go
│   ├── parser_test.go
│   ├── transpiler_test.go
│   └── integration_test.go
├── examples/
│   ├── basic_query.promql
│   ├── aggregations.promql
│   ├── alert_rules.yaml
│   └── grafana_dashboard.json
├── docs/
│   ├── ARCHITECTURE.md
│   ├── PROMETHEUS_SUPPORT.md
│   ├── ALERTMANAGER_SUPPORT.md
│   └── GRAFANA_SUPPORT.md
├── IMPLEMENTATION_CHECKLIST.md  # Detailed progress tracking
├── README.md                     # This file
├── go.mod
└── go.sum
```

## 🚀 Features

### Prometheus Support

#### Selectors
- ✅ Instant vector selectors: `http_requests_total`
- ✅ Range vector selectors: `http_requests_total[5m]`
- ✅ Label matchers: `=`, `!=`, `=~`, `!~`
- ✅ Offset modifier: `http_requests_total offset 5m`

#### Operators
- ✅ Arithmetic: `+`, `-`, `*`, `/`, `%`, `^`
- ✅ Comparison: `==`, `!=`, `>`, `<`, `>=`, `<=`
- ✅ Logical: `and`, `or`, `unless`
- ✅ Aggregation: `sum`, `avg`, `count`, `min`, `max`, `stddev`, `stdvar`
- ✅ Advanced: `topk`, `bottomk`, `quantile`, `count_values`

#### Functions (60+)
**Rate/Increase:**
- ✅ `rate()`, `irate()`, `increase()`
- ✅ `delta()`, `idelta()`
- ✅ `deriv()`, `predict_linear()`

**Aggregations over time:**
- ✅ `avg_over_time()`, `sum_over_time()`
- ✅ `min_over_time()`, `max_over_time()`
- ✅ `count_over_time()`, `quantile_over_time()`
- ✅ `stddev_over_time()`, `stdvar_over_time()`

**Math functions:**
- ✅ `abs()`, `ceil()`, `floor()`, `round()`
- ✅ `sqrt()`, `exp()`, `ln()`, `log2()`, `log10()`

**Label manipulation:**
- ✅ `label_replace()`, `label_join()`

**Time functions:**
- ✅ `time()`, `timestamp()`

**Other:**
- ✅ `absent()`, `absent_over_time()`
- ✅ `changes()`, `resets()`
- ✅ `clamp()`, `clamp_max()`, `clamp_min()`
- ✅ `sort()`, `sort_desc()`
- ✅ `vector()`, `scalar()`

### Alertmanager Support

#### Alert Rules
- ✅ Alert condition expressions
- ✅ `FOR` duration handling
- ✅ Alert labels and annotations
- ✅ Alert state tracking (pending, firing, inactive)
- ✅ State history and transitions

#### Alert Metrics
- ✅ `ALERTS` metric queries
- ✅ `ALERTS_FOR_STATE` metric
- ✅ Alert metadata queries

#### Features
```go
// Alert rule evaluation
rule := &AlertRule{
    Name:     "HighRequestRate",
    Expr:     "rate(http_requests_total[5m]) > 100",
    Duration: 5 * time.Minute,
    Labels:   map[string]string{"severity": "warning"},
}

transpiler.TranspileAlertExpression(rule, ctx)
```

### Grafana Support

#### Template Variables
- ✅ `$__interval` - Auto-calculated interval
- ✅ `$__interval_ms` - Interval in milliseconds
- ✅ `$__range` - Dashboard time range
- ✅ `$__range_s` - Range in seconds
- ✅ `$__range_ms` - Range in milliseconds
- ✅ `$__from`, `$__to` - Time range boundaries
- ✅ `$__dashboard`, `$__panel` - Metadata
- ✅ Custom template variables

#### Macros
- ✅ `$__timeFilter(column)` - Time range filter
- ✅ `$__timeGroup(column, interval)` - Time bucketing
- ✅ `$__timeFrom()`, `$__timeTo()` - Time boundaries
- ✅ `$__unixEpochFilter(column)` - Unix timestamp filter
- ✅ `$__contains(column, $variable)` - Multi-value support

#### Panel Support
```go
processor := NewGrafanaVariableProcessor(builder)

// Process template variables
query := "rate(http_requests_total{job=~'$job'}[$__interval])"
processed, _ := processor.ProcessVariables(query, ctx)

// Process macros
query = "SELECT * FROM metrics WHERE $__timeFilter(timestamp)"
processed, _ := processor.ProcessMacros(query, ctx)
```

## 📖 Usage

### Basic Transpilation

```go
package main

import (
    "transpiler/pkg/lexer"
    "transpiler/pkg/parser"
    "transpiler/pkg/transpiler"
    "transpiler/pkg/builder"
    "transpiler/pkg/factory"
)

func main() {
    // Create components using factory
    factory := factory.NewClickHouseComponentFactory()
    builder := factory.CreateSQLBuilder()
    
    // Create transpiler
    t := transpiler.New(builder, factory)
    
    // Transpile PromQL
    promQL := "rate(http_requests_total{job='api'}[5m])"
    sql, err := t.Transpile(promQL)
    if err != nil {
        log.Fatal(err)
    }
    
    fmt.Println(sql)
}
```

### CLI Usage

```bash
# Basic transpilation
./promql-transpiler "rate(http_requests_total[5m])"

# With options
./promql-transpiler \
    --expr "sum(rate(http_requests_total[5m])) by (job)" \
    --start-time 1609459200 \
    --end-time 1609545600 \
    --step 60 \
    --table metrics \
    --database prometheus

# Grafana mode
./promql-transpiler \
    --expr "rate(http_requests_total{job=~'\$job'}[\$__interval])" \
    --grafana \
    --variables "job=api,web" \
    --interval "1m"

# Alert rule mode
./promql-transpiler \
    --alert-file alerts.yaml \
    --mode alertmanager
```

### Advanced Usage

#### Custom Strategy

```go
type CustomStrategy struct {
    strategy.BaseStrategy
}

func (s *CustomStrategy) Transpile(expr ast.Expr, ctx *strategy.Context) (string, error) {
    // Custom transpilation logic
    return "SELECT custom_query", nil
}

func (s *CustomStrategy) CanHandle(expr ast.Expr) bool {
    // Determine if this strategy applies
    return true
}

// Register the strategy
registry := strategy.NewStrategyRegistry()
registry.Register(&CustomStrategy{}, 20) // High priority
```

#### Custom Function Handler

```go
type CustomFunctionHandler struct{}

func (h *CustomFunctionHandler) Handle(call *ast.Call, ctx *factory.TranspilationContext) (string, error) {
    return fmt.Sprintf("customFunction(%s)", ctx.ValueColumn), nil
}

func (h *CustomFunctionHandler) GetRequiredArgs() int { return 1 }
func (h *CustomFunctionHandler) GetOptionalArgs() int { return 0 }

// Register the handler
factory := factory.NewClickHouseComponentFactory()
factory.RegisterFunctionHandler("custom_func", &CustomFunctionHandler{})
```

## 🔧 Configuration

### Schema Configuration

```go
schema := clickhouse.Schema{
    Database:     "prometheus",
    Table:        "metrics",
    TimeColumn:   "timestamp",
    ValueColumn:  "value",
    LabelColumns: []string{"job", "instance", "__name__"},
}
```

### Optimization Settings

```go
config := Config{
    EnableSampling:       true,
    SampleRatio:          0.1,      // 10% sampling
    EnablePrewhere:       true,
    MaxCardinality:       10000,
    EnableQueryCache:     true,
    EnableParallelQueries: true,
}
```

## 🧪 Testing

```bash
# Run all tests
go test ./...

# Run specific package tests
go test ./pkg/lexer
go test ./pkg/parser
go test ./pkg/transpiler

# Run with coverage
go test -cover ./...

# Run with race detector
go test -race ./...

# Integration tests
go test -tags=integration ./test/integration
```

## 📊 Performance

### Optimizations Implemented

1. **PREWHERE Optimization**
   - Pushes filters before reading all columns
   - Significant performance improvement for selective queries

2. **Sampling Support**
   - Configurable sampling ratio for large datasets
   - Maintains statistical accuracy

3. **Cardinality Estimation**
   - Prevents high-cardinality queries
   - Automatic query rewriting for optimization

4. **Query Caching**
   - Caches compiled queries
   - Reduces transpilation overhead

5. **Parallel Processing**
   - Concurrent processing of independent sub-queries
   - Improved throughput for complex queries

## 🤝 Contributing

1. Follow SOLID principles
2. Use design patterns appropriately
3. Write comprehensive tests
4. Document public APIs
5. Update IMPLEMENTATION_CHECKLIST.md

## 📝 License

MIT License

## 🔗 References

- [PromQL Documentation](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [ClickHouse SQL Reference](https://clickhouse.com/docs/en/sql-reference/)
- [Grafana Macros](https://grafana.com/docs/grafana/latest/datasources/prometheus/)
- [Alertmanager](https://prometheus.io/docs/alerting/latest/alertmanager/)

## 📈 Roadmap

See [IMPLEMENTATION_CHECKLIST.md](IMPLEMENTATION_CHECKLIST.md) for detailed progress.

### Completed ✅
- Design patterns implementation
- SOLID principles adherence
- Core Prometheus support (60+ functions)
- Alertmanager support (alert rules, state tracking)
- Grafana support (variables, macros, panels)
- Builder pattern for SQL generation
- Factory pattern for component creation
- Strategy pattern for extensibility

### In Progress 🔄
- Decorator pattern for query enhancements
- Additional date/time functions
- Subquery support
- @ modifier support

### Planned 📋
- holt_winters() function
- KEEP_FIRING_FOR support
- $__rate_interval variable
- Performance benchmarks
- Distributed query support
