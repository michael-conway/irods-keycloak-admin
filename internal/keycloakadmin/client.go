package keycloakadmin

import "context"

type MutationOutcome string

const (
	MutationOutcomeCreated   MutationOutcome = "created"
	MutationOutcomeUpdated   MutationOutcome = "updated"
	MutationOutcomeDeleted   MutationOutcome = "deleted"
	MutationOutcomeUnchanged MutationOutcome = "unchanged"
)

type User struct {
	ID         string              `json:"id,omitempty"`
	Username   string              `json:"username"`
	Email      string              `json:"email,omitempty"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

type Group struct {
	ID         string              `json:"id,omitempty"`
	Path       string              `json:"path,omitempty"`
	Name       string              `json:"name,omitempty"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

type Client interface {
	GetUser(ctx context.Context, realm string, userID string) (*User, error)
	FindUserByUsername(ctx context.Context, realm string, username string) (*User, error)
	// ListGroups returns Keycloak groups visible to the admin client. Repair
	// workflows filter this list to iRODS mirror groups before planning changes.
	ListGroups(ctx context.Context, realm string) ([]Group, error)
	// ListGroupMembers returns current Keycloak mirror members for a group ID.
	ListGroupMembers(ctx context.Context, realm string, groupID string) ([]User, error)
	CreateOrUpdateUser(ctx context.Context, realm string, user User) (*User, error)
	CreateOrUpdateGroup(ctx context.Context, realm string, group Group) (*Group, MutationOutcome, error)
	DeleteGroup(ctx context.Context, realm string, groupIDOrPath string) (MutationOutcome, error)
	AddUserToGroup(ctx context.Context, realm string, userIDOrUsername string, groupIDOrPath string) (MutationOutcome, error)
	RemoveUserFromGroup(ctx context.Context, realm string, userIDOrUsername string, groupIDOrPath string) (MutationOutcome, error)
}
