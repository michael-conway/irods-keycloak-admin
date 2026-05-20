package mapper

import (
	"strings"

	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
)

type IdentityMapping struct {
	Realm         string
	IRODSUsername string
	IRODSZone     string
}

type GroupMapping struct {
	Realm          string
	KeycloakPath   string
	IRODSGroupName string
	IRODSZone      string
}

type Mapper struct {
	DefaultZone string
}

func (m Mapper) UserToIRODS(realm string, user keycloakadmin.User) IdentityMapping {
	return IdentityMapping{
		Realm:         strings.TrimSpace(realm),
		IRODSUsername: strings.TrimSpace(user.Username),
		IRODSZone:     strings.TrimSpace(m.DefaultZone),
	}
}

func (m Mapper) GroupToIRODS(realm string, group keycloakadmin.Group) GroupMapping {
	return GroupMapping{
		Realm:          strings.TrimSpace(realm),
		KeycloakPath:   strings.TrimSpace(group.Path),
		IRODSGroupName: strings.TrimSpace(group.Name),
		IRODSZone:      strings.TrimSpace(m.DefaultZone),
	}
}
