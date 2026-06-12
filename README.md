# iRODS Keycloak Admin Toolkit

[![Test](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml)
[![CodeQL](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml)

Go toolkit for planning and applying Keycloak mirror-group repair from
authoritative iRODS group state.

## Current Status

This repository is in active development. The implemented vertical slice today
is the Keycloak mirror repair workflow:

- read iRODS groups and memberships directly through `go-irodsclient`
- read Keycloak groups and memberships through Admin REST
- produce a deterministic repair plan with explicit evidence
- apply a saved plan back to Keycloak only

What is implemented now:

- `irods-kc-sync repair-keycloak --dry-run`
- `irods-kc-sync apply --plan ...`
- mirror root configuration via `IRODS_KC_KEYCLOAK_MIRROR_ROOT` or
  `--keycloak-mirror-root`
- prompt policy for guarded apply operations
- private HTTP route scaffolding under `/admin/v1`
- unit tests plus live e2e coverage against the disposable Docker stack and
  `irods-grid-stack`

What is still scaffolded or reserved:

- generic `plan`
- `bootstrap-keycloak`
- provisioning workflows
- diagnostics workflows behind `irods-kc-doctor`
- most `/admin/v1` orchestration endpoints

## Repository Boundary

This project is not a replacement for iCommands, `gocmd`, or `irods-go-rest`.
Generic iRODS administration still belongs in those tools.

This repository owns:

- Keycloak-facing orchestration
- repair planning and guarded apply
- mirror-path policy
- plan contracts and review behavior
- a Keycloak Admin REST client used by the repair/apply flow

iRODS remains authoritative for users, groups, memberships, ACLs, collections,
data objects, resources, tickets, and metadata.

## Commands

### `irods-kc-sync`

This is the only command with real workflow behavior today.

Implemented subcommands:

- `repair-keycloak --dry-run`
- `apply --plan PLAN.json`

Scaffolded subcommands:

- `plan`
- `bootstrap-keycloak`

`repair-keycloak --dry-run`:

- reads iRODS rods-group state for the selected zone
- reads managed Keycloak mirror groups under the configured mirror root
- emits a JSON plan to stdout
- never mutates iRODS or Keycloak

`apply --plan`:

- validates a saved plan
- applies supported operations to Keycloak only
- never mutates iRODS
- supports repeat apply on already converged state

Supported plan operations today:

- `keycloak.group.create`
- `keycloak.group.member.add`
- `keycloak.group.member.remove`
- `keycloak.group.delete`

Prompt behavior for `apply` is controlled by `--prompts`:

- `required`: default; prompt only for operations marked
  `requires_approval`
- `all`: prompt for every operation
- `none`: run without prompts

Interactive decisions are `accept`, `skip`, `accept all`, and `skip all`.

### `irods-kc-admin-server`

Runs the private HTTP control-plane server.

What works now:

- `GET /healthz`
- `GET /admin/v1/status`
- `GET /admin/v1/config/summary`

The broader `/admin/v1` orchestration routes are present, but the backing
services are still wired to `not implemented` stubs. Treat the server as a
scaffolded API surface, not a production workflow endpoint.

### `irods-kc-doctor`

The command exists, but `check-config`, `check-mapping`, and `check-drift` are
scaffolded and currently exit with a not-implemented message.

### `irods-kc-admin`

Reserved for future narrow administrative helpers. It is not implemented.

## Quick Start

Run the package tests:

```bash
go test ./...
```

Generate a read-only repair plan:

```bash
go run ./cmd/irods-kc-sync repair-keycloak --dry-run --realm irods
```

Write the same plan to a file while preserving stdout JSON:

```bash
go run ./cmd/irods-kc-sync repair-keycloak --dry-run --realm irods --out plan.json
```

`--plan-path` is accepted as an alias for `--out`. If both are supplied, they
must match.

Apply a saved plan to Keycloak:

```bash
go run ./cmd/irods-kc-sync apply --plan plan.json
```

Run apply with explicit prompt policy:

```bash
go run ./cmd/irods-kc-sync apply --plan plan.json --prompts=all
go run ./cmd/irods-kc-sync apply --plan plan.json --prompts=none
```

Install the commands:

```bash
make install
export PATH="$(go env GOPATH)/bin:$PATH"
```

Run the server scaffold:

```bash
go run ./cmd/irods-kc-admin-server --listen-address :8081
```

## Configuration

Common service-level configuration:

- `IRODS_KC_SERVICE_NAME`
- `IRODS_KC_LISTEN_ADDRESS`
- `IRODS_KC_IRODS_ZONE`
- `IRODS_KC_KEYCLOAK_REALM`
- `IRODS_KC_KEYCLOAK_MIRROR_ROOT`

`repair-keycloak` can connect to iRODS either with explicit connection
parameters or by falling back to an iCommands-compatible environment.

Direct iRODS settings:

- `IRODS_KC_IRODS_HOST`
- `IRODS_KC_IRODS_PORT`
- `IRODS_KC_IRODS_USER`
- `IRODS_KC_IRODS_PASSWORD`
- `IRODS_KC_IRODS_RESOURCE`
- `IRODS_KC_IRODS_ZONE`
- `IRODS_ENVIRONMENT_FILE`

If direct host settings are omitted, `irods-kc-sync` falls back to the
iCommands environment file. `--irods-env` can point to a specific environment
file.

Keycloak Admin REST settings:

- `IRODS_KC_KEYCLOAK_BASE_URL`
- `IRODS_KC_KEYCLOAK_ADMIN_REALM`
- `IRODS_KC_KEYCLOAK_ADMIN_USER`
- `IRODS_KC_KEYCLOAK_ADMIN_PASSWORD`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET`
- `IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY`

The same values are available as CLI flags on `repair-keycloak` and `apply`.

Example local dry-run invocation:

```bash
irods-kc-sync repair-keycloak --dry-run \
  --realm irods \
  --keycloak-url https://127.0.0.1:8443 \
  --keycloak-admin-user admin \
  --keycloak-admin-password admin \
  --keycloak-insecure-skip-verify
```

Example service-client invocation:

```bash
irods-kc-sync repair-keycloak --dry-run \
  --realm irods \
  --keycloak-url https://keycloak.example.org \
  --keycloak-admin-realm master \
  --keycloak-client-id irods-kc-admin-cli \
  --keycloak-client-secret "$IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET"
```

## Repair Plan Semantics

The current repair workflow treats iRODS as the authority and Keycloak as the
mirror target.

The dry-run plan:

- is emitted as JSON
- includes plan metadata, summary counts, and ordered operations
- records evidence such as iRODS group name, zone, Keycloak realm, mirror path,
  and group or user identifiers when available
- marks destructive stale-mirror deletes with `requires_approval`

The configured mirror root defaults to `/irods`. Managed Keycloak groups are
resolved relative to that root unless another root is provided.

## HTTP API

The route surface is documented in [api/openapi.yaml](./api/openapi.yaml), but
that document is broader than the currently implemented service behavior.

Today:

- health and config summary endpoints work
- orchestration routes exist
- most orchestration routes return not-implemented responses because the app is
  still wired to stub services

Use the CLI for real repair/apply behavior.

## Testing

Unit and integration coverage currently focus on the repair/apply slice.

Included coverage:

- workflow and plan unit tests
- prompt-review and HTTP handler tests
- Keycloak Admin REST client tests
- live e2e tests for dry-run planning and apply behavior
- repeat-apply checks on converged Keycloak mirror state

Run all package tests:

```bash
go test ./...
```

Run e2e tests against the internal disposable deployment:

```bash
cd deployments/docker-test-framework/5-0
docker compose --profile rest up -d --build
cd ../../..
set -a
. e2e/config/internal.env
set +a
go test ./e2e
```

Run e2e tests against `irods-grid-stack`:

```bash
cd ../irods-grid-stack
docker compose --profile rest up -d --build
cd ../irods-keycloak-admin
set -a
. e2e/config/grid-stack.env
set +a
go test ./e2e
```

The two e2e environments intentionally share the same endpoint contract.

## References

- [Developer Notes](./DEVELOPER_NOTES.md)
- [OpenAPI Control-Plane Contract](./api/openapi.yaml)
- [e2e Configuration](./e2e/README.md)
- [Deployment Notes](./deployments/README.md)
- [go-irodsclient](https://github.com/cyverse/go-irodsclient)
- [go-irodsclient-extensions](https://github.com/michael-conway/go-irodsclient-extensions)
- [irods-go-rest](https://github.com/michael-conway/irods-go-rest)
- [irods-grid-stack](https://github.com/michael-conway/irods-grid-stack)
