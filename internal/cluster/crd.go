package cluster

import (
	"context"
	"fmt"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextclientset "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
)

// CustomResourceDefinition represents a CRD in the cluster
type CustomResourceDefinition struct {
	Name        string
	Group       string
	Versions    []CRDVersion
	Kind        string
	Scope       string
	Labels      map[string]string
	Annotations map[string]string
}

// CRDVersion represents a version of a CRD
type CRDVersion struct {
	Name    string
	Served  bool
	Storage bool
}

// CRDClient handles Custom Resource Definition operations
type CRDClient struct {
	clientset *apiextclientset.Clientset
}

// NewCRDClient creates a new CRD client from REST config
func NewCRDClient(config *rest.Config) (*CRDClient, error) {
	clientset, err := apiextclientset.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create apiextensions clientset: %w", err)
	}

	return &CRDClient{
		clientset: clientset,
	}, nil
}

// NewCRDClientFromKubeClient creates a new CRD client from KubeClient
func NewCRDClientFromKubeClient(kubeClient *KubeClient) (*CRDClient, error) {
	return NewCRDClient(kubeClient.GetConfig())
}

// ListCRDs lists all CRDs in the cluster
func (c *CRDClient) ListCRDs(ctx context.Context) ([]CustomResourceDefinition, error) {
	crdList, err := c.clientset.ApiextensionsV1().CustomResourceDefinitions().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list CRDs: %w", err)
	}

	crds := make([]CustomResourceDefinition, 0, len(crdList.Items))
	for _, crd := range crdList.Items {
		crds = append(crds, c.convertCRD(&crd))
	}

	return crds, nil
}

// GetCRD retrieves a specific CRD by name
func (c *CRDClient) GetCRD(ctx context.Context, name string) (*CustomResourceDefinition, error) {
	crd, err := c.clientset.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get CRD %s: %w", name, err)
	}

	result := c.convertCRD(crd)
	return &result, nil
}

// convertCRD converts k8s CRD to our internal representation
func (c *CRDClient) convertCRD(crd *apiextv1.CustomResourceDefinition) CustomResourceDefinition {
	versions := make([]CRDVersion, 0, len(crd.Spec.Versions))
	for _, v := range crd.Spec.Versions {
		versions = append(versions, CRDVersion{
			Name:    v.Name,
			Served:  v.Served,
			Storage: v.Storage,
		})
	}

	return CustomResourceDefinition{
		Name:        crd.Name,
		Group:       crd.Spec.Group,
		Versions:    versions,
		Kind:        crd.Spec.Names.Kind,
		Scope:       string(crd.Spec.Scope),
		Labels:      crd.Labels,
		Annotations: crd.Annotations,
	}
}

// GetHelmOwnerInfo extracts Helm owner information from CRD labels/annotations
func (c *CRDClient) GetHelmOwnerInfo(crd CustomResourceDefinition) (string, string, bool) {
	// Check for Helm labels/annotations
	// Helm v3 uses: app.kubernetes.io/managed-by: Helm
	// and meta.helm.sh/release-name, meta.helm.sh/release-namespace

	if crd.Labels != nil {
		if managedBy, ok := crd.Labels["app.kubernetes.io/managed-by"]; ok && managedBy == "Helm" {
			releaseName := crd.Annotations["meta.helm.sh/release-name"]
			releaseNamespace := crd.Annotations["meta.helm.sh/release-namespace"]
			if releaseName != "" && releaseNamespace != "" {
				return releaseName, releaseNamespace, true
			}
		}
	}

	return "", "", false
}

// GetCRDInstances retrieves instances of a specific CRD (placeholder)
func (c *CRDClient) GetCRDInstances(ctx context.Context, crd CustomResourceDefinition) ([]interface{}, error) {
	// This would require dynamic client to fetch actual CR instances
	// Placeholder for future implementation
	return nil, nil
}

// StoreCRDsToInventory stores CRDs to the inventory database
func (c *CRDClient) StoreCRDsToInventory(ctx context.Context, clusterID string, store *inventory.Store) error {
	crds, err := c.ListCRDs(ctx)
	if err != nil {
		return fmt.Errorf("failed to list CRDs: %w", err)
	}

	for _, crd := range crds {
		// Extract served versions
		servedVersions := make([]string, 0)
		for _, v := range crd.Versions {
			if v.Served {
				servedVersions = append(servedVersions, v.Name)
			}
		}

		// Get Helm owner info
		helmOwnerName, helmOwnerNamespace, _ := c.GetHelmOwnerInfo(crd)

		// Save to database
		entCRD, err := store.GetClient().CRD.
			Create().
			SetName(crd.Name).
			SetGroup(crd.Group).
			SetKind(crd.Kind).
			SetVersions(servedVersions).
			SetClusterID(clusterID).
			SetNillableHelmOwnerName(&helmOwnerName).
			SetNillableHelmOwnerNamespace(&helmOwnerNamespace).
			Save(ctx)

		if err != nil {
			return fmt.Errorf("failed to save CRD %s: %w", crd.Name, err)
		}

		fmt.Printf("Stored CRD: %s (Kind: %s, ID: %d)\n", crd.Name, crd.Kind, entCRD.ID)
	}

	return nil
}
