package main

import (
	"fmt"
	"log"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/manifests"
)

func main() {
	// Create parser
	parser := manifests.NewParser()

	// Parse folder (change this to your manifest folder)
	folderPath := "./manifests"
	resources, err := parser.ParseFolder(folderPath)
	if err != nil {
		log.Fatalf("Failed to parse folder: %v", err)
	}

	fmt.Printf("Found %d Kubernetes resources\n\n", len(resources))

	// Get statistics
	stats := parser.GetResourceStats(resources)
	fmt.Println("Resource Statistics:")
	for resourceType, count := range stats {
		fmt.Printf("  %s: %d\n", resourceType, count)
	}

	fmt.Println("\nAPI Versions:")
	apiVersions := parser.ExtractAPIVersions(resources)
	for _, apiVersion := range apiVersions {
		fmt.Printf("  %s\n", apiVersion)
	}

	fmt.Println("\nAPI Details:")
	apiInfos := parser.ExtractAPIInfo(resources)
	seen := make(map[string]bool)
	for _, api := range apiInfos {
		key := fmt.Sprintf("%s/%s %s", api.Group, api.Version, api.Kind)
		if api.Group == "" {
			key = fmt.Sprintf("%s %s", api.Version, api.Kind)
		}
		if !seen[key] {
			seen[key] = true
			fmt.Printf("  %s\n", key)
		}
	}
}
