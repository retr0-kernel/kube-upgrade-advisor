package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/analysis"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
)

var (
	analyzer *analysis.Analyzer
	store    *inventory.Store
)

func main() {
	// Initialize store
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "kube-advisor.db"
	}

	var err error
	store, err = inventory.NewStore(dbPath)
	if err != nil {
		log.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	// Initialize analyzer
	apiKnowledgePath := os.Getenv("API_KNOWLEDGE_PATH")
	if apiKnowledgePath == "" {
		apiKnowledgePath = "knowledge-base/apis.json"
	}

	chartKnowledgePath := os.Getenv("CHART_KNOWLEDGE_PATH")
	if chartKnowledgePath == "" {
		chartKnowledgePath = "knowledge-base/chart-matrix.json"
	}

	analyzer, err = analysis.NewAnalyzer(apiKnowledgePath, chartKnowledgePath, store)
	if err != nil {
		log.Fatalf("Failed to create analyzer: %v", err)
	}

	// Setup routes
	http.HandleFunc("/health", healthHandler)
	http.HandleFunc("/impact", impactHandler)
	http.HandleFunc("/clusters", clustersHandler)

	// Start server
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Starting server on port %s...", port)
	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status": "healthy",
	})
}

func impactHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get query parameters
	clusterID := r.URL.Query().Get("cluster")
	if clusterID == "" {
		clusterID = "cluster-1" // Default cluster
	}

	targetVersion := r.URL.Query().Get("target")
	if targetVersion == "" {
		http.Error(w, "Missing required parameter: target", http.StatusBadRequest)
		return
	}

	// Compute impact
	ctx := context.Background()
	assessment, err := analyzer.ComputeUpgradeImpact(ctx, clusterID, targetVersion)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to compute impact: %v", err), http.StatusInternalServerError)
		return
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(assessment)
}

func clustersHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ctx := context.Background()
	clusters, err := store.ListClusters(ctx)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list clusters: %v", err), http.StatusInternalServerError)
		return
	}

	// Convert to simple response
	type ClusterInfo struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Version string `json:"version"`
	}

	clusterInfos := make([]ClusterInfo, len(clusters))
	for i, cluster := range clusters {
		clusterInfos[i] = ClusterInfo{
			ID:      cluster.ID,
			Name:    cluster.Name,
			Version: cluster.KubeVersion,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(clusterInfos)
}
