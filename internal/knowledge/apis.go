package knowledge

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// APIDeprecation represents deprecation information for a Kubernetes API
type APIDeprecation struct {
	Group          string `json:"group"`
	Version        string `json:"version"`
	Kind           string `json:"kind"`
	DeprecatedIn   string `json:"deprecatedIn"`
	RemovedIn      string `json:"removedIn"`
	ReplacementAPI string `json:"replacementAPI"`
	MigrationNotes string `json:"migrationNotes"`
}

// APIKnowledgeBase manages API deprecation knowledge
type APIKnowledgeBase struct {
	deprecations map[string]APIDeprecation
	apiList      []APIDeprecation
}

// APIKnowledgeData represents the structure of apis.json
type APIKnowledgeData struct {
	Deprecations []APIDeprecation `json:"deprecations"`
}

// NewAPIKnowledgeBase creates a new API knowledge base
func NewAPIKnowledgeBase() *APIKnowledgeBase {
	return &APIKnowledgeBase{
		deprecations: make(map[string]APIDeprecation),
		apiList:      make([]APIDeprecation, 0),
	}
}

// LoadFromFile loads API deprecation data from a JSON file
func (kb *APIKnowledgeBase) LoadFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	var apiData APIKnowledgeData
	if err := json.Unmarshal(data, &apiData); err != nil {
		return fmt.Errorf("failed to unmarshal JSON: %w", err)
	}

	for _, dep := range apiData.Deprecations {
		key := makeKey(dep.Group, dep.Version, dep.Kind)
		kb.deprecations[key] = dep
		kb.apiList = append(kb.apiList, dep)
	}

	return nil
}

// CheckDeprecation checks if an API version is deprecated
func (kb *APIKnowledgeBase) CheckDeprecation(group, version, kind string) (*APIDeprecation, bool) {
	key := makeKey(group, version, kind)
	dep, found := kb.deprecations[key]
	if found {
		return &dep, true
	}
	return nil, false
}

// IsAPIRemoved checks if an API is removed in the target Kubernetes version
func (kb *APIKnowledgeBase) IsAPIRemoved(group, version, kind, targetVersion string) bool {
	dep, found := kb.CheckDeprecation(group, version, kind)
	if !found {
		return false
	}

	// Compare versions
	return isVersionGreaterOrEqual(targetVersion, dep.RemovedIn)
}

// IsAPIDeprecated checks if an API is deprecated (but not yet removed) in target version
func (kb *APIKnowledgeBase) IsAPIDeprecated(group, version, kind, targetVersion string) bool {
	dep, found := kb.CheckDeprecation(group, version, kind)
	if !found {
		return false
	}

	// Deprecated if target >= deprecatedIn AND target < removedIn
	deprecated := isVersionGreaterOrEqual(targetVersion, dep.DeprecatedIn)
	removed := isVersionGreaterOrEqual(targetVersion, dep.RemovedIn)

	return deprecated && !removed
}

// GetRemovalVersion returns the version where an API will be removed
func (kb *APIKnowledgeBase) GetRemovalVersion(group, version, kind string) string {
	dep, found := kb.CheckDeprecation(group, version, kind)
	if !found {
		return ""
	}
	return dep.RemovedIn
}

// GetReplacementAPI returns the replacement API for a deprecated API
func (kb *APIKnowledgeBase) GetReplacementAPI(group, version, kind string) string {
	dep, found := kb.CheckDeprecation(group, version, kind)
	if !found {
		return ""
	}
	return dep.ReplacementAPI
}

// GetAllDeprecations returns all deprecations
func (kb *APIKnowledgeBase) GetAllDeprecations() []APIDeprecation {
	return kb.apiList
}

// makeKey creates a unique key for an API
func makeKey(group, version, kind string) string {
	if group == "" {
		return fmt.Sprintf("core/%s/%s", version, kind)
	}
	return fmt.Sprintf("%s/%s/%s", group, version, kind)
}

// isVersionGreaterOrEqual compares Kubernetes versions
// Returns true if version >= minVersion
// Examples: "1.22" >= "1.22" = true, "1.23" >= "1.22" = true, "1.21" >= "1.22" = false
func isVersionGreaterOrEqual(version, minVersion string) bool {
	v1 := normalizeVersion(version)
	v2 := normalizeVersion(minVersion)

	v1Major, v1Minor := parseVersion(v1)
	v2Major, v2Minor := parseVersion(v2)

	if v1Major > v2Major {
		return true
	}
	if v1Major < v2Major {
		return false
	}
	return v1Minor >= v2Minor
}

// normalizeVersion removes 'v' prefix and converts to standard format
func normalizeVersion(version string) string {
	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")
	return version
}

// parseVersion extracts major and minor version numbers
func parseVersion(version string) (int, int) {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return 0, 0
	}

	major, _ := strconv.Atoi(parts[0])
	minor, _ := strconv.Atoi(parts[1])

	return major, minor
}
