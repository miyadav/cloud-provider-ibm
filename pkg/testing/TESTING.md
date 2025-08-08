# Testing IBM Cloud Provider

This document describes how to run end-to-end (e2e) tests for the IBM Cloud Provider using the [cloud-provider-testing-interface](https://github.com/miyadav/cloud-provider-testing-interface).

## Overview

The IBM Cloud Provider uses the cloud-provider-testing-interface for comprehensive e2e testing. This interface provides a cloud-agnostic way to test cloud provider implementations by abstracting away cloud-specific details and focusing on the behavior and functionality that should be consistent across all cloud providers.

## Prerequisites

Before running the tests, ensure you have:

1. **Kubernetes Cluster**: A running Kubernetes cluster with the IBM Cloud Provider installed
2. **Kubeconfig**: Valid kubeconfig file at `~/.kube/config` with cluster access
3. **Go Environment**: Go 1.24 or later installed
4. **IBM Cloud Credentials**: Proper IBM Cloud credentials configured (if testing against real IBM Cloud)

## Test Structure

The testing implementation consists of:

- **Interface Implementation** (`pkg/testing/interface.go`): Implements the cloud-provider-testing-interface for IBM Cloud
- **Test Suites** (`pkg/testing/test_suites.go`): Defines test suites for different cloud provider functionalities
- **Integration Tests** (`pkg/testing/integration_test.go`): Go test functions that use the testing interface
- **Test Runner** (`cmd/test-runner/main.go`): Command-line tool for running tests
- **Configuration** (`config/test-config.yaml`): Test configuration file

## Running Tests

### 1. Unit Tests

Run the standard Go unit tests:

```bash
go test -v ./pkg/testing/...
```

### 2. Integration Tests

Run the integration tests that use the testing interface:

```bash
go test -v ./pkg/testing/ -run TestIntegration
```

### 3. Test Runner

Use the test runner to execute specific test suites:

```bash
# Build the test runner
go build -o test-runner cmd/test-runner/main.go

# Run all test suites
./test-runner -suite=all -verbose

# Run specific test suites
./test-runner -suite=loadbalancer -verbose
./test-runner -suite=nodes -verbose
./test-runner -suite=services -verbose

# Run with custom timeout
./test-runner -suite=all -timeout=1h -verbose
```

### 4. Individual Test Suites

You can also run individual test suites:

```bash
# Load Balancer tests
go test -v ./pkg/testing/ -run TestLoadBalancerIntegration

# Node tests
go test -v ./pkg/testing/ -run TestNodeIntegration

# Service tests
go test -v ./pkg/testing/ -run TestServiceIntegration

# All integration tests
go test -v ./pkg/testing/ -run TestFullIntegration
```

## Test Suites

### Load Balancer Test Suite

Tests load balancer functionality:

- **CreateLoadBalancer**: Creates a load balancer service and verifies it's provisioned
- **UpdateLoadBalancer**: Updates an existing load balancer service
- **DeleteLoadBalancer**: Deletes a load balancer service

### Node Test Suite

Tests node management functionality:

- **CreateNode**: Creates a test node in the cluster
- **DeleteNode**: Deletes a test node from the cluster

### Service Test Suite

Tests service management functionality:

- **CreateService**: Creates a test service
- **DeleteService**: Deletes a test service

## Configuration

### Test Configuration

The test configuration is defined in `config/test-config.yaml`:

```yaml
test:
  provider:
    name: "ibm"
    region: "us-south"
    zone: "us-south-1"
  
  cluster:
    name: "test-cluster"
    version: "1.24.0"
  
  resources:
    cleanup: true
    timeout: "5m"
  
  external_services:
    mock: false  # Use real cloud services for integration tests

  kubeconfig:
    path: "~/.kube/config"  # Path to kubeconfig file
```

### Environment Variables

You can override configuration using environment variables:

```bash
export IBM_CLOUD_REGION="us-south"
export IBM_CLOUD_ZONE="us-south-1"
export KUBECONFIG="/path/to/your/kubeconfig"
```

## Test Implementation Details

### IBM Cloud Provider Test Implementation

The `IBMCloudProviderTestImplementation` implements the `testing.TestInterface` and provides:

- **SetupTestEnvironment**: Initializes the test environment with Kubernetes client
- **TeardownTestEnvironment**: Cleans up test resources
- **CreateTestNode**: Creates test nodes with proper IBM Cloud annotations
- **DeleteTestNode**: Deletes test nodes
- **CreateTestService**: Creates test services with IBM Cloud load balancer annotations
- **DeleteTestService**: Deletes test services
- **CreateTestRoute**: Creates test routes (IBM Cloud doesn't support routes)
- **DeleteTestRoute**: Deletes test routes
- **WaitForCondition**: Waits for specific conditions to be met
- **ResetTestState**: Resets the test state to a clean state

### Test Results

The test runner provides detailed results including:

- **Test Summary**: Total tests, passed, failed, and skipped counts
- **Duration**: Total test execution time
- **Detailed Results**: Individual test results with status and duration
- **Logs**: Test execution logs for debugging

## Debugging Tests

### Verbose Output

Enable verbose output to see detailed test execution:

```bash
./test-runner -suite=all -verbose
```

### Test Logs

Test logs are available through the test results:

```go
results := runner.GetTestResults()
for _, log := range results.Logs {
    fmt.Println(log)
}
```

### Individual Test Debugging

To debug individual tests, you can run them with verbose output:

```bash
go test -v ./pkg/testing/ -run TestLoadBalancerIntegration -timeout=30m
```

## CI/CD Integration

### GitHub Actions

Add the test runner to your CI/CD pipeline:

```yaml
# .github/workflows/test.yml
name: Cloud Provider Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    
    steps:
    - uses: actions/checkout@v3
    
    - name: Set up Go
      uses: actions/setup-go@v4
      with:
        go-version: '1.24'
    
    - name: Install dependencies
      run: go mod download
    
    - name: Run unit tests
      run: go test -v ./pkg/...
    
    - name: Run integration tests
      run: go test -v ./pkg/testing/...
      env:
        KUBECONFIG: ${{ secrets.KUBECONFIG }}
    
    - name: Run test runner
      run: go run cmd/test-runner/main.go -suite=all -verbose
      env:
        KUBECONFIG: ${{ secrets.KUBECONFIG }}
```

### Local Development

For local development, you can run tests against a local cluster:

```bash
# Start a local cluster (e.g., kind, minikube)
kind create cluster

# Run tests
./test-runner -suite=all -verbose
```

## Troubleshooting

### Common Issues

1. **Kubeconfig not found**: Ensure your kubeconfig is at `~/.kube/config` or set the `KUBECONFIG` environment variable
2. **Permission denied**: Ensure your kubeconfig has proper permissions to create/delete resources
3. **Timeout errors**: Increase the timeout using the `-timeout` flag
4. **Resource cleanup failures**: Check if resources were properly cleaned up in the cluster

### Debug Commands

```bash
# Check cluster connectivity
kubectl cluster-info

# List test resources
kubectl get nodes -l test=true
kubectl get services -l test=true

# Check test logs
kubectl logs -n kube-system -l app=cloud-controller-manager
```

## Contributing

When adding new tests:

1. **Create test functions** in `pkg/testing/test_suites.go`
2. **Add test suites** to the appropriate test suite creation function
3. **Update integration tests** in `pkg/testing/integration_test.go`
4. **Update test runner** in `cmd/test-runner/main.go` if needed
5. **Add documentation** to this file

### Test Guidelines

- **Isolation**: Each test should be independent and not rely on other tests
- **Cleanup**: Always clean up resources created during tests
- **Timeouts**: Set appropriate timeouts for long-running operations
- **Logging**: Add meaningful log messages for debugging
- **Error handling**: Provide clear error messages for failures

## References

- [cloud-provider-testing-interface](https://github.com/miyadav/cloud-provider-testing-interface)
- [Kubernetes Cloud Provider Interface](https://kubernetes.io/docs/concepts/architecture/cloud-controller/)
- [IBM Cloud Provider Documentation](https://cloud.ibm.com/docs/containers?topic=containers-cloud_provider)
