package testing

import (
	"context"
	"fmt"
	"time"

	testing "github.com/miyadav/cloud-provider-testing-interface"
)

// TestRunner implements the test runner functionality
type TestRunner struct {
	testInterface testing.TestInterface
	testSuites    []testing.TestSuite
	results       []TestResult
	summary       TestSummary
}

// TestResult represents the result of a single test
type TestResult struct {
	Test     testing.Test
	Success  bool
	Error    error
	Duration time.Duration
	Skip     bool
}

// TestSummary represents the summary of all test results
type TestSummary struct {
	TotalTests    int
	PassedTests   int
	FailedTests   int
	SkippedTests  int
	TotalDuration time.Duration
}

// NewTestRunner creates a new test runner
func NewTestRunner(testInterface testing.TestInterface) *TestRunner {
	return &TestRunner{
		testInterface: testInterface,
		testSuites:    make([]testing.TestSuite, 0),
		results:       make([]TestResult, 0),
		summary:       TestSummary{},
	}
}

// AddTestSuite adds a test suite to the runner
func (tr *TestRunner) AddTestSuite(suite testing.TestSuite) {
	tr.testSuites = append(tr.testSuites, suite)
}

// RunTests runs all test suites
func (tr *TestRunner) RunTests(ctx context.Context) error {
	startTime := time.Now()

	// Setup test environment
	if err := tr.testInterface.SetupTestEnvironment(nil); err != nil {
		return fmt.Errorf("failed to setup test environment: %w", err)
	}

	// Run each test suite
	for _, suite := range tr.testSuites {
		if err := tr.runTestSuite(ctx, suite); err != nil {
			return fmt.Errorf("failed to run test suite %s: %w", suite.Name, err)
		}
	}

	// Teardown test environment
	if err := tr.testInterface.TeardownTestEnvironment(); err != nil {
		return fmt.Errorf("failed to teardown test environment: %w", err)
	}

	tr.summary.TotalDuration = time.Since(startTime)
	return nil
}

// runTestSuite runs a single test suite
func (tr *TestRunner) runTestSuite(ctx context.Context, suite testing.TestSuite) error {
	// Reset test state before running each test suite
	if err := tr.testInterface.ResetTestState(); err != nil {
		return fmt.Errorf("failed to reset test state for suite %s: %w", suite.Name, err)
	}

	for _, test := range suite.Tests {
		result := tr.runTest(ctx, test)
		tr.results = append(tr.results, result)

		// Log test result
		if result.Success {
			fmt.Printf("  ✓ %s: PASSED\n", test.Name)
		} else if result.Skip {
			fmt.Printf("  - %s: SKIPPED\n", test.Name)
		} else {
			fmt.Printf("  ✗ %s: FAILED - %v\n", test.Name, result.Error)
		}

		// Update summary
		tr.summary.TotalTests++
		if result.Skip {
			tr.summary.SkippedTests++
		} else if result.Success {
			tr.summary.PassedTests++
		} else {
			tr.summary.FailedTests++
		}
	}
	return nil
}

// runTest runs a single test
func (tr *TestRunner) runTest(ctx context.Context, test testing.Test) TestResult {
	startTime := time.Now()
	result := TestResult{
		Test: test,
	}

	// Check if test should be skipped
	if test.Skip {
		result.Skip = true
		result.Duration = time.Since(startTime)
		return result
	}

	// Execute the test
	err := test.Run(tr.testInterface)
	result.Duration = time.Since(startTime)

	if err != nil {
		result.Success = false
		result.Error = err
	} else {
		result.Success = true
	}

	return result
}

// GetResults returns all test results
func (tr *TestRunner) GetResults() []TestResult {
	return tr.results
}

// GetSummary returns the test summary
func (tr *TestRunner) GetSummary() TestSummary {
	return tr.summary
}
