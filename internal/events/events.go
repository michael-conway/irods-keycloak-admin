package events

import "github.com/michael-conway/irods-keycloak-admin/internal/domain"

type Normalizer struct{}

func (Normalizer) Normalize(req domain.KeycloakEventRequest) domain.KeycloakEventRequest {
	return req
}
