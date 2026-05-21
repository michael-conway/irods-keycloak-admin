# iRODS Keycloak Admin Toolkit

[![Test](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml)
[![CodeQL](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml)

Go toolkit for coordinating iRODS identity state with Keycloak mirror groups,
provisioning workflows, repair planning, and control-plane operations.

## Overview

This project provides the Keycloak-facing control plane for iRODS identity
administration. It reads authoritative iRODS users, groups, and memberships,
compares that state with Keycloak mirror state, and produces or applies
explicit plans for Keycloak repair and synchronization.

It includes:

* command-line tools for Keycloak mirror repair planning and controlled apply
* direct iRODS integration through `go-irodsclient`
* an adapter boundary for shared reconciliation helpers from
  `go-irodsclient-extensions/usersync`
* a Keycloak Admin REST client for mirror group and membership reads/mutations
* a private `/admin/v1` orchestration API scaffold
* live e2e tests for the disposable Docker stack and `irods-grid-stack`

This repository does not replace iCommands, `gocmd`, or the generic
`irods-go-rest` API. Generic iRODS user, group, path, ticket, resource, and
metadata operations belong in those tools and services.

## Project Metadata

| Field | Value |
| --- | --- |
| Project Name | `iRODS Keycloak Admin Toolkit` |
| Current Version | `0.1.0-dev` |
| Status | `Active Development` |
| Primary Developer | `Mike Conway` |
| Organization | `NIEHS` |
| Repository | `https://github.com/michael-conway/irods-keycloak-admin` |
| Contact | `mike.conway@nih.gov` |
| Issue Tracker | `https://github.com/michael-conway/irods-keycloak-admin/issues` |
| License | `BSD-2-Clause` |

## Master Index

* [Developer Notes](./DEVELOPER_NOTES.md)
* [OpenAPI Control-Plane Contract](./api/openapi.yaml)
* [e2e Configuration](./e2e/README.md)
* [Deployment Notes](./deployments/README.md)

## Project Structure

The repository follows a conventional Go layout centered around orchestration
workflows, a direct iRODS adapter, a Keycloak Admin REST client, and a private
control-plane API.

| Path | Purpose |
| --- | --- |
| `cmd/irods-kc-sync/` | Sync and repair CLI; currently implements `repair-keycloak --dry-run` and `apply --plan` |
| `cmd/irods-kc-admin-server/` | Private HTTP control-plane service entrypoint |
| `cmd/irods-kc-doctor/` | Scaffolded diagnostics CLI for config, mapping, and drift checks |
| `cmd/irods-kc-admin/` | Reserved command for optional diagnostics or narrow admin helpers |
| `internal/irodsadapter/` | Direct iRODS boundary over `go-irodsclient`; integration point for `go-irodsclient-extensions/usersync` |
| `internal/keycloakadmin/` | Keycloak Admin REST client and mirror mutation methods |
| `internal/workflow/repair/` | Repair planning and apply orchestration |
| `internal/plan/` | Plan contract helpers and validation |
| `internal/planreview/` | Prompt policy and terminal review support for apply |
| `internal/httpapi/` | Private `/admin/v1` route scaffold and responses |
| `internal/domain/` | API-facing and workflow-facing domain models |
| `api/` | OpenAPI source for the private control-plane API |
| `e2e/` | Live tests against iRODS and Keycloak |
| `deployments/` | Disposable iRODS + Keycloak Docker test framework |

## Use Case and Toolkit Model

The main use case is operating an iRODS deployment where Keycloak provides the
modern identity integration layer while iRODS remains the data authorization
authority.

Keycloak can provide:

* OIDC/SAML login, federation, MFA, and service-account flows
* an account and admin UX for users, groups, and identity provider integration
* mirror groups that make iRODS authorization state visible to applications

iRODS remains authoritative for:

* users, groups, and membership used for data access
* ACLs, resources, collections, data objects, tickets, and metadata
* native iRODS administrative workflows where elevated iRODS changes are needed

The toolkit approach keeps those responsibilities separate. This repository
contains orchestration, planning, Keycloak mirror repair, provisioning stubs,
diagnostics, and Keycloak-facing API surfaces. Reusable low-level iRODS access
and synchronization behavior should be pushed into shared libraries rather than
duplicated locally.

## Repository Boundaries

`go-irodsclient` is the canonical Go client library for iRODS connections,
sessions, iRODS users, groups, AVUs, ACLs, and administrative primitives. This
project should reuse those types and operations instead of defining parallel
iRODS representations.

`go-irodsclient-extensions` is the shared library target for common
reconciliation, user-sync, OIDC middleware, JWT/introspection, and audit-related
helpers that should be usable by more than one service or command. In this
project, `internal/irodsadapter` is the integration boundary for
`go-irodsclient-extensions/usersync`.

`irods-go-rest` is the generic HTTP API for iRODS resources. It owns reusable
REST endpoints such as user/group administration, logical-path access, metadata,
content, and future generic iRODS resources. Keycloak workflows that need an
external HTTP iRODS API should use those routes rather than adding generic iRODS
CRUD under this repository's `/admin/v1` API.

`irods-keycloak-admin` is the orchestration and control-plane toolkit. Local
`irods-kc-*` commands use direct `go-irodsclient` and shared extension-library
code, not `irods-go-rest` as an intermediate hop. The private API is reserved
for Keycloak integration, service callbacks, sync planning/apply, repair,
bootstrap, provisioning, events, and diagnostics.

## Commands

### `irods-kc-sync`

`irods-kc-sync` is the primary operator CLI for synchronization workflows.

Implemented commands:

* `repair-keycloak --dry-run`: reads iRODS group state, reads Keycloak mirror
  group state, and emits a read-only repair plan.
* `apply --plan plan.json`: validates and applies a saved plan to Keycloak
  mirror state. It mutates Keycloak only and does not mutate iRODS.

Scaffolded commands:

* `plan`: reserved for broader sync planning.
* `bootstrap-keycloak`: reserved for initial Keycloak mirror setup from iRODS
  authority.

Prompt policy for `apply` is controlled by `--prompts`:

* `--prompts=required`: default; prompt only for operations marked
  `requires_approval`.
* `--prompts=all`: prompt for every operation.
* `--prompts=none`: run all valid operations without prompts.

Prompted choices are accept, skip, accept all, and skip all. `accept all`
switches the remaining apply run to the same behavior as `--prompts=none`.

### `irods-kc-admin-server`

`irods-kc-admin-server` runs the private `/admin/v1` HTTP control-plane API.
This API is for orchestration workflows such as sync planning, sync apply,
Keycloak repair/bootstrap, provisioning decisions, Keycloak events, and
diagnostics. It is not a generic iRODS REST API.

### `irods-kc-doctor`

`irods-kc-doctor` is scaffolded for operator checks:

* `check-config`
* `check-mapping`
* `check-drift`

These commands are intended to validate configuration, identity mapping, and
Keycloak/iRODS drift without introducing another source of authority.

### `irods-kc-admin`

`irods-kc-admin` is reserved for optional diagnostics or narrow administrative
helpers. It should not grow duplicate iRODS user/group commands that already
belong in iCommands or `gocmd`.

## Quick Start

Run the package tests:

```bash
go test ./...
```

Generate a Keycloak repair plan without mutating iRODS or Keycloak:

```bash
go run ./cmd/irods-kc-sync repair-keycloak --dry-run --realm irods
```

Write the same plan to a file while preserving JSON on stdout:

```bash
go run ./cmd/irods-kc-sync repair-keycloak --dry-run --realm irods --out plan.json
```

`--plan-path` is accepted as an alias for `--out`. If both are supplied, they
must name the same file.

Apply a saved plan to Keycloak mirror state:

```bash
go run ./cmd/irods-kc-sync apply --plan plan.json
```

Run apply with full prompts or unattended operation:

```bash
go run ./cmd/irods-kc-sync apply --plan plan.json --prompts=all
go run ./cmd/irods-kc-sync apply --plan plan.json --prompts=none
```

Install the commands onto your Go binary path:

```bash
make install
export PATH="$(go env GOPATH)/bin:$PATH"
irods-kc-sync repair-keycloak --dry-run --realm irods --out plan.json
irods-kc-sync apply --plan plan.json
```

Run the control-plane server:

```bash
go run ./cmd/irods-kc-admin-server --listen-address :8081
```

Then check:

* `GET /healthz`
* `GET /admin/v1/status`
* `GET /admin/v1/config/summary`

## Configuration

The command line tools can read iRODS connection details from explicit
environment variables or from the iCommands-compatible environment. This keeps
local command behavior aligned with iCommands, `gocmd`, and `drscmd` instead of
creating a separate configuration model.

Common direct iRODS settings include:

* `IRODS_KC_IRODS_HOST`
* `IRODS_KC_IRODS_PORT`
* `IRODS_KC_IRODS_USER`
* `IRODS_KC_IRODS_PASSWORD`
* `IRODS_KC_IRODS_RESOURCE`
* `IRODS_KC_IRODS_ZONE`
* `IRODS_ENVIRONMENT_FILE`

When direct iRODS host settings are not supplied, `irods-kc-sync` falls back to
the iCommands environment file. The `--irods-env` flag can point at a specific
iCommands environment file.

## Keycloak Credentials

`repair-keycloak --dry-run` reads Keycloak through Admin REST. `apply` mutates
Keycloak mirror groups and memberships through the same Admin REST credential
model.

For the local Docker test framework:

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

irods-kc-sync repair-keycloak --dry-run --realm irods --out plan.json
irods-kc-sync apply --plan plan.json
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

The corresponding environment variables are:

* `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID`
* `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET`

The client must be allowed to call Keycloak Admin REST for the target realm.

## API Boundary

`/admin/v1` is an orchestration API. It is for sync planning/apply, Keycloak
repair/bootstrap, provisioning decisions, Keycloak events, and diagnostics. It
does not expose generic iRODS user, group, path, ticket, resource, or metadata
CRUD.

Generic iRODS HTTP resources belong in `irods-go-rest`. Direct shell-oriented
iRODS administration belongs in iCommands and `gocmd`.

Current control-plane routes are documented in [api/openapi.yaml](./api/openapi.yaml).

## Stack and Testing Strategy

The implementation is written in Go. The current vertical slice uses direct
`go-irodsclient` reads for iRODS authority and Keycloak Admin REST for Keycloak
mirror reads and mutations. The workflow layer keeps planning, apply
validation, prompt behavior, and service calls separated so the same plan
contract can be used by CLI, API, and future Keycloak integration paths.

Testing includes:

* package-local unit tests for workflow, plan, prompt, HTTP, and Keycloak client
  behavior
* live e2e tests against a reachable iRODS provider and Keycloak instance
* repeat-apply coverage to confirm already-converged Keycloak mirror state is
  handled safely

Run unit tests:

```bash
go test ./...
```

Run against the internal disposable deployment:

```bash
cd deployments/docker-test-framework/5-0
docker compose --profile rest up -d --build
cd ../../..
set -a
. e2e/config/internal.env
set +a
go test ./e2e
```

Run against `irods-grid-stack`:

```bash
cd ../irods-grid-stack
docker compose --profile rest up -d --build
cd ../irods-keycloak-admin
set -a
. e2e/config/grid-stack.env
set +a
go test ./e2e
```

The two e2e configurations intentionally use the same endpoint contract so
integration tests can target either the local deployment or `irods-grid-stack`
with the same expected behavior.

## References

* [go-irodsclient](https://github.com/cyverse/go-irodsclient)
* [go-irodsclient-extensions](https://github.com/michael-conway/go-irodsclient-extensions)
* [irods-go-rest](https://github.com/michael-conway/irods-go-rest)
* [irods-grid-stack](https://github.com/michael-conway/irods-grid-stack)
* [Keycloak Admin REST API](https://www.keycloak.org/docs-api/latest/rest-api/index.html)
