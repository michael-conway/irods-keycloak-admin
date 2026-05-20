package app

import (
	"net/http"

	"github.com/michael-conway/irods-keycloak-admin/internal/config"
	"github.com/michael-conway/irods-keycloak-admin/internal/httpapi"
	"github.com/michael-conway/irods-keycloak-admin/internal/service"
)

type App struct {
	Config  config.Config
	Handler http.Handler
}

func New(cfg config.Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	services := service.NewNotImplementedServices()
	handler := httpapi.NewHandler(cfg, services)
	return &App{
		Config:  cfg,
		Handler: handler.Routes(),
	}, nil
}
