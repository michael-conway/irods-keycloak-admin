# Disposable Keycloak Test Environment

This directory contains the `irods-keycloak-admin` side of the local integration
and e2e test environment.

The deployment is intentionally Keycloak-only. iRODS, optional `irods-go-rest`,
and optional Starbase services should be started from `irods-grid-stack`.

This stack starts only:

- A dedicated PostgreSQL database for Keycloak.
- Keycloak with the imported `irods` test realm.

It does not start an iRODS catalog provider, iRODS ICAT database, REST API, web
frontend, or storage gateway containers. This allows it to run alongside
`irods-grid-stack` without competing for iRODS, REST, or ICAT database ports and
volumes.

## Migration Model

Use two Compose projects:

- `irods-grid-stack` owns iRODS, the iRODS ICAT database, optional REST, and
  optional Starbase.
- `irods-keycloak-admin` owns Keycloak and a separate Keycloak PostgreSQL
  database.

Start `irods-grid-stack` without its frontend or Keycloak profiles. Enable the
REST and Starbase profiles there when tests need those services.

Then start this stack to provide the Keycloak realm used by
`irods-keycloak-admin` tests.

Both stacks expose the same host endpoint contract used by the e2e environment
files:

| Service | Owner | Default endpoint |
|---|---|---|
| iRODS provider | `irods-grid-stack` | `127.0.0.1:1247` |
| Provider REST | `irods-grid-stack` | `http://127.0.0.1:8080` |
| Resource REST | `irods-grid-stack` | `http://127.0.0.1:8082` |
| Starbase | `irods-grid-stack` | configured by `irods-grid-stack` |
| Keycloak | `irods-keycloak-admin` | `https://127.0.0.1:8443` |
| Keycloak management | `irods-keycloak-admin` | `http://127.0.0.1:19090` |

## Start iRODS Dependencies

From the `irods-grid-stack` repository, start iRODS and any optional services
needed by the test run. Do not enable that stack's frontend or Keycloak
profiles.

Example:

```bash
cd ../irods-grid-stack
docker compose --profile rest --profile starbase up -d --build
```

If a test only needs iRODS, omit optional profiles as appropriate.

## Start Keycloak

```bash
cd deployments/docker-test-framework/5-0
docker compose up -d --build
```

Key endpoints and credentials:

| Service | Value |
|---|---|
| Keycloak admin console | `https://localhost:8443/admin` |
| Keycloak management | `http://127.0.0.1:19090` |
| Keycloak admin | `admin` / `admin` |
| Keycloak realm | `irods` |
| Admin API client | `irods-kc-admin-api` |
| Admin CLI client | `irods-kc-admin-cli` / `irods-kc-admin-cli-secret` |
| Keycloak database | `KEYCLOAK` |
| Keycloak database user | `keycloak` / `keycloak` |

The imported Keycloak realm is intentionally minimal. It provides fixture users,
fixture groups, and confidential clients suitable for exercising the
`irods-keycloak-admin` control-plane API and CLI workflows.

The Keycloak fixture groups mirror groups expected from `irods-grid-stack`:

- `project-alpha`
- `project-beta`
- `irods-admins`

## Run E2E Tests

Use the `grid-stack` e2e environment because iRODS and REST now come from
`irods-grid-stack`:

```bash
set -a
. e2e/config/grid-stack.env
set +a
go test ./e2e
```

The endpoint defaults remain:

| Service | Value |
|---|---|
| iRODS host | `localhost:1247` |
| Provider REST | `http://127.0.0.1:8080` |
| Resource REST | `http://127.0.0.1:8082` |
| iRODS zone | `tempZone` |
| iRODS provider resource | `providerResc` |
| iRODS admin | `rods` / `rods` |
| iRODS test admin | `test1` / `test` |
| iRODS test users | `test2` / `test`, `test3` / `test` |
| Keycloak admin console | `https://localhost:8443/admin` |
| Keycloak management | `http://127.0.0.1:19090` |

## Reset The Stack

The Keycloak environment is disposable. Remove its containers and Keycloak
database volume with:

```bash
cd deployments/docker-test-framework/5-0
docker compose down -v
```

Reset `irods-grid-stack` separately when the iRODS or REST state needs to be
discarded.

## Smoke Checks

Check the iRODS and REST services from `irods-grid-stack`:

```bash
docker ps --filter name=irods-grid-stack-irods-provider
curl -k http://127.0.0.1:8080/healthz
curl -k http://127.0.0.1:8082/healthz
```

Check this stack's Keycloak service:

```bash
docker compose ps keycloak
```

Open the admin console at `https://localhost:8443/admin` and inspect the
`irods` realm.
