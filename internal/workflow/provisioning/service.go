package provisioning

import (
	"github.com/michael-conway/irods-keycloak-admin/internal/irodsadapter"
	"github.com/michael-conway/irods-keycloak-admin/internal/keycloakadmin"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

type Service struct {
	service.NotImplementedService
	IRODS    irodsadapter.Client
	Keycloak keycloakadmin.Client
}

var _ service.ProvisioningService = (*Service)(nil)
