<!-- markdownlint-disable MD013 -->
# IBM Cloud Provider

This is the IBM Cloud Provider repository which implements the
IBM Cloud Controller Manager (CCM). The IBM CCM can be used to provide IBM Cloud
infrastructure node and load balancer support to
[Kubernetes](https://kubernetes.io/docs/home/) or
[OpenShift](https://docs.openshift.com/) clusters running on
[IBM Cloud](https://cloud.ibm.com/docs). This repository branch is based on
[Kubernetes version v1.34.0-beta.0](https://github.com/kubernetes/kubernetes/tree/v1.34.0-beta.0).

See [CONTRIBUTING.md](./CONTRIBUTING.md) for contribution guidelines.

## Testing

The IBM Cloud Provider includes comprehensive testing using the [cloud-provider-testing-interface](https://github.com/miyadav/cloud-provider-testing-interface) for end-to-end (e2e) testing.

### Unit Testing

The [GO GitHub Action](.github/workflows/go.yml) workflow will run the GO unit tests on pull requests.
The GO unit tests can also be invoked locally by running:

```bash
make test
```

### Integration Testing

Run integration tests that use the testing interface:

```bash
make test-integration
```

### End-to-End Testing

Run comprehensive e2e tests using the test runner:

```bash
# Run all e2e tests
make test-e2e

# Run specific test suites
make test-loadbalancer
make test-nodes
make test-services
```

### Test Runner

Use the test runner for more control over test execution:

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

### Individual Test Suites

Run individual test suites directly:

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

### Prerequisites

Before running e2e tests, ensure you have:

1. **Kubernetes Cluster**: A running Kubernetes cluster with the IBM Cloud Provider installed
2. **Kubeconfig**: Valid kubeconfig file at `~/.kube/config` with cluster access
3. **Go Environment**: Go 1.24 or later installed
4. **IBM Cloud Credentials**: Proper IBM Cloud credentials configured (if testing against real IBM Cloud)

### Test Configuration

Tests can be configured using `config/test-config.yaml` or environment variables:

```bash
export IBM_CLOUD_REGION="us-south"
export IBM_CLOUD_ZONE="us-south-1"
export KUBECONFIG="/path/to/your/kubeconfig"
```

For detailed testing documentation, see [TESTING.md](./TESTING.md).

## Dependencies

GO library dependencies are managed by [Dependabot](.github/dependabot.yml).

## Kubernetes Patch Update Process

The [kube-update GitHub Action](.github/workflows/kube-update.yml) workflow will detect Kubernetes updates
and create pull requests to update the required files.
