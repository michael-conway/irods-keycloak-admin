package provisioning

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"

	"github.com/michael-conway/irods-keycloak-admin/internal/avu"
	"github.com/michael-conway/irods-keycloak-admin/internal/domain"
	"github.com/michael-conway/irods-keycloak-admin/internal/irodsadapter"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	planvalidator "github.com/michael-conway/irods-keycloak-admin/internal/plan"
	"github.com/michael-conway/irods-keycloak-admin/internal/planreview"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

const (
	defaultManagedByValue = "irods-keycloak-admin"

	flowSelfServiceAccountRequest = "self_service_account_request"
	flowAdministratorProvisioning = "administrator_provisioning"
	flowMappedUserSync            = "synchronization_of_mapped_user"

	operationIRODSUserSync  = "irods.user.sync"
	operationIRODSGroupSync = "irods.group.sync"
)

type Service struct {
	service.NotImplementedService
	IRODS        irodsadapter.Client
	Keycloak     keycloakadmin.Client
	DefaultRealm string
	DefaultZone  string
	ManagedBy    string
	Authority    string
	PromptMode   planreview.PromptMode
	Reviewer     planreview.Reviewer
}

var _ service.ProvisioningService = (*Service)(nil)
var _ service.SyncService = (*Service)(nil)

type userSyncState struct {
	realm           string
	zone            string
	keycloakUser    keycloakadmin.User
	irodsUser       *irodstypes.IRODSUser
	userExists      bool
	mappedUser      bool
	flow            string
	desiredMetadata []*irodstypes.IRODSMeta
	missingMetadata []*irodstypes.IRODSMeta
}

type groupSyncState struct {
	realm           string
	zone            string
	keycloakGroup   keycloakadmin.Group
	irodsGroup      *irodstypes.IRODSUser
	groupName       string
	groupExists     bool
	mappedGroup     bool
	desiredMetadata []*irodstypes.IRODSMeta
	missingMetadata []*irodstypes.IRODSMeta
	membersToAdd    []groupMemberSyncState
	membersToRemove []groupMemberSyncState
}

type groupMemberSyncState struct {
	username       string
	keycloakUserID string
	keycloakUser   keycloakadmin.User
}

func (s *Service) PlanUser(ctx context.Context, req domain.ProvisionUserRequest) (domain.SyncPlan, error) {
	state, err := s.inspectUserSyncState(ctx, req)
	if err != nil {
		return domain.SyncPlan{}, err
	}

	plan := domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            newPlanID(),
		Mode:              domain.SyncPlanModeSync,
		TargetSystem:      domain.SyncTargetIRODS,
		Authority:         s.authority(),
		Realm:             state.realm,
		Zone:              state.zone,
		Summary:           domain.PlanSummary{},
		Operations:        []domain.PlanOperation{},
	}

	operationIndex := 1
	if !state.userExists {
		evidence := map[string]any{
			"request_flow":          state.flow,
			"keycloak_user_id":      state.keycloakUser.ID,
			"keycloak_username":     state.keycloakUser.Username,
			"keycloak_realm":        state.realm,
			"irods_username":        state.keycloakUser.Username,
			"keycloak_user_present": true,
			"irods_user_present":    false,
			"irods_zone":            state.zone,
		}
		addSyncModelEvidence(evidence, domain.SyncDirectionKeycloakToIRODS, domain.SyncClassificationCandidateAddition, true)
		plan.Operations = append(plan.Operations, domain.PlanOperation{
			OperationID: operationID(operationIndex),
			Action:      domain.PlanActionIRODSUserCreate,
			Target:      state.keycloakUser.Username,
			Risk:        "low",
			Evidence:    evidence,
		})
		plan.Summary.CreateIRODSUsers++
		operationIndex++
	}

	if len(state.missingMetadata) > 0 {
		evidence := map[string]any{
			"request_flow":           state.flow,
			"keycloak_user_id":       state.keycloakUser.ID,
			"keycloak_username":      state.keycloakUser.Username,
			"keycloak_realm":         state.realm,
			"irods_username":         state.keycloakUser.Username,
			"irods_user_present":     state.userExists,
			"irods_mapping_present":  state.mappedUser,
			"irods_zone":             state.zone,
			"missing_avu_attributes": missingAttributeNames(state.missingMetadata),
			"desired_avus":           metadataEvidence(state.desiredMetadata),
		}
		addSyncModelEvidence(evidence, domain.SyncDirectionKeycloakToIRODS, domain.SyncClassificationMappedUpdate, true)
		plan.Operations = append(plan.Operations, domain.PlanOperation{
			OperationID: operationID(operationIndex),
			Action:      domain.PlanActionIRODSUserMetadataSync,
			Target:      state.keycloakUser.Username,
			Risk:        "low",
			Evidence:    evidence,
		})
		plan.Summary.UpdateIRODSUserMetadata++
	}

	return plan, nil
}

func (s *Service) PlanGroup(ctx context.Context, req domain.ProvisionGroupRequest) (domain.SyncPlan, error) {
	state, err := s.inspectGroupSyncState(ctx, req)
	if err != nil {
		return domain.SyncPlan{}, err
	}

	plan := domain.SyncPlan{
		PlanFormatVersion: domain.SyncPlanFormatVersion,
		PlanID:            newPlanID(),
		Mode:              domain.SyncPlanModeSync,
		TargetSystem:      domain.SyncTargetIRODS,
		Authority:         s.authority(),
		Realm:             state.realm,
		Zone:              state.zone,
		Summary:           domain.PlanSummary{},
		Operations:        []domain.PlanOperation{},
	}

	operationIndex := 1
	if !state.groupExists {
		evidence := groupOperationEvidence(state, false)
		addSyncModelEvidence(evidence, domain.SyncDirectionKeycloakToIRODS, domain.SyncClassificationCandidateAddition, true)
		plan.Operations = append(plan.Operations, domain.PlanOperation{
			OperationID: operationID(operationIndex),
			Action:      domain.PlanActionIRODSGroupCreate,
			Target:      state.groupName,
			Risk:        "low",
			Evidence:    evidence,
		})
		plan.Summary.CreateIRODSGroups++
		operationIndex++
	}

	if len(state.missingMetadata) > 0 {
		evidence := groupOperationEvidence(state, state.groupExists)
		evidence["irods_mapping_present"] = state.mappedGroup
		evidence["missing_avu_attributes"] = missingAttributeNames(state.missingMetadata)
		evidence["desired_avus"] = metadataEvidence(state.desiredMetadata)
		addSyncModelEvidence(evidence, domain.SyncDirectionKeycloakToIRODS, domain.SyncClassificationMappedUpdate, true)
		plan.Operations = append(plan.Operations, domain.PlanOperation{
			OperationID: operationID(operationIndex),
			Action:      domain.PlanActionIRODSGroupMetadataSync,
			Target:      state.groupName,
			Risk:        "low",
			Evidence:    evidence,
		})
		plan.Summary.UpdateIRODSGroupMetadata++
	}

	for _, member := range state.membersToAdd {
		evidence := groupMemberOperationEvidence(state, member, false, true)
		addSyncModelEvidence(evidence, domain.SyncDirectionKeycloakToIRODS, domain.SyncClassificationCandidateAddition, true)
		plan.Operations = append(plan.Operations, domain.PlanOperation{
			OperationID: operationID(operationIndex),
			Action:      domain.PlanActionIRODSGroupMemberAdd,
			Target:      irodsMemberTarget(state.groupName, member.username),
			Risk:        "low",
			Evidence:    evidence,
		})
		plan.Summary.UpdateIRODSMemberships++
		operationIndex++
	}

	for _, member := range state.membersToRemove {
		evidence := groupMemberOperationEvidence(state, member, true, false)
		addSyncModelEvidence(evidence, domain.SyncDirectionKeycloakToIRODS, domain.SyncClassificationCandidateRemoval, true)
		plan.Operations = append(plan.Operations, domain.PlanOperation{
			OperationID: operationID(operationIndex),
			Action:      domain.PlanActionIRODSGroupMemberRemove,
			Target:      irodsMemberTarget(state.groupName, member.username),
			Risk:        "medium",
			Evidence:    evidence,
		})
		plan.Summary.UpdateIRODSMemberships++
		operationIndex++
	}

	return plan, nil
}

func (s *Service) ApplyUser(ctx context.Context, req domain.ProvisionUserRequest) (domain.MutationResult, error) {
	state, err := s.inspectUserSyncState(ctx, req)
	if err != nil {
		return domain.MutationResult{}, err
	}

	result := domain.MutationResult{
		Status:    "unchanged",
		Operation: operationIRODSUserSync,
		Target:    state.keycloakUser.Username,
		IRODS: &domain.SystemMutationResult{
			User:   state.keycloakUser.Username,
			Zone:   state.zone,
			Status: "unchanged",
		},
		Warnings: []domain.Warning{},
	}

	applied := false
	if !state.userExists {
		if _, err := s.IRODS.CreateUser(ctx, state.keycloakUser.Username, state.zone, irodstypes.IRODSUserRodsUser); err != nil {
			return domain.MutationResult{}, fmt.Errorf("create iRODS user %q in zone %q: %w", state.keycloakUser.Username, state.zone, err)
		}
		applied = true
	}

	for _, metadata := range state.missingMetadata {
		if err := s.IRODS.AddUserMetadata(ctx, state.keycloakUser.Username, state.zone, metadata); err != nil {
			return domain.MutationResult{}, fmt.Errorf("add iRODS AVU %q=%q to user %q in zone %q: %w", metadata.Name, metadata.Value, state.keycloakUser.Username, state.zone, err)
		}
		applied = true
	}

	if applied {
		result.Status = "applied"
		result.IRODS.Status = "applied"
	}
	return result, nil
}

func (s *Service) ApplyGroup(ctx context.Context, req domain.ProvisionGroupRequest) (domain.MutationResult, error) {
	state, err := s.inspectGroupSyncState(ctx, req)
	if err != nil {
		return domain.MutationResult{}, err
	}

	result := domain.MutationResult{
		Status:    "unchanged",
		Operation: operationIRODSGroupSync,
		Target:    state.groupName,
		IRODS: &domain.SystemMutationResult{
			Group:  state.groupName,
			Zone:   state.zone,
			Status: "unchanged",
		},
		Warnings: []domain.Warning{},
	}

	applied := false
	if !state.groupExists {
		if _, err := s.IRODS.CreateUser(ctx, state.groupName, state.zone, irodstypes.IRODSUserRodsGroup); err != nil {
			return domain.MutationResult{}, fmt.Errorf("create iRODS group %q in zone %q: %w", state.groupName, state.zone, err)
		}
		applied = true
	}

	for _, metadata := range state.missingMetadata {
		if err := s.IRODS.AddUserMetadata(ctx, state.groupName, state.zone, metadata); err != nil {
			return domain.MutationResult{}, fmt.Errorf("add iRODS group AVU %q=%q to group %q in zone %q: %w", metadata.Name, metadata.Value, state.groupName, state.zone, err)
		}
		applied = true
	}

	for _, member := range state.membersToAdd {
		if err := s.IRODS.AddGroupMember(ctx, state.groupName, member.username, state.zone); err != nil {
			return domain.MutationResult{}, fmt.Errorf("add iRODS group member %q to group %q in zone %q: %w", member.username, state.groupName, state.zone, err)
		}
		applied = true
	}

	for _, member := range state.membersToRemove {
		if err := s.IRODS.RemoveGroupMember(ctx, state.groupName, member.username, state.zone); err != nil {
			return domain.MutationResult{}, fmt.Errorf("remove iRODS group member %q from group %q in zone %q: %w", member.username, state.groupName, state.zone, err)
		}
		applied = true
	}

	if applied {
		result.Status = "applied"
		result.IRODS.Status = "applied"
	}
	return result, nil
}

func (s *Service) Plan(ctx context.Context, req domain.PlanRequest) (domain.SyncPlan, error) {
	return domain.SyncPlan{}, errors.New("provisioning sync planning requires a user-specific request")
}

func (s *Service) Apply(ctx context.Context, req domain.ApplyRequest) (domain.ApplyResult, error) {
	if err := s.validateIRODS(); err != nil {
		return domain.ApplyResult{}, err
	}
	if req.Plan == nil {
		return domain.ApplyResult{}, errors.New("plan is required")
	}
	syncPlan := *req.Plan
	realm := firstNonEmpty(req.Realm, syncPlan.Realm, s.DefaultRealm)
	zone := firstNonEmpty(req.Zone, syncPlan.Zone, s.DefaultZone)
	if err := planvalidator.ValidateForApply(syncPlan, planvalidator.ApplyValidationOptions{
		ExpectedRealm:  realm,
		ExpectedZone:   zone,
		ExpectedTarget: domain.SyncTargetIRODS,
	}); err != nil {
		return domain.ApplyResult{}, err
	}
	reviewSession, err := planreview.NewSession(s.PromptMode, s.Reviewer)
	if err != nil {
		return domain.ApplyResult{}, err
	}
	return s.applyPlan(ctx, syncPlan, reviewSession)
}

func (s *Service) applyPlan(ctx context.Context, syncPlan domain.SyncPlan, reviewSession *planreview.Session) (domain.ApplyResult, error) {
	result := domain.ApplyResult{
		Status:     "applied",
		PlanID:     syncPlan.PlanID,
		Warnings:   []domain.Warning{},
		Operations: []domain.MutationResult{},
	}
	if len(syncPlan.Operations) == 0 {
		result.Status = "skipped"
		return result, nil
	}

	for _, operation := range syncPlan.Operations {
		mutation := newPlanMutationResult(syncPlan, operation)
		decision, err := reviewSession.Decide(ctx, syncPlan, operation)
		if err != nil {
			return domain.ApplyResult{}, err
		}
		if decision == planreview.DecisionSkip {
			markPlanMutationStatus(&mutation, "skipped")
			result.Skipped++
			result.Operations = append(result.Operations, mutation)
			continue
		}

		outcome, err := s.applyPlanOperation(ctx, syncPlan, operation)
		if err != nil {
			markPlanMutationStatus(&mutation, "failed")
			mutation.Warnings = append(mutation.Warnings, domain.Warning{Code: "apply.irods.operation_failed", Message: err.Error()})
			result.Failed++
			result.Warnings = append(result.Warnings, mutation.Warnings...)
			result.Operations = append(result.Operations, mutation)
			continue
		}
		if outcome == keycloakadmin.MutationOutcomeUnchanged {
			markPlanMutationStatus(&mutation, "unchanged")
			result.Skipped++
			result.Operations = append(result.Operations, mutation)
			continue
		}
		markPlanMutationStatus(&mutation, "applied")
		result.Applied++
		result.Operations = append(result.Operations, mutation)
	}
	finalizePlanApplyResult(&result)
	return result, nil
}

func (s *Service) applyPlanOperation(ctx context.Context, syncPlan domain.SyncPlan, operation domain.PlanOperation) (keycloakadmin.MutationOutcome, error) {
	zone := firstNonEmpty(planvalidator.EvidenceString(operation, "irods_zone"), syncPlan.Zone, s.DefaultZone)
	switch operation.Action {
	case domain.PlanActionIRODSUserCreate:
		username, err := planvalidator.IRODSUserTarget(operation)
		if err != nil {
			return "", err
		}
		existing, err := s.IRODS.GetUser(ctx, username, zone)
		if err != nil {
			return "", err
		}
		if existing != nil {
			return keycloakadmin.MutationOutcomeUnchanged, nil
		}
		if _, err := s.IRODS.CreateUser(ctx, username, zone, irodstypes.IRODSUserRodsUser); err != nil {
			return "", err
		}
		return keycloakadmin.MutationOutcomeCreated, nil
	case domain.PlanActionIRODSUserMetadataSync:
		username, err := planvalidator.IRODSUserTarget(operation)
		if err != nil {
			return "", err
		}
		existing, err := s.IRODS.GetUser(ctx, username, zone)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return "", fmt.Errorf("iRODS user %q does not exist in zone %q", username, zone)
		}
		currentMetadata, err := s.IRODS.ListUserMetadata(ctx, username, zone)
		if err != nil {
			return "", err
		}
		desiredMetadata := desiredMetadataFromOperation(syncPlan, operation, s.managedBy(), s.authority())
		missingMetadata := missingUserMetadata(currentMetadata, desiredMetadata)
		if len(missingMetadata) == 0 {
			return keycloakadmin.MutationOutcomeUnchanged, nil
		}
		for _, metadata := range missingMetadata {
			if err := s.IRODS.AddUserMetadata(ctx, username, zone, metadata); err != nil {
				return "", err
			}
		}
		return keycloakadmin.MutationOutcomeUpdated, nil
	case domain.PlanActionIRODSGroupCreate:
		groupName, err := planvalidator.IRODSGroupTarget(operation)
		if err != nil {
			return "", err
		}
		existing, err := s.IRODS.GetUser(ctx, groupName, zone)
		if err != nil {
			return "", err
		}
		if existing != nil {
			return keycloakadmin.MutationOutcomeUnchanged, nil
		}
		if _, err := s.IRODS.CreateUser(ctx, groupName, zone, irodstypes.IRODSUserRodsGroup); err != nil {
			return "", err
		}
		return keycloakadmin.MutationOutcomeCreated, nil
	case domain.PlanActionIRODSGroupMetadataSync:
		groupName, err := planvalidator.IRODSGroupTarget(operation)
		if err != nil {
			return "", err
		}
		existing, err := s.IRODS.GetUser(ctx, groupName, zone)
		if err != nil {
			return "", err
		}
		if existing == nil {
			return "", fmt.Errorf("iRODS group %q does not exist in zone %q", groupName, zone)
		}
		currentMetadata, err := s.IRODS.ListUserMetadata(ctx, groupName, zone)
		if err != nil {
			return "", err
		}
		desiredMetadata := desiredGroupMetadataFromOperation(syncPlan, operation, s.managedBy(), s.authority())
		missingMetadata := missingUserMetadata(currentMetadata, desiredMetadata)
		if len(missingMetadata) == 0 {
			return keycloakadmin.MutationOutcomeUnchanged, nil
		}
		for _, metadata := range missingMetadata {
			if err := s.IRODS.AddUserMetadata(ctx, groupName, zone, metadata); err != nil {
				return "", err
			}
		}
		return keycloakadmin.MutationOutcomeUpdated, nil
	case domain.PlanActionIRODSGroupMemberAdd:
		groupName, username, err := planvalidator.IRODSMemberTarget(operation)
		if err != nil {
			return "", err
		}
		exists, err := s.irodsGroupMemberExists(ctx, zone, groupName, username)
		if err != nil {
			return "", err
		}
		if exists {
			return keycloakadmin.MutationOutcomeUnchanged, nil
		}
		if err := s.IRODS.AddGroupMember(ctx, groupName, username, zone); err != nil {
			return "", err
		}
		return keycloakadmin.MutationOutcomeUpdated, nil
	case domain.PlanActionIRODSGroupMemberRemove:
		groupName, username, err := planvalidator.IRODSMemberTarget(operation)
		if err != nil {
			return "", err
		}
		exists, err := s.irodsGroupMemberExists(ctx, zone, groupName, username)
		if err != nil {
			return "", err
		}
		if !exists {
			return keycloakadmin.MutationOutcomeUnchanged, nil
		}
		if err := s.IRODS.RemoveGroupMember(ctx, groupName, username, zone); err != nil {
			return "", err
		}
		return keycloakadmin.MutationOutcomeUpdated, nil
	default:
		return "", fmt.Errorf("unsupported operation action %q", operation.Action)
	}
}

func (s *Service) inspectUserSyncState(ctx context.Context, req domain.ProvisionUserRequest) (*userSyncState, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	realm := strings.TrimSpace(req.Realm)
	if realm == "" {
		realm = strings.TrimSpace(s.DefaultRealm)
	}
	if realm == "" {
		return nil, errors.New("realm is required")
	}

	zone := strings.TrimSpace(req.Zone)
	if zone == "" {
		zone = strings.TrimSpace(s.DefaultZone)
	}
	if zone == "" {
		return nil, errors.New("zone is required")
	}

	keycloakUserID := strings.TrimSpace(req.KeycloakUserID)
	if keycloakUserID == "" {
		return nil, errors.New("keycloak_user_id is required")
	}

	keycloakUser, err := s.Keycloak.GetUser(ctx, realm, keycloakUserID)
	if err != nil {
		return nil, err
	}
	if keycloakUser == nil {
		return nil, &keycloakadmin.UserNotFoundError{Realm: realm, Ref: keycloakUserID}
	}

	keycloakUser.Username = strings.TrimSpace(keycloakUser.Username)
	if keycloakUser.Username == "" {
		return nil, fmt.Errorf("keycloak user %q has no username", keycloakUserID)
	}

	irodsUser, err := s.IRODS.GetUser(ctx, keycloakUser.Username, zone)
	if err != nil {
		return nil, err
	}

	currentMetadata := []*irodstypes.IRODSMeta{}
	if irodsUser != nil {
		currentMetadata, err = s.IRODS.ListUserMetadata(ctx, keycloakUser.Username, zone)
		if err != nil {
			return nil, err
		}
	}

	desiredMetadata := []*irodstypes.IRODSMeta{
		{Name: avu.ManagedByAttribute, Value: s.managedBy()},
		{Name: avu.KeycloakRealmAttribute, Value: realm},
		{Name: avu.KeycloakUserIDAttribute, Value: keycloakUser.ID},
		{Name: avu.AuthorityAttribute, Value: s.authority()},
	}
	missingMetadata := missingUserMetadata(currentMetadata, desiredMetadata)
	mappedUser := hasMetadataValue(currentMetadata, avu.KeycloakUserIDAttribute, keycloakUser.ID)

	return &userSyncState{
		realm:           realm,
		zone:            zone,
		keycloakUser:    *keycloakUser,
		irodsUser:       irodsUser,
		userExists:      irodsUser != nil,
		mappedUser:      mappedUser,
		flow:            classifyProvisioningFlow(req, mappedUser),
		desiredMetadata: desiredMetadata,
		missingMetadata: missingMetadata,
	}, nil
}

func (s *Service) inspectGroupSyncState(ctx context.Context, req domain.ProvisionGroupRequest) (*groupSyncState, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}

	realm := strings.TrimSpace(req.Realm)
	if realm == "" {
		realm = strings.TrimSpace(s.DefaultRealm)
	}
	if realm == "" {
		return nil, errors.New("realm is required")
	}

	zone := strings.TrimSpace(req.Zone)
	if zone == "" {
		zone = strings.TrimSpace(s.DefaultZone)
	}
	if zone == "" {
		return nil, errors.New("zone is required")
	}

	keycloakGroup, err := s.findKeycloakGroup(ctx, realm, req)
	if err != nil {
		return nil, err
	}
	keycloakGroup.ID = strings.TrimSpace(keycloakGroup.ID)
	if keycloakGroup.ID == "" {
		return nil, fmt.Errorf("keycloak group %q has no stable id", groupRequestRef(req))
	}

	groupName := keycloakGroupNameForIRODS(keycloakGroup)
	if _, err := planvalidator.IRODSGroupTarget(domain.PlanOperation{Target: groupName}); err != nil {
		return nil, fmt.Errorf("keycloak group %q cannot map to an iRODS group: %w", groupRequestRef(req), err)
	}

	irodsGroup, err := s.IRODS.GetUser(ctx, groupName, zone)
	if err != nil {
		return nil, err
	}
	if irodsGroup != nil && irodsGroup.Type != "" && irodsGroup.Type != irodstypes.IRODSUserRodsGroup {
		return nil, fmt.Errorf("iRODS object %q exists but is not a group", groupName)
	}

	currentMetadata := []*irodstypes.IRODSMeta{}
	if irodsGroup != nil {
		currentMetadata, err = s.IRODS.ListUserMetadata(ctx, groupName, zone)
		if err != nil {
			return nil, err
		}
	}

	desiredMetadata := []*irodstypes.IRODSMeta{
		{Name: avu.ManagedByAttribute, Value: s.managedBy()},
		{Name: avu.KeycloakRealmAttribute, Value: realm},
		{Name: avu.KeycloakGroupIDAttribute, Value: keycloakGroup.ID},
		{Name: avu.AuthorityAttribute, Value: s.authority()},
	}
	missingMetadata := missingUserMetadata(currentMetadata, desiredMetadata)
	mappedGroup := hasMetadataValue(currentMetadata, avu.KeycloakGroupIDAttribute, keycloakGroup.ID)
	membersToAdd, membersToRemove, err := s.inspectGroupMembershipSync(ctx, realm, zone, keycloakGroup, groupName, irodsGroup != nil, mappedGroup)
	if err != nil {
		return nil, err
	}

	return &groupSyncState{
		realm:           realm,
		zone:            zone,
		keycloakGroup:   keycloakGroup,
		irodsGroup:      irodsGroup,
		groupName:       groupName,
		groupExists:     irodsGroup != nil,
		mappedGroup:     mappedGroup,
		desiredMetadata: desiredMetadata,
		missingMetadata: missingMetadata,
		membersToAdd:    membersToAdd,
		membersToRemove: membersToRemove,
	}, nil
}

func (s *Service) inspectGroupMembershipSync(ctx context.Context, realm string, zone string, keycloakGroup keycloakadmin.Group, groupName string, groupExists bool, mappedGroup bool) ([]groupMemberSyncState, []groupMemberSyncState, error) {
	keycloakMembers, err := s.Keycloak.ListGroupMembers(ctx, realm, keycloakGroup.ID)
	if err != nil {
		return nil, nil, err
	}
	irodsMembers := []*irodstypes.IRODSUser{}
	if groupExists {
		irodsMembers, err = s.IRODS.ListGroupMembers(ctx, zone, groupName)
		if err != nil {
			return nil, nil, err
		}
	}

	irodsMemberSet := irodsMemberNames(irodsMembers)
	keycloakMemberSet := map[string]keycloakadmin.User{}
	toAdd := []groupMemberSyncState{}
	for _, keycloakMember := range keycloakMembers {
		username := strings.TrimSpace(keycloakMember.Username)
		if username == "" {
			continue
		}
		keycloakMemberSet[username] = keycloakMember
		keycloakUserID := strings.TrimSpace(keycloakMember.ID)
		if keycloakUserID == "" {
			continue
		}
		if _, exists := irodsMemberSet[username]; exists {
			continue
		}
		if ok, err := s.irodsUserHasKeycloakMapping(ctx, username, zone, keycloakUserID); err != nil {
			return nil, nil, err
		} else if !ok {
			continue
		}
		toAdd = append(toAdd, groupMemberSyncState{
			username:       username,
			keycloakUserID: keycloakUserID,
			keycloakUser:   keycloakMember,
		})
	}

	toRemove := []groupMemberSyncState{}
	if mappedGroup {
		for _, irodsMember := range irodsMembers {
			if irodsMember == nil {
				continue
			}
			username := strings.TrimSpace(irodsMember.Name)
			if username == "" {
				continue
			}
			if _, exists := keycloakMemberSet[username]; exists {
				continue
			}
			keycloakUserID, err := s.irodsUserKeycloakID(ctx, username, zone)
			if err != nil {
				return nil, nil, err
			}
			if keycloakUserID == "" {
				continue
			}
			toRemove = append(toRemove, groupMemberSyncState{
				username:       username,
				keycloakUserID: keycloakUserID,
				keycloakUser: keycloakadmin.User{
					ID:       keycloakUserID,
					Username: username,
				},
			})
		}
	}

	sortGroupMemberSyncStates(toAdd)
	sortGroupMemberSyncStates(toRemove)
	return toAdd, toRemove, nil
}

func (s *Service) findKeycloakGroup(ctx context.Context, realm string, req domain.ProvisionGroupRequest) (keycloakadmin.Group, error) {
	groupID := strings.TrimSpace(req.KeycloakGroupID)
	groupPath := planvalidator.NormalizeGroupPath(req.KeycloakGroupPath)
	if groupID == "" && groupPath == "" {
		return keycloakadmin.Group{}, errors.New("keycloak_group_id or keycloak_group_path is required")
	}

	groups, err := s.Keycloak.ListGroups(ctx, realm)
	if err != nil {
		return keycloakadmin.Group{}, err
	}
	for _, group := range groups {
		if groupID != "" && strings.TrimSpace(group.ID) == groupID {
			return group, nil
		}
		if groupPath != "" && planvalidator.NormalizeGroupPath(group.Path) == groupPath {
			return group, nil
		}
	}
	return keycloakadmin.Group{}, &keycloakadmin.GroupNotFoundError{Realm: realm, Ref: groupRequestRef(req)}
}

func (s *Service) validate() error {
	if s == nil {
		return errors.New("provisioning service is required")
	}
	if s.IRODS == nil {
		return errors.New("irods adapter is required")
	}
	if s.Keycloak == nil {
		return errors.New("keycloak admin client is required")
	}
	return nil
}

func (s *Service) validateIRODS() error {
	if s == nil {
		return errors.New("provisioning service is required")
	}
	if s.IRODS == nil {
		return errors.New("irods adapter is required")
	}
	return nil
}

func (s *Service) managedBy() string {
	if value := strings.TrimSpace(s.ManagedBy); value != "" {
		return value
	}
	return defaultManagedByValue
}

func (s *Service) authority() string {
	if value := strings.TrimSpace(s.Authority); value != "" {
		return value
	}
	return domain.SyncPlanAuthorityIRODS
}

func classifyProvisioningFlow(req domain.ProvisionUserRequest, mappedUser bool) string {
	source := strings.ToLower(strings.TrimSpace(req.Source))
	actorType := strings.ToLower(strings.TrimSpace(req.Actor.Type))

	switch {
	case strings.Contains(source, "self"):
		return flowSelfServiceAccountRequest
	case strings.Contains(source, "admin"), actorType == "admin":
		return flowAdministratorProvisioning
	case mappedUser:
		return flowMappedUserSync
	default:
		return flowAdministratorProvisioning
	}
}

func keycloakGroupNameForIRODS(group keycloakadmin.Group) string {
	if name := firstAttributeValue(group.Attributes, "irods_group_name"); name != "" {
		return name
	}
	if name := strings.TrimSpace(group.Name); name != "" {
		return name
	}
	return planvalidator.GroupNameFromPath(group.Path)
}

func groupOperationEvidence(state *groupSyncState, groupPresent bool) map[string]any {
	return map[string]any{
		"keycloak_group_id":      state.keycloakGroup.ID,
		"keycloak_group_name":    state.keycloakGroup.Name,
		"keycloak_path":          state.keycloakGroup.Path,
		"keycloak_realm":         state.realm,
		"irods_group_name":       state.groupName,
		"keycloak_group_present": true,
		"irods_group_present":    groupPresent,
		"irods_zone":             state.zone,
	}
}

func groupMemberOperationEvidence(state *groupSyncState, member groupMemberSyncState, irodsMemberPresent bool, keycloakMemberPresent bool) map[string]any {
	evidence := groupOperationEvidence(state, state.groupExists)
	evidence["keycloak_user_id"] = member.keycloakUserID
	evidence["keycloak_username"] = member.username
	evidence["irods_username"] = member.username
	evidence["keycloak_member_present"] = keycloakMemberPresent
	evidence["irods_member_present"] = irodsMemberPresent
	return evidence
}

func addSyncModelEvidence(evidence map[string]any, direction string, classification string, mappingIdentityKnown bool) {
	evidence["sync_direction"] = direction
	evidence["sync_classification"] = classification
	evidence["mapping_identity_known"] = mappingIdentityKnown
	evidence["authority_role"] = "directional_hint"
	evidence["conflict_status"] = "none"
	evidence["credential_policy"] = domain.SyncCredentialPolicyExternalAuthority
	evidence["credential_action"] = domain.SyncCredentialActionNone
	evidence["failure_domain"] = domain.SyncFailureDomainIdentityMapping
}

func groupRequestRef(req domain.ProvisionGroupRequest) string {
	return firstNonEmpty(req.KeycloakGroupID, req.KeycloakGroupPath)
}

func firstAttributeValue(attributes map[string][]string, name string) string {
	for _, value := range attributes[name] {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func missingUserMetadata(current []*irodstypes.IRODSMeta, desired []*irodstypes.IRODSMeta) []*irodstypes.IRODSMeta {
	result := make([]*irodstypes.IRODSMeta, 0, len(desired))
	for _, metadata := range desired {
		if metadata == nil || strings.TrimSpace(metadata.Name) == "" {
			continue
		}
		if hasMetadataValue(current, metadata.Name, metadata.Value) {
			continue
		}
		result = append(result, &irodstypes.IRODSMeta{
			Name:  metadata.Name,
			Value: metadata.Value,
			Units: metadata.Units,
		})
	}
	return result
}

func hasMetadataValue(metadata []*irodstypes.IRODSMeta, name string, value string) bool {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	for _, entry := range metadata {
		if entry == nil {
			continue
		}
		if strings.TrimSpace(entry.Name) == name && strings.TrimSpace(entry.Value) == value {
			return true
		}
	}
	return false
}

func missingAttributeNames(metadata []*irodstypes.IRODSMeta) []string {
	names := make([]string, 0, len(metadata))
	for _, entry := range metadata {
		if entry == nil {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func metadataEvidence(metadata []*irodstypes.IRODSMeta) map[string]string {
	result := map[string]string{}
	for _, entry := range metadata {
		if entry == nil {
			continue
		}
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		result[name] = strings.TrimSpace(entry.Value)
	}
	return result
}

func (s *Service) irodsGroupMemberExists(ctx context.Context, zone string, groupName string, username string) (bool, error) {
	members, err := s.IRODS.ListGroupMembers(ctx, zone, groupName)
	if err != nil {
		return false, err
	}
	_, exists := irodsMemberNames(members)[strings.TrimSpace(username)]
	return exists, nil
}

func (s *Service) irodsUserHasKeycloakMapping(ctx context.Context, username string, zone string, keycloakUserID string) (bool, error) {
	existing, err := s.IRODS.GetUser(ctx, username, zone)
	if err != nil {
		return false, err
	}
	if existing == nil {
		return false, nil
	}
	metadata, err := s.IRODS.ListUserMetadata(ctx, username, zone)
	if err != nil {
		return false, err
	}
	return hasMetadataValue(metadata, avu.KeycloakUserIDAttribute, keycloakUserID), nil
}

func (s *Service) irodsUserKeycloakID(ctx context.Context, username string, zone string) (string, error) {
	metadata, err := s.IRODS.ListUserMetadata(ctx, username, zone)
	if err != nil {
		return "", err
	}
	for _, entry := range metadata {
		if entry == nil {
			continue
		}
		if strings.TrimSpace(entry.Name) == avu.KeycloakUserIDAttribute {
			return strings.TrimSpace(entry.Value), nil
		}
	}
	return "", nil
}

func irodsMemberNames(members []*irodstypes.IRODSUser) map[string]struct{} {
	result := map[string]struct{}{}
	for _, member := range members {
		if member == nil {
			continue
		}
		username := strings.TrimSpace(member.Name)
		if username == "" {
			continue
		}
		result[username] = struct{}{}
	}
	return result
}

func sortGroupMemberSyncStates(members []groupMemberSyncState) {
	sort.Slice(members, func(i, j int) bool {
		if members[i].username == members[j].username {
			return members[i].keycloakUserID < members[j].keycloakUserID
		}
		return members[i].username < members[j].username
	})
}

func desiredMetadataFromOperation(syncPlan domain.SyncPlan, operation domain.PlanOperation, managedBy string, authority string) []*irodstypes.IRODSMeta {
	desired := map[string]string{
		avu.ManagedByAttribute:      managedBy,
		avu.KeycloakRealmAttribute:  firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_realm"), syncPlan.Realm),
		avu.KeycloakUserIDAttribute: planvalidator.EvidenceString(operation, "keycloak_user_id"),
		avu.AuthorityAttribute:      authority,
	}
	if raw, ok := operation.Evidence["desired_avus"]; ok {
		for name, value := range stringMapEvidence(raw) {
			desired[name] = value
		}
	}
	result := make([]*irodstypes.IRODSMeta, 0, len(desired))
	for _, name := range []string{avu.ManagedByAttribute, avu.KeycloakRealmAttribute, avu.KeycloakUserIDAttribute, avu.AuthorityAttribute} {
		value := strings.TrimSpace(desired[name])
		if value == "" {
			continue
		}
		result = append(result, &irodstypes.IRODSMeta{Name: name, Value: value})
	}
	return result
}

func desiredGroupMetadataFromOperation(syncPlan domain.SyncPlan, operation domain.PlanOperation, managedBy string, authority string) []*irodstypes.IRODSMeta {
	desired := map[string]string{
		avu.ManagedByAttribute:       managedBy,
		avu.KeycloakRealmAttribute:   firstNonEmpty(planvalidator.EvidenceString(operation, "keycloak_realm"), syncPlan.Realm),
		avu.KeycloakGroupIDAttribute: planvalidator.EvidenceString(operation, "keycloak_group_id"),
		avu.AuthorityAttribute:       authority,
	}
	if raw, ok := operation.Evidence["desired_avus"]; ok {
		for name, value := range stringMapEvidence(raw) {
			desired[name] = value
		}
	}
	result := make([]*irodstypes.IRODSMeta, 0, len(desired))
	for _, name := range []string{avu.ManagedByAttribute, avu.KeycloakRealmAttribute, avu.KeycloakGroupIDAttribute, avu.AuthorityAttribute} {
		value := strings.TrimSpace(desired[name])
		if value == "" {
			continue
		}
		result = append(result, &irodstypes.IRODSMeta{Name: name, Value: value})
	}
	return result
}

func stringMapEvidence(raw any) map[string]string {
	result := map[string]string{}
	switch typed := raw.(type) {
	case map[string]string:
		for key, value := range typed {
			result[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	case map[string]any:
		for key, value := range typed {
			result[strings.TrimSpace(key)] = strings.TrimSpace(fmt.Sprint(value))
		}
	}
	delete(result, "")
	return result
}

func newPlanMutationResult(syncPlan domain.SyncPlan, operation domain.PlanOperation) domain.MutationResult {
	target := strings.TrimSpace(operation.Target)
	mutation := domain.MutationResult{
		OperationID: operation.OperationID,
		Status:      "pending",
		Operation:   operation.Action,
		Target:      target,
		IRODS: &domain.SystemMutationResult{
			Zone:   syncPlan.Zone,
			Status: "pending",
		},
		Warnings: []domain.Warning{},
	}
	switch operation.Action {
	case domain.PlanActionIRODSGroupCreate, domain.PlanActionIRODSGroupMetadataSync:
		mutation.IRODS.Group = target
	case domain.PlanActionIRODSGroupMemberAdd, domain.PlanActionIRODSGroupMemberRemove:
		groupName, username, err := planvalidator.IRODSMemberTarget(operation)
		if err == nil {
			mutation.IRODS.Group = groupName
			mutation.IRODS.User = username
		} else {
			mutation.IRODS.Group = target
		}
	default:
		mutation.IRODS.User = target
	}
	return mutation
}

func markPlanMutationStatus(mutation *domain.MutationResult, status string) {
	if mutation == nil {
		return
	}
	mutation.Status = status
	if mutation.IRODS != nil {
		mutation.IRODS.Status = status
	}
}

func finalizePlanApplyResult(result *domain.ApplyResult) {
	result.WarningCount = len(result.Warnings)
	if result.Failed > 0 {
		result.Status = "failed"
		return
	}
	if result.Applied == 0 && result.Skipped > 0 {
		result.Status = "skipped"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func operationID(index int) string {
	return fmt.Sprintf("op-%03d", index)
}

func irodsMemberTarget(groupName string, username string) string {
	return strings.TrimSpace(groupName) + "#member:" + strings.TrimSpace(username)
}

func newPlanID() string {
	return "plan-" + time.Now().UTC().Format("20060102T150405.000000000Z")
}
