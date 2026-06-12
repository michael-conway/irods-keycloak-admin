package repair

import (
	"context"
	"errors"
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/irodsadapter"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/mapper"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

const (
	mirrorAttrGroupName = "irods_group_name"
	mirrorAttrZone      = "irods_zone"
	mirrorAttrAuthority = "authority"
)

type Service struct {
	service.NotImplementedService
	IRODS        irodsadapter.Client
	Keycloak     keycloakadmin.Client
	Mapper       mapper.Mapper
	DefaultRealm string
	DefaultZone  string
	MirrorRoot   string
	PromptMode   planreview.PromptMode
	Reviewer     planreview.Reviewer
}

var _ service.RepairService = (*Service)(nil)
var _ service.SyncService = (*Service)(nil)

type irodsGroupSnapshot struct {
	Name    string
	Zone    string
	Members map[string]struct{}
}

type keycloakGroupSnapshot struct {
	ID      string
	Name    string
	Path    string
	Zone    string
	Members map[string]string
}

func (s *Service) RepairKeycloak(ctx context.Context, req domain.RepairRequest) (domain.SyncPlan, error) {
	if err := s.validate(); err != nil {
		return domain.SyncPlan{}, err
	}

	realm, zone, err := s.resolveRepairScope(req)
	if err != nil {
		return domain.SyncPlan{}, err
	}
	irodsGroups, keycloakGroups, err := s.readRepairSnapshots(ctx, realm, zone)
	if err != nil {
		return domain.SyncPlan{}, err
	}

	return newRepairPlanner(realm, zone, s.mirrorPolicy(), irodsGroups, keycloakGroups).build(), nil
}

func (s *Service) Apply(ctx context.Context, req domain.ApplyRequest) (domain.ApplyResult, error) {
	if err := s.validateKeycloak(); err != nil {
		return domain.ApplyResult{}, err
	}
	if req.Plan == nil {
		return domain.ApplyResult{}, errors.New("plan is required")
	}

	syncPlan := *req.Plan
	if _, _, err := s.resolveApplyScope(req, syncPlan); err != nil {
		return domain.ApplyResult{}, err
	}
	reviewSession, err := s.newReviewSession()
	if err != nil {
		return domain.ApplyResult{}, err
	}
	return s.applyPlan(ctx, syncPlan, reviewSession)
}

func (s *Service) resolveRepairScope(req domain.RepairRequest) (string, string, error) {
	realm := s.realmFor(req.Realm)
	if realm == "" {
		return "", "", errors.New("realm is required")
	}
	zone := s.zoneFor(req.Zone)
	if zone == "" {
		return "", "", errors.New("zone is required")
	}
	return realm, zone, nil
}

func (s *Service) resolveApplyScope(req domain.ApplyRequest, syncPlan domain.SyncPlan) (string, string, error) {
	realm := s.realmFor(firstNonEmpty(req.Realm, syncPlan.Realm))
	zone := s.zoneFor(firstNonEmpty(req.Zone, syncPlan.Zone))
	if err := validateApplyPlan(syncPlan, realm, zone, s.mirrorPolicy().Root()); err != nil {
		return "", "", err
	}
	return realm, zone, nil
}

func (s *Service) newReviewSession() (*planreview.Session, error) {
	return planreview.NewSession(s.PromptMode, s.Reviewer)
}

func (s *Service) validate() error {
	if s == nil {
		return errors.New("repair service is required")
	}
	if s.IRODS == nil {
		return errors.New("irods adapter is required")
	}
	if s.Keycloak == nil {
		return errors.New("keycloak admin client is required")
	}
	return nil
}

func (s *Service) validateKeycloak() error {
	if s == nil {
		return errors.New("repair service is required")
	}
	if s.Keycloak == nil {
		return errors.New("keycloak admin client is required")
	}
	return nil
}

func (s *Service) realmFor(realm string) string {
	realm = strings.TrimSpace(realm)
	if realm != "" {
		return realm
	}
	return strings.TrimSpace(s.DefaultRealm)
}

func (s *Service) zoneFor(zone string) string {
	zone = strings.TrimSpace(zone)
	if zone != "" {
		return zone
	}
	if defaultZone := strings.TrimSpace(s.DefaultZone); defaultZone != "" {
		return defaultZone
	}
	return strings.TrimSpace(s.Mapper.DefaultZone)
}

func (s *Service) mirrorPolicy() mirrorPathPolicy {
	return newMirrorPathPolicy(s.MirrorRoot)
}
