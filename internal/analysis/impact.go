package analysis

import (
	"context"
	"fmt"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/inventory"
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/knowledge"
)

// ImpactLevel represents the severity of upgrade impact
type ImpactLevel string

const (
	ImpactCritical ImpactLevel = "critical"
	ImpactHigh     ImpactLevel = "high"
	ImpactMedium   ImpactLevel = "medium"
	ImpactLow      ImpactLevel = "low"
	ImpactNone     ImpactLevel = "none"
)

// ImpactAssessment represents the analysis of upgrade impact
type ImpactAssessment struct {
	ClusterID              string
	CurrentVersion         string
	TargetVersion          string
	DeprecatedManifestAPIs []DeprecatedAPIImpact
	DeprecatedCRDAPIs      []DeprecatedAPIImpact
	OverallRisk            ImpactLevel
	TotalIssues            int
}

// DeprecatedAPIImpact represents impact from deprecated APIs
type DeprecatedAPIImpact struct {
	Group          string
	Version        string
	Kind           string
	AffectedCount  int
	ImpactLevel    ImpactLevel
	RemovedIn      string
	ReplacementAPI string
	MigrationNotes string
	Source         string // "manifest" or "crd"
}

// Analyzer performs upgrade impact analysis
type Analyzer struct {
	apiKB *knowledge.APIKnowledgeBase
	store *inventory.Store
}

// NewAnalyzer creates a new impact analyzer
func NewAnalyzer(apiKnowledgeBasePath string, store *inventory.Store) (*Analyzer, error) {
	apiKB := knowledge.NewAPIKnowledgeBase()
	if err := apiKB.LoadFromFile(apiKnowledgeBasePath); err != nil {
		return nil, fmt.Errorf("failed to load API knowledge base: %w", err)
	}

	return &Analyzer{
		apiKB: apiKB,
		store: store,
	}, nil
}

// ComputeUpgradeImpact analyzes the impact of upgrading to a target version
func (a *Analyzer) ComputeUpgradeImpact(ctx context.Context, clusterID, targetVersion string) (*ImpactAssessment, error) {
	// Get cluster info
	cluster, err := a.store.GetCluster(ctx, clusterID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster: %w", err)
	}

	assessment := &ImpactAssessment{
		ClusterID:              clusterID,
		CurrentVersion:         cluster.KubeVersion,
		TargetVersion:          targetVersion,
		DeprecatedManifestAPIs: make([]DeprecatedAPIImpact, 0),
		DeprecatedCRDAPIs:      make([]DeprecatedAPIImpact, 0),
	}

	// Check ManifestAPIs
	manifestAPIs, err := cluster.QueryManifestApis().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query manifest APIs: %w", err)
	}

	manifestAPIMap := make(map[string]int)
	for _, api := range manifestAPIs {
		key := fmt.Sprintf("%s/%s/%s", api.Group, api.Version, api.Kind)
		manifestAPIMap[key]++

		if a.apiKB.IsAPIRemoved(api.Group, api.Version, api.Kind, targetVersion) {
			dep, _ := a.apiKB.CheckDeprecation(api.Group, api.Version, api.Kind)

			impact := DeprecatedAPIImpact{
				Group:          api.Group,
				Version:        api.Version,
				Kind:           api.Kind,
				AffectedCount:  1,
				ImpactLevel:    ImpactCritical,
				RemovedIn:      dep.RemovedIn,
				ReplacementAPI: dep.ReplacementAPI,
				MigrationNotes: dep.MigrationNotes,
				Source:         "manifest",
			}
			assessment.DeprecatedManifestAPIs = append(assessment.DeprecatedManifestAPIs, impact)
		}
	}

	// Check CRDs
	crds, err := cluster.QueryCrds().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query CRDs: %w", err)
	}

	for _, crd := range crds {
		// Check each served version
		for _, version := range crd.Versions {
			if a.apiKB.IsAPIRemoved(crd.Group, version, crd.Kind, targetVersion) {
				dep, _ := a.apiKB.CheckDeprecation(crd.Group, version, crd.Kind)

				impact := DeprecatedAPIImpact{
					Group:          crd.Group,
					Version:        version,
					Kind:           crd.Kind,
					AffectedCount:  1,
					ImpactLevel:    ImpactHigh,
					RemovedIn:      dep.RemovedIn,
					ReplacementAPI: dep.ReplacementAPI,
					MigrationNotes: dep.MigrationNotes,
					Source:         "crd",
				}
				assessment.DeprecatedCRDAPIs = append(assessment.DeprecatedCRDAPIs, impact)
			}
		}
	}

	// Calculate overall risk
	assessment.TotalIssues = len(assessment.DeprecatedManifestAPIs) + len(assessment.DeprecatedCRDAPIs)
	assessment.OverallRisk = a.calculateOverallRisk(assessment)

	return assessment, nil
}

// calculateOverallRisk determines the overall risk level
func (a *Analyzer) calculateOverallRisk(assessment *ImpactAssessment) ImpactLevel {
	if assessment.TotalIssues == 0 {
		return ImpactNone
	}

	criticalCount := 0
	for _, api := range assessment.DeprecatedManifestAPIs {
		if api.ImpactLevel == ImpactCritical {
			criticalCount++
		}
	}

	if criticalCount > 0 {
		return ImpactCritical
	}

	if len(assessment.DeprecatedCRDAPIs) > 0 {
		return ImpactHigh
	}

	return ImpactMedium
}

// GenerateReport generates a human-readable report
func (a *Analyzer) GenerateReport(assessment *ImpactAssessment) string {
	report := fmt.Sprintf("\n=== Upgrade Impact Assessment ===\n")
	report += fmt.Sprintf("Cluster: %s\n", assessment.ClusterID)
	report += fmt.Sprintf("Current Version: %s\n", assessment.CurrentVersion)
	report += fmt.Sprintf("Target Version: %s\n", assessment.TargetVersion)
	report += fmt.Sprintf("Overall Risk: %s\n", assessment.OverallRisk)
	report += fmt.Sprintf("Total Issues: %d\n\n", assessment.TotalIssues)

	if len(assessment.DeprecatedManifestAPIs) > 0 {
		report += fmt.Sprintf("⚠️  DEPRECATED MANIFEST APIs (%d)\n", len(assessment.DeprecatedManifestAPIs))
		report += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
		for i, api := range assessment.DeprecatedManifestAPIs {
			gv := api.Group + "/" + api.Version
			if api.Group == "" {
				gv = api.Version
			}
			report += fmt.Sprintf("%d. %s %s\n", i+1, gv, api.Kind)
			report += fmt.Sprintf("   Impact: %s\n", api.ImpactLevel)
			report += fmt.Sprintf("   Removed In: v%s\n", api.RemovedIn)
			report += fmt.Sprintf("   Replacement: %s\n", api.ReplacementAPI)
			report += fmt.Sprintf("   Migration: %s\n\n", api.MigrationNotes)
		}
	}

	if len(assessment.DeprecatedCRDAPIs) > 0 {
		report += fmt.Sprintf("⚠️  DEPRECATED CRD APIs (%d)\n", len(assessment.DeprecatedCRDAPIs))
		report += "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n"
		for i, api := range assessment.DeprecatedCRDAPIs {
			report += fmt.Sprintf("%d. %s/%s %s\n", i+1, api.Group, api.Version, api.Kind)
			report += fmt.Sprintf("   Impact: %s\n", api.ImpactLevel)
			report += fmt.Sprintf("   Removed In: v%s\n", api.RemovedIn)
			report += fmt.Sprintf("   Replacement: %s\n", api.ReplacementAPI)
			report += fmt.Sprintf("   Migration: %s\n\n", api.MigrationNotes)
		}
	}

	if assessment.TotalIssues == 0 {
		report += "✅ No deprecated APIs found. Safe to upgrade!\n"
	}

	return report
}
