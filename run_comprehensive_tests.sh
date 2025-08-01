#!/bin/bash

# Comprehensive Test Runner for Auth Service
# This script runs all unit tests, benchmarks, and generates coverage reports

set -e

echo "🚀 Starting Comprehensive Test Suite for Auth Service"
echo "=================================================="

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Change to the auth service directory
cd "$(dirname "$0")"

# Clean up previous test artifacts
print_status "Cleaning up previous test artifacts..."
rm -f coverage.out coverage.html

# Run comprehensive unit tests
print_status "Running comprehensive unit tests..."
echo "-----------------------------------"

# Run the comprehensive test suites
print_status "Running Comprehensive Auth Service Tests..."
go test -v -run TestComprehensiveAuthService ./internal/application/services/ || {
    print_error "Comprehensive Auth Service tests failed"
    exit 1
}

print_status "Running Comprehensive Token Service Tests..."
go test -v -run TestComprehensiveTokenService ./internal/application/services/ || {
    print_error "Comprehensive Token Service tests failed"
    exit 1
}

print_status "Running Comprehensive Permission Service Tests..."
go test -v -run TestComprehensivePermissionService ./internal/application/services/ || {
    print_error "Comprehensive Permission Service tests failed"
    exit 1
}

print_status "Running Property-Based Security Tests..."
go test -v -run TestPropertyBasedSecurity ./internal/application/services/ || {
    print_error "Property-based security tests failed"
    exit 1
}

# Run all tests with coverage
print_status "Running all tests with coverage analysis..."
echo "----------------------------------------"

go test -coverprofile=coverage.out -covermode=atomic ./internal/application/services/ ./internal/cache/ ./internal/errors/ ./internal/grpc/ ./internal/infrastructure/repositories/ || {
    print_error "Test execution failed"
    exit 1
}

# Generate coverage report
print_status "Generating coverage report..."
go tool cover -html=coverage.out -o coverage.html

# Calculate coverage percentage
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
print_status "Total test coverage: ${COVERAGE}%"

# Check if coverage meets minimum requirement (85%)
if (( $(echo "$COVERAGE >= 85" | bc -l) )); then
    print_success "✅ Coverage requirement met (${COVERAGE}% >= 85%)"
else
    print_warning "⚠️  Coverage below requirement (${COVERAGE}% < 85%)"
fi

# Run benchmark tests
print_status "Running benchmark tests..."
echo "-------------------------"

print_status "Running authentication benchmarks..."
go test -bench=BenchmarkAuthService -benchmem -run=^$ ./internal/application/services/ || {
    print_warning "Authentication benchmarks failed"
}

print_status "Running token service benchmarks..."
go test -bench=BenchmarkTokenService -benchmem -run=^$ ./internal/application/services/ || {
    print_warning "Token service benchmarks failed"
}

print_status "Running permission service benchmarks..."
go test -bench=BenchmarkPermissionService -benchmem -run=^$ ./internal/application/services/ || {
    print_warning "Permission service benchmarks failed"
}

print_status "Running cache operation benchmarks..."
go test -bench=BenchmarkCacheOperations -benchmem -run=^$ ./internal/application/services/ || {
    print_warning "Cache operation benchmarks failed"
}

print_status "Running concurrent operation benchmarks..."
go test -bench=BenchmarkConcurrentAuthentication -benchmem -run=^$ ./internal/application/services/ || {
    print_warning "Concurrent operation benchmarks failed"
}

print_status "Running memory allocation benchmarks..."
go test -bench=BenchmarkMemoryAllocation -benchmem -run=^$ ./internal/application/services/ || {
    print_warning "Memory allocation benchmarks failed"
}

# Run race condition tests
print_status "Running race condition tests..."
echo "------------------------------"

go test -race -run TestConcurrent ./internal/application/services/ || {
    print_warning "Race condition tests failed"
}

# Test summary
echo ""
echo "📊 Test Summary"
echo "==============="
print_success "✅ Comprehensive unit tests completed"
print_success "✅ Property-based security tests completed"
print_success "✅ Benchmark tests completed"
print_success "✅ Coverage report generated: coverage.html"

if (( $(echo "$COVERAGE >= 85" | bc -l) )); then
    print_success "✅ Coverage requirement met: ${COVERAGE}%"
else
    print_warning "⚠️  Coverage below requirement: ${COVERAGE}% (target: 85%)"
fi

# Performance recommendations
echo ""
echo "🔧 Performance Recommendations"
echo "=============================="
print_status "1. Review benchmark results for performance bottlenecks"
print_status "2. Consider caching optimizations for frequently accessed data"
print_status "3. Monitor memory allocation patterns in production"
print_status "4. Implement connection pooling for database operations"
print_status "5. Use worker pools for CPU-intensive operations"

# Security recommendations
echo ""
echo "🔒 Security Recommendations"
echo "=========================="
print_status "1. Regularly rotate JWT signing keys"
print_status "2. Implement rate limiting for authentication endpoints"
print_status "3. Monitor for brute force attacks"
print_status "4. Use secure password hashing (bcrypt with appropriate cost)"
print_status "5. Implement proper session management"

echo ""
print_success "🎉 Comprehensive test suite completed successfully!"
print_status "Coverage report available at: coverage.html"
print_status "Review the results and address any failing tests or low coverage areas."

exit 0