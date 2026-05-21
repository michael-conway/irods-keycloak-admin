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
- `irods-kc-sync`: CLI for sync planning workflows. The first implemented
  slice is `repair-keycloak --dry-run`; `plan`, `apply`, and
  `bootstrap-keycloak` remain scaffolded.
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

The first vertical slice emits a JSON plan without mutating iRODS or Keycloak:

```bash
irods-kc-sync repair-keycloak --dry-run --realm irods
```

From a checkout, run it directly with Go:

```bash
go run ./cmd/irods-kc-sync repair-keycloak --dry-run --realm irods
```

To install the command onto your Go binary path:

```bash
make install
export PATH="$(go env GOPATH)/bin:$PATH"
irods-kc-sync repair-keycloak --dry-run --realm irods
```

### Keycloak Credentials

`repair-keycloak --dry-run` reads Keycloak through Admin REST. For the local
Docker test framework, use the realm admin account from `keycloak.env`:

```bash
irods-kc-sync repair-keycloak --dry-run \
  --realm irods \
  --keycloak-url https://127.0.0.1:8443 \
  --keycloak-admin-realm master \
  --keycloak-admin-user admin \
  --keycloak-admin-password admin \
  --keycloak-insecure-skip-verify
```

The same values can be supplied with environment variables:

```bash
export IRODS_KC_KEYCLOAK_BASE_URL=https://127.0.0.1:8443
export IRODS_KC_KEYCLOAK_ADMIN_REALM=master
export IRODS_KC_KEYCLOAK_ADMIN_USER=admin
export IRODS_KC_KEYCLOAK_ADMIN_PASSWORD=admin
export IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY=true
irods-kc-sync repair-keycloak --dry-run --realm irods
```

For a service client, pass a client ID and secret instead of admin
username/password:

```bash
irods-kc-sync repair-keycloak --dry-run \
  --realm irods \
  --keycloak-url https://keycloak.example.org \
  --keycloak-admin-realm master \
  --keycloak-client-id irods-kc-admin-cli \
  --keycloak-client-secret "$IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET"
```

The corresponding environment variables are
`IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID` and
`IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET`. The client must be allowed to call
Keycloak Admin REST for the target realm.

The command reads iRODS connection details from explicit/e2e environment
variables such as `IRODS_KC_E2E_IRODS_PROVIDER_HOST` when present. Otherwise it
falls back to the iCommands environment. Keycloak is read through Admin REST
using `--keycloak-url`, `--keycloak-admin-user`, and
`--keycloak-admin-password`, with the local test stack defaulting to
`https://127.0.0.1:8443`.

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

See `DEVELOPER_NOTES.md` for the current strategy and package boundaries.
