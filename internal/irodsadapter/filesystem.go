package irodsadapter

import (
	"context"
	"errors"
	"fmt"
	"strings"

	irodsfs "github.com/cyverse/go-irodsclient/fs"
	"github.com/cyverse/go-irodsclient/irods/common"
	irodstypes "github.com/cyverse/go-irodsclient/irods/types"
	usersandgroups "github.com/michael-conway/go-irodsclient-extensions/usersandgroups"
	usersandgroupsirodsfs "github.com/michael-conway/go-irodsclient-extensions/usersandgroups/irodsfs"
)

const defaultApplicationName = "irods-keycloak-admin"

// ConnectionConfig describes a direct iRODS connection for local integration
// tests or scripted runs.
type ConnectionConfig struct {
	Host            string
	Port            int
	Zone            string
	Username        string
	Password        string
	DefaultResource string
}

// FileSystemClient adapts go-irodsclient's FileSystem to the local sync
// boundary. It intentionally exposes go-irodsclient types unchanged.
type FileSystemClient struct {
	filesystem *irodsfs.FileSystem
}

var _ Client = (*FileSystemClient)(nil)

// NewFromConnectionConfig creates a client from explicit connection settings.
func NewFromConnectionConfig(cfg ConnectionConfig, applicationName string) (*FileSystemClient, *irodstypes.IRODSAccount, error) {
	if strings.TrimSpace(cfg.Host) == "" {
		return nil, nil, errors.New("irods host is required")
	}
	if cfg.Port <= 0 {
		return nil, nil, errors.New("irods port is required")
	}
	if strings.TrimSpace(cfg.Zone) == "" {
		return nil, nil, errors.New("irods zone is required")
	}
	if strings.TrimSpace(cfg.Username) == "" {
		return nil, nil, errors.New("irods username is required")
	}
	account, err := irodstypes.CreateIRODSAccount(
		strings.TrimSpace(cfg.Host),
		cfg.Port,
		strings.TrimSpace(cfg.Username),
		strings.TrimSpace(cfg.Zone),
		irodstypes.AuthSchemeNative,
		cfg.Password,
		strings.TrimSpace(cfg.DefaultResource),
	)
	if err != nil {
		return nil, nil, err
	}
	client, err := NewFromAccount(account, applicationName)
	if err != nil {
		return nil, nil, err
	}
	return client, account, nil
}

// NewFromAccount creates a client from an existing go-irodsclient account.
func NewFromAccount(account *irodstypes.IRODSAccount, applicationName string) (*FileSystemClient, error) {
	if account == nil {
		return nil, errors.New("irods account is required")
	}
	if strings.TrimSpace(applicationName) == "" {
		applicationName = defaultApplicationName
	}
	filesystem, err := irodsfs.NewFileSystemWithDefault(account, applicationName)
	if err != nil {
		return nil, err
	}
	return &FileSystemClient{filesystem: filesystem}, nil
}

// Close releases the underlying go-irodsclient sessions.
func (c *FileSystemClient) Close() error {
	if c == nil || c.filesystem == nil {
		return nil
	}
	c.filesystem.Release()
	c.filesystem = nil
	return nil
}

func (c *FileSystemClient) GetUser(ctx context.Context, username string, zone string) (*irodstypes.IRODSUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return nil, err
	}
	user, err := filesystem.GetUser(username, zone, "")
	if err != nil {
		if isUserNotFound(err) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

func isUserNotFound(err error) bool {
	return irodstypes.IsUserNotFoundError(err) || irodstypes.GetIRODSErrorCode(err) == common.CAT_NO_ROWS_FOUND
}

func (c *FileSystemClient) CreateUser(ctx context.Context, username string, zone string, userType irodstypes.IRODSUserType) (*irodstypes.IRODSUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return nil, err
	}
	return filesystem.CreateUser(username, zone, userType)
}

func (c *FileSystemClient) CreateGroup(ctx context.Context, groupName string, zone string) (*irodstypes.IRODSUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return nil, err
	}
	service := usersandgroups.NewService(usersandgroupsirodsfs.NewAdapter(filesystem), zone)
	group, err := service.CreateGroup(ctx, usersandgroups.GroupRequest{
		Zone: zone,
		Name: groupName,
	})
	if err != nil {
		return nil, err
	}
	return &irodstypes.IRODSUser{
		ID:   group.ID,
		Name: group.Name,
		Zone: group.Zone,
		Type: group.Type,
	}, nil
}

func (c *FileSystemClient) RemoveUser(ctx context.Context, username string, zone string, userType irodstypes.IRODSUserType) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return err
	}
	return filesystem.RemoveUser(username, zone, userType)
}

func (c *FileSystemClient) ListUsers(ctx context.Context, zone string, userType irodstypes.IRODSUserType) ([]*irodstypes.IRODSUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return nil, err
	}
	return filesystem.ListUsers(zone, userType)
}

func (c *FileSystemClient) ListGroupMembers(ctx context.Context, zone string, groupName string) ([]*irodstypes.IRODSUser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return nil, err
	}
	return filesystem.ListGroupMembers(zone, groupName)
}

func (c *FileSystemClient) AddGroupMember(ctx context.Context, groupName string, username string, zone string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return err
	}
	return filesystem.AddGroupMember(groupName, username, zone)
}

func (c *FileSystemClient) RemoveGroupMember(ctx context.Context, groupName string, username string, zone string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return err
	}
	return filesystem.RemoveGroupMember(groupName, username, zone)
}

func (c *FileSystemClient) AddUserMetadata(ctx context.Context, username string, zone string, metadata *irodstypes.IRODSMeta) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if metadata == nil {
		return errors.New("metadata is required")
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return err
	}
	return filesystem.AddUserMetadata(username, zone, metadata.Name, metadata.Value, metadata.Units)
}

func (c *FileSystemClient) ListUserMetadata(ctx context.Context, username string, zone string) ([]*irodstypes.IRODSMeta, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	filesystem, err := c.requireFileSystem()
	if err != nil {
		return nil, err
	}
	return filesystem.ListUserMetadata(username, zone)
}

func (c *FileSystemClient) requireFileSystem() (*irodsfs.FileSystem, error) {
	if c == nil || c.filesystem == nil {
		return nil, fmt.Errorf("irods filesystem client is not initialized")
	}
	return c.filesystem, nil
}
