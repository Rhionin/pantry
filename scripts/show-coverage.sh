#!/bin/bash
set -e

# Run tests with coverage across all packages
echo "Running tests with coverage..."
go test -cover -coverpkg=./... -coverprofile=coverage.out ./...

if [ $? -ne 0 ]; then
    echo "Tests failed"
    exit 1
fi

# Show total coverage
echo ""
echo "================================"
go tool cover -func=coverage.out | tail -1
echo "================================"

# Optionally generate HTML report
if [ "$1" = "--html" ]; then
    go tool cover -html=coverage.out -o coverage.html
    echo "HTML coverage report generated: coverage.html"
fi

# Clean up
rm -f coverage.out
