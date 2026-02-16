@echo off
REM Build and Test Script for PromQL Transpiler on Windows

echo ========================================
echo PromQL to ClickHouse SQL Transpiler
echo Build and Test Script
echo ========================================
echo.

REM Check if Go is installed
where go >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Go is not installed or not in PATH
    echo Please install Go from https://golang.org/dl/
    exit /b 1
)

echo [1/6] Checking Go version...
go version
echo.

echo [2/6] Downloading dependencies...
go mod download
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Failed to download dependencies
    exit /b 1
)
echo Dependencies downloaded successfully
echo.

echo [3/6] Running tests...
go test -v ./...
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Tests failed
    exit /b 1
)
echo All tests passed!
echo.

echo [4/6] Running tests with race detector...
go env CGO_ENABLED > nul 2>&1
for /f %%i in ('go env CGO_ENABLED') do set CGO_VAL=%%i
if "%CGO_VAL%"=="0" (
    echo SKIPPED: Race detector requires CGO_ENABLED=1 ^(needs a C compiler like gcc^)
    echo   To enable: set CGO_ENABLED=1 ^(requires gcc in PATH, e.g. via MinGW/MSYS2^)
) else (
    go test -race ./...
    if %ERRORLEVEL% NEQ 0 (
        echo WARNING: Race conditions detected
    ) else (
        echo Race detector passed!
    )
)
echo.

echo [5/6] Building binary...
if not exist bin mkdir bin
go build -o bin\promql-transpiler.exe .\cmd\promql-transpiler
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: Build failed
    exit /b 1
)
echo Binary built successfully: bin\promql-transpiler.exe
echo.

echo [6/6] Testing the CLI...
bin\promql-transpiler.exe -q "up" >nul 2>nul
if %ERRORLEVEL% NEQ 0 (
    echo ERROR: CLI test failed
    exit /b 1
)
echo CLI test passed!
echo.

echo ========================================
echo Build and test completed successfully!
echo ========================================
echo.
echo Next steps:
echo   1. Run the CLI: bin\promql-transpiler.exe -q "your_query"
echo   2. Run examples: go run examples\basic_usage.go
echo   3. Read documentation: README.md
echo.

pause
