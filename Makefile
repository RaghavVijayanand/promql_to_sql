.PHONY: build test clean install run fmt lint

BINARY_NAME=promql-transpiler
MAIN_PATH=./cmd/promql-transpiler

build:
	go build -o bin/$(BINARY_NAME) $(MAIN_PATH)

test:
	go test -v -race -cover ./...

test-coverage:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

clean:
	rm -rf bin/
	rm -f coverage.out coverage.html

install:
	go install $(MAIN_PATH)

run:
	go run $(MAIN_PATH)

fmt:
	go fmt ./...

lint:
	golangci-lint run

deps:
	go mod download
	go mod tidy

benchmark:
	go test -bench=. -benchmem ./...

.DEFAULT_GOAL := build
