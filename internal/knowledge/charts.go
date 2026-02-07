package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ChartCompatibility represents compatibility info for a Helm chart
type ChartCompatibility struct {
	ChartVersion   string   `json:"chartVersion"`
	MinKubeVersion string   `json:"minKubeVersion"`
	MaxKubeVersion string   `json:"maxKubeVersion"`
	CompatibleWith []string `json:"compatibleWith"`
	KnownIssues    []string `json:"knownIssues"`
}

// ChartInfo represents a Helm chart with all its versions
type ChartInfo struct {
	ChartName  string               `json:"chartName"`
	Repository string               `json:"repository"`
	Versions   []ChartCompatibility `json:"versions"`
}

// ChartKnowledgeBase manages Helm chart compatibility knowledge
type ChartKnowledgeBase struct {
	charts map[string]ChartInfo
}

// ChartKnowledgeData represents the structure of chart-matrix.json
type ChartKnowledgeData struct {
	Charts []ChartInfo `json:"charts"`
}

// NewChartKnowledgeBase creates a new chart knowledge base
func NewChartKnowledgeBase() *ChartKnowledgeBase {
	return &ChartKnowledgeBase{
		charts: make(map[string]ChartInfo),
	}
}

// LoadFromFile loads chart compatibility data from a JSON file
func (kb *ChartKnowledgeBase) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var chartData ChartKnowledgeData
	if err := json.Unmarshal(data, &chartData); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	for _, chart := range chartData.Charts {
		kb.charts[chart.ChartName] = chart
	}

	return nil
}

// CheckCompatibility checks if a chart version is compatible with a Kubernetes version
func (kb *ChartKnowledgeBase) CheckCompatibility(chartName, chartVersion, kubeVersion string) (bool, []string) {
	chart, exists := kb.charts[chartName]
	if !exists {
		// Chart not in knowledge base, assume compatible
		return true, nil
	}

	// Find the chart version
	for _, compat := range chart.Versions {
		if compat.ChartVersion == chartVersion {
			// Check if kubeVersion is in compatible list
			normalizedKube := normalizeVersion(kubeVersion)
			for _, compatVersion := range compat.CompatibleWith {
				if normalizeVersion(compatVersion) == normalizedKube {
					return true, compat.KnownIssues
				}
			}

			// Not in compatible list, return known issues
			return false, compat.KnownIssues
		}
	}

	// Chart version not found in knowledge base, assume compatible
	return true, nil
}

// FindCompatibleChartVersion finds a compatible chart version for target Kubernetes version
func (kb *ChartKnowledgeBase) FindCompatibleChartVersion(chartName, currentVersion, targetK8sVersion string) *ChartRecommendation {
	chart, exists := kb.charts[chartName]
	if !exists {
		// Chart not in knowledge base
		return &ChartRecommendation{
			ChartName:      chartName,
			CurrentVersion: currentVersion,
			IsCompatible:   true,
			Message:        "Chart not in knowledge base - compatibility unknown",
		}
	}

	normalizedTarget := normalizeVersion(targetK8sVersion)

	// First check if current version is compatible
	currentCompatible := false
	var currentIssues []string

	for _, compat := range chart.Versions {
		if compat.ChartVersion == currentVersion {
			for _, compatVersion := range compat.CompatibleWith {
				if normalizeVersion(compatVersion) == normalizedTarget {
					currentCompatible = true
					currentIssues = compat.KnownIssues
					break
				}
			}
			break
		}
	}

	if currentCompatible && len(currentIssues) == 0 {
		return &ChartRecommendation{
			ChartName:      chartName,
			CurrentVersion: currentVersion,
			IsCompatible:   true,
			Message:        "Current version is compatible",
		}
	}

	// Find the best compatible version
	var bestVersion *ChartCompatibility

	for i := range chart.Versions {
		compat := &chart.Versions[i]

		// Check if compatible with target
		isCompatible := false
		for _, compatVersion := range compat.CompatibleWith {
			if normalizeVersion(compatVersion) == normalizedTarget {
				isCompatible = true
				break
			}
		}

		if !isCompatible {
			continue
		}

		// Prefer versions with no known issues
		if len(compat.KnownIssues) > 0 {
			continue
		}

		// If this is the first compatible version or it's newer than the current best
		if bestVersion == nil || compareVersions(compat.ChartVersion, bestVersion.ChartVersion) > 0 {
			bestVersion = compat
		}
	}

	if bestVersion != nil {
		return &ChartRecommendation{
			ChartName:          chartName,
			CurrentVersion:     currentVersion,
			IsCompatible:       false,
			RecommendedVersion: bestVersion.ChartVersion,
			Message:            fmt.Sprintf("Upgrade required for Kubernetes %s", targetK8sVersion),
			KnownIssues:        currentIssues,
		}
	}

	// No compatible version found
	return &ChartRecommendation{
		ChartName:      chartName,
		CurrentVersion: currentVersion,
		IsCompatible:   false,
		Message:        fmt.Sprintf("No compatible version found for Kubernetes %s", targetK8sVersion),
		KnownIssues:    currentIssues,
	}
}

// GetRecommendedVersion returns the recommended chart version for a Kubernetes version
func (kb *ChartKnowledgeBase) GetRecommendedVersion(chartName, kubeVersion string) string {
	chart, exists := kb.charts[chartName]
	if !exists {
		return ""
	}

	normalizedKube := normalizeVersion(kubeVersion)

	// Find the latest version compatible with this Kubernetes version
	var latestVersion string

	for _, compat := range chart.Versions {
		for _, compatVersion := range compat.CompatibleWith {
			if normalizeVersion(compatVersion) == normalizedKube {
				if latestVersion == "" || compareVersions(compat.ChartVersion, latestVersion) > 0 {
					// Skip if it has known issues
					if len(compat.KnownIssues) == 0 {
						latestVersion = compat.ChartVersion
					}
				}
				break
			}
		}
	}

	return latestVersion
}

// ChartRecommendation represents a recommendation for chart upgrade
type ChartRecommendation struct {
	ChartName          string
	CurrentVersion     string
	RecommendedVersion string
	IsCompatible       bool
	Message            string
	KnownIssues        []string
}

// compareVersions compares two version strings
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Remove 'v' prefix if present
	v1 = strings.TrimPrefix(v1, "v")
	v2 = strings.TrimPrefix(v2, "v")

	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int

		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}

		if n1 > n2 {
			return 1
		}
		if n1 < n2 {
			return -1
		}
	}

	return 0
}
