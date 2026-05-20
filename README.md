# irods-keycloak-admin
iRODS and Keycloak: main external toolkit: admin API, CLI, sync, repair, user/group management

## Current State

This repository is being scaffolded as the Keycloak-facing control plane for
iRODS identity administration. Generic iRODS REST resources remain candidates
for `irods-go-rest`; this service owns Keycloak mirror, sync, repair,
provisioning, and control-plane intent workflows.

## Commands

- `irods-kc-admin-server`: private `/admin/v1` HTTP control-plane API.
- `irods-kc-sync`: planned CLI for `plan`, `apply`, `bootstrap-keycloak`, and
  `repair-keycloak`.
- `irods-kc-doctor`: planned CLI for configuration, mapping, and drift checks.
- `irods-kc-admin`: reserved for optional diagnostics.

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
