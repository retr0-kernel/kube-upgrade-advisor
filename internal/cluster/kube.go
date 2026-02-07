package cluster

import (
	"context"
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeClient handles Kubernetes cluster operations
type KubeClient struct {
	clientset *kubernetes.Clientset
	config    *rest.Config
}

// NewKubeClient creates a new Kubernetes client from kubeconfig
func NewKubeClient(kubeconfig string) (*KubeClient, error) {
	var config *rest.Config
	var err error

	if kubeconfig == "" {
		// Use in-cluster config
		config, err = rest.InClusterConfig()
		if err != nil {
			return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
		}
	} else {
		// Use kubeconfig file
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create clientset: %w", err)
	}

	return &KubeClient{
		clientset: clientset,
		config:    config,
	}, nil
}

// NewKubeClientInCluster creates a new Kubernetes client using in-cluster config
func NewKubeClientInCluster() (*KubeClient, error) {
	return NewKubeClient("")
}

// GetClusterVersion retrieves the Kubernetes cluster version
func (k *KubeClient) GetClusterVersion(ctx context.Context) (string, error) {
	version, err := k.clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	return version.GitVersion, nil
}

// GetServerVersionInfo retrieves detailed server version information
func (k *KubeClient) GetServerVersionInfo(ctx context.Context) (*ServerVersionInfo, error) {
	version, err := k.clientset.Discovery().ServerVersion()
	if err != nil {
		return nil, fmt.Errorf("failed to get server version: %w", err)
	}

	return &ServerVersionInfo{
		Major:        version.Major,
		Minor:        version.Minor,
		GitVersion:   version.GitVersion,
		GitCommit:    version.GitCommit,
		GitTreeState: version.GitTreeState,
		BuildDate:    version.BuildDate,
		GoVersion:    version.GoVersion,
		Compiler:     version.Compiler,
		Platform:     version.Platform,
	}, nil
}

// ListAPIResources lists all API resources in the cluster
func (k *KubeClient) ListAPIResources(ctx context.Context) ([]APIResource, error) {
	_, apiResourceLists, err := k.clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		return nil, fmt.Errorf("failed to list API resources: %w", err)
	}

	var resources []APIResource
	for _, list := range apiResourceLists {
		for _, resource := range list.APIResources {
			resources = append(resources, APIResource{
				Name:       resource.Name,
				Kind:       resource.Kind,
				Group:      list.GroupVersion,
				Namespaced: resource.Namespaced,
				Verbs:      resource.Verbs,
			})
		}
	}

	return resources, nil
}

// GetClientset returns the underlying Kubernetes clientset
func (k *KubeClient) GetClientset() *kubernetes.Clientset {
	return k.clientset
}

// GetConfig returns the REST config
func (k *KubeClient) GetConfig() *rest.Config {
	return k.config
}

// ServerVersionInfo represents detailed server version information
type ServerVersionInfo struct {
	Major        string
	Minor        string
	GitVersion   string
	GitCommit    string
	GitTreeState string
	BuildDate    string
	GoVersion    string
	Compiler     string
	Platform     string
}

// APIResource represents a Kubernetes API resource
type APIResource struct {
	Name       string
	Kind       string
	Group      string
	Namespaced bool
	Verbs      []string
}
