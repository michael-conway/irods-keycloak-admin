package reconcile

import "github.com/michael-conway/irods-keycloak-admin/internal/domain"

type Snapshot struct {
	Realm string
	Zone  string
}

type Comparator struct{}

func (Comparator) PlanRepair(_ Snapshot, _ Snapshot) domain.SyncPlan {
	return domain.SyncPlan{
		Mode:         "sync",
		TargetSystem: domain.SyncTargetKeycloak,
		Authority:    "irods",
		Summary:      domain.PlanSummary{},
		Operations:   []domain.PlanOperation{},
	}
}
