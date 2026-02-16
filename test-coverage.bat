@echo off
REM Test Coverage Script for PromQL Transpiler
REM Runs all tests with coverage analysis and generates reports

echo ================================================
echo PromQL Transpiler - Test Coverage Analysis
echo ================================================
echo.

REM Clean previous coverage data
if exist coverage.out del coverage.out
if exist coverage.html del coverage.html
if exist coverage-summary.txt del coverage-summary.txt

echo [1/4] Running all tests with coverage...
go test ./... -coverprofile=coverage.out -covermode=atomic -v

if %ERRORLEVEL% NEQ 0 (
    echo.
    echo ERROR: Tests failed! Fix failing tests before checking coverage.
    exit /b 1
)

echo.
echo [2/4] Generating coverage summary...
go tool cover -func=coverage.out > coverage-summary.txt

echo.
echo [3/4] Generating HTML coverage report...
go tool cover -html=coverage.out -o coverage.html

echo.
echo [4/4] Coverage Summary:
echo ================================================
findstr /C:"total:" coverage-summary.txt

echo.
echo ================================================
echo Package-level coverage:
echo ================================================
findstr /C:"pkg/metrics" coverage-summary.txt | findstr /C:".go:"
findstr /C:"pkg/middleware" coverage-summary.txt | findstr /C:".go:"
findstr /C:"pkg/validation" coverage-summary.txt | findstr /C:".go:"
findstr /C:"pkg/grafana" coverage-summary.txt | findstr /C:".go:"
findstr /C:"pkg/parser" coverage-summary.txt | findstr /C:".go:"

echo.
echo ================================================
echo Reports Generated:
echo - coverage.out (machine-readable)
echo - coverage.html (interactive report)
echo - coverage-summary.txt (detailed breakdown)
echo ================================================
echo.
echo To view HTML report: start coverage.html
echo.
