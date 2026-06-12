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

Run only the live apply coverage:

```bash
go test ./e2e -run 'TestKCSyncApply' -count=1 -v
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

The live tests generate repair plans from the running systems and apply filtered
plans against temporary `kcapply*`, `kcdry*`, and `kcirodsuser*` fixtures. Apply
tests cover Keycloak mirror repair plus one narrow Keycloak-to-iRODS user
mutation through `irods-kc-sync apply`; iRODS fixture setup and cleanup still use
`iadmin`. The apply coverage also replays the same plan after convergence to
verify repeat apply behavior.

Some live fixture setup uses `docker exec` against the iRODS provider container
to run the same `iadmin` operations used by the disposable stack setup scripts.
Override `IRODS_KC_E2E_IRODS_PROVIDER_CONTAINER` when the compose project or
provider service name differs from the defaults.

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
