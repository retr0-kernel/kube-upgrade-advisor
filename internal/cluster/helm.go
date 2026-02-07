package cluster

import (
	"context"
	"fmt"
	"log"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/release"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// HelmRelease represents a Helm release in the cluster
type HelmRelease struct {
	Name         string
	Namespace    string
	Chart        string
	ChartVersion string
	AppVersion   string
	Status       string
	Revision     int
	Updated      string
	Description  string
}

// HelmClient handles Helm operations
type HelmClient struct {
	settings *cli.EnvSettings
}

// NewHelmClient creates a new Helm client
func NewHelmClient() (*HelmClient, error) {
	settings := cli.New()
	return &HelmClient{
		settings: settings,
	}, nil
}

// NewHelmClientWithKubeconfig creates a new Helm client with specific kubeconfig
func NewHelmClientWithKubeconfig(kubeconfig string) (*HelmClient, error) {
	settings := cli.New()
	if kubeconfig != "" {
		settings.KubeConfig = kubeconfig
	}
	return &HelmClient{
		settings: settings,
	}, nil
}

// ListReleases lists all Helm releases across all namespaces
func (h *HelmClient) ListReleases(ctx context.Context) ([]HelmRelease, error) {
	return h.ListReleasesInNamespace(ctx, "")
}

// ListReleasesInNamespace lists Helm releases in a specific namespace
// Pass empty string for all namespaces
func (h *HelmClient) ListReleasesInNamespace(ctx context.Context, namespace string) ([]HelmRelease, error) {
	var releases []HelmRelease

	// If namespace is empty, we need to list across all namespaces
	if namespace == "" {
		// Get all namespaces by iterating through them
		// Helm doesn't have a built-in "all namespaces" feature in action config
		// So we use the --all-namespaces equivalent approach

		actionConfig, err := h.getActionConfig("")
		if err != nil {
			return nil, fmt.Errorf("failed to get action config: %w", err)
		}

		listClient := action.NewList(actionConfig)
		listClient.AllNamespaces = true
		listClient.All = true // Include all releases (deployed, failed, etc.)

		results, err := listClient.Run()
		if err != nil {
			return nil, fmt.Errorf("failed to list releases: %w", err)
		}

		releases = h.convertReleases(results)
	} else {
		// List releases in specific namespace
		actionConfig, err := h.getActionConfig(namespace)
		if err != nil {
			return nil, fmt.Errorf("failed to get action config for namespace %s: %w", namespace, err)
		}

		listClient := action.NewList(actionConfig)
		listClient.All = true

		results, err := listClient.Run()
		if err != nil {
			return nil, fmt.Errorf("failed to list releases in namespace %s: %w", namespace, err)
		}

		releases = h.convertReleases(results)
	}

	return releases, nil
}

// GetRelease retrieves a specific Helm release
func (h *HelmClient) GetRelease(ctx context.Context, name, namespace string) (*HelmRelease, error) {
	actionConfig, err := h.getActionConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get action config: %w", err)
	}

	getClient := action.NewGet(actionConfig)
	rel, err := getClient.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get release %s: %w", name, err)
	}

	releases := h.convertReleases([]*release.Release{rel})
	if len(releases) == 0 {
		return nil, fmt.Errorf("release not found")
	}

	return &releases[0], nil
}

// GetReleaseManifest retrieves the manifest for a specific release
func (h *HelmClient) GetReleaseManifest(ctx context.Context, name, namespace string) (string, error) {
	actionConfig, err := h.getActionConfig(namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get action config: %w", err)
	}

	getClient := action.NewGet(actionConfig)
	rel, err := getClient.Run(name)
	if err != nil {
		return "", fmt.Errorf("failed to get release %s: %w", name, err)
	}

	return rel.Manifest, nil
}

// GetReleaseValues retrieves the values for a specific release
func (h *HelmClient) GetReleaseValues(ctx context.Context, name, namespace string) (map[string]interface{}, error) {
	actionConfig, err := h.getActionConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get action config: %w", err)
	}

	getClient := action.NewGetValues(actionConfig)
	values, err := getClient.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get values for release %s: %w", name, err)
	}

	return values, nil
}

// getActionConfig creates an action configuration for Helm operations
func (h *HelmClient) getActionConfig(namespace string) (*action.Configuration, error) {
	actionConfig := new(action.Configuration)

	// Set namespace
	if namespace == "" {
		namespace = h.settings.Namespace()
	}

	// Create config flags
	configFlags := &genericclioptions.ConfigFlags{
		Namespace:  &namespace,
		KubeConfig: &h.settings.KubeConfig,
	}

	// Initialize action configuration
	if err := actionConfig.Init(configFlags, namespace, "secret", log.Printf); err != nil {
		return nil, fmt.Errorf("failed to initialize action config: %w", err)
	}

	return actionConfig, nil
}

// convertReleases converts Helm releases to our internal representation
func (h *HelmClient) convertReleases(releases []*release.Release) []HelmRelease {
	results := make([]HelmRelease, 0, len(releases))

	for _, rel := range releases {
		chartVersion := ""
		chartName := ""
		appVersion := ""

		if rel.Chart != nil {
			if rel.Chart.Metadata != nil {
				chartName = rel.Chart.Metadata.Name
				chartVersion = rel.Chart.Metadata.Version
				appVersion = rel.Chart.Metadata.AppVersion
			}
		}

		results = append(results, HelmRelease{
			Name:         rel.Name,
			Namespace:    rel.Namespace,
			Chart:        chartName,
			ChartVersion: chartVersion,
			AppVersion:   appVersion,
			Status:       string(rel.Info.Status),
			Revision:     rel.Version,
			Updated:      rel.Info.LastDeployed.String(),
			Description:  rel.Info.Description,
		})
	}

	return results
}

// StoreReleasesToInventory stores Helm releases to the inventory database
func (h *HelmClient) StoreReleasesToInventory(ctx context.Context, clusterID string, store *inventory.Store) error {
	releases, err := h.ListReleases(ctx)
	if err != nil {
		return fmt.Errorf("failed to list releases: %w", err)
	}

	fmt.Printf("Found %d Helm releases\n", len(releases))

	for _, rel := range releases {
		// Create HelmReleaseEntry
		entry := inventory.HelmReleaseEntry{
			Name:         rel.Name,
			Namespace:    rel.Namespace,
			Chart:        rel.Chart,
			ChartVersion: rel.ChartVersion,
			AppVersion:   rel.AppVersion,
			Status:       rel.Status,
		}

		// Save to database
		entRelease, err := store.SaveHelmRelease(ctx, clusterID, entry)
		if err != nil {
			return fmt.Errorf("failed to save helm release %s/%s: %w", rel.Namespace, rel.Name, err)
		}

		fmt.Printf("Stored Helm release: %s/%s (ID: %d)\n", rel.Namespace, rel.Name, entRelease.ID)
	}

	return nil
}

// GetReleaseHistory retrieves the history of a specific release
func (h *HelmClient) GetReleaseHistory(ctx context.Context, name, namespace string) ([]*release.Release, error) {
	actionConfig, err := h.getActionConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get action config: %w", err)
	}

	historyClient := action.NewHistory(actionConfig)
	historyClient.Max = 256 // Get all history

	history, err := historyClient.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get history for release %s: %w", name, err)
	}

	return history, nil
}

// GetReleaseStatus retrieves the status of a specific release
func (h *HelmClient) GetReleaseStatus(ctx context.Context, name, namespace string) (*release.Release, error) {
	actionConfig, err := h.getActionConfig(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get action config: %w", err)
	}

	statusClient := action.NewStatus(actionConfig)
	rel, err := statusClient.Run(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get status for release %s: %w", name, err)
	}

	return rel, nil
}
