package inventory

import (
	"context"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/db/ent"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/db/ent/cluster"
	entcrd "github.com/retr0-kernel/kube-upgrade-advisor/internal/db/ent/crd"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/db/ent/helmrelease"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/db/ent/manifestapi"
)

// Store handles persistent storage of inventory data using Ent
type Store struct {
	client *ent.Client
}

// NewStore creates a new inventory store with SQLite backend
func NewStore(dbPath string) (*Store, error) {
	client, err := ent.Open("sqlite3", fmt.Sprintf("file:%s?cache=shared&_fk=1", dbPath))
	if err != nil {
		return nil, fmt.Errorf("failed opening connection to sqlite: %w", err)
	}

	// Run auto migration
	if err := client.Schema.Create(context.Background()); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed creating schema: %w", err)
	}

	return &Store{
		client: client,
	}, nil
}

// GetClient returns the underlying Ent client
func (s *Store) GetClient() *ent.Client {
	return s.client
}

// SaveCluster saves cluster information (creates or updates)
func (s *Store) SaveCluster(ctx context.Context, id, name, kubeVersion string) (*ent.Cluster, error) {
	// Try to get existing cluster
	existing, err := s.client.Cluster.Get(ctx, id)
	if err == nil {
		// Cluster exists, update it
		fmt.Printf("Updating existing cluster: %s\n", id)
		return existing.Update().
			SetName(name).
			SetKubeVersion(kubeVersion).
			Save(ctx)
	}

	// Cluster doesn't exist, create new one
	return s.client.Cluster.
		Create().
		SetID(id).
		SetName(name).
		SetKubeVersion(kubeVersion).
		Save(ctx)
}

// GetCluster retrieves a cluster by ID
func (s *Store) GetCluster(ctx context.Context, id string) (*ent.Cluster, error) {
	return s.client.Cluster.
		Get(ctx, id)
}

// ListClusters lists all clusters
func (s *Store) ListClusters(ctx context.Context) ([]*ent.Cluster, error) {
	return s.client.Cluster.
		Query().
		All(ctx)
}

// ClearClusterData deletes all data for a cluster (Helm releases, CRDs, ManifestAPIs)
func (s *Store) ClearClusterData(ctx context.Context, clusterID string) error {
	// Delete Helm releases
	_, err := s.client.HelmRelease.
		Delete().
		Where(helmrelease.HasClusterWith(cluster.ID(clusterID))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete helm releases: %w", err)
	}

	// Delete CRDs
	_, err = s.client.CRD.
		Delete().
		Where(entcrd.HasClusterWith(cluster.ID(clusterID))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete CRDs: %w", err)
	}

	// Delete ManifestAPIs
	_, err = s.client.ManifestAPI.
		Delete().
		Where(manifestapi.HasClusterWith(cluster.ID(clusterID))).
		Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete manifest APIs: %w", err)
	}

	return nil
}

// SaveHelmRelease saves a Helm release (creates or updates)
func (s *Store) SaveHelmRelease(ctx context.Context, clusterID string, release HelmReleaseEntry) (*ent.HelmRelease, error) {
	// Check if release already exists
	existing, err := s.client.HelmRelease.
		Query().
		Where(
			helmrelease.Name(release.Name),
			helmrelease.Namespace(release.Namespace),
			helmrelease.HasClusterWith(cluster.ID(clusterID)),
		).
		Only(ctx)

	if err == nil {
		// Release exists, update it
		return existing.Update().
			SetChart(release.Chart).
			SetChartVersion(release.ChartVersion).
			SetAppVersion(release.AppVersion).
			Save(ctx)
	}

	// Release doesn't exist, create new one
	return s.client.HelmRelease.
		Create().
		SetName(release.Name).
		SetNamespace(release.Namespace).
		SetChart(release.Chart).
		SetChartVersion(release.ChartVersion).
		SetAppVersion(release.AppVersion).
		SetClusterID(clusterID).
		Save(ctx)
}

// SaveCRD saves a CRD entry (creates or updates)
func (s *Store) SaveCRD(ctx context.Context, clusterID string, crd CRDEntry) (*ent.CRD, error) {
	versions := []string{crd.Version}

	// Check if CRD already exists
	existing, err := s.client.CRD.
		Query().
		Where(
			entcrd.Name(crd.Name),
			entcrd.HasClusterWith(cluster.ID(clusterID)),
		).
		Only(ctx)

	if err == nil {
		// CRD exists, update it
		return existing.Update().
			SetGroup(crd.Group).
			SetKind(crd.Kind).
			SetVersions(versions).
			Save(ctx)
	}

	// CRD doesn't exist, create new one
	return s.client.CRD.
		Create().
		SetName(crd.Name).
		SetGroup(crd.Group).
		SetKind(crd.Kind).
		SetVersions(versions).
		SetClusterID(clusterID).
		Save(ctx)
}

// SaveManifestAPI saves a manifest API entry (creates or updates)
func (s *Store) SaveManifestAPI(ctx context.Context, clusterID, group, version, kind, source string) (*ent.ManifestAPI, error) {
	// Check if ManifestAPI already exists
	existing, err := s.client.ManifestAPI.
		Query().
		Where(
			manifestapi.Group(group),
			manifestapi.Version(version),
			manifestapi.Kind(kind),
			manifestapi.HasClusterWith(cluster.ID(clusterID)),
		).
		Only(ctx)

	if err == nil {
		// ManifestAPI exists, update source if needed
		return existing.Update().
			SetSource(manifestapi.Source(source)).
			Save(ctx)
	}

	// ManifestAPI doesn't exist, create new one
	return s.client.ManifestAPI.
		Create().
		SetGroup(group).
		SetVersion(version).
		SetKind(kind).
		SetSource(manifestapi.Source(source)).
		SetClusterID(clusterID).
		Save(ctx)
}

// SaveSnapshot saves an inventory snapshot
func (s *Store) SaveSnapshot(ctx context.Context, snapshot InventorySnapshot) error {
	// Create or update cluster
	clusterEntity, err := s.SaveCluster(ctx, snapshot.ID, "cluster", snapshot.Inventory.ClusterVersion)
	if err != nil {
		return err
	}

	// Clear existing data
	err = s.ClearClusterData(ctx, clusterEntity.ID)
	if err != nil {
		return fmt.Errorf("failed to clear cluster data: %w", err)
	}

	// Save helm releases
	for _, release := range snapshot.Inventory.HelmReleases {
		_, err := s.SaveHelmRelease(ctx, clusterEntity.ID, release)
		if err != nil {
			return err
		}
	}

	// Save CRDs
	for _, crd := range snapshot.Inventory.CRDs {
		_, err := s.SaveCRD(ctx, clusterEntity.ID, crd)
		if err != nil {
			return err
		}
	}

	return nil
}

// GetSnapshot retrieves a snapshot by ID
func (s *Store) GetSnapshot(ctx context.Context, id string) (*InventorySnapshot, error) {
	clusterEntity, err := s.GetCluster(ctx, id)
	if err != nil {
		return nil, err
	}

	// Load helm releases
	helmReleases, err := clusterEntity.QueryHelmReleases().All(ctx)
	if err != nil {
		return nil, err
	}

	// Load CRDs
	crds, err := clusterEntity.QueryCrds().All(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to InventorySnapshot
	snapshot := &InventorySnapshot{
		ID:        clusterEntity.ID,
		Timestamp: clusterEntity.CreatedAt,
		Inventory: ClusterInventory{
			ClusterVersion: clusterEntity.KubeVersion,
			HelmReleases:   make([]HelmReleaseEntry, len(helmReleases)),
			CRDs:           make([]CRDEntry, len(crds)),
		},
	}

	for i, hr := range helmReleases {
		snapshot.Inventory.HelmReleases[i] = HelmReleaseEntry{
			Name:         hr.Name,
			Namespace:    hr.Namespace,
			Chart:        hr.Chart,
			ChartVersion: hr.ChartVersion,
			AppVersion:   hr.AppVersion,
		}
	}

	for i, crd := range crds {
		version := ""
		if len(crd.Versions) > 0 {
			version = crd.Versions[0]
		}
		snapshot.Inventory.CRDs[i] = CRDEntry{
			Name:    crd.Name,
			Group:   crd.Group,
			Version: version,
		}
	}

	return snapshot, nil
}

// ListSnapshots lists all snapshots
func (s *Store) ListSnapshots(ctx context.Context) ([]InventorySnapshot, error) {
	clusters, err := s.ListClusters(ctx)
	if err != nil {
		return nil, err
	}

	snapshots := make([]InventorySnapshot, len(clusters))
	for i, clusterEntity := range clusters {
		snapshots[i] = InventorySnapshot{
			ID:        clusterEntity.ID,
			Timestamp: clusterEntity.CreatedAt,
			Inventory: ClusterInventory{
				ClusterVersion: clusterEntity.KubeVersion,
			},
		}
	}

	return snapshots, nil
}

// DeleteSnapshot deletes a snapshot
func (s *Store) DeleteSnapshot(ctx context.Context, id string) error {
	return s.client.Cluster.DeleteOneID(id).Exec(ctx)
}

// Close closes the store connection
func (s *Store) Close() error {
	return s.client.Close()
}
