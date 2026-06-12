package domain

const (
	SyncPlanFormatVersion = "irods-keycloak-admin.sync-plan.v1"

	SyncPlanModeRepairKeycloak = "repair-keycloak"
	SyncPlanAuthorityIRODS     = "irods"

	PlanActionKeycloakGroupCreate       = "keycloak.group.create"
	PlanActionKeycloakGroupMemberAdd    = "keycloak.group.member.add"
	PlanActionKeycloakGroupMemberRemove = "keycloak.group.member.remove"
	PlanActionKeycloakGroupDelete       = "keycloak.group.delete"

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
	Mode string `json:"mode,omitempty"`
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

type KeycloakEventRequest struct {
	RequestMetadata
	EventID   string         `json:"event_id,omitempty"`
	EventType string         `json:"event_type,omitempty"`
	Payload   map[string]any `json:"payload,omitempty"`
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
	CreateKeycloakGroups      int `json:"create_keycloak_groups,omitempty"`
	UpdateKeycloakMemberships int `json:"update_keycloak_memberships,omitempty"`
	DeleteKeycloakMirrors     int `json:"delete_keycloak_mirror_groups,omitempty"`
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
	PlanFormatVersion string          `json:"plan_format_version"`
	PlanID            string          `json:"plan_id"`
	Mode              string          `json:"mode"`
	Authority         string          `json:"authority"`
	Realm             string          `json:"realm"`
	Zone              string          `json:"zone"`
	MappingPolicyHash string          `json:"mapping_policy_hash,omitempty"`
	Summary           PlanSummary     `json:"summary"`
	Operations        []PlanOperation `json:"operations"`
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
	ServiceName        string `json:"service_name"`
	ListenAddress      string `json:"listen_address"`
	IRODSZone          string `json:"irods_zone,omitempty"`
	KeycloakRealm      string `json:"keycloak_realm,omitempty"`
	KeycloakMirrorRoot string `json:"keycloak_mirror_root,omitempty"`
}

type ErrorResponse struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
	Details   any    `json:"details,omitempty"`
}
