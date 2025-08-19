package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"cloud.ibm.com/cloud-provider-ibm/ibm"
	testinterface "github.com/miyadav/cloud-provider-testing-interface"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	cloudprovider "k8s.io/cloud-provider"
)

// TestConfig represents the test configuration structure
type TestConfig struct {
	Test struct {
		Provider struct {
			Name   string `yaml:"name"`
			Region string `yaml:"region"`
			Zone   string `yaml:"zone"`
		} `yaml:"provider"`
		Cluster struct {
			Name    string `yaml:"name"`
			Version string `yaml:"version"`
		} `yaml:"cluster"`
		Resources struct {
			Cleanup bool   `yaml:"cleanup"`
			Timeout string `yaml:"timeout"`
		} `yaml:"resources"`
		ExternalServices struct {
			Mock bool `yaml:"mock"`
		} `yaml:"external_services"`
		Kubeconfig struct {
			Path string `yaml:"path"`
		} `yaml:"kubeconfig"`
	} `yaml:"test"`
}

// IBMCloudProviderTestImplementation implements the testinterface.TestInterface
type IBMCloudProviderTestImplementation struct {
	cloudProvider   *ibm.Cloud
	kubeClient      *kubernetes.Clientset
	testConfig      *TestConfig
	testResults     *testinterface.TestResults
	createdNodes    []*v1.Node
	createdServices []*v1.Service
	createdRoutes   []*cloudprovider.Route
}

// NewIBMCloudProviderTestImplementation creates a new test implementation for IBM cloud provider
func NewIBMCloudProviderTestImplementation(cloudProvider *ibm.Cloud) testinterface.TestInterface {
	return &IBMCloudProviderTestImplementation{
		cloudProvider:   cloudProvider,
		testResults:     &testinterface.TestResults{},
		createdNodes:    make([]*v1.Node, 0),
		createdServices: make([]*v1.Service, 0),
		createdRoutes:   make([]*cloudprovider.Route, 0),
	}
}

// loadTestConfig loads the test configuration from config/test-config.yaml
func (i *IBMCloudProviderTestImplementation) loadTestConfig() error {
	// Try multiple possible paths for the config file
	possiblePaths := []string{
		"config/test-config.yaml",
		"../config/test-config.yaml",
		"../../config/test-config.yaml",
	}

	var configPath string
	for _, path := range possiblePaths {
		if _, err := os.Stat(path); err == nil {
			configPath = path
			break
		}
	}

	if configPath == "" {
		klog.Warningf("Test config file not found in any of the expected locations: %v, using default configuration", possiblePaths)
		return nil
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read test config file: %w", err)
	}

	var config TestConfig
	if err := yaml.Unmarshal(configData, &config); err != nil {
		return fmt.Errorf("failed to parse test config file: %w", err)
	}

	i.testConfig = &config
	return nil
}

// SetupTestEnvironment sets up the test environment
func (i *IBMCloudProviderTestImplementation) SetupTestEnvironment(config *testinterface.TestConfig) error {
	// Load test configuration first
	if err := i.loadTestConfig(); err != nil {
		klog.Warningf("Failed to load test config: %v, continuing with default settings", err)
	}

	// Check if mock mode is enabled
	if i.testConfig != nil && i.testConfig.Test.ExternalServices.Mock {
		klog.Infof("Mock mode enabled, skipping real kubeconfig setup")
		i.kubeClient = nil
		i.testResults.AddLog("Test environment setup completed with mock client (mock mode enabled)")
		return nil
	}

	// Determine kubeconfig path
	var kubeconfig string
	if i.testConfig != nil && i.testConfig.Test.Kubeconfig.Path != "" {
		kubeconfig = i.testConfig.Test.Kubeconfig.Path
		// Expand ~ to home directory
		if kubeconfig == "~/.kube/config" {
			kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		}
	} else {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
	}

	// Load kubernetes client
	clientConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		// If kubeconfig is not available, create a mock environment for testing
		klog.Warningf("Failed to load kubeconfig from %s: %v. Using mock environment for testing.", kubeconfig, err)

		// Create a mock kubernetes client for testing
		// In a real test environment, you would use a test server or mock client
		i.kubeClient = nil
		i.testResults.AddLog("Test environment setup completed with mock client")
		return nil
	}

	i.kubeClient, err = kubernetes.NewForConfig(clientConfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Clean up any existing test resources before starting
	if err := i.cleanupExistingTestResources(); err != nil {
		klog.Warningf("Failed to cleanup existing test resources: %v", err)
	}

	i.testResults.AddLog("Test environment setup completed")
	return nil
}

// TeardownTestEnvironment cleans up the test environment
func (i *IBMCloudProviderTestImplementation) TeardownTestEnvironment() error {
	ctx := context.Background()

	// If using mock environment, just log the cleanup
	if i.kubeClient == nil {
		i.testResults.AddLog("Test environment teardown completed (mock environment)")
		return nil
	}

	// Clean up created services with retry logic
	for _, service := range i.createdServices {
		if err := i.cleanupService(ctx, service); err != nil {
			klog.Warningf("Failed to delete service %s/%s: %v", service.Namespace, service.Name, err)
		}
	}

	// Clean up created nodes with retry logic
	for _, node := range i.createdNodes {
		if err := i.cleanupNode(ctx, node); err != nil {
			klog.Warningf("Failed to delete node %s: %v", node.Name, err)
		}
	}

	i.testResults.AddLog("Test environment teardown completed")
	return nil
}

// cleanupService cleans up a service with retry logic
func (i *IBMCloudProviderTestImplementation) cleanupService(ctx context.Context, service *v1.Service) error {
	// First, check if the service exists and is not already being deleted
	currentService, err := i.kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
	if err != nil {
		// Service doesn't exist, nothing to clean up
		return nil
	}

	// Check if service is already being deleted
	if currentService.DeletionTimestamp != nil {
		klog.Infof("Service %s/%s is already being deleted, waiting for completion", service.Namespace, service.Name)
	} else {
		// Delete the service
		err = i.kubeClient.CoreV1().Services(service.Namespace).Delete(ctx, service.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		klog.Infof("Initiated deletion of service %s/%s", service.Namespace, service.Name)
	}

	// Wait for the service to be fully deleted using WaitForCondition
	condition := testinterface.TestCondition{
		Type:    "ServiceDeletion",
		Timeout: 2 * time.Minute, // Increased timeout for LoadBalancer cleanup
		CheckFunction: func() (bool, error) {
			_, err := i.kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
			if err != nil {
				// Service is deleted
				klog.Infof("Service %s/%s has been successfully deleted", service.Namespace, service.Name)
				return true, nil
			}
			klog.V(2).Infof("Service %s/%s still exists, waiting for deletion to complete...", service.Namespace, service.Name)
			return false, nil
		},
	}

	return i.WaitForCondition(ctx, condition)
}

// cleanupNode cleans up a node with retry logic
func (i *IBMCloudProviderTestImplementation) cleanupNode(ctx context.Context, node *v1.Node) error {
	// First, check if the node exists and is not already being deleted
	currentNode, err := i.kubeClient.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
	if err != nil {
		// Node doesn't exist, nothing to clean up
		return nil
	}

	// Check if node is already being deleted
	if currentNode.DeletionTimestamp != nil {
		klog.Infof("Node %s is already being deleted, waiting for completion", node.Name)
	} else {
		// Delete the node
		err = i.kubeClient.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{})
		if err != nil {
			return err
		}
		klog.Infof("Initiated deletion of node %s", node.Name)
	}

	// Wait for the node to be fully deleted using WaitForCondition
	condition := testinterface.TestCondition{
		Type:    "NodeDeletion",
		Timeout: 1 * time.Minute, // Increased timeout for node cleanup
		CheckFunction: func() (bool, error) {
			_, err := i.kubeClient.CoreV1().Nodes().Get(ctx, node.Name, metav1.GetOptions{})
			if err != nil {
				// Node is deleted
				klog.Infof("Node %s has been successfully deleted", node.Name)
				return true, nil
			}
			klog.V(2).Infof("Node %s still exists, waiting for deletion to complete...", node.Name)
			return false, nil
		},
	}

	return i.WaitForCondition(ctx, condition)
}

// cleanupExistingTestResources cleans up any existing test resources that might conflict
func (i *IBMCloudProviderTestImplementation) cleanupExistingTestResources() error {
	if i.kubeClient == nil {
		return nil // No cleanup needed for mock environment
	}

	ctx := context.Background()

	// Clean up existing test services
	services, err := i.kubeClient.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list services: %w", err)
	}

	for _, service := range services.Items {
		// Check if this is a test service (by name pattern)
		if isTestService(service.Name) {
			klog.Infof("Cleaning up existing test service: %s/%s", service.Namespace, service.Name)
			if err := i.cleanupService(ctx, &service); err != nil {
				klog.Warningf("Failed to cleanup existing test service %s/%s: %v", service.Namespace, service.Name, err)
			}
		}
	}

	// Clean up existing test nodes
	nodes, err := i.kubeClient.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	for _, node := range nodes.Items {
		// Check if this is a test node (by name pattern)
		if isTestNode(node.Name) {
			klog.Infof("Cleaning up existing test node: %s", node.Name)
			if err := i.cleanupNode(ctx, &node); err != nil {
				klog.Warningf("Failed to cleanup existing test node %s: %v", node.Name, err)
			}
		}
	}

	return nil
}

// isTestService checks if a service name matches test service patterns
func isTestService(name string) bool {
	testPatterns := []string{
		"test-lb-service",
		"test-lb-update",
		"test-lb-delete",
		"test-service",
	}

	for _, pattern := range testPatterns {
		if name == pattern {
			return true
		}
	}
	return false
}

// isTestNode checks if a node name matches test node patterns
func isTestNode(name string) bool {
	testPatterns := []string{
		"test-node-",
		"testnode",
	}

	for _, pattern := range testPatterns {
		if len(name) >= len(pattern) && name[:len(pattern)] == pattern {
			return true
		}
	}
	return false
}

// GetCloudProvider returns the cloud provider interface
func (i *IBMCloudProviderTestImplementation) GetCloudProvider() cloudprovider.Interface {
	return i.cloudProvider
}

// waitForLoadBalancerExternalIP waits for a LoadBalancer service to get an external IP using WaitForCondition
func (i *IBMCloudProviderTestImplementation) waitForLoadBalancerExternalIP(ctx context.Context, service *v1.Service) error {
	if i.kubeClient == nil {
		// Mock environment - simulate external IP assignment
		i.testResults.AddLog(fmt.Sprintf("Simulating external IP assignment for service %s/%s (mock mode)", service.Namespace, service.Name))
		return nil
	}

	// Create a condition to check for external IP assignment
	condition := testinterface.TestCondition{
		Type:    "LoadBalancerExternalIP",
		Timeout: 5 * time.Minute,
		CheckFunction: func() (bool, error) {
			// Get the current service status
			currentService, err := i.kubeClient.CoreV1().Services(service.Namespace).Get(ctx, service.Name, metav1.GetOptions{})
			if err != nil {
				return false, fmt.Errorf("failed to get service %s/%s: %w", service.Namespace, service.Name, err)
			}

			// Check if external IP is assigned
			if len(currentService.Status.LoadBalancer.Ingress) > 0 {
				externalIP := currentService.Status.LoadBalancer.Ingress[0].IP
				if externalIP != "" {
					klog.Infof("External IP %s assigned to service %s/%s", externalIP, service.Namespace, service.Name)
					i.testResults.AddLog(fmt.Sprintf("External IP %s assigned to service %s/%s", externalIP, service.Namespace, service.Name))
					return true, nil
				}
			}

			klog.V(2).Infof("Waiting for external IP assignment for service %s/%s...", service.Namespace, service.Name)
			return false, nil
		},
	}

	return i.WaitForCondition(ctx, condition)
}

// CreateTestNode creates a test node
func (i *IBMCloudProviderTestImplementation) CreateTestNode(ctx context.Context, nodeConfig *testinterface.TestNodeConfig) (*v1.Node, error) {
	node := &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nodeConfig.Name,
			Labels:      nodeConfig.Labels,
			Annotations: nodeConfig.Annotations,
		},
		Spec: v1.NodeSpec{
			Unschedulable: false,
			ProviderID:    nodeConfig.ProviderID,
		},
		Status: v1.NodeStatus{
			Capacity: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("4"),
				v1.ResourceMemory: resource.MustParse("8Gi"),
				v1.ResourcePods:   resource.MustParse("110"),
			},
			Allocatable: v1.ResourceList{
				v1.ResourceCPU:    resource.MustParse("4"),
				v1.ResourceMemory: resource.MustParse("8Gi"),
				v1.ResourcePods:   resource.MustParse("110"),
			},
			Conditions: nodeConfig.Conditions,
			Addresses:  nodeConfig.Addresses,
		},
	}

	// Set default conditions if not provided
	if len(node.Status.Conditions) == 0 {
		node.Status.Conditions = []v1.NodeCondition{
			{
				Type:   v1.NodeReady,
				Status: v1.ConditionTrue,
			},
		}
	}

	// Set default labels if not provided
	if node.Labels == nil {
		node.Labels = map[string]string{
			"kubernetes.io/hostname": nodeConfig.Name,
			"kubernetes.io/os":       "linux",
			"kubernetes.io/arch":     "amd64",
		}
	}

	// If using mock environment, just simulate the creation
	if i.kubeClient == nil {
		// Check if node already exists in mock environment
		for _, existingNode := range i.createdNodes {
			if existingNode.Name == nodeConfig.Name {
				return nil, fmt.Errorf("node %s already exists in mock environment", nodeConfig.Name)
			}
		}

		i.createdNodes = append(i.createdNodes, node)
		i.testResults.AddLog(fmt.Sprintf("Created test node (mock): %s", nodeConfig.Name))
		return node, nil
	}

	createdNode, err := i.kubeClient.CoreV1().Nodes().Create(ctx, node, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create test node: %w", err)
	}

	i.createdNodes = append(i.createdNodes, createdNode)
	i.testResults.AddLog(fmt.Sprintf("Created test node: %s", nodeConfig.Name))

	return createdNode, nil
}

// DeleteTestNode deletes a test node
func (i *IBMCloudProviderTestImplementation) DeleteTestNode(ctx context.Context, nodeName string) error {
	// If using mock environment, just simulate the deletion
	if i.kubeClient == nil {
		// Remove from created nodes list
		for idx, node := range i.createdNodes {
			if node.Name == nodeName {
				i.createdNodes = append(i.createdNodes[:idx], i.createdNodes[idx+1:]...)
				break
			}
		}
		i.testResults.AddLog(fmt.Sprintf("Deleted test node (mock): %s", nodeName))
		return nil
	}

	err := i.kubeClient.CoreV1().Nodes().Delete(ctx, nodeName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete test node %s: %w", nodeName, err)
	}

	// Remove from created nodes list
	for idx, node := range i.createdNodes {
		if node.Name == nodeName {
			i.createdNodes = append(i.createdNodes[:idx], i.createdNodes[idx+1:]...)
			break
		}
	}

	i.testResults.AddLog(fmt.Sprintf("Deleted test node: %s", nodeName))
	return nil
}

// CreateTestService creates a test service
func (i *IBMCloudProviderTestImplementation) CreateTestService(ctx context.Context, serviceConfig *testinterface.TestServiceConfig) (*v1.Service, error) {
	service := &v1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        serviceConfig.Name,
			Namespace:   serviceConfig.Namespace,
			Labels:      serviceConfig.Labels,
			Annotations: serviceConfig.Annotations,
		},
		Spec: v1.ServiceSpec{
			Type:                  serviceConfig.Type,
			Ports:                 serviceConfig.Ports,
			LoadBalancerIP:        serviceConfig.LoadBalancerIP,
			ExternalTrafficPolicy: serviceConfig.ExternalTrafficPolicy,
			InternalTrafficPolicy: serviceConfig.InternalTrafficPolicy,
			Selector:              map[string]string{"app": serviceConfig.Name},
		},
	}

	// Set default annotations for IBM cloud provider
	if service.Annotations == nil {
		service.Annotations = make(map[string]string)
	}
	service.Annotations["service.kubernetes.io/ibm-load-balancer-cloud-provider-ip-type"] = "public"

	// Set default ports if not provided
	if len(service.Spec.Ports) == 0 {
		service.Spec.Ports = []v1.ServicePort{
			{
				Port:       80,
				TargetPort: intstr.FromInt32(8080),
				Protocol:   v1.ProtocolTCP,
			},
		}
	}

	// If using mock environment, just simulate the creation
	if i.kubeClient == nil {
		// Check if service already exists in mock environment
		for _, existingService := range i.createdServices {
			if existingService.Namespace == serviceConfig.Namespace && existingService.Name == serviceConfig.Name {
				return nil, fmt.Errorf("service %s/%s already exists in mock environment", serviceConfig.Namespace, serviceConfig.Name)
			}
		}

		i.createdServices = append(i.createdServices, service)
		i.testResults.AddLog(fmt.Sprintf("Created test service (mock): %s/%s", serviceConfig.Namespace, serviceConfig.Name))
		return service, nil
	}

	createdService, err := i.kubeClient.CoreV1().Services(serviceConfig.Namespace).Create(ctx, service, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to create test service: %w", err)
	}

	i.createdServices = append(i.createdServices, createdService)
	i.testResults.AddLog(fmt.Sprintf("Created test service: %s/%s", serviceConfig.Namespace, serviceConfig.Name))

	// If this is a LoadBalancer service, wait for external IP to be assigned
	if serviceConfig.Type == v1.ServiceTypeLoadBalancer {
		if err := i.waitForLoadBalancerExternalIP(ctx, createdService); err != nil {
			return nil, fmt.Errorf("failed to wait for load balancer external IP: %w", err)
		}
	}

	return createdService, nil
}

// DeleteTestService deletes a test service
func (i *IBMCloudProviderTestImplementation) DeleteTestService(ctx context.Context, serviceName string) error {
	// Find the service in our list to get the namespace
	var namespace string
	for _, service := range i.createdServices {
		if service.Name == serviceName {
			namespace = service.Namespace
			break
		}
	}

	if namespace == "" {
		return fmt.Errorf("service %s not found in created services", serviceName)
	}

	// If using mock environment, just simulate the deletion
	if i.kubeClient == nil {
		// Remove from created services list
		for idx, service := range i.createdServices {
			if service.Namespace == namespace && service.Name == serviceName {
				i.createdServices = append(i.createdServices[:idx], i.createdServices[idx+1:]...)
				break
			}
		}
		i.testResults.AddLog(fmt.Sprintf("Deleted test service (mock): %s/%s", namespace, serviceName))
		return nil
	}

	err := i.kubeClient.CoreV1().Services(namespace).Delete(ctx, serviceName, metav1.DeleteOptions{})
	if err != nil {
		return fmt.Errorf("failed to delete test service %s/%s: %w", namespace, serviceName, err)
	}

	// Remove from created services list
	for idx, service := range i.createdServices {
		if service.Namespace == namespace && service.Name == serviceName {
			i.createdServices = append(i.createdServices[:idx], i.createdServices[idx+1:]...)
			break
		}
	}

	i.testResults.AddLog(fmt.Sprintf("Deleted test service: %s/%s", namespace, serviceName))
	return nil
}

// CreateTestRoute creates a test route
func (i *IBMCloudProviderTestImplementation) CreateTestRoute(ctx context.Context, routeConfig *testinterface.TestRouteConfig) (*cloudprovider.Route, error) {
	route := &cloudprovider.Route{
		Name:            routeConfig.Name,
		TargetNode:      routeConfig.TargetNode,
		DestinationCIDR: routeConfig.DestinationCIDR,
		Blackhole:       routeConfig.Blackhole,
	}

	i.createdRoutes = append(i.createdRoutes, route)
	i.testResults.AddLog(fmt.Sprintf("Created test route: %s", routeConfig.Name))

	return route, nil
}

// DeleteTestRoute deletes a test route
func (i *IBMCloudProviderTestImplementation) DeleteTestRoute(ctx context.Context, routeName string) error {
	// Remove from created routes list
	for idx, route := range i.createdRoutes {
		if route.Name == routeName {
			i.createdRoutes = append(i.createdRoutes[:idx], i.createdRoutes[idx+1:]...)
			break
		}
	}

	i.testResults.AddLog(fmt.Sprintf("Deleted test route: %s", routeName))
	return nil
}

// WaitForCondition waits for a condition to be met
func (i *IBMCloudProviderTestImplementation) WaitForCondition(ctx context.Context, condition testinterface.TestCondition) error {
	timeout := condition.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for condition: %s", condition.Type)
		case <-ticker.C:
			if condition.CheckFunction != nil {
				met, err := condition.CheckFunction()
				if err != nil {
					continue
				}
				if met {
					return nil
				}
			}
		}
	}
}

// GetTestResults returns the test results
func (i *IBMCloudProviderTestImplementation) GetTestResults() *testinterface.TestResults {
	return i.testResults
}

// ResetTestState resets the test state
func (i *IBMCloudProviderTestImplementation) ResetTestState() error {
	// Clean up all created resources
	if err := i.TeardownTestEnvironment(); err != nil {
		return err
	}

	// Reset test results
	i.testResults = &testinterface.TestResults{}

	// Clear all created resources
	i.createdNodes = make([]*v1.Node, 0)
	i.createdServices = make([]*v1.Service, 0)
	i.createdRoutes = make([]*cloudprovider.Route, 0)

	i.testResults.AddLog("Test state reset completed")
	return nil
}

// GetKubeClient returns the kubernetes client
func (i *IBMCloudProviderTestImplementation) GetKubeClient() kubernetes.Interface {
	return i.kubeClient
}
