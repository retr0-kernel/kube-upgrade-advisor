package planner

import (
	"fmt"
	"sort"
	"strings"

	"github.com/retr0-kernel/kube-upgrade-advisor/internal/analysis"
)

// UpgradeStep represents a single step in the upgrade plan
type UpgradeStep struct {
	ID           string
	Description  string
	Type         StepType
	Dependencies []string
	Impact       analysis.ImpactLevel
	Actions      []Action
	Order        int // Topological order
}

// StepType defines the type of upgrade step
type StepType string

const (
	StepPreCheck       StepType = "precheck"
	StepBackup         StepType = "backup"
	StepAPIMigration   StepType = "api_migration"
	StepChartUpgrade   StepType = "chart_upgrade"
	StepClusterUpgrade StepType = "cluster_upgrade"
	StepValidation     StepType = "validation"
	StepRollback       StepType = "rollback"
)

// Action represents an action to perform
type Action struct {
	Command     string
	Description string
	Required    bool
}

// UpgradePlan represents the complete upgrade plan
type UpgradePlan struct {
	FromVersion         string
	ToVersion           string
	Steps               []UpgradeStep
	OrderedUpgradeSteps []string
	Timeline            string
	TotalSteps          int
}

// Planner generates upgrade plans
type Planner struct {
	graph map[string]*UpgradeStep
	edges map[string][]string
}

// NewPlanner creates a new upgrade planner
func NewPlanner() *Planner {
	return &Planner{
		graph: make(map[string]*UpgradeStep),
		edges: make(map[string][]string),
	}
}

// GeneratePlan generates an upgrade plan based on impact assessment
func (p *Planner) GeneratePlan(assessment *analysis.ImpactAssessment) (*UpgradePlan, error) {
	p.graph = make(map[string]*UpgradeStep)
	p.edges = make(map[string][]string)

	// Step 1: Pre-check
	precheck := &UpgradeStep{
		ID:          "precheck",
		Description: "Pre-upgrade validation and checks",
		Type:        StepPreCheck,
		Impact:      analysis.ImpactLow,
		Actions: []Action{
			{
				Command:     "kubectl version",
				Description: "Verify cluster connectivity",
				Required:    true,
			},
			{
				Command:     "kubectl get nodes",
				Description: "Check node status",
				Required:    true,
			},
		},
	}
	p.addNode(precheck)

	// Step 2: Backup
	backup := &UpgradeStep{
		ID:           "backup",
		Description:  "Backup cluster state and critical resources",
		Type:         StepBackup,
		Impact:       analysis.ImpactHigh,
		Dependencies: []string{"precheck"},
		Actions: []Action{
			{
				Command:     "velero backup create pre-upgrade-backup --wait",
				Description: "Create full cluster backup",
				Required:    true,
			},
			{
				Command:     "etcdctl snapshot save /backup/etcd-snapshot.db",
				Description: "Backup etcd",
				Required:    true,
			},
		},
	}
	p.addNode(backup)
	p.addEdge("precheck", "backup")

	// Step 3: API Migrations
	apiMigrationSteps := p.createAPIMigrationSteps(assessment)
	for _, step := range apiMigrationSteps {
		step.Dependencies = append(step.Dependencies, "backup")
		p.addNode(step)
		p.addEdge("backup", step.ID)
	}

	// Step 4: Chart Upgrades
	chartUpgradeSteps := p.createChartUpgradeSteps(assessment)
	for _, step := range chartUpgradeSteps {
		step.Dependencies = append(step.Dependencies, "backup")

		// Chart upgrades depend on API migrations
		for _, apiStep := range apiMigrationSteps {
			step.Dependencies = append(step.Dependencies, apiStep.ID)
			p.addEdge(apiStep.ID, step.ID)
		}

		p.addNode(step)
	}

	// Step 5: Cluster Upgrade
	clusterUpgrade := &UpgradeStep{
		ID:           "cluster-upgrade",
		Description:  fmt.Sprintf("Upgrade Kubernetes from %s to %s", assessment.CurrentVersion, assessment.TargetVersion),
		Type:         StepClusterUpgrade,
		Impact:       analysis.ImpactCritical,
		Dependencies: []string{"backup"},
		Actions: []Action{
			{
				Command:     "kubeadm upgrade plan",
				Description: "Review upgrade plan",
				Required:    true,
			},
			{
				Command:     fmt.Sprintf("kubeadm upgrade apply %s", assessment.TargetVersion),
				Description: "Apply Kubernetes upgrade",
				Required:    true,
			},
			{
				Command:     "kubectl drain <node> --ignore-daemonsets",
				Description: "Drain nodes before upgrade",
				Required:    true,
			},
			{
				Command:     "kubectl uncordon <node>",
				Description: "Uncordon nodes after upgrade",
				Required:    true,
			},
		},
	}

	// Cluster upgrade depends on all API migrations and chart upgrades
	for _, step := range apiMigrationSteps {
		clusterUpgrade.Dependencies = append(clusterUpgrade.Dependencies, step.ID)
		p.addEdge(step.ID, "cluster-upgrade")
	}
	for _, step := range chartUpgradeSteps {
		clusterUpgrade.Dependencies = append(clusterUpgrade.Dependencies, step.ID)
		p.addEdge(step.ID, "cluster-upgrade")
	}

	p.addNode(clusterUpgrade)

	// Step 6: Validation
	validation := &UpgradeStep{
		ID:           "validation",
		Description:  "Post-upgrade validation",
		Type:         StepValidation,
		Impact:       analysis.ImpactMedium,
		Dependencies: []string{"cluster-upgrade"},
		Actions: []Action{
			{
				Command:     "kubectl get nodes",
				Description: "Verify all nodes are ready",
				Required:    true,
			},
			{
				Command:     "kubectl get pods --all-namespaces",
				Description: "Check all pods are running",
				Required:    true,
			},
			{
				Command:     "kubectl api-resources",
				Description: "Verify API resources",
				Required:    true,
			},
		},
	}
	p.addNode(validation)
	p.addEdge("cluster-upgrade", "validation")

	// Perform topological sort
	orderedSteps, err := p.topologicalSort()
	if err != nil {
		return nil, fmt.Errorf("failed to create upgrade plan: %w", err)
	}

	// Build plan
	plan := &UpgradePlan{
		FromVersion:         assessment.CurrentVersion,
		ToVersion:           assessment.TargetVersion,
		Steps:               orderedSteps,
		OrderedUpgradeSteps: make([]string, len(orderedSteps)),
		TotalSteps:          len(orderedSteps),
	}

	for i, step := range orderedSteps {
		plan.OrderedUpgradeSteps[i] = fmt.Sprintf("%d. [%s] %s", i+1, step.Type, step.Description)
	}

	plan.Timeline = p.estimateTimeline(len(orderedSteps))

	return plan, nil
}

// createAPIMigrationSteps creates steps for migrating deprecated APIs
func (p *Planner) createAPIMigrationSteps(assessment *analysis.ImpactAssessment) []*UpgradeStep {
	var steps []*UpgradeStep

	// Group by API
	apiMap := make(map[string]analysis.DeprecatedAPIImpact)
	for _, api := range assessment.DeprecatedManifestAPIs {
		key := fmt.Sprintf("%s/%s/%s", api.Group, api.Version, api.Kind)
		apiMap[key] = api
	}

	for key, api := range apiMap {
		gv := api.Group + "/" + api.Version
		if api.Group == "" {
			gv = api.Version
		}

		step := &UpgradeStep{
			ID:          fmt.Sprintf("migrate-api-%s", sanitizeID(key)),
			Description: fmt.Sprintf("Migrate %s %s to %s", gv, api.Kind, api.ReplacementAPI),
			Type:        StepAPIMigration,
			Impact:      api.ImpactLevel,
			Actions: []Action{
				{
					Command:     fmt.Sprintf("kubectl get %s -o yaml > backup-%s.yaml", api.Kind, strings.ToLower(api.Kind)),
					Description: fmt.Sprintf("Backup existing %s resources", api.Kind),
					Required:    true,
				},
				{
					Command:     fmt.Sprintf("kubectl convert -f backup-%s.yaml --output-version=%s", strings.ToLower(api.Kind), api.ReplacementAPI),
					Description: fmt.Sprintf("Convert to %s", api.ReplacementAPI),
					Required:    true,
				},
				{
					Command:     "Manual review required",
					Description: api.MigrationNotes,
					Required:    true,
				},
			},
		}
		steps = append(steps, step)
	}

	return steps
}

// createChartUpgradeSteps creates steps for upgrading Helm charts
func (p *Planner) createChartUpgradeSteps(assessment *analysis.ImpactAssessment) []*UpgradeStep {
	var steps []*UpgradeStep

	for _, chart := range assessment.IncompatibleCharts {
		step := &UpgradeStep{
			ID:          fmt.Sprintf("upgrade-chart-%s", sanitizeID(chart.ChartName)),
			Description: fmt.Sprintf("Upgrade %s from %s to %s", chart.ChartName, chart.CurrentVersion, chart.RecommendedVersion),
			Type:        StepChartUpgrade,
			Impact:      chart.ImpactLevel,
			Actions:     []Action{},
		}

		if chart.RecommendedVersion != "" {
			step.Actions = append(step.Actions, Action{
				Command:     fmt.Sprintf("helm upgrade %s %s --version %s -n %s", chart.ChartName, chart.ChartName, chart.RecommendedVersion, chart.Namespace),
				Description: fmt.Sprintf("Upgrade to version %s", chart.RecommendedVersion),
				Required:    true,
			})
		} else {
			step.Actions = append(step.Actions, Action{
				Command:     "Manual intervention required",
				Description: chart.Message,
				Required:    true,
			})
		}

		if len(chart.Issues) > 0 {
			step.Actions = append(step.Actions, Action{
				Command:     "Review known issues",
				Description: strings.Join(chart.Issues, "; "),
				Required:    true,
			})
		}

		steps = append(steps, step)
	}

	return steps
}

// addNode adds a node to the graph
func (p *Planner) addNode(step *UpgradeStep) {
	p.graph[step.ID] = step
}

// addEdge adds a directed edge from -> to
func (p *Planner) addEdge(from, to string) {
	p.edges[from] = append(p.edges[from], to)
}

// topologicalSort performs topological sort using Kahn's algorithm
func (p *Planner) topologicalSort() ([]UpgradeStep, error) {
	// Calculate in-degrees
	inDegree := make(map[string]int)
	for id := range p.graph {
		inDegree[id] = 0
	}
	for _, neighbors := range p.edges {
		for _, neighbor := range neighbors {
			inDegree[neighbor]++
		}
	}

	// Queue for nodes with no incoming edges
	queue := []string{}
	for id, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, id)
		}
	}

	// Sort result
	var result []UpgradeStep
	order := 0

	for len(queue) > 0 {
		// Sort queue for deterministic ordering
		sort.Strings(queue)

		current := queue[0]
		queue = queue[1:]

		step := p.graph[current]
		step.Order = order
		result = append(result, *step)
		order++

		// Reduce in-degree for neighbors
		for _, neighbor := range p.edges[current] {
			inDegree[neighbor]--
			if inDegree[neighbor] == 0 {
				queue = append(queue, neighbor)
			}
		}
	}

	// Check for cycles
	if len(result) != len(p.graph) {
		return nil, fmt.Errorf("cycle detected in dependency graph")
	}

	return result, nil
}

// ValidatePlan validates that the plan is executable
func (p *Planner) ValidatePlan(plan *UpgradePlan) error {
	if len(plan.Steps) == 0 {
		return fmt.Errorf("plan has no steps")
	}

	// Verify all dependencies exist
	stepIDs := make(map[string]bool)
	for _, step := range plan.Steps {
		stepIDs[step.ID] = true
	}

	for _, step := range plan.Steps {
		for _, dep := range step.Dependencies {
			if !stepIDs[dep] {
				return fmt.Errorf("step %s depends on non-existent step %s", step.ID, dep)
			}
		}
	}

	return nil
}

// estimateTimeline estimates the time needed for the upgrade
func (p *Planner) estimateTimeline(stepCount int) string {
	// Rough estimate: 30 min per step
	hours := (stepCount * 30) / 60
	if hours < 1 {
		return "Less than 1 hour"
	}
	if hours == 1 {
		return "Approximately 1 hour"
	}
	return fmt.Sprintf("Approximately %d hours", hours)
}

// sanitizeID creates a valid ID from a string
func sanitizeID(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, ".", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ToLower(s)
	return s
}
