# Configuration

`irods-keycloak-admin` reads configuration from defaults, an optional YAML file,
and environment variable overrides.

The YAML format intentionally uses PascalCase keys, matching the style used by
`irods-go-rest`.

## Loading Order

1. Built-in defaults are applied.
2. If `IRODS_KC_CONFIG_FILE` is set, the YAML file at that path is loaded.
3. `IRODS_KC_*` environment variables override YAML values.
4. `*File` secret settings are resolved if the direct secret value is empty.

The admin server uses the error-returning loader and fails fast if
`IRODS_KC_CONFIG_FILE` points to a missing or invalid file.

## Sample

Use the local sample for `irods-grid-stack` plus this repository's disposable
Keycloak deployment:

```bash
export IRODS_KC_CONFIG_FILE=internal/config/keycloak-admin.grid-stack.sample.yaml
irods-kc-admin-server
```

The sample assumes:

- iRODS provider: `127.0.0.1:1247`
- iRODS zone: `tempZone`
- iRODS admin user/password: `rods` / `rods`
- default resource: `providerResc`
- Keycloak URL: `https://127.0.0.1:8443`
- Keycloak realm: `irods`
- Keycloak bootstrap admin: `admin` / `admin`
- Keycloak admin API client: `irods-kc-admin-api`
- Keycloak event shared-secret header: `X-IRODS-KC-Shared-Secret`

## YAML Keys

```yaml
ServiceName: irods-keycloak-admin
ListenAddress: :8081
PublicURL: http://127.0.0.1:8081
LogLevel: info

IRODSHost: 127.0.0.1
IRODSPort: 1247
IRODSZone: tempZone
IRODSAdminUser: rods
IRODSAdminPassword: rods
IRODSAdminPasswordFile:
IRODSDefaultResource: providerResc
IRODSAuthScheme: native
IRODSNegotiationPolicy: CS_NEG_DONT_CARE

KeycloakBaseURL: https://127.0.0.1:8443
KeycloakRealm: irods
KeycloakAdminRealm: master
KeycloakAdminUser: admin
KeycloakAdminPassword: admin
KeycloakAdminPasswordFile:
KeycloakAdminClientID: irods-kc-admin-api
KeycloakAdminClientSecret: irods-kc-admin-api-secret
KeycloakAdminClientSecretFile:
KeycloakInsecureSkipVerify: true
KeycloakMirrorRoot: /irods
KeycloakEventSharedSecret: local-irods-keycloak-admin-event-secret
KeycloakEventSharedSecretFile:
```

## Environment Overrides

General service settings:

- `IRODS_KC_CONFIG_FILE`
- `IRODS_KC_SERVICE_NAME`
- `IRODS_KC_LISTEN_ADDRESS`
- `IRODS_KC_PUBLIC_URL`
- `IRODS_KC_LOG_LEVEL`

iRODS settings:

- `IRODS_KC_IRODS_HOST`
- `IRODS_KC_IRODS_PORT`
- `IRODS_KC_IRODS_ZONE`
- `IRODS_KC_IRODS_ADMIN_USER`
- `IRODS_KC_IRODS_USER`
- `IRODS_KC_IRODS_ADMIN_PASSWORD`
- `IRODS_KC_IRODS_PASSWORD`
- `IRODS_KC_IRODS_ADMIN_PASSWORD_FILE`
- `IRODS_KC_IRODS_DEFAULT_RESOURCE`
- `IRODS_KC_IRODS_RESOURCE`
- `IRODS_KC_IRODS_AUTH_SCHEME`
- `IRODS_KC_IRODS_NEGOTIATION_POLICY`

Keycloak settings:

- `IRODS_KC_KEYCLOAK_BASE_URL`
- `IRODS_KC_KEYCLOAK_REALM`
- `IRODS_KC_KEYCLOAK_ADMIN_REALM`
- `IRODS_KC_KEYCLOAK_ADMIN_USER`
- `IRODS_KC_KEYCLOAK_ADMIN_PASSWORD`
- `IRODS_KC_KEYCLOAK_ADMIN_PASSWORD_FILE`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_ID`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET`
- `IRODS_KC_KEYCLOAK_ADMIN_CLIENT_SECRET_FILE`
- `IRODS_KC_KEYCLOAK_INSECURE_SKIP_VERIFY`
- `IRODS_KC_KEYCLOAK_MIRROR_ROOT`
- `IRODS_KC_KEYCLOAK_EVENT_SHARED_SECRET`
- `IRODS_KC_KEYCLOAK_EVENT_SHARED_SECRET_FILE`

## Secrets

For local disposable stacks, direct password values are acceptable. For any
shared or persistent environment, prefer file-based secrets:

```yaml
IRODSAdminPasswordFile: /run/secrets/irods-admin-password
KeycloakAdminPasswordFile: /run/secrets/keycloak-admin-password
KeycloakAdminClientSecretFile: /run/secrets/keycloak-admin-client-secret
KeycloakEventSharedSecretFile: /run/secrets/irods-kc-event-shared-secret
```

If both a direct value and a `*File` value are set, the direct value wins.

## Event Callback Authentication

The current OpenAPI contract exposes:

```http
POST /admin/v1/keycloak/events
X-IRODS-KC-Shared-Secret: ...
```

The API validates `X-IRODS-KC-Shared-Secret` against
`KeycloakEventSharedSecret`, or against the contents of
`KeycloakEventSharedSecretFile`.

This shared-secret model is the initial private-service trust boundary. It is
not the final hardening story. Production deployments should consider mTLS,
signed event bodies, replay windows, key rotation, and service-account tokens.
