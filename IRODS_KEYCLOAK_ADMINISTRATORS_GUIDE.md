# iRODS Keycloak Administrators Guide

## Purpose

This guide explains how administrators should operate the current
iRODS-Keycloak synchronization tooling.

It focuses on what is implemented now:

- reviewable `irods-kc-sync` plans
- explicit plan apply
- selected Keycloak user provisioning into iRODS
- selected Keycloak group provisioning into iRODS
- selected group membership reconciliation into iRODS
- conservative iRODS-to-Keycloak mirror repair
- mapping AVUs used to keep identities unambiguous

Developer roadmaps and sprint details belong in `DEVELOPER_NOTES.md`. This
guide should stay focused on administrator-facing behavior and should not imply
that planned or scaffolded workflows already exist.

## Current Operating Model

The current workflow is plan-first:

```text
Generate a plan.
Review the plan.
Apply the accepted plan.
Run the same plan command again to confirm convergence.
```

Request and approval intake are out of band. This toolkit does not currently
provide request forms, approval queues, or delegated project-owner workflows.

The current authority model is:

- A Keycloak realm administrator manages users, groups, and membership on the
  Keycloak side.
- An iRODS `rodsadmin` or `groupadmin` identity performs direct iRODS-side
  changes.
- `irods-kc-sync --target=irods` can push a selected Keycloak user, group, or
  group membership state into iRODS.
- `irods-kc-sync --target=keycloak` can repair Keycloak mirror state from
  iRODS.

## Synchronization Terms

`Managed` means the toolkit is allowed to mutate an object intentionally. It
does not mean the object can be deleted just because it is missing on the other
side.

`Mapped` means the toolkit knows which iRODS object and Keycloak object
correspond. Mapping does not automatically imply deletion authority.

`Authority` is a policy hint for directional repair. It is not a blanket rule
that unmatched objects are disposable.

`Candidate addition` means an object or membership exists on one side and can
reasonably be reflected to the other side.

`Candidate removal` means a removal may be appropriate, but should not happen
without stronger evidence or review.

`Conflict` means the toolkit cannot safely decide the correct action from the
available evidence. Conflicts should be resolved by an operator, not silently
mutated.

## Command Reference

`sync` only plans. It must be run with `--dry-run`.

`apply` applies a reviewed plan file.

### Common Connection Flags

Use direct iRODS and Keycloak connection settings:

```bash
--realm "$KC_REALM" \
--zone "$IRODS_ZONE" \
--irods-host "$IRODS_HOST" \
--irods-port "$IRODS_PORT" \
--irods-user "$IRODS_USER" \
--irods-password "$IRODS_PASSWORD" \
--irods-resource "$IRODS_RESOURCE" \
--keycloak-url "$KEYCLOAK_URL" \
--keycloak-admin-user "$KEYCLOAK_ADMIN_USER" \
--keycloak-admin-password "$KEYCLOAK_ADMIN_PASSWORD"
```

Set these values once in your shell when following the examples:

```bash
export KC_REALM=irods
export IRODS_ZONE=tempZone
export IRODS_HOST=irods.example.org
export IRODS_PORT=1247
export IRODS_USER=rods
export IRODS_PASSWORD='...'
export IRODS_RESOURCE=providerResc
export KEYCLOAK_URL=https://keycloak.example.org
export KEYCLOAK_ADMIN_USER=admin
export KEYCLOAK_ADMIN_PASSWORD='...'
```

### Planning Flags

- `--target=keycloak` repairs Keycloak mirror state from iRODS.
- `--target=irods` provisions or reconciles selected Keycloak state into iRODS.
- `--keycloak-user-id` selects one Keycloak user for `--target=irods`.
- `--keycloak-group-id` selects one Keycloak group for `--target=irods`.
- `--keycloak-group-path` selects one Keycloak group by path for
  `--target=irods`.
- `--out` writes the plan JSON to a file and also writes it to stdout.
- `--password-action-report` writes an optional scenario-3 report. It never
  contains password values and does not apply credential changes.

For `--target=irods`, provide exactly one selector:

```text
--keycloak-user-id
--keycloak-group-id
--keycloak-group-path
```

This keeps Keycloak-to-iRODS mutation explicit and bounded. It is not a
full-system bidirectional sync.

### Apply Flags

- `--plan` is required.
- `--prompts=required` prompts only for guarded operations.
- `--prompts=all` prompts for every operation.
- `--prompts=none` applies without interactive confirmation and should only be
  used when the plan has already been reviewed.

## Administrator Runbook

### Review Checklist

Before applying any plan, confirm:

- `target_system` is the intended target.
- `realm` and `zone` are correct.
- Each operation action is expected.
- Each target username or group name is expected.
- Evidence contains the expected Keycloak ID or path.
- Membership removals have been reviewed as permission-impacting changes.
- `warning_count` is zero, or every warning is understood.
- The plan does not contain password values or password mutation operations.

Stop before applying if identity evidence is ambiguous, the selected Keycloak
object is wrong, or the plan includes an unexpected destructive action.

### Provision a Keycloak User into iRODS

A Keycloak realm administrator creates or selects the Keycloak user, then gets
the stable Keycloak user UUID.

Generate the plan:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=irods \
  --keycloak-user-id "$KEYCLOAK_USER_ID" \
  --realm "$KC_REALM" \
  --zone "$IRODS_ZONE" \
  --irods-host "$IRODS_HOST" \
  --irods-port "$IRODS_PORT" \
  --irods-user "$IRODS_USER" \
  --irods-password "$IRODS_PASSWORD" \
  --irods-resource "$IRODS_RESOURCE" \
  --keycloak-url "$KEYCLOAK_URL" \
  --keycloak-admin-user "$KEYCLOAK_ADMIN_USER" \
  --keycloak-admin-password "$KEYCLOAK_ADMIN_PASSWORD" \
  --out user-plan.json
```

Expected operations when the iRODS user is missing:

```json
{
  "target_system": "irods",
  "operations": [
    {
      "operation": "irods.user.create",
      "target": "alice"
    },
    {
      "operation": "irods.user.metadata.sync",
      "target": "alice"
    }
  ]
}
```

Apply the reviewed plan:

```bash
irods-kc-sync apply \
  --plan user-plan.json \
  --prompts required \
  --irods-host "$IRODS_HOST" \
  --irods-port "$IRODS_PORT" \
  --irods-user "$IRODS_USER" \
  --irods-password "$IRODS_PASSWORD" \
  --irods-resource "$IRODS_RESOURCE"
```

Expected result shape:

```json
{
  "status": "applied",
  "applied": 2,
  "skipped": 0,
  "failed": 0
}
```

Run the same `sync --dry-run` command again. Converged output has an empty
`operations` array. If operations remain, check mapping AVUs, selected
Keycloak user ID, and any pre-existing iRODS user state.

### Provision a Keycloak Group into iRODS

A Keycloak realm administrator creates or selects the Keycloak group, then uses
the group UUID or group path.

Generate the plan:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=irods \
  --keycloak-group-path /projects/alpha \
  --realm "$KC_REALM" \
  --zone "$IRODS_ZONE" \
  --irods-host "$IRODS_HOST" \
  --irods-port "$IRODS_PORT" \
  --irods-user "$IRODS_USER" \
  --irods-password "$IRODS_PASSWORD" \
  --irods-resource "$IRODS_RESOURCE" \
  --keycloak-url "$KEYCLOAK_URL" \
  --keycloak-admin-user "$KEYCLOAK_ADMIN_USER" \
  --keycloak-admin-password "$KEYCLOAK_ADMIN_PASSWORD" \
  --out group-plan.json
```

Expected operations when the iRODS group is missing:

```json
{
  "target_system": "irods",
  "operations": [
    {
      "operation": "irods.group.create",
      "target": "alpha"
    },
    {
      "operation": "irods.group.metadata.sync",
      "target": "alpha"
    }
  ]
}
```

Apply `group-plan.json`, then rerun the same dry-run command. Converged output
has an empty `operations` array.

### Reconcile Selected Group Membership

Membership reconciliation is selected-group scoped. It is not an unbounded
Keycloak-to-iRODS membership mirror.

Before planning, confirm:

- the Keycloak group is the intended group
- the corresponding iRODS group exists or will be created by the plan
- users being added have stable Keycloak identity
- users being added have corresponding iRODS user mappings

Generate the selected group plan:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=irods \
  --keycloak-group-path /projects/alpha \
  --realm "$KC_REALM" \
  --zone "$IRODS_ZONE" \
  --irods-host "$IRODS_HOST" \
  --irods-port "$IRODS_PORT" \
  --irods-user "$IRODS_USER" \
  --irods-password "$IRODS_PASSWORD" \
  --irods-resource "$IRODS_RESOURCE" \
  --keycloak-url "$KEYCLOAK_URL" \
  --keycloak-admin-user "$KEYCLOAK_ADMIN_USER" \
  --keycloak-admin-password "$KEYCLOAK_ADMIN_PASSWORD" \
  --out membership-plan.json
```

Representative membership operations:

```json
{
  "operations": [
    {
      "operation": "irods.group.member.add",
      "target": "alpha#member:alice"
    },
    {
      "operation": "irods.group.member.remove",
      "target": "alpha#member:bob"
    }
  ]
}
```

Review membership removals carefully. iRODS ACLs often grant data access to
groups, so changing group membership can change effective data permissions.

Ambiguous, unmanaged, or unmapped users are left untouched. Fix the mapping
evidence before forcing synchronization.

### Repeat Apply

After a successful apply, applying the same plan again is a useful idempotency
check. Already-completed operations should be `unchanged` or `skipped`, not
duplicated.

Representative repeat result:

```json
{
  "status": "skipped",
  "applied": 0,
  "skipped": 2,
  "failed": 0
}
```

If repeat apply fails, check whether another administrator changed state after
the original apply or whether the connection context differs.

## Planning Decision Table

| Observed state | Target | Planned response | Notes |
| --- | --- | --- | --- |
| iRODS user exists; Keycloak user is missing | `keycloak` | `keycloak.user.create`, then `irods.user.metadata.sync` | The iRODS AVU step is separate because the Keycloak UUID is only known after create or lookup. |
| iRODS user and Keycloak user exist; mapping AVUs are missing | `keycloak` | `irods.user.metadata.sync` | Adds only missing mapping AVUs. |
| iRODS user and Keycloak user exist; mapping AVUs are complete | `keycloak` | No operation | User is converged. |
| iRODS group exists; mirror group is missing | `keycloak` | `keycloak.group.create` | Creates the Keycloak mirror group under the configured mirror root. |
| iRODS group member is missing from the managed mirror group | `keycloak` | `keycloak.group.member.add` | User creation is ordered before membership if needed. |
| Managed mirror group has a member absent from iRODS | `keycloak` | `keycloak.group.member.remove` | Repairs the mirror only; does not remove the iRODS user. |
| Managed mirror group exists but iRODS group is missing | `keycloak` | `keycloak.group.delete` with approval | Stale mirror deletion is guarded. |
| Selected Keycloak user is missing in iRODS | `irods` | `irods.user.create`, then `irods.user.metadata.sync` | Explicit selected-user provisioning. |
| Selected Keycloak user exists in iRODS; mapping AVUs are missing | `irods` | `irods.user.metadata.sync` | Requires stable Keycloak user ID evidence. |
| Selected Keycloak group is missing in iRODS | `irods` | `irods.group.create`, then `irods.group.metadata.sync` | Explicit selected-group provisioning. |
| Selected Keycloak group exists in iRODS; mapping AVUs are missing | `irods` | `irods.group.metadata.sync` | Requires stable Keycloak group ID evidence. |
| Selected Keycloak group has a mapped member missing from iRODS | `irods` | `irods.group.member.add` | Adds the existing mapped iRODS user to the group. |
| Mapped iRODS group has a mapped member absent from selected Keycloak group | `irods` | `irods.group.member.remove` | Unmapped or ambiguous members are left alone. |
| User, group, or membership evidence is ambiguous | either | No destructive operation | Resolve mapping evidence or policy first. |

## Mapping AVUs

The toolkit uses minimal iRODS AVUs to make mappings explicit:

| AVU attribute | Meaning |
| --- | --- |
| `irods_keycloak_managed_by` | Marks the mapping as managed by this toolkit. |
| `irods_keycloak_realm` | Keycloak realm. |
| `irods_keycloak_user_id` | Stable Keycloak user UUID. |
| `irods_keycloak_group_id` | Stable Keycloak group UUID. |
| `irods_keycloak_authority` | Current authority policy value. |

## Scenario Notes

### Scenario 2: LDAP Federation

Scenario 2 uses Keycloak LDAP federation for login and iRODS PAM LDAP for
direct iRODS authentication.

Use `irods-kc-sync` for:

- user existence
- group existence
- group membership
- mapping metadata

Do not use it for password setup or password repair. If login fails, check
LDAP, PAM, account existence, and identity mapping.

### Scenario 3: iRODS Native Auth with Keycloak Password UX

Scenario 3 still uses ordinary sync for users, groups, membership, and mapping
metadata. Password setup and reset are a separate credential path.

`irods-kc-sync` does not set, mirror, generate, or reset passwords.

When useful, generate a password-action report:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=irods \
  --keycloak-user-id KEYCLOAK_USER_UUID \
  --realm irods \
  --zone tempZone \
  --out plan.json \
  --password-action-report password-actions.json
```

The report is a signal for future credential handling or external
notification. It is not proof of password mismatch, does not contain password
material, and does not send notifications.

## Future Scenario Placeholders

The following scenarios are part of the broader strategy but do not yet have
administrator workflows in this repository:

- Scenario 1: pure OIDC/SAML federation; iRODS does not natively support OIDC
- Scenario 4: Keycloak authenticates directly against iRODS
- Scenario 5: Keycloak-only REST, no human direct iRODS login
- Scenario 6: service accounts and machine clients
- Scenario 7: multi-realm / multi-zone
- Scenario 8: existing iRODS brownfield migration
- Scenario 9: read-only institutional identity
