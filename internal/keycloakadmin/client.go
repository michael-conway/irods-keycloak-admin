package keycloakadmin

import "context"

type User struct {
	ID         string              `json:"id"`
	Username   string              `json:"username"`
	Email      string              `json:"email,omitempty"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

type Group struct {
	ID         string              `json:"id"`
	Path       string              `json:"path"`
	Name       string              `json:"name"`
	Attributes map[string][]string `json:"attributes,omitempty"`
}

type Client interface {
	GetUser(ctx context.Context, realm string, userID string) (*User, error)
	FindUserByUsername(ctx context.Context, realm string, username string) (*User, error)
	// ListGroups returns Keycloak groups visible to the admin client. Repair
	// workflows filter this list to iRODS mirror groups before planning changes.
	ListGroups(ctx context.Context, realm string) ([]Group, error)
	// ListGroupMembers returns current Keycloak mirror members for a group path.
	ListGroupMembers(ctx context.Context, realm string, groupPath string) ([]User, error)
	CreateOrUpdateUser(ctx context.Context, realm string, user User) (*User, error)
	CreateOrUpdateGroup(ctx context.Context, realm string, group Group) (*Group, error)
	DeleteGroup(ctx context.Context, realm string, groupPath string) error
	AddUserToGroup(ctx context.Context, realm string, userID string, groupPath string) error
	RemoveUserFromGroup(ctx context.Context, realm string, userID string, groupPath string) error
}
