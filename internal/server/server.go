package server

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/michael-conway/irods-keycloak-admin/internal/config"
)

func Run(ctx context.Context, cfg config.Config, handler http.Handler) error {
	httpServer := &http.Server{
		Addr:              cfg.ListenAddress,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		err := httpServer.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}
