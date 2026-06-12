package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/michael-conway/irods-keycloak-admin/internal/app"
	"github.com/michael-conway/irods-keycloak-admin/internal/config"
	"github.com/michael-conway/irods-keycloak-admin/internal/server"
)

func main() {
	cfg := config.FromEnv()
	flag.StringVar(&cfg.ListenAddress, "listen-address", cfg.ListenAddress, "HTTP listen address")
	flag.StringVar(&cfg.IRODSZone, "irods-zone", cfg.IRODSZone, "default iRODS zone")
	flag.StringVar(&cfg.KeycloakRealm, "keycloak-realm", cfg.KeycloakRealm, "default Keycloak realm")
	flag.StringVar(&cfg.KeycloakMirrorRoot, "keycloak-mirror-root", cfg.KeycloakMirrorRoot, "managed Keycloak mirror group root")
	flag.Parse()

	application, err := app.New(cfg)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	slog.Info("starting irods-keycloak-admin", "listen_address", cfg.ListenAddress)
	if err := server.Run(ctx, application.Config, application.Handler); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
