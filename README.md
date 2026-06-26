# iRODS Keycloak Admin Toolkit

[![Test](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/test.yml)
[![CodeQL](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml/badge.svg)](https://github.com/michael-conway/irods-keycloak-admin/actions/workflows/codeql.yml)

`irods-keycloak-admin` is an alpha Go toolkit for coordinating selected iRODS
and Keycloak administrative workflows. It focuses on reviewable identity,
group, membership, and mapping operations where Keycloak provides the
operator-facing control surface and iRODS remains the data authorization
system.

## Status

| Field | Value |
| --- | --- |
| Release | `0.1.0-dev` |
| Stability | Alpha / active development |
| License | `BSD-3-Clause` |
| Repository | `https://github.com/michael-conway/irods-keycloak-admin` |
| Issues | `https://github.com/michael-conway/irods-keycloak-admin/issues` |

The current working behavior is CLI-first. The HTTP API is intentionally narrow
while the Keycloak event-listener integration is being specified.

## Purpose

This project is not a generic iRODS administration API and does not replace
iRODS ACLs, users, groups, tickets, collections, data objects, resources, or
metadata administration.

The purpose is to provide a focused control plane for deployments that need
Keycloak and iRODS to cooperate around identity administration without adding a
separate identity synchronization database.

## Use Cases

Primary use cases:

- Provision selected Keycloak users into iRODS.
- Provision selected Keycloak groups into iRODS.
- Reconcile selected Keycloak group membership into iRODS group membership.
- Maintain minimal iRODS AVUs needed for stable Keycloak/iRODS mapping.
- Repair Keycloak mirror state from iRODS state through reviewable plans.
- Accept private Keycloak event-listener callbacks for future activity-driven
  iRODS user, group, and membership mutation.

Explicit non-goals:

- Generic iRODS REST CRUD.
- A separate permission database.
- Broad unreviewed destructive deprovisioning.
- Java-side iRODS administration logic inside Keycloak.

## Main Components

| Component | Purpose |
| --- | --- |
| `irods-kc-sync` | CLI for current plan/apply workflows. |
| `irods-kc-admin-server` | Private HTTP control-plane server. |
| `api/openapi.yaml` | Current HTTP contract. |
| `internal/config` | Runtime configuration loader and samples. |
| `deployments/docker-test-framework/5-0` | Disposable Keycloak test deployment. |
| `e2e` | Live test fixtures and notes. |

## Quick Start

Run tests:

```bash
go test ./...
```

Install commands:

```bash
make install
export PATH="$(go env GOPATH)/bin:$PATH"
```

Run the admin server with the local sample configuration:

```bash
export IRODS_KC_CONFIG_FILE=internal/config/keycloak-admin.grid-stack.sample.yaml
irods-kc-admin-server
```

## Runtime Model

The current administrator workflow is plan-first:

```text
Generate a plan.
Review the plan.
Apply the accepted plan.
Run a follow-up check for convergence.
```

Keycloak event-listener work is being shaped as a thin Java plugin that calls
this Go service. The Go service owns iRODS mutation behavior, mapping policy,
audit contracts, and callback validation.

See the documents below for command examples, configuration, and roadmap
details.

## Documentation

- [Administrators Guide](./IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md) - operator
  model, command examples, review checklist, and runbook.
- [Configuration](./internal/config/CONFIGURATION.md) - YAML format,
  environment overrides, secret-file support, and sample config.
- [Developer Notes](./DEVELOPER_NOTES.md) - architecture decisions, sprint
  planning, repository boundaries, and implementation strategy.
- [OpenAPI Contract](./api/openapi.yaml) - current private HTTP API contract.
- [Deployment Notes](./deployments/README.md) - local deployment context.
- [E2E Notes](./e2e/README.md) - live test setup and fixtures.

## References

- [go-irodsclient](https://github.com/cyverse/go-irodsclient)
- [go-irodsclient-extensions](https://github.com/michael-conway/go-irodsclient-extensions)
- [irods-go-rest](https://github.com/michael-conway/irods-go-rest)
- [irods-grid-stack](https://github.com/michael-conway/irods-grid-stack)
