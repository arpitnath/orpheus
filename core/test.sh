#!/bin/bash
# Orpheus Core Test Suite Runner
# Uses gotestsum for better test output visualization

set -e

echo "=========================================="
echo "Orpheus Core Test Suite"
echo "=========================================="
echo ""

# Run all tests with gotestsum
gotestsum --format pkgname-and-test-fails -- \
    -race \
    -timeout=60s \
    -cover \
    ./pkg/...

echo ""
echo "=========================================="
echo "Test Summary"
echo "=========================================="
echo "All tests completed successfully!"
echo ""
echo "Next: Run 'go test -coverprofile=coverage.out ./pkg/...' for detailed coverage"
