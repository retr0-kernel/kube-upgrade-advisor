package planner

import (
	"github.com/retr0-kernel/kube-upgrade-advisor/internal/analysis"
)

// UpgradeAssessmentWithPlan combines impact assessment with upgrade plan
type UpgradeAssessmentWithPlan struct {
	*analysis.ImpactAssessment
	OrderedUpgradeSteps []string     `json:"orderedUpgradeSteps"`
	UpgradePlan         *UpgradePlan `json:"upgradePlan,omitempty"`
}
