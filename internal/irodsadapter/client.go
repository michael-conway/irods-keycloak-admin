package irodsadapter

import (
	"context"

	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
)

// Client exposes the direct iRODS operations needed by sync and repair
// workflows. It deliberately reuses go-irodsclient domain types so this project
// does not invent parallel iRODS user, group, or AVU representations.
type Client interface {
	GetUser(ctx context.Context, username string, zone string) (*irodstypes.IRODSUser, error)
	CreateUser(ctx context.Context, username string, zone string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error)
	RemoveUser(ctx context.Context, username string, zone string, userType irodstypes.IRODSUserType) error
	ListUsers(ctx context.Context, zone string, userType irodstypes.IRODSUserType) ([]*irodstypes.IRODSUser, error)
	ListGroupMembers(ctx context.Context, zone string, groupName string) ([]*irodstypes.IRODSUser, error)
	AddGroupMember(ctx context.Context, groupName string, username string, zone string) error
	RemoveGroupMember(ctx context.Context, groupName string, username string, zone string) error
	AddUserMetadata(ctx context.Context, username string, zone string, metadata *irodstypes.IRODSMeta) error
	ListUserMetadata(ctx context.Context, username string, zone string) ([]*irodstypes.IRODSMeta, error)
}
