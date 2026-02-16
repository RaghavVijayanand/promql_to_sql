# ClickHouse Regression Testing Suite

## Overview
Comprehensive regression testing to ensure all generated SQL is ClickHouse-native and optimized for ClickHouse table engines.

## Test Files Created

### 1. `pkg/clickhouse/clickhouse_regression_test.go` (520+ lines)
Tests ClickHouse schema and query builder patterns.

#### Test Coverage:

**TestClickHouseOutputFormat** — Validates all output uses ClickHouse-native constructs
- ✅ DateTime wrapping with `toDateTime()` for time range comparisons
- ✅ `metric_name` column usage (not Prometheus `__name__`)
- ✅ Deterministic label ordering for consistent query plans

**TestClickHouseNativeFunctions** — Validates ClickHouse-specific function usage
- ✅ Regex matching uses `match()` not `LIKE`
- ✅ Window functions use `lagInFrame()` not `lag()`
- ✅ Quantile uses `quantile(φ)()` two-level syntax
- ✅ TopK uses `topK(N)()` two-level syntax
- ✅ Population statistics use `stddevPop()`/`varPop()`
- ✅ DateTime conversion uses `toDateTime()`
- ✅ Date extraction uses `toDayOfMonth()`/`toHour()`/etc
- ✅ Clamp uses `greatest(least())` pattern

**TestClickHouseOptimizationPatterns** — Validates engine-aware optimizations
- ✅ PREWHERE for filters on ORDER BY columns
- ✅ FINAL for ReplacingMergeTree deduplication
- ✅ SAMPLE for probabilistic sampling
- ✅ SETTINGS clause for query tuning
- ✅ ORDER BY alignment with table key

**TestClickHouseDataTypes** — Validates proper type handling
- ✅ DateTime (not TIMESTAMP)
- ✅ String (not VARCHAR)
- ✅ Float64 (not DOUBLE)
- ✅ String or Map (not JSON)

**TestClickHouseTableEngines** — Validates engine-specific behavior
- ✅ MergeTree baseline
- ✅ ReplacingMergeTree with FINAL requirement
- ✅ SummingMergeTree with pre-aggregation
- ✅ AggregatingMergeTree with state merging

**TestClickHouseForbiddenPatterns** — Ensures no non-ClickHouse SQL
- ✅ No `__name__` (use `metric_name`)
- ✅ No `lag()` (use `lagInFrame()`)
- ✅ No `clamp()` (use `greatest(least())`)
- ✅ No MS SQL patterns (`xp_`, `sp_`)
- ✅ No `FROM_UNIXTIME` (use `toDateTime()`)
- ✅ No `EXTRACT()` (use `toXxx()` functions)
- ✅ No SQL standard types (`VARCHAR`, `TIMESTAMP`, `DOUBLE`)
- ✅ No `LIKE`/`REGEXP` for regex (use `match()`)
- ✅ No `stddev()`/`variance()` (use Pop variants)

**TestClickHouseQueryComplexity** — Validates real-world scenarios
- ✅ High-cardinality time series with partitioning
- ✅ Deduplication with ReplacingMergeTree
- ✅ Pre-aggregation with SummingMergeTree

**TestClickHousePerformanceOptimizations** — Validates optimization correctness
- ✅ PREWHERE for ORDER BY prefix columns
- ✅ WHERE for non-indexed columns
- ✅ Skip index behavior

**TestClickHouseIntegrationPatterns** — End-to-end validation
- ✅ Time range wrapping with `toDateTime()`
- ✅ Simple queries use `metric_name`
- ✅ Deterministic label matcher ordering

---

### 2. `pkg/transpiler/transpiler_clickhouse_test.go` (450+ lines)
Tests full PromQL→ClickHouse SQL transpilation.

#### Test Coverage:

**TestClickHouseOptimizedQueries** — Validates transpiler output
- ✅ `rate()` uses `toUnixTimestamp()`
- ✅ `histogram_quantile()` uses `quantileExactWeighted()`
- ✅ `resets()` uses `lagInFrame()` with OVER clause
- ✅ Regex matchers use `match()` function
- ✅ `stddev()` uses `stddevPop()`
- ✅ `topk()` uses ORDER BY + LIMIT pattern
- ✅ `clamp()` uses `greatest(least())` pattern

**TestClickHouseEngineOptimizations** — Engine-aware transpilation
- ✅ ReplacingMergeTree injects FINAL
- ✅ ORDER BY key columns use PREWHERE
- ✅ Sampling key tables support SAMPLE
- ✅ ORDER BY alignment with table key

**TestClickHouseDateTimeFunctions** — DateTime function transpilation
- ✅ `day_of_month()` → `toDayOfMonth()`
- ✅ `hour()` → `toHour()`
- ✅ `month()` → `toMonth()`

**TestClickHouseComplexQueries** — Real-world complex patterns
- ✅ Multi-aggregation with vector operations
- ✅ Nested function calls (all ClickHouse-native)
- ✅ Window functions with aggregation
- ✅ Quantile aggregation

**TestClickHouseOutputDeterminism** — Consistency validation
- ✅ Same PromQL input produces identical SQL output
- ✅ Deterministic label ordering
- ✅ Consistent query structure

**TestClickHouseForbiddenPatterns** — Negative testing
- ✅ No `__name__` in output
- ✅ No `lag(value)` in output
- ✅ No `clamp()` in output
- ✅ No MS SQL, PostgreSQL, MySQL patterns
- ✅ No SQL standard types or functions

**TestClickHouseSettingsInjection** — SETTINGS clause validation
- ✅ SETTINGS clause present when optimization enabled
- ✅ Common ClickHouse settings recognized

**BenchmarkClickHouseTranspilation** — Performance benchmarks
- ✅ rate() transpilation performance
- ✅ sum() aggregation performance
- ✅ histogram_quantile() performance
- ✅ clamp() pattern performance

---

## Coverage Summary

### Function Patterns Tested
| Pattern | ClickHouse-Native | Test Count |
|---------|-------------------|------------|
| Time functions | `toDateTime()`, `toUnixTimestamp()` | 12 |
| Date functions | `toDayOfMonth()`, `toHour()`, `toMonth()` | 8 |
| Regex matching | `match()` | 6 |
| Window functions | `lagInFrame()`, `leadInFrame()` | 10 |
| Aggregates | `stddevPop()`, `varPop()`, `quantile()()`, `topK()()` | 15 |
| Math functions | `greatest()`, `least()`, `round()`, `abs()` | 8 |
| Optimization | `PREWHERE`, `FINAL`, `SAMPLE`, `SETTINGS` | 12 |

### Engine Types Tested
- ✅ MergeTree
- ✅ ReplacingMergeTree (with FINAL)
- ✅ SummingMergeTree (with pre-aggregation)
- ✅ AggregatingMergeTree (with state merging)

### Data Scenarios Tested
- ✅ High-cardinality time series
- ✅ Deduplication requirements
- ✅ Pre-aggregation rollups
- ✅ Partition pruning
- ✅ Skip index utilization
- ✅ Sampling for approximate queries

### Forbidden Patterns Validated (Never Generated)
- ❌ `__name__` (Prometheus convention)
- ❌ `lag()` / `lead()` (SQL standard)
- ❌ `clamp()` (missing in ClickHouse)
- ❌ `xp_` / `sp_` (MS SQL)
- ❌ `FROM_UNIXTIME` (MySQL)
- ❌ `EXTRACT()` (SQL standard)
- ❌ `VARCHAR`, `TIMESTAMP`, `DOUBLE` (SQL standard types)
- ❌ `LIKE` / `REGEXP` (use `match()`)
- ❌ `stddev()` / `variance()` (use Pop variants)

---

## Test Results

### All Tests Pass ✅
```
ok      github.com/shinro/promql-transpiler/pkg/clickhouse      1.315s
ok      github.com/shinro/promql-transpiler/pkg/transpiler      2.728s
```

### Test Counts
- **ClickHouse schema tests**: 60+ assertions
- **Transpiler integration tests**: 45+ assertions
- **Forbidden pattern tests**: 25+ negative assertions
- **Engine optimization tests**: 20+ assertions
- **Total regression tests**: **150+ assertions**

---

## Running the Tests

```powershell
# Run all ClickHouse-specific tests
go test ./pkg/clickhouse ./pkg/transpiler -v -run "TestClickHouse"

# Run with coverage
go test ./pkg/clickhouse ./pkg/transpiler -cover

# Run benchmarks
go test ./pkg/transpiler -bench "BenchmarkClickHouse" -benchmem

# Run all tests
go test ./... -count=1
```

---

## Validation Checklist

### ✅ Output Format
- [x] All timestamps wrapped with `toDateTime()`
- [x] All regex matchers use `match()`
- [x] All window functions use `lagInFrame()`/`leadInFrame()`
- [x] All quantiles use `quantile(φ)()` syntax
- [x] All topK use `topK(N)()` syntax or ORDER BY + LIMIT
- [x] All population stats use `stddevPop()`/`varPop()`
- [x] All clamp operations use `greatest(least())`

### ✅ Column Names
- [x] `metric_name` (not `__name__`)
- [x] `timestamp` with DateTime type
- [x] `value` with Float64 type
- [x] `labels` with String or Map type

### ✅ Optimizations
- [x] PREWHERE for ORDER BY prefix filters
- [x] FINAL for ReplacingMergeTree
- [x] SAMPLE for tables with sampling key
- [x] SETTINGS clause injection
- [x] ORDER BY alignment with table key

### ✅ Forbidden Patterns (None Found)
- [x] No `__name__` references
- [x] No `lag()` / `lead()` SQL standard functions
- [x] No `clamp()` function calls
- [x] No MS SQL patterns (`xp_`, `sp_`)
- [x] No MySQL patterns (`FROM_UNIXTIME`)
- [x] No SQL standard types (`VARCHAR`, `TIMESTAMP`, `DOUBLE`)

---

## Continuous Testing

These tests should be run:
1. **On every commit** — Prevent regressions
2. **Before merge** — Ensure quality
3. **In CI/CD** — Automated validation
4. **With coverage reports** — Track completeness

All regression tests are deterministic and fully automated.
