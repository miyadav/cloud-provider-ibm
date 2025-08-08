package testing

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	testing "github.com/miyadav/cloud-provider-testing-interface"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	cloudprovider "k8s.io/cloud-provider"
	"k8s.io/klog/v2"
)

// IBMCloudProviderTestImplementation implements the testing.TestInterface
type IBMCloudProviderTestImplementation struct {
	cloudProvider   cloudprovider.Interface
	kubeClient      kubernetes.Interface
	testConfig      *testing.TestConfig
	testResults     *testing.TestResults
	createdNodes    []*v1.Node
	createdServices []*v1.Service
	createdRoutes   []*cloudprovider.Route
}

// NewIBMCloudProviderTestImplementation creates a new test implementation for IBM cloud provider
func NewIBMCloudProviderTestImplementation(cloudProvider cloudprovider.Interface) testing.TestInterface {
	return &IBMCloudProviderTestImplementation{
		cloudProvider:   cloudProvider,
		testResults:     &testing.TestResults{},
		createdNodes:    make([]*v1.Node, 0),
		createdServices: make([]*v1.Service, 0),
		createdRoutes:   make([]*cloudprovider.Route, 0),
	}
}

// SetupTestEnvironment sets up the test environment
func (i *IBMCloudProviderTestImplementation) SetupTestEnvironment(config *testing.TestConfig) error {
	i.testConfig = config

	// Load kubeconfig from ~/.kube/config
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")

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

	// Clean up created services
	for _, service := range i.createdServices {
		if err := i.kubeClient.CoreV1().Services(service.Namespace).Delete(ctx, service.Name, metav1.DeleteOptions{}); err != nil {
			klog.Warningf("Failed to delete service %s/%s: %v", service.Namespace, service.Name, err)
		}
	}

	// Clean up created nodes
	for _, node := range i.createdNodes {
		if err := i.kubeClient.CoreV1().Nodes().Delete(ctx, node.Name, metav1.DeleteOptions{}); err != nil {
			klog.Warningf("Failed to delete node %s: %v", node.Name, err)
		}
	}

	i.testResults.AddLog("Test environment teardown completed")
	return nil
}

// GetCloudProvider returns the cloud provider interface
func (i *IBMCloudProviderTestImplementation) GetCloudProvider() cloudprovider.Interface {
	return i.cloudProvider
}

// CreateTestNode creates a test node
func (i *IBMCloudProviderTestImplementation) CreateTestNode(ctx context.Context, nodeConfig *testing.TestNodeConfig) (*v1.Node, error) {
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
func (i *IBMCloudProviderTestImplementation) CreateTestService(ctx context.Context, serviceConfig *testing.TestServiceConfig) (*v1.Service, error) {
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
func (i *IBMCloudProviderTestImplementation) CreateTestRoute(ctx context.Context, routeConfig *testing.TestRouteConfig) (*cloudprovider.Route, error) {
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
func (i *IBMCloudProviderTestImplementation) WaitForCondition(ctx context.Context, condition testing.TestCondition) error {
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
func (i *IBMCloudProviderTestImplementation) GetTestResults() *testing.TestResults {
	return i.testResults
}

// ResetTestState resets the test state
func (i *IBMCloudProviderTestImplementation) ResetTestState() error {
	// Clean up all created resources
	if err := i.TeardownTestEnvironment(); err != nil {
		return err
	}

	// Reset test results
	i.testResults = &testing.TestResults{}

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
