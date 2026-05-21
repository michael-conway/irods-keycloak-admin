# Disposable iRODS + Keycloak Test Environment

This directory contains a local Docker Compose environment for integration and
e2e testing of `irods-keycloak-admin`.

The stack starts only the services needed by this repository:

- PostgreSQL for iRODS ICAT and Keycloak.
- iRODS 5.x catalog provider.
- Keycloak with an imported test realm.
- Optional provider-side and resource-port-compatible `irods-go-rest`
  services under the `rest` profile.

It intentionally does not start application services or storage gateway
containers.

## Start The Stack

```bash
cd deployments/docker-test-framework/5-0
docker compose up -d --build
```

Start the optional REST endpoints with the same host ports used by
`irods-grid-stack`:

```bash
cd deployments/docker-test-framework/5-0
docker compose --profile rest up -d --build
```

Key endpoints and credentials:

| Service | Value |
|---|---|
| iRODS host | `localhost:1247` |
| Provider REST | `http://127.0.0.1:8080` |
| Resource REST compatibility endpoint | `http://127.0.0.1:8082` |
| iRODS zone | `tempZone` |
| iRODS provider resource | `providerResc` |
| iRODS admin | `rods` / `rods` |
| iRODS test admin | `test1` / `test` |
| iRODS test users | `test2` / `test`, `test3` / `test` |
| Keycloak admin console | `https://localhost:8443/admin` |
| Keycloak management | `http://127.0.0.1:19090` |
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

Check REST when the `rest` profile is enabled:

```bash
curl -k http://127.0.0.1:8080/healthz
curl -k http://127.0.0.1:8082/healthz
```

Open the admin console at `https://localhost:8443/admin` and inspect the
`irods` realm.

## Notes

The imported Keycloak realm is intentionally minimal. It provides fixture users,
fixture groups, and confidential clients suitable for exercising the
`irods-keycloak-admin` control-plane API and future CLI workflows.

The service names and default host ports intentionally match
`irods-grid-stack` for the provider-side endpoints:

- `irods-provider` on host port `1247`.
- `irods-go-rest-provider` on host port `8080`.
- `irods-go-rest-resource` on host port `8082`.
- `keycloak` on host port `8443`, with management on `19090`.

Unlike `irods-grid-stack`, this repository's internal deployment does not start
a second iRODS resource server. The `irods-go-rest-resource` service is a
port/name compatibility endpoint backed by `irods-provider`, which is sufficient
for keycloak-admin integration tests that only need stable REST endpoint names.

This lets e2e and integration tests source either `e2e/config/internal.env` or
`e2e/config/grid-stack.env` and exercise the same endpoint contract.
