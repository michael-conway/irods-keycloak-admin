# iRODS Keycloak Administrators Guide

## Introduction

This document is the working guide for administrators planning, operating, and
evaluating iRODS and Keycloak integrations with this toolkit.

This guide is authoritative for user stories.

It should describe:

- the integration scenarios administrators care about
- the operational goals and constraints in each scenario
- the tools, frameworks, and deployment themes that belong to each scenario

`DEVELOPER_NOTES.md` should follow this guide, not compete with it.
Developer-facing sprint plans, roadmaps, implementation tasks, and tactical
breakdowns belong in `DEVELOPER_NOTES.md`.

This guide should stay aligned with the actual implemented behavior in the
repository. It should not imply that scaffolded workflows already exist.

## Sync Terminology

These terms are used to describe synchronization behavior across iRODS and
Keycloak.

### Managed

A managed user or group is an object that the toolkit is allowed to mutate and
reconcile intentionally.

Managed does not automatically mean:

- the object was originally created by only one side
- the object may be deleted just because it is missing on the other side

Managed should mean the toolkit has enough confidence and policy approval to
update the object as part of synchronization.

### Mapped

A mapped user or group is an object for which the toolkit has determined the
corresponding identity or group on the other side.

Mapped does not automatically mean managed.

Examples:

- an existing iRODS user may be mapped to a Keycloak user before the toolkit is
  allowed to mutate either side
- an existing iRODS group may be mapped to a Keycloak group without implying
  that either side should be deleted if the other changes unexpectedly

### Authority

Authority is an administrative policy hint for conflict resolution and
directional repair behavior.

Authority should not be treated as a universal ownership rule for all sync
workflows.

Authority may be useful when:

- a deployment explicitly wants mirror-style repair behavior
- one side should win conflicts by policy
- a workflow is intentionally one-directional

Authority should not imply that unmatched objects are automatically disposable.

### Candidate Addition

A candidate addition is an object or membership that exists on one side and can
reasonably be reflected to the other side.

In a bi-directional sync model, unmatched users or groups should generally be
treated as candidate additions before they are treated as drift requiring
deletion.

Example:

- an unmanaged iRODS user appears and is a candidate addition into Keycloak

### Candidate Removal

A candidate removal is an object or membership that appears removable, but
should not be deleted without stronger evidence or explicit policy.

Candidate removal is a more conservative classification than ordinary drift.

Example:

- a user-group membership exists only on one side, but the system needs policy
  or review before deciding whether to remove it

### Conflict

A conflict is a state where the toolkit cannot safely decide the correct action
from synchronization rules alone.

Examples:

- ambiguous user mapping
- two plausible group matches
- both sides changed in incompatible ways
- administrative policy does not clearly state which side should win

Conflicts should prefer review and explicit operator decision over silent
destructive action.

## iRODS/Keycloak Integration Scenarios

Each scenario section should capture the user story and answer two practical
questions:

1. Which tools and frameworks should be applied to this scenario?
2. How should an administrator operate and support this scenario?

Current planning focus:

- Scenario 2: LDAP federation; Keycloak uses LDAP and iRODS uses PAM LDAP
- Scenario 3: iRODS native auth; Keycloak provides password self-service

For the current planning slice, both scenarios are treated as synchronization
scenarios for users, groups, and group memberships. Administrative actions may
be initiated either from iRODS administration tools or from Keycloak-facing
workflow surfaces, but the synchronization model must keep iRODS and Keycloak
aligned.

## Current `irods-kc-sync` Operations

`irods-kc-sync` is the current administrator-facing command for generating and
applying reviewable synchronization plans.

Current operational rule:

```text
Generate a plan first.
Review the plan.
Apply the accepted plan explicitly.
```

The `sync` command currently supports planning only. It must be run with
`--dry-run`. It writes plan JSON to standard output and can also save the same
plan to a file with `--out`.

The `apply` command applies a plan file created by `sync --dry-run`.

### Command Summary

Create a Keycloak-targeted mirror-repair plan from iRODS state:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=keycloak \
  --realm irods \
  --zone tempZone \
  --out plan.json
```

Create an iRODS-targeted plan for one Keycloak user:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=irods \
  --keycloak-user-id KEYCLOAK_USER_UUID \
  --realm irods \
  --zone tempZone \
  --out plan.json
```

Create an iRODS-targeted plan for one Keycloak group and conservative
membership drift:

```bash
irods-kc-sync sync \
  --dry-run \
  --target=irods \
  --keycloak-group-path /projects/alpha \
  --realm irods \
  --zone tempZone \
  --out plan.json
```

Apply a reviewed plan:

```bash
irods-kc-sync apply \
  --plan plan.json \
  --prompts required
```

### Important Parameters

Planning parameters:

- `--dry-run` is required for `sync`. It means generate a plan only; do not
  mutate iRODS or Keycloak.
- `--target=keycloak` means the plan repairs Keycloak mirror state from iRODS.
- `--target=irods` means the plan provisions or reconciles selected Keycloak
  state into iRODS.
- `--keycloak-user-id` selects one stable Keycloak user ID for
  `--target=irods`.
- `--keycloak-group-id` selects one stable Keycloak group ID for
  `--target=irods`.
- `--keycloak-group-path` selects one Keycloak group by path for
  `--target=irods`.
- `--realm` identifies the Keycloak realm containing the selected users or
  groups.
- `--zone` identifies the iRODS zone used for lookup and mutation planning.
- `--out` saves the generated plan JSON to a file while still writing it to
  standard output.
- `--password-action-report` writes optional scenario-3 JSON reporting derived
  from planned user operations. It is informational only. It does not contain
  password material and does not apply credential changes.

Apply parameters:

- `--plan` is required. It points to a JSON plan generated by
  `irods-kc-sync sync --dry-run`.
- `--prompts=required` prompts only for operations marked as requiring
  approval.
- `--prompts=all` prompts for every operation.
- `--prompts=none` applies without interactive confirmation and should be used
  only when the plan has already been reviewed by another process.
- `--realm` and `--zone` act as expected runtime context for the plan. When
  omitted and not configured in the environment, apply uses the realm and zone
  recorded in the plan.

Connection parameters:

- iRODS access is supplied directly with `--irods-host`,
  `--irods-port`, `--irods-user`, `--irods-password`, and `--irods-resource`.
- Keycloak access is configured with `--keycloak-url`,
  `--keycloak-admin-realm`, `--keycloak-client-id`,
  `--keycloak-client-secret`, `--keycloak-admin-user`, and
  `--keycloak-admin-password`.
- `--keycloak-insecure-skip-verify` is only for local test stacks with
  self-signed certificates.
- `--keycloak-mirror-root` identifies the managed Keycloak mirror group root
  used by Keycloak-targeted mirror repair, commonly `/irods`.

### Target-Specific Behavior

For `--target=keycloak`, the command plans mirror repair from iRODS into
Keycloak. This is the conservative iRODS-to-Keycloak direction for making
Keycloak mirror managed iRODS users, groups, and memberships.

For `--target=irods`, the command requires exactly one selector:

```text
--keycloak-user-id
--keycloak-group-id
--keycloak-group-path
```

This explicit selector is intentional. The current iRODS-targeted slice is for
reviewable Keycloak-originating user, group, and membership mutations, not for
unbounded full-system bidirectional sync.

Selected-user planning can create an iRODS user and synchronize minimal mapping
metadata when Keycloak state provides stable identity evidence.

Selected-group planning can create an iRODS group, synchronize minimal mapping
metadata, and plan conservative membership add/remove operations. Membership
adds require stable Keycloak user identity and a matching iRODS user mapping.
Membership removals are conservative and leave ambiguous, unmanaged, or
unmapped iRODS members untouched.

### Sync Decision Table

This table summarizes the current `irods-kc-sync` planning responses for common
states. The command always produces an ordered JSON plan first; apply executes
the accepted operations in that order.

| Observed state | Sync target | Planned response | Apply behavior | Notes |
| --- | --- | --- | --- | --- |
| iRODS rods user exists and exact Keycloak user is missing | `keycloak` | `keycloak.user.create`, followed by `irods.user.metadata.sync` | Create or resolve the Keycloak user, then attach managed mapping AVUs to the iRODS user using the assigned Keycloak user ID | The AVU operation is intentionally separate because the Keycloak UUID is only known after create or lookup. |
| iRODS rods user exists, exact Keycloak user exists, and iRODS mapping AVUs are missing or incomplete | `keycloak` | `irods.user.metadata.sync` | Add only missing managed mapping AVUs to the iRODS user | This lets a partially applied user-create flow converge on a later run. |
| iRODS rods user exists, exact Keycloak user exists, and mapping AVUs are complete | `keycloak` | No user operation | No mutation | The user is already mapped and converged for user-existence sync. |
| iRODS group exists and managed Keycloak mirror group is missing | `keycloak` | `keycloak.group.create` | Create or update the Keycloak mirror group under the configured mirror root | Group creation carries iRODS group name, zone, and authority attributes in Keycloak. |
| iRODS group member exists and mapped Keycloak user is not in the managed mirror group | `keycloak` | `keycloak.group.member.add` | Add the Keycloak user to the mirror group | If the user is also missing in Keycloak, the plan orders user creation before membership changes. |
| Managed Keycloak mirror group has a member that is not in the authoritative iRODS group | `keycloak` | `keycloak.group.member.remove` | Remove the user from the Keycloak mirror group | This is mirror repair from iRODS into Keycloak. It does not remove the iRODS user. |
| Managed Keycloak mirror group exists but corresponding iRODS group is missing | `keycloak` | `keycloak.group.delete` with `requires_approval` | Delete only when review policy accepts the guarded operation | Stale mirror deletion is intentionally guarded. |
| Keycloak user selected by `--keycloak-user-id` is missing in iRODS | `irods` | `irods.user.create`, followed by `irods.user.metadata.sync` | Create the iRODS user, then attach managed mapping AVUs | This is explicit selected-user provisioning, not unbounded Keycloak-to-iRODS sync. |
| Keycloak user selected by `--keycloak-user-id` exists in iRODS but mapping AVUs are missing or incomplete | `irods` | `irods.user.metadata.sync` | Add only missing managed mapping AVUs | Stable Keycloak user ID evidence is required. |
| Keycloak group selected by ID or path is missing in iRODS | `irods` | `irods.group.create`, followed by `irods.group.metadata.sync` | Create the iRODS group, then attach managed mapping AVUs | Group selectors keep the current iRODS-targeted slice explicit and bounded. |
| Keycloak selected group exists in iRODS but group mapping AVUs are missing or incomplete | `irods` | `irods.group.metadata.sync` | Add only missing managed mapping AVUs | Stable Keycloak group ID evidence is required. |
| Keycloak selected group has a mapped member missing from the iRODS group | `irods` | `irods.group.member.add` | Add the existing mapped iRODS user to the iRODS group | Membership adds require both stable Keycloak identity and matching iRODS user mapping. |
| Mapped iRODS group has a mapped member absent from the selected Keycloak group | `irods` | `irods.group.member.remove` | Remove the user from the iRODS group | Ambiguous, unmanaged, or unmapped members are left untouched. |
| User, group, or membership state is ambiguous or lacks stable identity evidence | either | No destructive operation; may omit the candidate or classify it as unresolved evidence | No mutation | Operators should resolve mapping evidence or policy before forcing synchronization. |

Managed mapping AVUs currently used for iRODS users and groups are:

| AVU attribute | Meaning |
| --- | --- |
| `irods_keycloak_managed_by` | Records that the mapping is managed by this toolkit. |
| `irods_keycloak_realm` | Records the Keycloak realm for the mapped identity or group. |
| `irods_keycloak_user_id` | Records the stable Keycloak user UUID for mapped users. |
| `irods_keycloak_group_id` | Records the stable Keycloak group UUID for mapped groups. |
| `irods_keycloak_authority` | Records the authority policy value, currently `irods` for implemented sync plans. |

### Recommended Review Workflow

1. Create or choose the user or group in the normal administrative surface.
2. Run `irods-kc-sync sync --dry-run` with the appropriate target and selector.
3. Review `plan.json`.
4. Confirm each operation has the expected action, target, realm, zone, and
   evidence.
5. Apply the plan with `irods-kc-sync apply --plan plan.json`.
6. Run the same `sync --dry-run` command again.
7. Confirm the follow-up plan converges or only reports intentionally deferred
   ambiguity.

Do not treat a plan file as an approval record by itself. The operator or
automation applying the plan is responsible for review.

### Scenario 2 Usage Notes

In scenario 2, LDAP remains the password authority and iRODS uses PAM LDAP.

Use `irods-kc-sync` for users, groups, group membership, and mapping metadata.
Do not use it to set or repair iRODS native passwords. If user login fails in
scenario 2, troubleshoot LDAP, PAM, identity mapping, and account existence
before looking for any password-bridge behavior.

### Scenario 3 Usage Notes

In scenario 3, iRODS native password setup or reset is a separate credential
path.

Ordinary `irods-kc-sync` planning and apply can handle user, group, membership,
and mapping operations. It does not set, mirror, generate, or reset passwords.

When useful, generate a password-action report alongside the plan:

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

The report is a signal for future credential handling or external notification
workflow. It is not proof of password mismatch, does not contain password
material, and does not deliver notifications.

### Scenario 1: Pure OIDC/SAML Federation; iRODS Does Not Natively Support OIDC

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD

### Scenario 2: LDAP Federation; Keycloak Uses LDAP and iRODS Uses PAM LDAP

#### Tools and Frameworks to Apply to This Scenario

This is the synchronized provisioning scenario for LDAP-backed identity.

Primary characteristics:

- Keycloak uses LDAP federation for authentication and account identity
- iRODS uses PAM LDAP for direct login
- user, group, and group-membership state must remain synchronized across the
  iRODS and Keycloak administrative surfaces
- password management is not part of the synchronization workflow

Administrative capabilities needed:

- user self-service to request an iRODS account
- administrator provisioning of users
- administrator and group-admin provisioning of groups and group memberships
- synchronization when actions are initiated either through direct iRODS
  administrative surfaces or through Keycloak-facing workflow surfaces

Tools and framework themes:

- Keycloak LDAP federation
- iRODS PAM LDAP
- iRODS administration through direct service tooling
- Keycloak Admin REST for user, group, and group-membership workflow surfaces
- synchronization and reconciliation workflows for users, groups, and
  memberships

Password handling:

- when a user is added in this scenario, the iRODS account should be created
  without introducing a separate iRODS native-password lifecycle
- password authority remains with LDAP
- this scenario should not require the toolkit to generate, store, or mirror an
  iRODS native password

#### Operations Guide for This Scenario

Operational model:

- treat this as a sync scenario, not a password-bridge scenario
- allow account and group actions to originate either in iRODS administration
  or in Keycloak-facing administrative workflow surfaces
- synchronize user existence, group existence, and group membership state
  without changing the LDAP/PAM password authority

User-account operations:

- self-service request should allow a user to request an iRODS account tied to
  their Keycloak identity
- administrator approval may be required before creating the iRODS user,
  depending on site policy
- administrator provisioning should create the iRODS user in a form suitable
  for PAM LDAP-backed access

Group operations:

- administrators must be able to provision groups
- delegated group administrators must be able to manage group membership within
  approved scope
- synchronization must tolerate changes that originate through either direct
  iRODS administrative surfaces or Keycloak workflow surfaces

Administrative expectation:

- if password troubleshooting is required, troubleshoot LDAP/PAM configuration
  and identity mapping, not a native-password bridge
- treat synchronization drift in this scenario as a user/group/membership
  problem, not a password problem

### Scenario 3: iRODS Native Auth; Keycloak Provides Password Self-Service

#### Tools and Frameworks to Apply to This Scenario

This is the synchronized provisioning scenario for iRODS native passwords, with
Keycloak providing the user-facing password experience.

Primary characteristics:

- user, group, and group-membership state must remain synchronized across iRODS
  and Keycloak administrative surfaces
- Keycloak must support account creation and password-setting workflows
- iRODS native password state must be set intentionally and kept aligned with
  the user-facing Keycloak flow

Administrative capabilities needed:

- user self-service to request an account
- administrator provisioning of users
- administrator and group-admin provisioning of groups and group memberships
- synchronization when actions are initiated either through direct iRODS
  administrative surfaces or through Keycloak-facing workflow surfaces
- a defined password-setting path for new users and password updates

Tools and framework themes:

- Keycloak account and credential-management workflow surfaces
- iRODS native-auth user administration
- Keycloak Admin REST for user, group, and group-membership workflow surfaces
- synchronization and reconciliation workflows for users, groups, and
  memberships
- a password-write integration path that can update iRODS when the user is
  setting or changing a password through Keycloak

Password handling:

- this scenario differs from scenario 2 because password handling is part of
  the workflow
- in the currently preferred model, ordinary synchronization does not directly
  repair password state
- in self-service onboarding and recovery, the user should set or reset their
  password through Keycloak rather than receiving a long-lived
  administrator-chosen password
- administrator-created accounts may still require a controlled recovery or
  setup step before the account is fully usable
- the system must define how a Keycloak password-setting action results in an
  iRODS password update

#### Operations Guide for This Scenario

Operational model:

- treat this as both a sync scenario and a password-write scenario
- allow account and group actions to originate either in iRODS administration
  or in Keycloak-facing workflow surfaces
- do not treat password handling as an afterthought to ordinary synchronization

User-account operations:

- self-service request should lead to a controlled account-creation flow where
  the user sets their own password
- administrator provisioning should create the user in a state that requires
  Keycloak-driven password setup or recovery before the account is fully usable
- the onboarding flow must make it clear whether the account is usable before
  the user completes password setup

Group operations:

- administrators must be able to provision groups
- delegated group administrators must be able to manage group membership within
  approved scope
- synchronization must tolerate changes that originate through either direct
  iRODS administrative surfaces or Keycloak workflow surfaces

Password operations:

- password setup and reset must update iRODS, not just Keycloak-facing account
  state
- the preferred operational model is to hook password-setting and reset flows
  at the point where the password is actively being handled, rather than
  relying on later synchronization of opaque password events
- event-driven mutation from Keycloak into iRODS may still be useful for user
  and group lifecycle changes, but password updates should be treated as a
  special path that requires direct credential-handling integration
- synchronization may optionally emit a password-action report in JSON for
  external notification or ticketing workflows
- notification itself should remain outside the toolkit scope
- sites may choose either:
  passive recovery, where the user discovers the issue and self-triggers
  password reset through Keycloak
  proactive recovery, where an external workflow consumes the JSON report and
  notifies the user or support staff

Administrative expectation:

- drift in this scenario can include user, group, membership, and password
  workflow failures
- operators should distinguish between ordinary sync drift and failures in the
  password-setting path
- a password-action report is best treated as a request for user action rather
  than proof of literal password mismatch

### Scenario 4: Keycloak Authenticates Directly Against iRODS

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD

### Scenario 5: Keycloak-only REST, No Human Direct iRODS Login

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD

### Scenario 6: Service Accounts and Machine Clients

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD

### Scenario 7: Multi-Realm / Multi-Zone

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD

### Scenario 8: Existing iRODS Brownfield Migration

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD

### Scenario 9: Read-Only Institutional Identity

#### Tools and Frameworks to Apply to This Scenario

TBD

#### Operations Guide for This Scenario

TBD
