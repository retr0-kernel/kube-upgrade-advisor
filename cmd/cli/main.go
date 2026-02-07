package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/cluster"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/manifests"
)

func main() {
	ctx := context.Background()

	// Get kubeconfig path
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	if kc := os.Getenv("KUBECONFIG"); kc != "" {
		kubeconfig = kc
	}

	fmt.Println("=== Kube Upgrade Advisor ===\n")

	// Create Kube client
	fmt.Println("Connecting to Kubernetes cluster...")
	kubeClient, err := cluster.NewKubeClient(kubeconfig)
	if err != nil {
		log.Fatalf("Failed to create kube client: %v", err)
	}

	// Get cluster version
	version, err := kubeClient.GetClusterVersion(ctx)
	if err != nil {
		log.Fatalf("Failed to get cluster version: %v", err)
	}
	fmt.Printf("Cluster version: %s\n\n", version)

	// Create inventory store
	fmt.Println("Initializing database...")
	store, err := inventory.NewStore("kube-advisor.db")
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Save cluster info
	clusterID := "cluster-1"
	clusterRec, err := store.SaveCluster(ctx, clusterID, "my-cluster", version)
	if err != nil {
		log.Fatalf("Failed to save cluster: %v", err)
	}
	fmt.Printf("Saved cluster: %s (version: %s)\n\n", clusterRec.ID, clusterRec.KubeVersion)

	// Create CRD client
	fmt.Println("Fetching CRDs...")
	crdClient, err := cluster.NewCRDClientFromKubeClient(kubeClient)
	if err != nil {
		log.Fatalf("Failed to create CRD client: %v", err)
	}

	// List and store CRDs
	err = crdClient.StoreCRDsToInventory(ctx, clusterID, store)
	if err != nil {
		log.Fatalf("Failed to store CRDs: %v", err)
	}
	fmt.Println()

	// Create Helm client
	fmt.Println("Fetching Helm releases...")
	helmClient, err := cluster.NewHelmClientWithKubeconfig(kubeconfig)
	if err != nil {
		log.Fatalf("Failed to create Helm client: %v", err)
	}

	// List and store Helm releases
	err = helmClient.StoreReleasesToInventory(ctx, clusterID, store)
	if err != nil {
		log.Fatalf("Failed to store Helm releases: %v", err)
	}
	fmt.Println()

	// Parse local manifests (if folder provided)
	manifestPath := os.Getenv("MANIFEST_PATH")
	if manifestPath == "" {
		manifestPath = "./manifests" // Default folder
	}

	if _, err := os.Stat(manifestPath); err == nil {
		fmt.Printf("Parsing manifests from %s...\n", manifestPath)
		parser := manifests.NewParser()
		err = parser.StoreManifestsToInventory(ctx, manifestPath, clusterID, store, "local")
		if err != nil {
			log.Fatalf("Failed to store manifests: %v", err)
		}
		fmt.Println()
	} else {
		fmt.Printf("Skipping manifest parsing (folder not found: %s)\n\n", manifestPath)
	}

	fmt.Println("=== Inventory Complete! ===")
	fmt.Println("Database: kube-advisor.db")
}
