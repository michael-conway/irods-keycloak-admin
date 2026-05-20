package plan

import (
	"errors"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
)

func ValidateForApply(plan domain.SyncPlan, expectedRealm string, expectedZone string) error {
	if plan.Realm != "" && expectedRealm != "" && plan.Realm != expectedRealm {
		return errors.New("plan realm does not match runtime configuration")
	}
	if plan.Zone != "" && expectedZone != "" && plan.Zone != expectedZone {
		return errors.New("plan zone does not match runtime configuration")
	}
	return nil
}
