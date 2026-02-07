package manifests

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
	"gopkg.in/yaml.v3"
)

// Resource represents a Kubernetes resource
type Resource struct {
	APIVersion string                 `yaml:"apiVersion"`
	Kind       string                 `yaml:"kind"`
	Metadata   map[string]interface{} `yaml:"metadata"`
	Spec       map[string]interface{} `yaml:"spec"`
}

// Parser handles parsing of Kubernetes manifests
type Parser struct {
	// Configuration options
	IgnorePatterns []string
}

// NewParser creates a new manifest parser
func NewParser() *Parser {
	return &Parser{
		IgnorePatterns: []string{
			".git",
			"node_modules",
			"vendor",
			".terraform",
		},
	}
}

// ParseFolder recursively parses all YAML files in a folder
func (p *Parser) ParseFolder(folderPath string) ([]Resource, error) {
	var allResources []Resource

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			// Check if this directory should be ignored
			if p.shouldIgnore(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}

		// Only process YAML files
		if !isYAMLFile(path) {
			return nil
		}

		// Parse the file
		resources, err := p.ParseFile(path)
		if err != nil {
			// Log error but continue processing other files
			fmt.Printf("Warning: failed to parse %s: %v\n", path, err)
			return nil
		}

		allResources = append(allResources, resources...)
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk directory: %w", err)
	}

	return allResources, nil
}

// ParseFile parses a single YAML file which may contain multiple documents
func (p *Parser) ParseFile(filePath string) ([]Resource, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return p.ParseStream(file)
}

// ParseYAML parses YAML manifest data
func (p *Parser) ParseYAML(data []byte) ([]Resource, error) {
	var resources []Resource

	// Split by YAML document separator
	documents := strings.Split(string(data), "\n---")

	for i, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		var resource Resource
		err := yaml.Unmarshal([]byte(doc), &resource)
		if err != nil {
			// Skip invalid YAML
			fmt.Printf("Warning: failed to parse document %d: %v\n", i, err)
			continue
		}

		// Validate it's a Kubernetes resource
		if !p.isKubernetesResource(resource) {
			continue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// ParseStream parses a stream of YAML documents
func (p *Parser) ParseStream(reader io.Reader) ([]Resource, error) {
	var resources []Resource
	decoder := yaml.NewDecoder(reader)

	for {
		var resource Resource
		err := decoder.Decode(&resource)
		if err == io.EOF {
			break
		}
		if err != nil {
			// Skip invalid documents
			continue
		}

		// Validate it's a Kubernetes resource
		if !p.isKubernetesResource(resource) {
			continue
		}

		resources = append(resources, resource)
	}

	return resources, nil
}

// ExtractAPIVersions extracts all unique API versions from resources
func (p *Parser) ExtractAPIVersions(resources []Resource) []string {
	seen := make(map[string]bool)
	var apiVersions []string

	for _, resource := range resources {
		if resource.APIVersion != "" && !seen[resource.APIVersion] {
			seen[resource.APIVersion] = true
			apiVersions = append(apiVersions, resource.APIVersion)
		}
	}

	return apiVersions
}

// ExtractAPIInfo extracts group, version, and kind from resources
func (p *Parser) ExtractAPIInfo(resources []Resource) []APIInfo {
	var apiInfos []APIInfo

	for _, resource := range resources {
		group, version := p.splitAPIVersion(resource.APIVersion)

		apiInfos = append(apiInfos, APIInfo{
			Group:   group,
			Version: version,
			Kind:    resource.Kind,
		})
	}

	return apiInfos
}

// splitAPIVersion splits apiVersion into group and version
// Examples:
//   - "v1" -> group: "", version: "v1"
//   - "apps/v1" -> group: "apps", version: "v1"
//   - "networking.k8s.io/v1" -> group: "networking.k8s.io", version: "v1"
func (p *Parser) splitAPIVersion(apiVersion string) (group, version string) {
	parts := strings.Split(apiVersion, "/")
	if len(parts) == 1 {
		// Core API (e.g., "v1")
		return "", parts[0]
	}
	// Group API (e.g., "apps/v1")
	return parts[0], parts[1]
}

// isKubernetesResource checks if the resource is a valid Kubernetes resource
func (p *Parser) isKubernetesResource(resource Resource) bool {
	// Must have apiVersion and kind
	if resource.APIVersion == "" || resource.Kind == "" {
		return false
	}

	// Filter out non-Kubernetes resources
	// Kubernetes resources should have specific kinds
	ignoredKinds := map[string]bool{
		"":              true,
		"Config":        true, // Generic config files
		"Kustomization": true, // Kustomize files (not actual K8s resources)
	}

	if ignoredKinds[resource.Kind] {
		return false
	}

	return true
}

// shouldIgnore checks if a path should be ignored
func (p *Parser) shouldIgnore(name string) bool {
	for _, pattern := range p.IgnorePatterns {
		if name == pattern {
			return true
		}
	}
	return false
}

// isYAMLFile checks if a file is a YAML file
func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// APIInfo represents API group/version/kind information
type APIInfo struct {
	Group   string
	Version string
	Kind    string
}

// StoreManifestsToInventory parses manifests from a folder and stores them to inventory
func (p *Parser) StoreManifestsToInventory(ctx context.Context, folderPath, clusterID string, store *inventory.Store, source string) error {
	// Parse all manifests in the folder
	resources, err := p.ParseFolder(folderPath)
	if err != nil {
		return fmt.Errorf("failed to parse folder: %w", err)
	}

	fmt.Printf("Found %d Kubernetes resources in %s\n", len(resources), folderPath)

	// Extract API info
	apiInfos := p.ExtractAPIInfo(resources)

	// Remove duplicates
	uniqueAPIs := p.deduplicateAPIInfo(apiInfos)

	fmt.Printf("Found %d unique API types\n", len(uniqueAPIs))

	// Store each unique API to database
	for _, api := range uniqueAPIs {
		_, err := store.SaveManifestAPI(ctx, clusterID, api.Group, api.Version, api.Kind, source)
		if err != nil {
			return fmt.Errorf("failed to save manifest API %s/%s %s: %w", api.Group, api.Version, api.Kind, err)
		}

		gvk := api.Group + "/" + api.Version
		if api.Group == "" {
			gvk = api.Version
		}
		fmt.Printf("Stored API: %s %s\n", gvk, api.Kind)
	}

	return nil
}

// deduplicateAPIInfo removes duplicate API info entries
func (p *Parser) deduplicateAPIInfo(apiInfos []APIInfo) []APIInfo {
	seen := make(map[string]bool)
	var unique []APIInfo

	for _, api := range apiInfos {
		key := fmt.Sprintf("%s/%s/%s", api.Group, api.Version, api.Kind)
		if !seen[key] {
			seen[key] = true
			unique = append(unique, api)
		}
	}

	return unique
}

// GetResourcesByKind filters resources by kind
func (p *Parser) GetResourcesByKind(resources []Resource, kind string) []Resource {
	var filtered []Resource
	for _, resource := range resources {
		if resource.Kind == kind {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

// GetResourcesByAPIVersion filters resources by API version
func (p *Parser) GetResourcesByAPIVersion(resources []Resource, apiVersion string) []Resource {
	var filtered []Resource
	for _, resource := range resources {
		if resource.APIVersion == apiVersion {
			filtered = append(filtered, resource)
		}
	}
	return filtered
}

// GetResourceStats returns statistics about the parsed resources
func (p *Parser) GetResourceStats(resources []Resource) map[string]int {
	stats := make(map[string]int)

	for _, resource := range resources {
		key := fmt.Sprintf("%s (%s)", resource.Kind, resource.APIVersion)
		stats[key]++
	}

	return stats
}
