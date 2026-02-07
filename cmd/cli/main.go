package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/analysis"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/cluster"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/manifests"
	"github.com/spf13/cobra"
)

var (
	kubeconfig       string
	dbPath           string
	manifestPath     string
	targetVersion    string
	apiKnowledgePath string
)

var rootCmd = &cobra.Command{
	Use:   "kube-upgrade-advisor",
	Short: "Kubernetes cluster upgrade advisor",
	Long:  `A tool to analyze Kubernetes clusters for upgrade compatibility issues`,
}

var scanCmd = &cobra.Command{
	Use:   "scan",
	Short: "Scan cluster for inventory",
	Long:  `Scans the Kubernetes cluster, Helm releases, and local manifests`,
	Run:   runScan,
}

var impactCmd = &cobra.Command{
	Use:   "impact",
	Short: "Analyze upgrade impact",
	Long:  `Analyzes the impact of upgrading to a target Kubernetes version`,
	Run:   runImpact,
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List inventory",
	Long:  `Lists all scanned resources in the database`,
	Run:   runList,
}

func init() {
	// Root flags
	rootCmd.PersistentFlags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (default: $HOME/.kube/config)")
	rootCmd.PersistentFlags().StringVar(&dbPath, "db", "kube-advisor.db", "Path to database file")
	rootCmd.PersistentFlags().StringVar(&apiKnowledgePath, "api-knowledge", "knowledge-base/apis.json", "Path to API knowledge base")

	// Scan flags
	scanCmd.Flags().StringVar(&manifestPath, "manifests", "./manifests", "Path to manifest folder")

	// Impact flags
	impactCmd.Flags().StringVarP(&targetVersion, "target", "t", "", "Target Kubernetes version (required)")
	impactCmd.MarkFlagRequired("target")

	rootCmd.AddCommand(scanCmd)
	rootCmd.AddCommand(impactCmd)
	rootCmd.AddCommand(listCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

func runScan(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	// Get kubeconfig path
	if kubeconfig == "" {
		kubeconfig = filepath.Join(os.Getenv("HOME"), ".kube", "config")
		if kc := os.Getenv("KUBECONFIG"); kc != "" {
			kubeconfig = kc
		}
	}

	fmt.Println("=== Kube Upgrade Advisor - Scan ===\n")

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
	store, err := inventory.NewStore(dbPath)
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

	// Parse local manifests
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

	fmt.Println("=== Scan Complete! ===")
	fmt.Printf("Database: %s\n", dbPath)
	fmt.Println("\nRun 'kube-upgrade-advisor impact --target <version>' to analyze upgrade impact")
}

func runImpact(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	fmt.Println("=== Kube Upgrade Advisor - Impact Analysis ===\n")

	// Create inventory store
	store, err := inventory.NewStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Create analyzer
	analyzer, err := analysis.NewAnalyzer(apiKnowledgePath, store)
	if err != nil {
		log.Fatalf("Failed to create analyzer: %v", err)
	}

	// Compute impact
	clusterID := "cluster-1"
	fmt.Printf("Analyzing upgrade impact for target version: %s\n", targetVersion)

	assessment, err := analyzer.ComputeUpgradeImpact(ctx, clusterID, targetVersion)
	if err != nil {
		log.Fatalf("Failed to compute impact: %v", err)
	}

	// Generate and print report
	report := analyzer.GenerateReport(assessment)
	fmt.Println(report)
}

func runList(cmd *cobra.Command, args []string) {
	ctx := context.Background()

	store, err := inventory.NewStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	clusterID := "cluster-1"
	cluster, err := store.GetCluster(ctx, clusterID)
	if err != nil {
		log.Fatalf("Failed to get cluster: %v", err)
	}

	fmt.Printf("=== Cluster Inventory ===\n")
	fmt.Printf("Cluster: %s\n", cluster.ID)
	fmt.Printf("Version: %s\n\n", cluster.KubeVersion)

	// List Helm Releases
	helmReleases, _ := cluster.QueryHelmReleases().All(ctx)
	fmt.Printf("Helm Releases (%d):\n", len(helmReleases))
	for _, hr := range helmReleases {
		fmt.Printf("  - %s/%s (chart: %s-%s)\n", hr.Namespace, hr.Name, hr.Chart, hr.ChartVersion)
	}
	fmt.Println()

	// List CRDs
	crds, _ := cluster.QueryCrds().All(ctx)
	fmt.Printf("CRDs (%d):\n", len(crds))
	for _, crd := range crds {
		fmt.Printf("  - %s (group: %s, kind: %s)\n", crd.Name, crd.Group, crd.Kind)
	}
	fmt.Println()

	// List Manifest APIs
	manifestAPIs, _ := cluster.QueryManifestApis().All(ctx)
	fmt.Printf("Manifest APIs (%d):\n", len(manifestAPIs))
	apiMap := make(map[string]int)
	for _, api := range manifestAPIs {
		key := fmt.Sprintf("%s/%s %s", api.Group, api.Version, api.Kind)
		if api.Group == "" {
			key = fmt.Sprintf("%s %s", api.Version, api.Kind)
		}
		apiMap[key]++
	}
	for api, count := range apiMap {
		fmt.Printf("  - %s (count: %d)\n", api, count)
	}
}
