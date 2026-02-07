package main

import (
	"context"
	"fmt"
	"log"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/cluster"
)

func main() {
	ctx := context.Background()

	// Create Helm client
	helmClient, err := cluster.NewHelmClient()
	if err != nil {
		log.Fatalf("Failed to create Helm client: %v", err)
	}

	// List all releases
	releases, err := helmClient.ListReleases(ctx)
	if err != nil {
		log.Fatalf("Failed to list releases: %v", err)
	}

	fmt.Printf("Found %d Helm releases:\n\n", len(releases))

	for _, rel := range releases {
		fmt.Printf("Release: %s/%s\n", rel.Namespace, rel.Name)
		fmt.Printf("  Chart: %s-%s\n", rel.Chart, rel.ChartVersion)
		fmt.Printf("  App Version: %s\n", rel.AppVersion)
		fmt.Printf("  Status: %s\n", rel.Status)
		fmt.Printf("  Revision: %d\n\n", rel.Revision)
	}
}
