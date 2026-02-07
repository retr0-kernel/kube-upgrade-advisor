package inventory

import (
	"time"
)

// ClusterInventory represents the complete inventory of a cluster
type ClusterInventory struct {
	ClusterVersion string
	ScanTime       time.Time
	Resources      []ResourceEntry
	HelmReleases   []HelmReleaseEntry
	CRDs           []CRDEntry
}

// ResourceEntry represents a single Kubernetes resource in inventory
type ResourceEntry struct {
	APIVersion  string
	Kind        string
	Namespace   string
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

// HelmReleaseEntry represents a Helm release in inventory
type HelmReleaseEntry struct {
	Name         string
	Namespace    string
	Chart        string
	ChartVersion string
	AppVersion   string
	Status       string
}

// CRDEntry represents a CRD in inventory
type CRDEntry struct {
	Name          string
	Group         string
	Version       string
	Kind          string
	InstanceCount int
}

// InventorySnapshot represents a point-in-time snapshot
type InventorySnapshot struct {
	ID        string
	Timestamp time.Time
	Inventory ClusterInventory
}
