# e2e Configuration

The e2e tests use a two-stack local environment:

- `irods-grid-stack` provides iRODS, optional `irods-go-rest`, and optional
  Starbase.
- `deployments/docker-test-framework/5-0` provides Keycloak and a dedicated
  Keycloak PostgreSQL database.

The combined environment should expose these host endpoints:

| Service | Default |
|---|---|
| iRODS provider | `127.0.0.1:1247` |
| provider REST | `http://127.0.0.1:8080` |
| resource REST | `http://127.0.0.1:8082` |
| Keycloak | `https://127.0.0.1:8443` |
| Keycloak management | `http://127.0.0.1:19090` |

Start `irods-grid-stack` without its frontend or Keycloak profiles. Enable REST
and Starbase there when tests need them:

```bash
cd ../irods-grid-stack
docker compose --profile rest --profile starbase up -d --build
```

Start the Keycloak-only deployment from this repository:

```bash
cd ../irods-keycloak-admin/deployments/docker-test-framework/5-0
docker compose up -d --build
```

Run the e2e tests:

```bash
cd ../../..
set -a
. e2e/config/grid-stack.env
set +a
go test ./e2e
```

Run only the live apply coverage:

```bash
go test ./e2e -run 'TestKCSyncApply' -count=1 -v
```

Tests should call `RequireConfig(t)` before touching live services. Unit tests
in this directory may call `LoadConfig()` directly and must not require live
containers.

The live tests generate repair plans from the running systems and apply filtered
plans against temporary `kcapply*`, `kcdry*`, `kcirodsuser*`, and `kcirods*`
fixtures. Apply tests cover Keycloak mirror repair, selected Keycloak-to-iRODS
user and group provisioning, membership drift, and iRODS-to-Keycloak user
creation through `irods-kc-sync apply`. iRODS fixture setup, cleanup, and AVU
assertions use `go-irodsclient` plus `go-irodsclient-extensions` directly rather
than shelling out to external command-line clients. The apply coverage also replays the same plan
after convergence to verify repeat apply behavior.

The e2e env files also provide Keycloak Admin REST credentials for dry-run and
apply tests:

```bash
IRODS_KC_E2E_KEYCLOAK_BASE_URL=https://127.0.0.1:8443
IRODS_KC_E2E_KEYCLOAK_REALM=irods
IRODS_KC_E2E_KEYCLOAK_ADMIN_USER=admin
IRODS_KC_E2E_KEYCLOAK_ADMIN_PASSWORD=admin
IRODS_KC_E2E_KEYCLOAK_INSECURE_SKIP_VERIFY=true
```

These map to the same command-line flags used by `irods-kc-sync
sync --dry-run` and `irods-kc-sync apply`.

The apply e2e tests use prompt policy explicitly:

- `--prompts=none` for unattended create/member repair.
- `--prompts=required` with scripted `accept` or `skip` input for stale mirror
  delete approval behavior.
