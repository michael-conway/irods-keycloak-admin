package domain

const (
	SyncPlanFormatVersion = "irods-keycloak-admin.sync-plan.v1"

	SyncPlanModeSync       = "sync"
	SyncPlanAuthorityIRODS = "irods"
	SyncTargetKeycloak     = "keycloak"
	SyncTargetIRODS        = "irods"

	SyncDirectionIRODSToKeycloak = "irods_to_keycloak"
	SyncDirectionKeycloakToIRODS = "keycloak_to_irods"

	SyncClassificationCandidateAddition = "candidate_addition"
	SyncClassificationCandidateRemoval  = "candidate_removal"
	SyncClassificationMappedUpdate      = "mapped_update"
	SyncClassificationConflict          = "conflict"

	SyncCredentialPolicyExternalAuthority = "external_authority"
	SyncCredentialActionNone              = "none"
	SyncFailureDomainIdentityMapping      = "identity_group_membership_mapping"

	PlanActionKeycloakUserCreate        = "keycloak.user.create"
	PlanActionKeycloakGroupCreate       = "keycloak.group.create"
	PlanActionKeycloakGroupMemberAdd    = "keycloak.group.member.add"
	PlanActionKeycloakGroupMemberRemove = "keycloak.group.member.remove"
	PlanActionKeycloakGroupDelete       = "keycloak.group.delete"
	PlanActionIRODSUserCreate           = "irods.user.create"
	PlanActionIRODSUserMetadataSync     = "irods.user.metadata.sync"
	PlanActionIRODSGroupCreate          = "irods.group.create"
	PlanActionIRODSGroupMetadataSync    = "irods.group.metadata.sync"
	PlanActionIRODSGroupMemberAdd       = "irods.group.member.add"
	PlanActionIRODSGroupMemberRemove    = "irods.group.member.remove"

	PlanRiskRequiresApproval = "requires_approval"
)

type Actor struct {
	Type     string `json:"type,omitempty"`
	ID       string `json:"id,omitempty"`
	Username string `json:"username,omitempty"`
	ClientID string `json:"client_id,omitempty"`
}

type RequestMetadata struct {
	Actor  Actor  `json:"actor,omitempty"`
	Source string `json:"source,omitempty"`
	Reason string `json:"reason,omitempty"`
	Realm  string `json:"realm,omitempty"`
	Zone   string `json:"zone,omitempty"`
	DryRun bool   `json:"dry_run,omitempty"`
}

type ProvisionUserRequest struct {
	RequestMetadata
	KeycloakUserID string            `json:"keycloak_user_id"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

type ProvisionGroupRequest struct {
	RequestMetadata
	KeycloakGroupID   string            `json:"keycloak_group_id,omitempty"`
	KeycloakGroupPath string            `json:"keycloak_group_path,omitempty"`
	Attributes        map[string]string `json:"attributes,omitempty"`
}

type DeprovisionUserRequest struct {
	RequestMetadata
	KeycloakUserID string `json:"keycloak_user_id"`
}

type ProvisioningRequest struct {
	RequestMetadata
	KeycloakUserID string            `json:"keycloak_user_id,omitempty"`
	Attributes     map[string]string `json:"attributes,omitempty"`
}

type ProvisioningDecisionRequest struct {
	RequestMetadata
	RequestID string `json:"request_id"`
}

type PlanRequest struct {
	RequestMetadata
	Mode         string `json:"mode,omitempty"`
	TargetSystem string `json:"target_system,omitempty"`
}

type ApplyRequest struct {
	RequestMetadata
	PlanID string    `json:"plan_id,omitempty"`
	Plan   *SyncPlan `json:"plan,omitempty"`
}

type BootstrapRequest struct {
	RequestMetadata
}

type RepairRequest struct {
	RequestMetadata
}

type EncodedCredential struct {
	Encoding string `json:"encoding"`
	Value    string `json:"value"`
	KeyID    string `json:"key_id,omitempty"`
}

type KeycloakAdminEvent struct {
	Realm          string         `json:"realm,omitempty"`
	EventID        string         `json:"event_id,omitempty"`
	Time           string         `json:"time,omitempty"`
	OperationType  string         `json:"operation_type,omitempty"`
	ResourceType   string         `json:"resource_type,omitempty"`
	ResourcePath   string         `json:"resource_path,omitempty"`
	Representation map[string]any `json:"representation,omitempty"`
	Details        map[string]any `json:"details,omitempty"`
}

type IRODSAdminContext struct {
	Zone       string            `json:"zone,omitempty"`
	Username   string            `json:"username,omitempty"`
	Host       string            `json:"host,omitempty"`
	Port       int               `json:"port,omitempty"`
	Resource   string            `json:"resource,omitempty"`
	Credential EncodedCredential `json:"credential"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

type KeycloakEventRequest struct {
	RequestMetadata
	EventID            string             `json:"event_id,omitempty"`
	EventType          string             `json:"event_type,omitempty"`
	IdempotencyKey     string             `json:"idempotency_key,omitempty"`
	KeycloakAdminEvent KeycloakAdminEvent `json:"keycloak_admin_event"`
	IRODSAdmin         IRODSAdminContext  `json:"irods_admin"`
	Payload            map[string]any     `json:"payload,omitempty"`
}

type DiagnosticsRequest struct {
	RequestMetadata
}

type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type SystemMutationResult struct {
	Realm  string `json:"realm,omitempty"`
	Group  string `json:"group,omitempty"`
	User   string `json:"user,omitempty"`
	Zone   string `json:"zone,omitempty"`
	Status string `json:"status"`
}

type MutationResult struct {
	OperationID    string                `json:"operation_id,omitempty"`
	Status         string                `json:"status"`
	RequestID      string                `json:"request_id,omitempty"`
	Operation      string                `json:"operation"`
	Target         string                `json:"target,omitempty"`
	IRODS          *SystemMutationResult `json:"irods,omitempty"`
	KeycloakMirror *SystemMutationResult `json:"keycloak_mirror,omitempty"`
	Warnings       []Warning             `json:"warnings"`
}

type PlanSummary struct {
	CreateKeycloakUsers       int `json:"create_keycloak_users,omitempty"`
	CreateKeycloakGroups      int `json:"create_keycloak_groups,omitempty"`
	UpdateKeycloakMemberships int `json:"update_keycloak_memberships,omitempty"`
	DeleteKeycloakMirrors     int `json:"delete_keycloak_mirror_groups,omitempty"`
	CreateIRODSUsers          int `json:"create_irods_users,omitempty"`
	UpdateIRODSUserMetadata   int `json:"update_irods_user_metadata,omitempty"`
	CreateIRODSGroups         int `json:"create_irods_groups,omitempty"`
	UpdateIRODSGroupMetadata  int `json:"update_irods_group_metadata,omitempty"`
	UpdateIRODSMemberships    int `json:"update_irods_memberships,omitempty"`
	RequiresApproval          int `json:"requires_approval,omitempty"`
}

type PlanOperation struct {
	OperationID string         `json:"operation_id"`
	Action      string         `json:"action"`
	Target      string         `json:"target"`
	Risk        string         `json:"risk"`
	Evidence    map[string]any `json:"evidence,omitempty"`
}

type SyncPlan struct {
	PlanFormatVersion  string          `json:"plan_format_version"`
	PlanID             string          `json:"plan_id"`
	Mode               string          `json:"mode"`
	TargetSystem       string          `json:"target_system,omitempty"`
	Authority          string          `json:"authority"`
	Realm              string          `json:"realm"`
	Zone               string          `json:"zone"`
	KeycloakMirrorRoot string          `json:"keycloak_mirror_root,omitempty"`
	MappingPolicyHash  string          `json:"mapping_policy_hash,omitempty"`
	Summary            PlanSummary     `json:"summary"`
	Operations         []PlanOperation `json:"operations"`
}

type PasswordActionReport struct {
	ReportFormatVersion string           `json:"report_format_version"`
	PlanID              string           `json:"plan_id"`
	Realm               string           `json:"realm"`
	Zone                string           `json:"zone"`
	TargetSystem        string           `json:"target_system"`
	Notification        string           `json:"notification"`
	CredentialPath      string           `json:"credential_path"`
	Actions             []PasswordAction `json:"actions"`
	Warnings            []Warning        `json:"warnings,omitempty"`
}

type PasswordAction struct {
	Action         string `json:"action"`
	KeycloakUserID string `json:"keycloak_user_id,omitempty"`
	IRODSUsername  string `json:"irods_username,omitempty"`
	Reason         string `json:"reason"`
}

type ApplyResult struct {
	Status       string           `json:"status"`
	RequestID    string           `json:"request_id,omitempty"`
	PlanID       string           `json:"plan_id,omitempty"`
	Applied      int              `json:"applied"`
	Skipped      int              `json:"skipped"`
	Failed       int              `json:"failed"`
	WarningCount int              `json:"warning_count"`
	Warnings     []Warning        `json:"warnings"`
	Operations   []MutationResult `json:"operations,omitempty"`
}

type EventResult struct {
	Status    string    `json:"status"`
	RequestID string    `json:"request_id,omitempty"`
	Warnings  []Warning `json:"warnings"`
}

type DiagnosticsResult struct {
	Status   string    `json:"status"`
	Warnings []Warning `json:"warnings"`
	Details  any       `json:"details,omitempty"`
}

type StatusResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
	Version string `json:"version,omitempty"`
}

type ConfigSummary struct {
	ServiceName                    string `json:"service_name"`
	ListenAddress                  string `json:"listen_address"`
	IRODSZone                      string `json:"irods_zone,omitempty"`
	IRODSHost                      string `json:"irods_host,omitempty"`
	IRODSPort                      int    `json:"irods_port,omitempty"`
	IRODSAdminUser                 string `json:"irods_admin_user,omitempty"`
	IRODSDefaultResource           string `json:"irods_default_resource,omitempty"`
	KeycloakBaseURL                string `json:"keycloak_base_url,omitempty"`
	KeycloakRealm                  string `json:"keycloak_realm,omitempty"`
	KeycloakAdminRealm             string `json:"keycloak_admin_realm,omitempty"`
	KeycloakAdminClientID          string `json:"keycloak_admin_client_id,omitempty"`
	KeycloakMirrorRoot             string `json:"keycloak_mirror_root,omitempty"`
	KeycloakEventSharedSecretSet   bool   `json:"keycloak_event_shared_secret_set"`
	KeycloakEventSharedSecretModel string `json:"keycloak_event_shared_secret_model,omitempty"`
}

type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}
