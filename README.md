# iRODS Keycloak Admin Toolkit

[![Test](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml)
[![CodeQL](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml)

Go toolkit for reviewable iRODS and Keycloak synchronization workflows.

The current implementation focuses on:

- planning and applying Keycloak mirror repair from iRODS state
- provisioning selected Keycloak users into iRODS
- provisioning selected Keycloak groups into iRODS
- reconciling selected Keycloak group membership into iRODS
- maintaining minimal iRODS mapping AVUs
- producing scenario-3 password-action reports without handling password values

For operator procedures, examples, review checklists, and sync behavior, start
with the [Administrators Guide](./IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md).

Developer planning and roadmap notes are in
[DEVELOPER_NOTES.md](./DEVELOPER_NOTES.md).

## Status

This repository is in active development.

Implemented:

- `irods-kc-sync sync --dry-run`
- `irods-kc-sync apply --plan`
- `--target=keycloak` mirror repair from iRODS into Keycloak
- `--target=irods` selected user, group, and group-membership planning from
  Keycloak into iRODS
- guarded apply prompts
- repeat apply on converged state
- live e2e coverage against the disposable Docker stack and `irods-grid-stack`

Scaffolded or reserved:

- REST orchestration routes under `/admin/v1`
- generic `plan`
- `bootstrap-keycloak`
- `irods-kc-doctor` diagnostics workflows
- scenario-3 password setup/reset execution

Use the CLI for real workflow behavior today. Treat the HTTP API as a scaffold
until the REST API equivalence sprint is implemented.

## Repository Boundary

This project is not a generic iRODS administration tool.

This repository owns:

- Keycloak-facing synchronization orchestration
- plan contracts and review behavior
- guarded apply behavior
- Keycloak mirror-path policy
- a Keycloak Admin REST client used by sync/apply
- direct iRODS adapter code over `go-irodsclient` and
  `go-irodsclient-extensions`

iRODS remains authoritative for data users, groups, ACLs, collections, data
objects, resources, tickets, and metadata.

## Commands

### `irods-kc-sync`

Main implemented workflow command.

```bash
irods-kc-sync sync --dry-run ...
irods-kc-sync apply --plan plan.json ...
```

See the [Administrators Guide](./IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md) for
complete examples covering user provisioning, group provisioning, membership
reconciliation, repeat apply, and convergence checks.

### `irods-kc-admin-server`

Runs the private HTTP control-plane server.

Implemented today:

- `GET /healthz`
- `GET /admin/v1/status`
- `GET /admin/v1/config/summary`

Most orchestration routes are present but still return not-implemented
responses.

### `irods-kc-doctor`

Reserved for diagnostics. Current subcommands are scaffolded.

### `irods-kc-admin`

Reserved for future narrow administrative helpers.

## Quick Start

Run all tests:

```bash
go test ./...
```

Install commands:

```bash
make install
export PATH="$(go env GOPATH)/bin:$PATH"
```

Generate a plan:

```bash
irods-kc-sync sync --dry-run --target=irods --keycloak-user-id USER_UUID --out plan.json
```

Apply a reviewed plan:

```bash
irods-kc-sync apply --plan plan.json --prompts required
```

Those short examples omit required iRODS and Keycloak connection settings. Use
the [Administrators Guide](./IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md) for the
full command forms.

## Configuration

The CLI uses direct iRODS and Keycloak connection settings. Common environment
variables are:

- `IRODS_KC_IRODS_HOST`
- `IRODS_KC_IRODS_PORT`
- `IRODS_KC_IRODS_USER`
- `IRODS_KC_IRODS_PASSWORD`
- `IRODS_KC_IRODS_RESOURCE`
- `IRODS_KC_IRODS_ZONE`
- `IRODS_KC_KEYCLOAK_BASE_URL`
- `IRODS_KC_KEYCLOAK_ADMIN_REALM`
- `IRODS_KC_KEYCLOAK_ADMIN_USER`
- `IRODS_KC_KEYCLOAK_ADMIN_PASSWORD`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET`
- `IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY`
- `IRODS_KC_KEYCLOAK_MIRROR_ROOT`

The same values are available as CLI flags.

## Testing

Run all package tests:

```bash
go test ./...
```

Run live e2e tests against `irods-grid-stack` plus the Keycloak-only disposable
deployment:

```bash
cd ../irods-grid-stack
docker compose --profile rest --profile starbase up -d --build
cd ../irods-keycloak-admin/deployments/docker-test-framework/5-0
docker compose up -d --build
cd ../../..
set -a
. e2e/config/grid-stack.env
set +a
go test ./e2e
```

Additional e2e notes are in [e2e/README.md](./e2e/README.md).

## Documentation

- [Administrators Guide](./IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md): current
  operator model, command examples, runbook, decision table, and scenario notes.
- [Developer Notes](./DEVELOPER_NOTES.md): roadmap, sprint planning,
  implementation decisions, and architecture notes.
- [OpenAPI Contract](./api/openapi.yaml): intended control-plane route surface.
- [Deployment Notes](./deployments/README.md): local deployment context.

## References

- [go-irodsclient](https://github.com/cyverse/go-irodsclient)
- [go-irodsclient-extensions](https://github.com/michael-conway/go-irodsclient-extensions)
- [irods-go-rest](https://github.com/michael-conway/irods-go-rest)
- [irods-grid-stack](https://github.com/michael-conway/irods-grid-stack)
