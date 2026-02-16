# Development Guide

## Prerequisites

- Go 1.21 or higher
- Make (optional, but recommended)
- Docker (for running ClickHouse locally)

## Setting Up Development Environment

### 1. Clone the Repository

```bash
cd transpiler
```

### 2. Install Dependencies

```bash
make deps
```

Or manually:

```bash
go mod download
go mod tidy
```

### 3. Build the Project

```bash
make build
```

This will create the binary in `bin/promql-transpiler`.

## Development Workflow

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage

# Run benchmarks
make benchmark
```

### Code Formatting

```bash
# Format all Go files
make fmt

# Run linter (requires golangci-lint)
make lint
```

### Building

```bash
# Build for current platform
make build

# Clean build artifacts
make clean
```

## Project Structure

```
transpiler/
├── cmd/                    # Command-line applications
│   └── promql-transpiler/  # Main CLI tool
├── pkg/                    # Public packages (importable by others)
│   ├── ast/               # Abstract Syntax Tree definitions
│   ├── clickhouse/        # ClickHouse schema and query builder
│   ├── lexer/             # Lexical analyzer (tokenizer)
│   ├── parser/            # Parser (tokens to AST)
│   └── transpiler/        # Core transpilation logic
├── internal/              # Private packages (not importable)
│   └── cardinality/       # Cardinality estimation and optimization
├── test/                  # Integration tests
├── examples/              # Example programs
└── docs/                  # Documentation
```

## Adding New Features

### Adding a New PromQL Function

1. **Add token to lexer** (`pkg/lexer/lexer.go`):
   ```go
   const (
       // ... existing tokens
       MY_FUNC // my_func
   )
   
   var keywords = map[string]TokenType{
       // ... existing keywords
       "my_func": MY_FUNC,
   }
   ```

2. **Add function type to AST** (`pkg/ast/ast.go`):
   ```go
   const (
       // ... existing functions
       FuncMyFunc FuncType = "my_func"
   )
   ```

3. **Update parser** (`pkg/parser/parser.go`):
   ```go
   func (p *Parser) isFunctionCall() bool {
       funcTokens := []lexer.TokenType{
           // ... existing functions
           lexer.MY_FUNC,
       }
       // ...
   }
   
   func tokenToFuncType(t lexer.TokenType) ast.FuncType {
       switch t {
       // ... existing cases
       case lexer.MY_FUNC:
           return ast.FuncMyFunc
       }
   }
   ```

4. **Implement transpilation** (`pkg/transpiler/transpiler.go`):
   ```go
   func (t *Transpiler) transpileCall(call *ast.Call) (string, error) {
       switch call.Func {
       // ... existing cases
       case ast.FuncMyFunc:
           return t.transpileMyFunc(call)
       }
   }
   
   func (t *Transpiler) transpileMyFunc(call *ast.Call) (string, error) {
       // Implementation here
   }
   ```

5. **Add tests**:
   - Add lexer test in `pkg/lexer/lexer_test.go`
   - Add parser test in `pkg/parser/parser_test.go`
   - Add transpiler test in `pkg/transpiler/transpiler_test.go`

### Adding a New Aggregation Operator

Similar process to functions, but work with `AggOp` instead of `FuncType`.

### Adding Custom ClickHouse Optimizations

1. Add optimization logic to `internal/cardinality/estimator.go`
2. Update `pkg/clickhouse/schema.go` with new query builder methods
3. Use the new methods in `pkg/transpiler/transpiler.go`

## Testing Guidelines

### Unit Tests

- Each package should have comprehensive unit tests
- Use table-driven tests for multiple cases
- Mock external dependencies when needed

Example:
```go
func TestTranspiler_MyFunction(t *testing.T) {
    tests := []struct {
        name     string
        promql   string
        expected string
    }{
        {"case1", "my_func(metric)", "expected SQL"},
        {"case2", "my_func(metric[5m])", "expected SQL"},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            trans := New(nil)
            sql, err := trans.Transpile(tt.promql)
            require.NoError(t, err)
            assert.Contains(t, sql, tt.expected)
        })
    }
}
```

### Integration Tests

Place integration tests in the `test/` directory.

### Benchmarks

Add benchmarks for performance-critical code:

```go
func BenchmarkTranspile(b *testing.B) {
    trans := New(nil)
    promql := "rate(http_requests_total[5m])"
    
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        _, _ = trans.Transpile(promql)
    }
}
```

## Debugging

### Enable Verbose Output

```bash
promql-transpiler -q "your_query" -v
```

### Print AST

Add debugging in your code:

```go
expr, _ := parser.Parse(promql)
fmt.Printf("AST: %+v\n", expr)
```

### Print Generated SQL

The transpiler outputs SQL to stdout by default, or use `-o` flag to save to file.

## Common Tasks

### Update Dependencies

```bash
go get -u ./...
go mod tidy
```

### Cross-Compilation

```bash
# Linux
GOOS=linux GOARCH=amd64 go build -o bin/promql-transpiler-linux ./cmd/promql-transpiler

# Windows
GOOS=windows GOARCH=amd64 go build -o bin/promql-transpiler.exe ./cmd/promql-transpiler

# macOS
GOOS=darwin GOARCH=amd64 go build -o bin/promql-transpiler-mac ./cmd/promql-transpiler
```

### Generate Coverage Report

```bash
make test-coverage
# Open coverage.html in browser
```

## Code Style Guidelines

1. **Follow Go conventions**: Use `gofmt` and `golint`
2. **Error handling**: Always check and handle errors appropriately
3. **Documentation**: Add godoc comments to exported functions/types
4. **Naming**: Use clear, descriptive names
5. **Keep functions focused**: Single responsibility principle
6. **Avoid global state**: Pass dependencies explicitly

## Performance Considerations

1. **Minimize allocations**: Reuse buffers and slices when possible
2. **Use string builders**: For concatenating multiple strings
3. **Benchmark changes**: Always benchmark performance-critical code
4. **Profile when needed**: Use `pprof` for performance analysis

```bash
go test -cpuprofile=cpu.prof -memprofile=mem.prof -bench=.
go tool pprof cpu.prof
```

## Debugging Common Issues

### Parser Errors

If the parser fails:
1. Check the lexer tokens: `lexer.Tokenize(input)`
2. Verify token types match expected patterns
3. Add parser debugging prints

### Incorrect SQL Generation

If SQL is incorrect:
1. Print the AST to verify parsing is correct
2. Step through transpilation logic
3. Check ClickHouse schema configuration
4. Verify time range settings

### Test Failures

If tests fail:
1. Run individual test: `go test -v -run TestName`
2. Check for race conditions: `go test -race`
3. Verify test data and expectations

## Contributing

1. Create a feature branch
2. Write tests for new functionality
3. Ensure all tests pass
4. Format code: `make fmt`
5. Run linter: `make lint`
6. Update documentation as needed
7. Submit pull request

## Resources

- [PromQL Documentation](https://prometheus.io/docs/prometheus/latest/querying/basics/)
- [ClickHouse Documentation](https://clickhouse.com/docs)
- [Go Testing](https://golang.org/pkg/testing/)
- [Go Performance](https://go.dev/doc/diagnostics)
