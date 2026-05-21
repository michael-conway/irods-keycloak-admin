# e2e Configuration

The e2e tests use one environment contract for both local deployments:

- `deployments/docker-test-framework/5-0`
- `irods-grid-stack`

Both targets should expose the same host endpoints:

| Service | Default |
|---|---|
| iRODS provider | `127.0.0.1:1247` |
| provider REST | `http://127.0.0.1:8080` |
| resource REST | `http://127.0.0.1:8082` |
| Keycloak | `https://127.0.0.1:8443` |
| Keycloak management | `http://127.0.0.1:19090` |

Run against the internal deployment:

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

Tests should call `RequireConfig(t)` before touching live services. Unit tests
in this directory may call `LoadConfig()` directly and must not require live
containers.
