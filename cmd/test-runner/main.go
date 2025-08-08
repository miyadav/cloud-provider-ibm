package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.ibm.com/cloud-provider-ibm/ibm"
	ibmtesting "cloud.ibm.com/cloud-provider-ibm/pkg/testing"
)

func main() {
	var (
		testSuite = flag.String("suite", "all", "Test suite to run (all, loadbalancer, nodes, services)")
		verbose   = flag.Bool("verbose", false, "Enable verbose output")
		timeout   = flag.Duration("timeout", 30*time.Minute, "Test timeout")
	)
	flag.Parse()

	// Create cloud provider instance
	// In a real implementation, you would load configuration from a file
	cloudProvider := &ibm.Cloud{}

	// Create test implementation
	testImpl := ibmtesting.NewIBMCloudProviderTestImplementation(cloudProvider)

	// Create test runner
	runner := ibmtesting.NewTestRunner(testImpl)

	// Add test suites based on flag
	switch *testSuite {
	case "all":
		runner.AddTestSuite(ibmtesting.CreateLoadBalancerTestSuite())
		runner.AddTestSuite(ibmtesting.CreateNodeTestSuite())
		runner.AddTestSuite(ibmtesting.CreateServiceTestSuite())
	case "loadbalancer":
		runner.AddTestSuite(ibmtesting.CreateLoadBalancerTestSuite())
	case "nodes":
		runner.AddTestSuite(ibmtesting.CreateNodeTestSuite())
	case "services":
		runner.AddTestSuite(ibmtesting.CreateServiceTestSuite())
	default:
		log.Fatalf("Unknown test suite: %s", *testSuite)
	}

	// Run tests
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	err := runner.RunTests(ctx)
	if err != nil {
		log.Fatalf("Test execution failed: %v", err)
	}

	// Get results
	results := runner.GetResults()
	summary := runner.GetSummary()

	// Print summary
	fmt.Printf("Test Summary:\n")
	fmt.Printf("  Total Tests: %d\n", summary.TotalTests)
	fmt.Printf("  Passed: %d\n", summary.PassedTests)
	fmt.Printf("  Failed: %d\n", summary.FailedTests)
	fmt.Printf("  Skipped: %d\n", summary.SkippedTests)
	fmt.Printf("  Duration: %v\n", summary.TotalDuration)

	// Print detailed results if verbose
	if *verbose {
		fmt.Printf("\nDetailed Results:\n")
		for _, result := range results {
			status := "PASSED"
			if !result.Success {
				status = "FAILED"
			}
			if result.Test.Skip {
				status = "SKIPPED"
			}
			fmt.Printf("  %s: %s (%v)\n", status, result.Test.Name, result.Duration)
			if result.Error != nil {
				fmt.Printf("    Error: %v\n", result.Error)
			}
		}
	}

	// Exit with error code if any tests failed
	if summary.FailedTests > 0 {
		os.Exit(1)
	}
}
