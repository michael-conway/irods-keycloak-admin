# irods-keycloak-admin
iRODS and Keycloak control-plane toolkit for sync, repair, provisioning, and
Keycloak mirror management.

## Current State

This repository is being scaffolded as the Keycloak-facing control plane for
iRODS identity administration. Generic iRODS REST resources belong in
`irods-go-rest`; this service owns Keycloak mirror, sync, repair,
provisioning, and control-plane intent workflows. Direct iRODS administration
from a shell remains the responsibility of `gocmd` and iCommands.

## Commands

- `irods-kc-admin-server`: private `/admin/v1` HTTP control-plane API.
- `irods-kc-sync`: planned CLI for `plan`, `apply`, `bootstrap-keycloak`, and
  `repair-keycloak`.
- `irods-kc-doctor`: planned CLI for configuration, mapping, and drift checks.
- `irods-kc-admin`: reserved for optional diagnostics.

The planned CLIs should follow the same initialization boundary as `gocmd` and
`drscmd`: reuse the iCommands-compatible iRODS environment when direct iRODS
access is needed, and avoid duplicate group/user administration commands that
`gocmd` already provides.

## Direct iRODS Sync

Local sync and repair commands should use `go-irodsclient-extensions/usersync`
through `internal/irodsadapter` and direct `go-irodsclient` connections. They
should reuse the same iCommands-compatible environment initialized by
`gocmd init`, `drscmd iinit`, or iCommands, and should not require an
`irods-go-rest` server as an intermediate hop.

`irods-go-rest` remains the generic REST API for external HTTP clients. The
`irods-keycloak-admin` HTTP API is reserved for Keycloak integration, service
callbacks, diagnostics, and remote control-plane workflows.

## API Boundary

`/admin/v1` is an orchestration API. It is for sync planning/apply, Keycloak
repair/bootstrap, provisioning decisions, Keycloak events, and diagnostics. It
does not expose generic iRODS user, group, path, ticket, or resource CRUD.

Generic iRODS HTTP resources belong in `irods-go-rest`. Local `irods-kc-*`
commands use direct `go-irodsclient` and `go-irodsclient-extensions/usersync`
through `internal/irodsadapter`.

## Development

```bash
go test ./...
go build ./cmd/...
```

## Local Integration Environment

A disposable iRODS + Keycloak Docker Compose stack is available under
`deployments/docker-test-framework/5-0`. It is intended for integration and e2e
tests of the admin, sync, repair, and provisioning workflows.

See `irods_keycloak_auth_strategy_summary.md` for the current strategy and
package boundaries.
