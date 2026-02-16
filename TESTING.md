# Quick Test Guide

## Run the Integration Test

The fastest way to verify that the transpiler is working correctly:

### Windows

```cmd
cd transpiler
build.bat
```

This will:
1. Download dependencies
2. Run all unit tests
3. Build the binary
4. Test the CLI

### Linux/macOS

```bash
cd transpiler
chmod +x build.sh
./build.sh
```

## Manual Test

### 1. Build the Project

```bash
go build -o bin/promql-transpiler ./cmd/promql-transpiler
```

### 2. Run a Simple Query

```bash
./bin/promql-transpiler -q "up"
```

Expected output: ClickHouse SQL query

### 3. Run Unit Tests

```bash
go test -v ./...
```

Expected: All tests pass

### 4. Run Integration Test

```bash
go run test/integration_test.go
```

Expected: All 16+ tests pass

### 5. Run Examples

```bash
go run examples/basic_usage.go
```

Expected: Multiple SQL queries printed for various PromQL expressions

## Quick Verification Checklist

- [ ] `go test ./...` passes
- [ ] `go build ./cmd/promql-transpiler` succeeds
- [ ] `./bin/promql-transpiler -q "up"` produces SQL
- [ ] `go run test/integration_test.go` - all tests pass
- [ ] `go run examples/basic_usage.go` shows examples

## Common Issues

### Issue: "package github.com/shinro/promql-transpiler not found"

**Solution:** Make sure you're in the transpiler directory and run:
```bash
go mod download
go mod tidy
```

### Issue: Build fails on Windows

**Solution:** Use the provided build.bat script or ensure Go is in your PATH

### Issue: Tests fail

**Solution:** Check that all files are present:
```bash
# Windows
dir /s /b *.go

# Linux/macOS
find . -name "*.go"
```

## Expected Output Examples

### Simple Query
```bash
./bin/promql-transpiler -q "http_requests_total"
```
Should output SQL with SELECT, FROM metrics, WHERE metric_name = 'http_requests_total'

### Rate Query
```bash
./bin/promql-transpiler -q "rate(http_requests_total[5m])"
```
Should output SQL with WITH clause, lagInFrame, and WINDOW

### Aggregation
```bash
./bin/promql-transpiler -q "sum(http_requests_total) by (status)"
```
Should output SQL with sum(value) and GROUP BY

## Next Steps

Once tests pass:
1. Read the full [README.md](../README.md)
2. Check out [QUICKSTART.md](../docs/QUICKSTART.md)
3. Set up ClickHouse following [SCHEMA.md](../docs/SCHEMA.md)
4. Try custom configurations from examples/

## Getting Help

- Check [DEVELOPMENT.md](../docs/DEVELOPMENT.md) for development setup
- Review examples in `examples/` directory
- Run with `-v` flag for verbose output
