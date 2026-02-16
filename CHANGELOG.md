# Changelog

All notable changes to the PromQL to ClickHouse SQL Transpiler will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2024-02-09

### Added

#### Core Features
- Complete PromQL lexer with support for all tokens
- Full PromQL parser generating Abstract Syntax Tree (AST)
- Comprehensive AST node definitions for all PromQL constructs
- Core transpiler converting PromQL AST to ClickHouse SQL

#### PromQL Support
- Vector and matrix selectors
- Label matchers (=, !=, =~, !~)
- Range selectors with duration support
- Offset modifier
- Binary operators (arithmetic, comparison, logical)
- Unary operators (+, -)

#### Aggregation Operators
- sum, min, max, avg
- count, count_values
- stddev, stdvar
- topk, bottomk
- quantile
- group

#### Functions
- rate(), irate()
- increase(), delta(), idelta()
- abs(), ceil(), floor(), round()
- clamp_max(), clamp_min()
- changes(), resets()
- Additional math functions

#### ClickHouse Integration
- Default schema configuration for metrics storage
- Custom schema support
- Query builder with optimization hints
- Time range handling
- Window functions for rate calculations
- CTE (Common Table Expression) generation
- Vector-to-vector operations with joins

#### Cardinality Management
- Cardinality estimator with configurable thresholds
- Query optimization strategies:
  - Direct execution
  - Sampling for high cardinality
  - Aggregate-first approach
  - Label pruning
- Cardinality tracking system
- Cost estimation for queries
- Sample size calculation
- Optimization hints with impact levels

#### CLI Tool
- Command-line interface with Cobra
- Query input via flag
- Output to file or stdout
- Time range specification (start, end, step)
- Verbose mode for debugging
- Pretty-printed SQL output

#### Testing
- Comprehensive unit tests for lexer
- Parser tests covering all PromQL constructs
- Transpiler tests for various query patterns
- Test coverage reporting
- Benchmark support

#### Documentation
- Comprehensive README with feature list
- Quick Start Guide
- Development Guide
- ClickHouse Schema Documentation
- Integration test documentation
- Example programs:
  - Basic usage examples
  - Custom schema examples
  - Cardinality estimation examples

#### Build & Development
- Makefile for common tasks
- Go module configuration
- Dockerfile for containerization
- .gitignore for Go projects
- Cross-compilation support

### Technical Details

#### Architecture
- Clean separation of concerns:
  - Lexer: Tokenization
  - Parser: AST generation
  - Transpiler: SQL generation
  - Schema: ClickHouse integration
  - Cardinality: Optimization
- Modular package structure
- Public API in `pkg/` packages
- Internal utilities in `internal/` packages

#### Performance Optimizations
- Efficient token scanning
- Minimal allocations in hot paths
- Query builder for SQL generation
- Support for materialized views
- Pre-aggregation strategies

#### Supported PromQL Patterns
- Simple metric queries
- Complex aggregations
- Rate/increase calculations
- Mathematical expressions
- Multi-level nested queries
- Vector arithmetic
- Label-based filtering and grouping

### Known Limitations

- Some advanced PromQL features not yet implemented:
  - Subqueries
  - histogram_quantile (partial)
  - Some temporal functions
  - Recording rules
- Vector matching limited to basic cases
- No support for alerts or recording rules DSL

### Future Roadmap

- [ ] Complete histogram support
- [ ] Subquery implementation
- [ ] Advanced vector matching
- [ ] Query result caching
- [ ] Distributed query optimization
- [ ] Prometheus remote write integration
- [ ] Web UI for query testing
- [ ] Query result visualization

## [0.1.0] - Initial Development

### Added
- Project structure
- Basic lexer implementation
- Parser foundation
- Initial transpiler logic

---

For more information, see the [README](README.md) and [documentation](docs/).
