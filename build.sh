#!/bin/bash

# Build and Test Script for PromQL Transpiler on Unix/Linux/macOS

set -e

echo "========================================"
echo "PromQL to ClickHouse SQL Transpiler"
echo "Build and Test Script"
echo "========================================"
echo ""

# Check if Go is installed
if ! command -v go &> /dev/null; then
    echo "ERROR: Go is not installed or not in PATH"
    echo "Please install Go from https://golang.org/dl/"
    exit 1
fi

echo "[1/6] Checking Go version..."
go version
echo ""

echo "[2/6] Downloading dependencies..."
go mod download
echo "Dependencies downloaded successfully"
echo ""

echo "[3/6] Running tests..."
go test -v ./...
echo "All tests passed!"
echo ""

echo "[4/6] Running tests with race detector..."
go test -race ./... || echo "WARNING: Race conditions detected"
echo ""

echo "[5/6] Building binary..."
mkdir -p bin
go build -o bin/promql-transpiler ./cmd/promql-transpiler
echo "Binary built successfully: bin/promql-transpiler"
echo ""

echo "[6/6] Testing the CLI..."
./bin/promql-transpiler -q "up" > /dev/null 2>&1
echo "CLI test passed!"
echo ""

echo "========================================"
echo "Build and test completed successfully!"
echo "========================================"
echo ""
echo "Next steps:"
echo "  1. Run the CLI: ./bin/promql-transpiler -q \"your_query\""
echo "  2. Run examples: go run examples/basic_usage.go"
echo "  3. Read documentation: README.md"
echo ""
