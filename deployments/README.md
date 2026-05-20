# Disposable iRODS + Keycloak Test Environment

This directory contains a local Docker Compose environment for integration and
e2e testing of `irods-keycloak-admin`.

The stack starts only the services needed by this repository:

- PostgreSQL for iRODS ICAT and Keycloak.
- iRODS 5.x catalog provider.
- Keycloak with an imported test realm.

It intentionally does not start application services or storage gateway
containers.

## Start The Stack

```bash
cd deployments/docker-test-framework/5-0
docker compose up -d --build
```

Key endpoints and credentials:

| Service | Value |
|---|---|
| iRODS host | `localhost:1247` |
| iRODS zone | `tempZone` |
| iRODS admin | `rods` / `rods` |
| iRODS test admin | `test1` / `test` |
| iRODS test users | `test2` / `test`, `test3` / `test` |
| Keycloak admin console | `https://localhost:8443/admin` |
| Keycloak admin | `admin` / `admin` |
| Keycloak realm | `irods` |
| Admin API client | `irods-kc-admin-api` |
| Admin CLI client | `irods-kc-admin-cli` / `irods-kc-admin-cli-secret` |

The iRODS setup creates test groups that are mirrored in the Keycloak realm:

- `project-alpha`
- `project-beta`
- `irods-admins`

## Reset The Stack

The environment is disposable. Remove containers and volumes with:

```bash
cd deployments/docker-test-framework/5-0
docker compose down -v
```

## Smoke Checks

Check iRODS:

```bash
docker compose exec irods-provider su - irods -c 'iadmin lu'
docker compose exec irods-provider su - irods -c 'iadmin lg project-alpha'
```

Check Keycloak:

```bash
docker compose ps keycloak
```

Open the admin console at `https://localhost:8443/admin` and inspect the
`irods` realm.

## Notes

The imported Keycloak realm is intentionally minimal. It provides fixture users,
fixture groups, and confidential clients suitable for exercising the
`irods-keycloak-admin` control-plane API and future CLI workflows.

Generic iRODS REST services and other application services should be run
separately when a test needs them. This stack is only the disposable identity
and iRODS backing environment.
