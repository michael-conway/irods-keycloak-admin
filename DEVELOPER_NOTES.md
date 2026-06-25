# iRODS + Keycloak Authentication and Authorization Strategy

## Purpose

This document captures the current strategy and design decisions for integrating iRODS and Keycloak for authentication, authorization, provisioning, group management, and REST/web access. It is intended as an orientation document for future AI-assisted development, project planning, repository setup, and implementation work.

The practical problem is not merely connecting iRODS to Keycloak. The real goal is to support a flexible toolkit where Keycloak provides the modern authentication and user/admin experience for OIDC, SAML, LDAP, MFA, federation, and web/REST clients, while iRODS remains the data system that owns users, groups, ACLs, data objects, and optionally native/PAM authentication.

The major design constraint is:

```text
Use only iRODS and Keycloak capabilities.
Do not introduce another state store for identity synchronization.
Minimal iRODS AVUs are allowed when they prevent ambiguity or support mapping.
```

---

## Current Tasks

This section is the active sprint planning area. It records near-term coding
plans derived from the larger strategy and phase plan. When the sprint changes,
update this section first, then reconcile any stale lower-level examples in the
rest of the document.

Authority split:

- `IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md` is authoritative for administrator
  user stories, scenario framing, and general operational themes.
- `DEVELOPER_NOTES.md` is authoritative for developer execution, sprint
  breakdown, roadmap sequencing, and implementation tactics.
- Developer tasks should derive from administrator-guide user stories rather
  than inventing an unrelated planning vocabulary.

### Sprint Plan: Scenario 2 and 3 iRODS Mutation in Sync Operations

Date: 2026-06-12

Goal:

```text
Use the administrator guide's scenario-2 and scenario-3 user stories to start
the next execution slice: iRODS mutation in synchronization operations.

This sprint should cover iRODS mutations for users, groups, and group
membership in the synchronization toolchain.

Scenario 2 is ready for user provisioning without an iRODS native-password
lifecycle.
Scenario 3 should support the same user/group/group-membership mutation model
while treating password recovery as a separate credential path, with optional
JSON password-action reporting.
```

Resume point:

```text
The repair/apply vertical slice remains stable and available as prior work.

The next active execution slice is Keycloak-driven synchronization that can
mutate iRODS users, groups, and group memberships.

IRODS_KEYCLOAK_ADMINISTRATORS_GUIDE.md owns the user stories for scenarios 2
and 3.
DEVELOPER_NOTES.md should now translate those user stories into concrete tool
shapes, sprint tasks, and implementation sequencing.
```

Important boundary decisions:

```text
This sprint is execution-oriented, but remains tightly bounded by scenarios 2
and 3.
Add iRODS mutation support for users, groups, and group memberships.
Do not collapse scenario-3 password handling into ordinary synchronization.
Treat scenario-3 password setup/reset as a separate credential path.
Optional password-action JSON reporting is in scope; notification delivery is
out of scope.
Keep roadmap, sprinting, and execution tactics in this developer guide.
```

#### Current Sprint Tasks

x1. Define the first usable sync surface.

- Keep the new work inside `irods-kc-sync`.
- Treat `sync --dry-run` plus `apply --plan ...` as the baseline operational
  model unless a narrower short-term split is required.
- Use an explicit first-slice target selector rather than introducing a new
  top-level command or fully mixed-system plans immediately.
- Preferred first-slice shape:
  `irods-kc-sync sync --dry-run --target=irods`
  `irods-kc-sync apply --plan plan.json`
- Keep the first implementation slice focused on Keycloak-to-iRODS mutation and
  tested end to end.
- Keep backward compatibility with the current Keycloak-side sync/apply slice.

x2. Add iRODS mutation plan/apply support for users.

- Plan iRODS user create/update operations from Keycloak-facing sync state.
- Distinguish between:
  self-service account request
  administrator provisioning
  synchronization of already mapped users
- Add minimal managed/mapping AVU handling for users.
- Keep scenario-2 and scenario-3 user mutation logic identical except for
  credential-related behavior.
- Current slice complete:
  `sync --dry-run --target=irods --keycloak-user-id ...` plans user create
  plus metadata sync operations from Keycloak state, and `apply --plan ...`
  executes those operations against iRODS.
- Apply validation requires `keycloak_user_id` evidence for iRODS user mutation
  operations so user creation and metadata sync stay anchored to a stable
  Keycloak mapping identity.

x3. Add iRODS mutation plan/apply support for groups.

- Plan iRODS group create/update operations.
- Support additive reflection of previously unmanaged iRODS groups into
  Keycloak rather than treating them as deletion candidates by default.
- Decide which group AVUs are mandatory versus optional in the first slice.
- Current slice complete:
  `sync --dry-run --target=irods --keycloak-group-id ...` and
  `sync --dry-run --target=irods --keycloak-group-path ...` plan iRODS group
  create plus metadata sync operations from Keycloak state, and
  `apply --plan ...` executes those operations against iRODS.
- Apply validation requires `keycloak_group_id` evidence for iRODS group
  mutation operations so group creation and metadata sync stay anchored to a
  stable Keycloak mapping identity.
- First-slice mandatory group AVUs match the user mapping shape:
  managed-by, Keycloak realm, Keycloak group ID, and authority.

x4. Add iRODS mutation plan/apply support for group membership.

- Plan add/remove operations for iRODS group membership.
- Support administrator and delegated group-admin workflows in scope, even if
  final authz enforcement remains scaffolded.
- Keep membership mutation rules conservative where mapping or policy is
  ambiguous.
- Current slice complete:
  selected-group `sync --dry-run --target=irods --keycloak-group-id ...` and
  `sync --dry-run --target=irods --keycloak-group-path ...` now compare
  Keycloak group members with iRODS group members and plan
  `irods.group.member.add` / `irods.group.member.remove` operations.
- Membership adds require the Keycloak member to have a stable user ID and an
  existing iRODS user with matching Keycloak-user-ID metadata.
- Membership removals require the iRODS group to be mapped to the selected
  Keycloak group and the removed iRODS user to carry Keycloak-user-ID metadata.
- Unmapped or unmanaged iRODS members are left untouched by this slice.

x5. Make the sync model explicitly bi-directional and conservative.

- Treat unmatched users/groups as candidate additions before candidate removals.
- Separate the meanings of:
  managed
  mapped
  authority
  conflict
- Preserve authority as an optional policy knob for directional repair and
  conflict resolution, not a universal deletion rule.
- This sprint still implements only the `--target=irods` direction, while
  keeping the broader bi-directional model as the intended later shape.
- Current slice complete:
  plan operations now expose `sync_direction`, `sync_classification`,
  `mapping_identity_known`, `authority_role`, and `conflict_status` evidence.
- Current classifications distinguish candidate additions, candidate removals,
  and mapped metadata updates without treating `authority=irods` as a universal
  deletion rule.
- Existing `--target=keycloak` mirror repair is labeled as
  `irods_to_keycloak` directional repair, while the new sprint work is labeled
  `keycloak_to_irods`.
- Ambiguous selected-group membership state remains conservative: username-only
  Keycloak membership blocks removal of the matching iRODS member but is not
  enough evidence to add a new iRODS membership.

x6. Keep scenario 2 clean on credentials.

- Support user provisioning without introducing an iRODS native-password
  lifecycle.
- Treat scenario-2 failures as user/group/membership/mapping failures, not
  password failures.
- Ensure scenario-2 plan/apply output does not imply native-password handling.
- Current slice complete:
  `--target=irods` plan operations now include `credential_policy:
  external_authority`, `credential_action: none`, and `failure_domain:
  identity_group_membership_mapping`.
- Scenario-2 user creation uses ordinary iRODS user creation only; no native
  credential setup, reset, generation, mirroring, or reporting is part of the
  synchronization plan/apply path.
- Apply failures in this slice remain `apply.irods.operation_failed` warnings
  whose messages come from user, group, membership, or metadata operations.

x7. Keep scenario 3 credentials on a separate path.

- Do not implement password repair as ordinary synchronization.
- Add optional JSON password-action reporting only.
- Keep notification delivery out of scope.
- Treat password setup/reset as a future direct Keycloak-to-iRODS credential
  path rather than part of generic sync.
- Current slice complete:
  `irods-kc-sync sync --dry-run --target=irods ... --password-action-report
  report.json` writes a separate JSON password-action report derived from the
  sync plan.
- The report is informational only. It does not change the sync plan, does not
  apply credentials, does not deliver notifications, and does not include
  password material.
- Current report actions are:
  `password_setup_required` for planned iRODS user creation
  `credential_state_unknown` for existing-user metadata synchronization
- The report records the future credential path as
  `future_keycloak_to_irods_direct` and notification handling as
  `out_of_scope`.

x8. Add tests before widening behavior.

- Add unit coverage for iRODS user plan/apply behavior.
- Add unit coverage for iRODS group and membership plan/apply behavior.
- Add coverage for managed/mapped/unmanaged classification.
- Add coverage for scenario-3 password-action report generation.
- Add focused live coverage for Keycloak-to-iRODS mutation, especially adding
  new users to iRODS from Keycloak-facing sync state.
- Keep e2e expansion narrow until the first iRODS mutation slice is stable.
- Current slice complete:
  unit coverage now covers iRODS user, group, and membership plan/apply
  behavior; managed, mapped, unmanaged, ambiguous, and conflict classification;
  and scenario-3 password-action report generation.
- Focused live coverage now includes
  `TestKCSyncApplyCreatesIRODSUserFromKeycloakE2E`, which creates a temporary
  Keycloak user, plans `sync --dry-run --target=irods --keycloak-user-id ...`,
  applies the generated plan, verifies the iRODS user exists, replays the plan
  idempotently, and confirms a follow-up plan converges.
- Focused live coverage now also includes selected Keycloak group provisioning
  into iRODS and selected-group membership add/remove drift through
  `sync --dry-run --target=irods --keycloak-group-id ...` plus
  `apply --plan ...`.
- The integrated `--password-action-report` CLI path is covered by live e2e
  setup and verifies the separate report file is written without credential
  material.
- The iRODS-to-Keycloak user-create e2e now verifies the post-create iRODS AVU
  sync that records the assigned Keycloak user ID.
- E2E expansion remains intentionally focused on exit-criteria workflows rather
  than broad matrix coverage.

#### Resolved First-Wave Decisions

1. First user-facing command shape:

- Keep the first iRODS-mutation slice inside `irods-kc-sync`.
- Use `sync --dry-run --target=irods` to generate reviewable plans.
- Use `apply --plan ...` to execute accepted plans.
- Use explicit selectors instead of mixed-system full synchronization for this
  first slice.

2. Default stance for unmatched objects:

- Treat unmatched Keycloak users and groups as candidate iRODS additions before
  treating unmatched objects as candidate removals.
- Require stronger evidence, explicit policy, or review for removal.
- Keep removal conservative when mapping or policy is ambiguous.

3. Meaning of `managed`, `mapped`, `authority`, and `conflict`:

- `managed` means the toolkit may mutate the object.
- `mapped` means the Keycloak-to-iRODS correspondence is known.
- `authority` is a directional-repair and conflict-resolution hint, not a
  universal deletion rule.
- `conflict` records ambiguity or unsafe state that should block or constrain
  mutation.

4. First-slice iRODS AVU minimums:

- User mutations require stable Keycloak user ID mapping evidence.
- Group mutations require stable Keycloak group ID mapping evidence.
- Current metadata sync writes managed-by, realm, stable Keycloak ID, and
  authority evidence for toolkit-managed iRODS users and groups.

5. Scenario-3 credential handling:

- Password setup/reset is not ordinary synchronization.
- The current slice emits optional JSON password-action reporting through
  `--password-action-report`.
- Notification delivery and direct Keycloak-to-iRODS password write paths remain
  future work.

Current implemented target selectors:

- `--keycloak-user-id` plans the selected Keycloak user into iRODS.
- `--keycloak-group-id` plans the selected Keycloak group and conservative
  membership drift into iRODS.
- `--keycloak-group-path` is the path-based equivalent for selected-group
  planning.
- A separate `--group-plus-members` selector is not needed for the current
  slice because selected-group planning already includes group metadata and
  conservative membership drift.

Exit criteria before selecting the next sprint:

- A real administrator can provision at least one Keycloak-originating user into
  iRODS without native-password handling.
- A real administrator can provision or reconcile at least one Keycloak group
  into iRODS.
- A real administrator can inspect membership drift and see conservative add or
  remove behavior, even if some ambiguous cases are intentionally skipped.
- Plan/apply output is understandable enough to decide whether the next sprint
  should improve UX, policy controls, live coverage, authz, or credential
  workflows.
- Any real-world testing gaps are recorded as next-sprint candidates rather
  than patched ad hoc into this first wave.

#### Next Sprint Planning Intake

The next sprint should be selected interactively after real-world testing of the
first-wave administrative workflow.

Candidate sprint directions:

1. Operator UX and workflow hardening.

- Improve plan readability, summaries, examples, command help, and review
  ergonomics around the current `sync --dry-run --target=irods` and
  `apply --plan` flow.
- Best fit if real-world testing shows the mechanics work but the operator path
  is too hard to trust or explain.

2. Policy and safety controls.

- Add explicit policy knobs for authority requirements, removal behavior,
  managed-object constraints, and conflict handling.
- Best fit if real-world testing shows the main risk is accidental mutation or
  insufficient control over conservative behavior.

3. Group and membership live coverage.

- Expand focused e2e coverage from user creation into group creation,
  membership add, membership remove, and ambiguous membership cases.
- Best fit if real-world testing shows the behavior is promising but needs a
  stronger safety net before widening use.

4. Delegated group-admin workflow.

- Turn the scaffolded delegated group-admin model into a clearer workflow,
  likely still CLI-first unless HTTP/API work becomes necessary.
- Best fit if real administrators need project/group owners to manage iRODS
  group membership without full iRODS administration rights.

5. Self-service account request workflow.

- Represent request, approval, and fulfillment separately from raw sync while
  reusing the current plan/apply mechanics beneath it.
- Best fit if the next priority is scenario-2 user onboarding rather than
  administrator-driven provisioning.

6. Scenario-3 credential path design.

- Keep credential operations separate from sync and define the direct
  Keycloak-to-iRODS password setup/reset path, report states, and failure
  handling.
- Best fit if password recovery/setup is the next operational blocker.

Next-sprint selection questions:

- Which manual workflow failed first: user provisioning, group provisioning,
  membership reconciliation, or plan review?
- Was the failure mechanical, policy-related, UX-related, authz-related, or
  credential-related?
- Should the next sprint optimize for administrator confidence, broader live
  coverage, delegated administration, self-service onboarding, or scenario-3
  credential support?

### Tool Class: Keycloak-Driven iRODS Mutation Tooling

This sprint introduces a separate tool class from the existing sync
workflow.

Purpose:

```text
Allow Keycloak-facing synchronization workflows to mutate iRODS users, groups,
and group memberships directly, while still respecting scenario-specific
credential rules.
```

This tool class should cover:

- Keycloak-driven user provisioning into iRODS
- Keycloak-driven group provisioning into iRODS
- Keycloak-driven group-membership mutation into iRODS
- synchronization and review behavior for these mutations
- optional scenario-3 password-action JSON reporting
- an initial CLI shape based on:
  `sync --dry-run --target=irods`
  `apply --plan ...`

This tool class should not yet cover:

- direct password mirroring as ordinary sync behavior
- in-band email delivery or notification workflows
- broad scenario support beyond scenarios 2 and 3
- full mixed-direction bi-directional execution in one plan/apply slice


---

## Core Architectural Position

Keycloak should be treated as:

```text
OIDC/SAML/LDAP integration point
OIDC token issuer
user login layer
admin/self-service UX layer
optional password/account UX layer
optional bridge to upstream identity providers
```

iRODS should be treated as:

```text
data system
user/group registry for data authorization
ACL enforcement layer
optional native/PAM password authority
source of truth for iRODS group membership unless explicitly configured otherwise
```

The toolkit should provide the glue:

```text
identity mapping
user provisioning
group administration
Keycloak <-> iRODS mirroring
sync/repair/bootstrap
OIDC middleware for REST services
optional Keycloak plugins
optional password bridge
optional iRODS-backed Keycloak auth provider
```

The highest-level rule:

```text
Keycloak is the auth integration and UX layer.
iRODS remains the data authorization layer.
Group/user changes may be initiated from Keycloak,
but dangerous mutations should be applied to iRODS first,
then mirrored back into Keycloak.
```

---

## Key Design Decisions

### 1. Do not build one monolithic integration mode

One size does not fit all. The project should expose a menu of capabilities that can be combined into deployment profiles.

Examples:

- Pure OIDC/SAML federation for REST/web clients.
- LDAP-backed Keycloak plus PAM-backed iRODS.
- iRODS native auth with Keycloak password self-service.
- Keycloak authenticating directly against iRODS.
- REST-only portal mode with no human direct iRODS login.
- Service-account / machine-client access.
- Brownfield bootstrap from existing iRODS users and groups.

### 2. Favor Go unless code must run inside Keycloak

Language rule:

```text
Java = anything loaded into Keycloak.
Go   = external services, CLIs, iRODS-facing tooling, reconciliation, REST middleware.
```

Reasoning:

- Keycloak extensions are Java SPI providers packaged as JARs and loaded into Keycloak.
- External tools can use Keycloak Admin REST APIs and iRODS clients from Go.
- Existing related iRODS/DRS/REST work is already Go-oriented.
- Avoid mixing languages inside a repo.

### 3. Reuse go-irodsclient for iRODS types and operations

`irods-keycloak-admin` should not define replacement representations for
iRODS users, groups, AVUs, ACLs, access levels, quotas, or related admin
operations that already exist in `go-irodsclient`.

Canonical iRODS domain types should come from:

```go
github.com/cyverse/go-irodsclient/irods/types
```

Known reusable types include:

- `types.IRODSUser`
- `types.IRODSUserType`
- `types.IRODSMeta`
- `types.IRODSAccess`
- `types.IRODSAccessLevelType`
- `types.IRODSAccessInheritance`
- `types.IRODSQuota`

Canonical iRODS admin behavior should use `go-irodsclient/fs.FileSystem` or
the lower-level `go-irodsclient/irods/fs` package where needed. Existing useful
operations include:

- `GetUser`
- `ListUsers`
- `CreateUser`
- `RemoveUser`
- `ChangeUserPassword`
- `ChangeUserType`
- `ListGroupMembers`
- `ListGroupMemberNames`
- `ListUserGroups`
- `ListUserGroupNames`
- `AddGroupMember`
- `RemoveGroupMember`
- `AddUserMetadata`
- `ListUserMetadata`
- `DeleteUserMetadata`
- `ListACLs`
- `ChangeACLs`

Local structs in `irods-keycloak-admin` should be limited to:

- Keycloak API request/response DTOs.
- Sync plan operations and plan evidence.
- Mapping policy and configuration.
- Guardrail and approval decisions.
- Audit records for service/API actions.
- Thin adapter interfaces used to test code that calls `go-irodsclient`.

If a local type needs to expose an iRODS user, group, AVU, ACL, or quota, it
should embed, reference, or translate from the canonical `go-irodsclient` type
instead of duplicating the fields as a new domain model.

Direct user/group administration tools also already exist through gocommands
and `go-irodsclient`. The command-line surface in this project
should focus on planning, applying, bootstrapping, repairing, and diagnosing
Keycloak/iRODS synchronization rather than recreating generic iRODS admin
commands.

### 4. Keep Keycloak plugins thin

Keycloak plugins should generally not contain the actual iRODS administration logic.

Preferred pattern:

```text
Keycloak Java SPI plugin
        |
        | HTTP call
        v
Go iRODS-Keycloak Admin Service
        |
        v
iRODS + Keycloak Admin REST
```

This avoids embedding heavy iRODS admin behavior inside Keycloak and reduces upgrade risk.

### 5. Prefer REST service boundaries; reserve irods4j for narrow in-process cases

Keycloak-to-iRODS behavior can be implemented in two ways:

1. Keycloak Java SPI providers call REST services.
2. Keycloak Java SPI providers call Java iRODS libraries such as
   [`irods4j`](https://github.com/irods/irods4j) directly.

Default strategy:

```text
Keycloak SPI / admin UX
        |
        | HTTPS, private service auth
        v
Go iRODS-Keycloak Admin Service
        |
        v
go-irodsclient + Keycloak Admin REST
```

Use REST service calls by default for:

- group create/delete/member add/member remove initiated from Keycloak;
- self-service provisioning requests and approvals;
- iRODS-first mutation workflows;
- Keycloak mirror updates;
- sync, bootstrap, repair, drift detection, and plan/apply workflows;
- audit-heavy operations that need uniform logging and policy checks.

Rationale:

- Keeps iRODS admin credentials, connection pooling, retries, audit behavior,
  and safety policy out of the Keycloak JVM.
- Lets the Go service reuse `go-irodsclient` and existing Go REST/DRS
  implementation patterns.
- Gives Keycloak plugins a narrow role: observe events, collect user intent,
  and call a stable service contract.
- Allows the same service contract to be used by Keycloak plugins, admin
  portals, automation, and tests.

Use `irods4j` inside Keycloak only for narrow cases where an in-process Java
provider is materially better than a REST hop:

- iRODS-backed Keycloak credential validation.
- A password bridge that must participate directly in a Keycloak credential
  flow.
- A deployment that explicitly rejects an external Go admin service and accepts
  the operational coupling.

Even in those cases, Java code should remain a thin adapter. It should not grow
its own broad user/group reconciliation engine, mirror-state model, or generic
iRODS administration toolkit.

`irods4j` is a valid option to track, but it is not the default admin/sync
implementation base. As of its upstream README, it is a Java 17 client for
iRODS 4.3.2+ with both low-level and high-level APIs, including high-level
administration APIs such as `IRODSUsers`, and the project notes that it is not
stable yet.

### 6. REST endpoint placement policy

Every REST endpoint identified for Keycloak/iRODS integration should be
classified before implementation:

| Endpoint kind | Home | Rule |
|---|---|---|
| Generic iRODS resource operation | `irods-go-rest` core API | Prefer `/api/v1/user`, `/api/v1/usergroup`, `/api/v1/path/*`, `/api/v1/ticket`, `/api/v1/resource`, or a new generic core resource. |
| Generic but higher-level iRODS workflow | `irods-go-rest` only if broadly useful | Consider `/api/v1/ext/*`, but avoid adding Keycloak-specific semantics there. |
| Keycloak mirror/sync/provisioning workflow | `irods-keycloak-admin` service API | Use a separate service interface; do not place it under `irods-go-rest` `/api/v1/ext/*`. |
| Keycloak SPI internal callback | `irods-keycloak-admin` service API | Keep private, authenticated, idempotent, and audit-heavy. |
| In-process login/password behavior | `irods-keycloak-spi`, optionally `irods4j` | Use only when direct Java participation in the Keycloak flow is necessary. |

Do not overuse `irods-go-rest` `/api/v1/ext/*`. The extension namespace is for
opinionated iRODS-facing features exposed by that REST service, not a dumping
ground for Keycloak-specific control-plane operations.

Current `irods-go-rest` generic endpoints already relevant to this work:

```http
GET    /api/v1/user
POST   /api/v1/user
GET    /api/v1/user/{user_name}
PUT    /api/v1/user/{user_name}
DELETE /api/v1/user/{user_name}

GET    /api/v1/usergroup
POST   /api/v1/usergroup
GET    /api/v1/usergroup/{group_name}
DELETE /api/v1/usergroup/{group_name}
POST   /api/v1/usergroup/{group_name}/member
DELETE /api/v1/usergroup/{group_name}/member/{user_name}
```

Those are generally suitable for `irods-go-rest` because they express iRODS
users, groups, and memberships without Keycloak-specific mirror semantics. They
may need hardening for Keycloak-driven use, especially around service-account
authorization, idempotency, audit fields, and zone handling, but they belong in
the generic REST surface rather than `/ext`.

Likely `irods-keycloak-admin` service endpoints should instead look like a
control-plane API:

```http
POST   /admin/v1/sync/plan
POST   /admin/v1/sync/apply
POST   /admin/v1/bootstrap/keycloak
POST   /admin/v1/repair/keycloak
POST   /admin/v1/provisioning/requests
POST   /admin/v1/keycloak/events
```

These endpoints are not generally suitable for `irods-go-rest` because they
refer to Keycloak realms, mirror attributes, reconciliation policy, event
payloads, or plan/apply workflows spanning both systems.

If deployment needs one public origin, an API gateway may mount
`irods-keycloak-admin` under a separate prefix such as `/keycloak-admin/v1` or
`/identity-admin/v1`. That is still a separate interface from
`irods-go-rest` core and extension APIs.

### 7. iRODS group membership should usually be authoritative

For iRODS groups, the preferred authority model is:

```text
iRODS group membership is authoritative.
Keycloak mirrors iRODS groups and memberships.
Keycloak/admin actions call iRODS first, then update or await Keycloak mirror.
```

This avoids ambiguous bidirectional reconciliation.

Without external state, a periodic sync cannot safely infer whether a membership difference means:

```text
User was added in Keycloak and should be added to iRODS
```

or:

```text
User was removed in iRODS and should be removed from Keycloak
```

So the repair/sync tool should mostly reconcile:

```text
iRODS -> Keycloak
```

Intentional Keycloak-originated group/user mutations should be direct commands that mutate iRODS first.

### 8. Avoid external state stores

The toolkit should not require a database for identity/group synchronization.

Allowed state:

- Keycloak users/groups/attributes.
- iRODS users/groups/memberships.
- Minimal iRODS AVUs for mapping and safety.
- Generated plan files for sync/apply workflows.

Not allowed by default:

- Separate sync database.
- Shadow membership table.
- External workflow state store for identity.

---

## Toolkit Capability Menu

The toolkit should be composed of the following capabilities.

### A. OIDC REST Middleware

Purpose:

```text
Validate Keycloak access token
Map token identity to effective iRODS user
Attach identity/audit context to REST request
Let REST service enforce iRODS ACLs/groups
```

Used by:

- `irods-go-drs`
- `irods-go-rest`
- `irods-keycloak-admin` API endpoints if exposed over HTTP
- future Go services above iRODS

Recommended implementation home:

```text
go-irodsclient-extensions
```

This is a good conceptual fit because the reusable value is not Keycloak
administration. The reusable value is an iRODS-facing Go service helper:
validate an OIDC credential, map it to an effective iRODS identity, attach that
identity to request context, and emit consistent audit fields. That belongs with
other shared higher-level iRODS application helpers.

Responsibilities:

- Validate JWTs or introspect opaque tokens.
- Verify issuer, audience, expiration, scopes.
- Map token claims to iRODS identity.
- Attach identity context to request.
- Emit audit fields.

Example identity context:

```go
type Identity struct {
    Issuer        string
    Subject       string
    Username      string
    Email         string
    IRODSUser     string
    IRODSZone     string
    AuthMode      string
}
```

Important guardrail:

```text
Token group claims may help UI decisions, but REST services should still enforce final data authorization through iRODS ACLs/groups.
```

Boundary:

```text
go-irodsclient-extensions may provide generic OIDC/JWT/introspection,
identity mapping, request context, and audit packages.

irods-keycloak-admin remains responsible for Keycloak Admin REST operations,
user/group reconciliation, provisioning policy, plan/apply, and iRODS-first
mutation workflows.
```

---

### B. Identity Mapper

Purpose:

```text
Keycloak identity -> iRODS identity
```

Supported mapping modes:

| Mode | Example |
|---|---|
| username claim | `preferred_username -> alice` |
| explicit claim | `irods_username -> alice` |
| email local part | `alice@example.org -> alice` |
| issuer + subject | `iss + sub -> iRODS user AVU mapping` |
| fixed realm/zone | `alice -> alice#tempZone` |

Minimal user AVUs for mapping:

```text
kc.managed       true
kc.realm         irods
kc.issuer        https://keycloak.example.org/realms/irods
kc.sub           abc-123
kc.username      alice
kc.auth_mode     oidc-federated | ldap-pam | native-mirror | irods-provider
kc.last_sync     2026-05-20T00:00:00Z
```

Guardrail:

```text
Detect normalized identity collisions and block provisioning.
```

Examples of dangerous collisions:

```text
Alice -> alice
alice -> alice
alice@example.org -> alice
alice@other.org -> alice
```

---

### C. iRODS User Provisioner

Purpose:

```text
Create/update iRODS users based on Keycloak-side events, login flows, or admin actions.
```

Operations:

- Create user.
- Disable/deprovision user.
- Ensure home collection.
- Set minimal mapping AVUs.
- Mark intended auth mode.

Implementation boundary:

```text
Use go-irodsclient for iRODS user creation, deletion, type changes, password
changes, user lookup, and user AVU operations. This project owns the
Keycloak-facing workflow, mapping policy, safety checks, and audit trail, not a
new iRODS user-management library.
```

Provisioning modes:

| Mode | iRODS user created as |
|---|---|
| OIDC-only | iRODS user for ACL/audit, no native password required |
| LDAP/PAM | iRODS user expected to authenticate through PAM LDAP |
| Native | iRODS user with native password |
| iRODS-provider | iRODS user authenticated by iRODS-backed Keycloak provider |

---

### D. Keycloak Self-Service Provisioner

Purpose:

```text
Allow authenticated Keycloak users to request or trigger iRODS provisioning.
```

Supported modes:

| Mode | Flow |
|---|---|
| automatic | first login creates iRODS user |
| request-only | first login creates access request |
| approval | admin approves, then iRODS user is created |
| invite | admin creates/invites user, then iRODS user follows |

Recommended MVP:

```text
User logs into Keycloak
  |
  v
If no matching iRODS user exists:
  show Request iRODS access
  |
  v
Admin approves
  |
  v
Create iRODS user and minimal AVUs
```

No external state means approval state should live in Keycloak attributes or be reflected after provisioning in iRODS AVUs.

---

### E. iRODS Group Manager

Purpose:

```text
Allow Keycloak/admin tooling to manage real iRODS groups.
```

Operations:

- Create iRODS group.
- Delete iRODS group.
- Add user to iRODS group.
- Remove user from iRODS group.
- List groups.
- List group members.

Implementation boundary:

```text
Use go-irodsclient user/group APIs and types for the underlying iRODS changes.
Groups are represented as iRODS users with type types.IRODSUserRodsGroup, and
members/users should use types.IRODSUser. This project should not create a
parallel Group/User domain model except for Keycloak-facing DTOs and sync plan
records.
```

Preferred flow:

```text
Admin action in Keycloak/tool
        |
        v
Call Go admin service
        |
        v
Apply operation to iRODS first
        |
        v
Update Keycloak mirror or wait for reconciler
```

Primary surface:

```text
These operations should be implemented as Go service/API capabilities surfaced
inside Keycloak or admin portals, not primarily as new command-line group tools.
Existing iRODS tools such as gocommands and go-irodsclient already cover direct
iRODS group administration.
```

Generic iRODS API surface already suitable for `irods-go-rest`:

```http
GET    /api/v1/usergroup
POST   /api/v1/usergroup
GET    /api/v1/usergroup/{group_name}
DELETE /api/v1/usergroup/{group_name}
POST   /api/v1/usergroup/{group_name}/member
DELETE /api/v1/usergroup/{group_name}/member/{user_name}
```

Keycloak-specific approval, mirror, sync, and repair behavior should stay in
the `irods-keycloak-admin` service API rather than being added to
`irods-go-rest` `/api/v1/ext/*`.

Optional diagnostic CLI wrappers may be added later, but they should not define
the primary product surface.

Recommended Keycloak mirror group attributes:

```json
{
  "irods_group_name": ["project-alpha"],
  "irods_zone": ["tempZone"],
  "managed_by": ["irods-keycloak-toolkit"],
  "authority": ["irods"],
  "mutation_order": ["irods-first"]
}
```

---

### F. Reconciler / Repair Tool

Purpose:

```text
Bootstrap, verify, and repair Keycloak mirror state from iRODS.
```

Commands:

```bash
irods-kc-sync plan
irods-kc-sync apply
irods-kc-sync bootstrap-keycloak
irods-kc-sync sync
irods-kc-sync verify
irods-kc-sync export
```

Recommended plan/apply model:

```bash
irods-kc-sync plan --config config.yaml --out plan.json
irods-kc-sync apply --plan plan.json
```

Safe stateless reconciliation:

| Thing | Safe? | Rule |
|---|---:|---|
| iRODS user -> Keycloak user | yes | create/update mirror |
| Keycloak user -> iRODS user | yes, if explicitly marked/requested | create iRODS user |
| iRODS group -> Keycloak group | yes | create/update mirror |
| iRODS membership -> Keycloak membership | yes | mirror iRODS |
| Keycloak membership -> iRODS membership | only as direct command | do not infer later |
| deletes | dangerous | disable/quarantine first |

The reconciler should generally make Keycloak look like iRODS for managed iRODS users/groups.

---

### G. Password Bridge: Keycloak -> iRODS Native Password

Purpose:

```text
When a user changes/resets a password through Keycloak,
call iRODS to set the native password.
```

This is only for native-auth scenarios.

Modes:

| Mode | Behavior |
|---|---|
| mirror password | Keycloak stores/verifies password and also updates iRODS |
| passthrough reset | Keycloak UI calls iRODS password reset |
| disabled | LDAP/OIDC owns passwords; do not touch iRODS native password |

This is sensitive and should be opt-in only.

Guardrails:

- Never enable for LDAP or upstream OIDC-federated users.
- Never log passwords.
- Fail closed if iRODS update fails.
- Prefer synchronous failure over async drift.
- Mark any failed sync state and block use if partial update occurs.

---

### H. iRODS Authentication Provider for Keycloak

Purpose:

```text
Keycloak login form
        |
        v
Validate username/password against iRODS native auth
        |
        v
Issue OIDC token
```

This supports the scenario where iRODS is the password authority and Keycloak is the OIDC broker/token issuer.

This should build on the prior `irods-keycloak-adapter` work.

---

## Usage Scenarios

## Scenario 1: Pure OIDC/SAML Federation; iRODS Does Not Natively Support OIDC

Example:

```text
Institutional OIDC/SAML/MFA
        |
        v
Keycloak brokered login
        |
        v
irods-go-rest / irods-go-drs
        |
        v
iRODS via mapped effective user
```

Flow:

1. User logs into a web app or REST client through Keycloak.
2. Keycloak handles upstream OIDC/SAML/MFA.
3. REST API validates Keycloak token.
4. Identity mapper maps token identity to iRODS user.
5. If user does not exist in iRODS, either deny, auto-provision, or require approval.
6. REST API accesses iRODS using the mapped effective identity or service/proxy pattern.
7. iRODS ACLs/groups remain the final data authorization layer.
8. Reconciler mirrors iRODS users/groups into Keycloak.

Important limitation:

```text
Native iRODS clients do not automatically work with OIDC in this mode.
```

Access path support:

| Access path | Supported? |
|---|---:|
| REST/web via Keycloak OIDC | yes |
| Native iRODS login | no, unless user also has native/PAM auth |
| Native client OIDC plugin | possible future scenario |
| service-account/proxy operations | yes, if carefully audited |

Toolkit profile:

```yaml
profile: oidc-federated-rest-only
capabilities:
  oidc_rest_middleware: true
  identity_mapper: true
  irods_user_provisioner: true
  self_service_provisioning: optional
  group_manager: true
  keycloak_mirror_reconciler: true
  password_bridge: false
  irods_keycloak_auth_provider: false
```

---

## Scenario 2: LDAP Federation; Keycloak Uses LDAP and iRODS Uses PAM LDAP

Architecture:

```text
LDAP / AD
  |              |
  v              v
Keycloak       iRODS PAM
  |
  v
REST / web OIDC clients
```

Flow:

1. User logs into Keycloak.
2. Keycloak validates password against LDAP.
3. Keycloak issues OIDC token to REST/web app.
4. REST service maps token identity to iRODS user.
5. iRODS user exists and is expected to authenticate through PAM LDAP.
6. iRODS groups/ACLs authorize data.
7. Native iRODS clients can use PAM LDAP directly, outside Keycloak.

Provisioning variants:

### Automatic first-login provisioning

```text
User logs in through LDAP-backed Keycloak
        |
        v
No iRODS user exists
        |
        v
Create iRODS user
        |
        v
Set AVUs:
  kc.realm
  kc.sub
  kc.username
  kc.auth_mode=ldap-pam
```

### Request and approval

```text
User logs in
        |
        v
Requests iRODS account
        |
        v
Admin approves
        |
        v
Toolkit creates iRODS user
```

Password handling:

```text
LDAP owns passwords.
No password bridge.
Do not mirror LDAP passwords into iRODS native auth.
```

Toolkit profile:

```yaml
profile: ldap-pam
capabilities:
  oidc_rest_middleware: true
  identity_mapper: true
  irods_user_provisioner: true
  self_service_provisioning: optional
  approval_workflow: optional
  group_manager: true
  keycloak_mirror_reconciler: true
  password_bridge: false
  irods_keycloak_auth_provider: false
```

---

## Scenario 3: iRODS Native Auth; Keycloak Provides Password Self-Service

Architecture:

```text
Keycloak account UI
        |
        v
Password change/reset action
        |
        v
Toolkit calls iRODS native password update
```

Two variants:

### 3A. Keycloak also stores the password

```text
User logs into Keycloak with Keycloak password
Password change updates:
  1. Keycloak credential
  2. iRODS native password
```

Pros:

- Familiar Keycloak login.
- REST/web OIDC works naturally.
- Native iRODS auth can use same password.

Cons:

- Two password stores.
- Risk of drift.
- Cleartext password exists transiently during change/reset.
- Failure handling must be strict.

Flow:

1. User logs into Keycloak.
2. User changes password in Keycloak Account Console.
3. Custom required action/credential handler receives new password.
4. Handler changes iRODS native password.
5. If iRODS update fails, Keycloak password update must fail.
6. Audit success/failure.

Toolkit profile:

```yaml
profile: irods-native-password-mirror
capabilities:
  oidc_rest_middleware: true
  identity_mapper: true
  irods_user_provisioner: true
  group_manager: true
  keycloak_mirror_reconciler: true
  password_bridge: true
  irods_keycloak_auth_provider: false
```

Warning:

```text
This profile is sensitive. Do not enable it alongside LDAP/OIDC-federated password authority.
```

---

## Scenario 4: Keycloak Authenticates Directly Against iRODS

Architecture:

```text
Keycloak login form
        |
        v
Custom Keycloak provider
        |
        v
iRODS native authentication
        |
        v
Keycloak issues OIDC token
```

Flow:

1. User enters username/password in Keycloak.
2. Custom provider validates credentials against iRODS.
3. If valid, Keycloak creates/updates local user representation.
4. Keycloak issues OIDC token to REST/web clients.
5. REST services map token to iRODS user.
6. Native iRODS clients continue to use iRODS native auth directly.

Password self-service:

```text
Password change should call iRODS.
Keycloak should not be treated as the password authority.
```

Toolkit profile:

```yaml
profile: irods-native-auth-provider
capabilities:
  oidc_rest_middleware: true
  identity_mapper: true
  irods_user_provisioner: optional
  group_manager: true
  keycloak_mirror_reconciler: true
  password_bridge: true
  irods_keycloak_auth_provider: true
```

---

## Additional Scenarios

### Scenario 5: Keycloak-only REST, no human direct iRODS login

Architecture:

```text
External OIDC/SAML/LDAP -> Keycloak -> REST -> iRODS
```

Users never authenticate directly to iRODS. iRODS users exist for ACLs and audit identity only.

This may be the cleanest model for portals, DRS, and web APIs.

---

### Scenario 6: Service Accounts and Machine Clients

Architecture:

```text
Keycloak client credentials
        |
        v
service account token
        |
        v
mapped to iRODS service user or constrained proxy identity
```

Guardrail:

```text
Service accounts must never be reconciled as normal human users.
```

---

### Scenario 7: Multi-Realm / Multi-Zone

Examples:

```text
Keycloak realm A -> iRODS zone A
Keycloak realm B -> iRODS zone B
```

or:

```text
one Keycloak realm -> multiple iRODS zones
```

Require explicit mapping:

```yaml
realms:
  irods-prod:
    zone: prodZone
  irods-test:
    zone: tempZone
```

Guardrail:

```text
Do not infer zone from username.
```

---

### Scenario 8: Existing iRODS Brownfield Migration

Start with existing iRODS users/groups.

Command:

```bash
irods-kc-sync bootstrap-keycloak
```

This creates or updates Keycloak mirror users/groups, configures attributes, and lets admins decide whether users are LDAP, native, OIDC-federated, or iRODS-provider backed.

---

### Scenario 9: Read-Only Institutional Identity

Some institutions allow login through OIDC/SAML/LDAP but do not allow local systems to write back users/passwords.

Model:

```text
Keycloak broker/federation is read-only.
iRODS provisioning is local.
Password changes happen at institution, not Keycloak.
```

---

## Repository Layout Decisions

Do not mix languages in a repository. Isolate by purpose and lifecycle.

## Recommended Minimum Repos

### 1. `irods-keycloak-admin`

Language: Go

Purpose: Keycloak-facing control-plane toolkit for admin API, sync, repair,
provisioning, mirror management, diagnostics, and operational sync CLI.

This is the main repo.

Design target:

```text
Keycloak-facing control plane for iRODS identity administration.
Use go-irodsclient for iRODS operations.
Use Keycloak Admin REST for Keycloak mirror operations.
Do not become a generic iRODS REST API.
Do not become a generic Keycloak administration CLI.
Do not expose generic iRODS user/group CRUD under `/admin/v1`.
```

Suggested implementation layout:

```text
irods-keycloak-admin/
  cmd/
    irods-kc-admin-server/
    irods-kc-sync/
    irods-kc-doctor/
    irods-kc-admin/        # optional diagnostics, not the main group-management surface

  internal/
    app/
    server/
    httpapi/
    service/
    domain/

    irodsadapter/          # direct go-irodsclient/usersync boundary; no REST calls
    keycloakadmin/         # Keycloak Admin REST client
    mapper/                # Keycloak identity/group <-> iRODS mapping policy
    reconcile/             # snapshot comparison and drift detection
    plan/                  # plan model, operation ordering, risk markers
    workflow/
      group/
      user/
      provisioning/
      bootstrap/
      repair/
    events/                # Keycloak event DTOs and normalization
    avu/                   # minimal AVU schema helpers
    authz/                 # admin API caller validation and scopes
    audit/                 # audit records and structured event fields
    config/                # validated runtime config

  api/
    openapi.yaml           # private admin/control-plane API
```

Responsibilities:

- Use go-irodsclient to create iRODS users.
- Use go-irodsclient to create/delete iRODS groups.
- Use go-irodsclient to add/remove users from iRODS groups.
- Mirror iRODS users/groups/memberships into Keycloak.
- Bootstrap Keycloak from iRODS.
- Repair Keycloak from iRODS.
- Validate drift.
- Apply minimal AVUs.
- Provide HTTP API for Keycloak plugins or admin portals.
- Provide sync/repair CLI for admins and CI.

Package responsibilities:

| Package | Responsibility |
|---|---|
| `internal/app` | Compose config, adapters, services, HTTP handlers, and CLI entrypoints. |
| `internal/server` | HTTP server lifecycle, timeouts, TLS/mTLS hooks, graceful shutdown. |
| `internal/httpapi` | Versioned `/admin/v1` routing, JSON DTOs, request validation, response/error mapping. |
| `internal/service` | Application service interfaces used by HTTP handlers and CLIs. |
| `internal/domain` | Local control-plane models only: plans, operation records, mirror state summaries, audit records. Do not duplicate iRODS domain structs. |
| `internal/irodsadapter` | Direct iRODS boundary over `go-irodsclient` and `go-irodsclient-extensions/usersync`. Owns direct connection setup, reuses upstream iRODS domain types, and converts direct-library errors into local service errors. It must not call `irods-go-rest`. |
| `internal/keycloakadmin` | Keycloak Admin REST client for users, groups, attributes, realm/client metadata, and service-account checks. |
| `internal/mapper` | Deterministic mapping from Keycloak realm/user/group claims to iRODS username, zone, and group name. Detects collisions. |
| `internal/reconcile` | Reads iRODS and Keycloak snapshots, compares state, and emits desired operations. |
| `internal/plan` | Defines sync plan schema, operation ordering, dependency rules, risk classes, and approval markers. |
| `internal/workflow/user` | iRODS user provisioning, deprovisioning, type/password operations when policy permits. |
| `internal/workflow/provisioning` | Self-service/request/approval flows backed by Keycloak attributes and reflected iRODS AVUs. |
| `internal/workflow/bootstrap` | Initial Keycloak mirror creation from iRODS users/groups/memberships. |
| `internal/workflow/repair` | Repair Keycloak mirror drift from authoritative iRODS state. |
| `internal/events` | Normalizes Keycloak SPI/admin events into idempotent internal commands. |
| `internal/avu` | Names and validates the minimal AVUs this toolkit owns. |
| `internal/authz` | Validates admin API callers, scopes, mTLS/service tokens, actor identity, and allowed operation classes. |
| `internal/audit` | Emits consistent audit fields: actor, source, request ID, iRODS target, Keycloak target, operation, result. |
| `internal/config` | Loads and validates iRODS, Keycloak, mapping, authz, audit, and plan/apply settings. |

Use `internal/` by default. Move code to `pkg/` only when another repository
needs to import it as a library. The first likely candidates for extraction are
stable plan schemas, event schemas, or mapping helpers, not the service
implementation itself.

Core services:

```go
type UserWorkflowService interface {
    ProvisionUser(ctx context.Context, req ProvisionUserRequest) (MutationResult, error)
    DeprovisionUser(ctx context.Context, req DeprovisionUserRequest) (MutationResult, error)
}

type SyncService interface {
    Plan(ctx context.Context, req PlanRequest) (SyncPlan, error)
    Apply(ctx context.Context, req ApplyRequest) (ApplyResult, error)
}

type BootstrapService interface {
    BootstrapKeycloak(ctx context.Context, req BootstrapRequest) (ApplyResult, error)
}

type RepairService interface {
    RepairKeycloak(ctx context.Context, req RepairRequest) (SyncPlan, error)
}

type EventService interface {
    IngestKeycloakEvent(ctx context.Context, req KeycloakEventRequest) (EventResult, error)
}
```

Service rules:

- Every mutation must be attributable to an actor and source.
- Every mutation should support dry-run through plan generation where practical.
- iRODS mutations happen before Keycloak mirror updates.
- Keycloak mirror update failure after iRODS success must be reported as a
  repairable partial result, not hidden.
- Sync/apply and provisioning operations must be idempotent where iRODS and
  Keycloak semantics allow it.
- Apply operations must refuse plans generated for a different config,
  authority mode, realm, zone, or mapping policy hash.

Admin API shape:

```http
GET    /healthz
GET    /admin/v1/status
GET    /admin/v1/config/summary

POST   /admin/v1/provisioning/users/{keycloak_user_id}/plan
POST   /admin/v1/provisioning/users/{keycloak_user_id}/apply
POST   /admin/v1/provisioning/requests
POST   /admin/v1/provisioning/requests/{request_id}/approve
POST   /admin/v1/provisioning/requests/{request_id}/reject

POST   /admin/v1/sync/plan
POST   /admin/v1/sync/apply
POST   /admin/v1/bootstrap/keycloak
POST   /admin/v1/repair/keycloak

POST   /admin/v1/keycloak/events
POST   /admin/v1/diagnostics/check-config
POST   /admin/v1/diagnostics/check-mapping
POST   /admin/v1/diagnostics/check-drift
```

Generic listing and direct CRUD for iRODS users and groups should remain
available through `irods-go-rest` core routes such as `/api/v1/user` and
`/api/v1/usergroup`. Do not add parallel `/admin/v1/irods/groups*` routes in
this service; Keycloak-facing control-plane behavior belongs in sync, repair,
bootstrap, provisioning, event, or diagnostics endpoints.

Command-line sync and repair workflows should use direct
`go-irodsclient-extensions/usersync` calls through `irodsadapter`, not
`irods-go-rest`. This keeps the CLI path aligned with `gocmd`/`drscmd`
initialization and avoids duplicating REST auth, error mapping, and local server
requirements. The admin API remains the Keycloak-facing integration boundary
for service callbacks and remote control-plane workflows.

Admin API conventions:

- Base path: `/admin/v1`.
- Authentication: private service auth, normally Keycloak-issued bearer tokens
  with service scopes; optionally mTLS for SPI-to-service calls.
- Authorization: scopes should distinguish `events`, `provision`, `mutate`,
  `plan`, `apply`, `bootstrap`, `repair`, and `diagnostics`.
- Idempotency: mutation requests should accept `Idempotency-Key`.
- Correlation: requests should accept `X-Request-ID` and emit it in audit
  fields and responses.
- Content type: JSON only for the control-plane API.
- Error shape: stable `code`, `message`, `details`, and `request_id`.
- Plan/apply: apply should accept an explicit plan object or immutable plan
  artifact reference; it should not silently recompute a different plan.

Representative mutation request:

```json
{
  "actor": {
    "type": "keycloak-user",
    "id": "admin-user-id",
    "username": "admin@example.org"
  },
  "source": "keycloak-admin-ui",
  "reason": "approved project membership",
  "realm": "example",
  "zone": "tempZone",
  "dry_run": false
}
```

Representative mutation response:

```json
{
  "status": "applied",
  "request_id": "01HX...",
  "operation": "group.add_member",
  "irods": {
    "group": "project-alpha",
    "user": "alice",
    "zone": "tempZone",
    "status": "applied"
  },
  "keycloak_mirror": {
    "realm": "example",
    "group": "/irods/project-alpha",
    "status": "updated"
  },
  "warnings": []
}
```

Representative sync plan response:

```json
{
  "plan_id": "plan-2026-05-20T15:04:05Z",
  "mode": "sync",
  "authority": "irods",
  "realm": "example",
  "zone": "tempZone",
  "mapping_policy_hash": "sha256:...",
  "summary": {
    "create_keycloak_groups": 2,
    "update_keycloak_memberships": 14,
    "delete_keycloak_mirror_groups": 0,
    "requires_approval": 0
  },
  "operations": [
    {
      "operation_id": "op-001",
      "action": "keycloak.group.create",
      "target": "/irods/project-alpha",
      "risk": "low",
      "evidence": {
        "irods_group": "project-alpha",
        "keycloak_group_missing": true
      }
    }
  ]
}
```

Non-responsibilities:

- Redefine `types.IRODSUser`, `types.IRODSMeta`, `types.IRODSAccess`, or
  related iRODS domain types.
- Rebuild generic iRODS user/group management commands already present in
  gocommands and `go-irodsclient`.

Example commands:

```bash
irods-kc-sync plan --config config.yaml --out plan.json
irods-kc-sync apply --plan plan.json
irods-kc-sync bootstrap-keycloak --config config.yaml
irods-kc-sync sync --config config.yaml

irods-kc-doctor check-config
irods-kc-doctor check-mapping
irods-kc-doctor check-drift
```

The group/user mutation code should exist as Go services and HTTP API handlers
so Keycloak extensions or admin portals can surface the operations. New CLI
wrappers for group create/delete/add/remove are optional diagnostics only,
because direct iRODS command-line administration already exists elsewhere.

---

### 2. `irods-keycloak-spi`

Language: Java

Purpose: optional Keycloak plugins only.

Suggested layout:

```text
irods-keycloak-spi/
  irods-event-listener/
  irods-required-action/
  irods-auth-provider/
  irods-password-bridge/
  common/
```

Responsibilities:

- Keycloak Event Listener Provider.
- Required Action Provider for self-service provisioning.
- iRODS User Storage/Auth Provider.
- Password bridge provider.
- Optional protocol mapper if needed.

Key decision:

```text
Keycloak event listeners are native in-process Java SPI providers, not simple generic webhook callbacks.
```

Preferred pattern:

```text
Java event listener sees user/group/admin event
        |
        v
Extract minimal event data
        |
        v
POST to Go admin service
        |
        v
Go service mutates iRODS and/or Keycloak mirror
```

Do not embed heavy iRODS admin code inside Keycloak plugin unless absolutely necessary.
If direct Java access is required, use `irods4j` only as a narrow adapter for
the specific Keycloak SPI flow, such as credential validation or password
bridging. Keep reconciliation, mirror maintenance, and iRODS-first group/user
administration in the Go admin service.

---

### 3. `go-irodsclient-extensions`

Language: Go

Purpose: shared higher-level iRODS Go library, including reusable OIDC-to-iRODS
middleware primitives for Go REST services above iRODS.

Suggested layout:

```text
go-irodsclient-extensions/
  auth/
    oidc/
    jwks/
    introspection/
    claims/
    identity/
    audit/
    middleware/
```

Responsibilities:

- Validate JWTs.
- Optional token introspection.
- Verify issuer/audience/expiration.
- Verify required scopes.
- Map token claims to iRODS username/zone.
- Attach effective iRODS identity to request context.
- Emit consistent audit fields for services.
- Provide middleware for Go HTTP routers.

Boundary:

```text
These packages should not manage Keycloak users/groups or iRODS users/groups.
They only authenticate requests, map identity, attach request context, and
standardize audit data. They may expose interfaces that callers implement when
mapping identities through iRODS AVUs or local policy.

Keycloak Admin REST, reconciliation, provisioning decisions, and group/user
mutation workflows remain in irods-keycloak-admin.
```

---

### 4. `irods-keycloak-deploy`

Language: YAML/Shell/Docs only.

Purpose: packaging, deployment examples, local dev environment, scenario docs.

Suggested layout:

```text
irods-keycloak-deploy/
  docker-compose/
    ldap-pam/
    oidc-federated/
    irods-native/
    irods-provider/

  keycloak/
    realm-templates/
    client-templates/
    mapper-templates/

  irods/
    sample-configs/
    pam-configs/

  docs/
    scenarios/
    threat-model/
    runbooks/
```

This repo should make it easy to run and understand the stack without mixing app code.

---

## Repository Recommendation Summary

Recommended starting repos:

```text
1. irods-keycloak-admin       Go
2. irods-keycloak-spi         Java
```

Add when ready:

```text
3. go-irodsclient-extensions  Go, existing shared library target for OIDC middleware primitives
4. irods-keycloak-deploy      YAML/Shell/Docs
```

Do not create a separate `irods-oidc-middleware-go` repository unless the OIDC
middleware work grows beyond the normal shared-library lifecycle of
`go-irodsclient-extensions`.

Rule:

```text
If it must be loaded by Keycloak, Java.
If it talks to iRODS, reconciles state, exposes CLI/API, or supports Go REST services, Go.
If it is deployment glue, keep it in a config/docs repo.
```

---

## Development Order

### Phase 1: Build Go Admin API and Sync First

Repo: `irods-keycloak-admin`

Deliver:

1. Keycloak-facing Go admin service/API for control-plane workflows:

```http
POST   /admin/v1/sync/plan
POST   /admin/v1/sync/apply
POST   /admin/v1/bootstrap/keycloak
POST   /admin/v1/repair/keycloak
POST   /admin/v1/provisioning/requests
POST   /admin/v1/keycloak/events
POST   /admin/v1/diagnostics/check-config
POST   /admin/v1/diagnostics/check-mapping
POST   /admin/v1/diagnostics/check-drift
```

Do not add generic iRODS group mutation routes such as
`/admin/v1/irods/groups*` to this service. Operator-facing direct iRODS
administration belongs in existing direct iRODS admin tooling, while local sync CLIs should call
`internal/irodsadapter`, which owns direct use of `go-irodsclient-extensions/usersync`
through explicit iRODS connection settings. If an operation includes
Keycloak mirror, approval, sync, repair, bootstrap, event, or diagnostics
semantics and must be invoked by Keycloak or another service, keep that API in
`irods-keycloak-admin`.

2. Sync/repair command-line workflows:

```bash
irods-kc-sync plan
irods-kc-sync apply
irods-kc-sync bootstrap-keycloak
irods-kc-sync sync
```

3. Optional doctor commands:

```bash
irods-kc-doctor check-config
irods-kc-doctor check-mapping
irods-kc-doctor check-drift
```

Do not make direct group create/delete/add-user/remove-user CLI commands the
Phase 1 product target. Existing iRODS CLIs already cover direct iRODS
administration. The Phase 1 target is the Go code and API surface that Keycloak
plugins or admin portals can call, plus sync lifecycle commands that operators
can review and automate.

The Phase 1 implementation should adapt `go-irodsclient` user, group, metadata,
and ACL APIs rather than defining a new iRODS management layer. New code should
model Keycloak synchronization intent, plan/apply safety, and service/API
contracts; the iRODS domain objects and low-level mutations remain
`go-irodsclient` responsibilities.

Why first:

- Gives immediate value.
- Defines stable admin API.
- Avoids Keycloak plugin complexity early.
- Java plugins and Keycloak-facing UX can later call this service.

---

### Phase 2: Build Go OIDC Middleware

Repo: `go-irodsclient-extensions`

Deliver:

- Token validation.
- Opaque token introspection.
- Issuer, audience, expiration, and scope checks.
- Claim mapping.
- Identity context.
- Audit field helpers.
- HTTP middleware adapters.
- Integration examples for `irods-go-drs` and `irods-go-rest`.

---

### Phase 3: Build Thin Java Event Listener

Repo: `irods-keycloak-spi`

Deliver:

```text
Event listener sees:
  user created
  user deleted
  group created/deleted
  group membership changed

Calls:
  Go admin service
```

Keep it thin and conservative.

---

### Phase 4: Required Action for Self-Service Provisioning

Repo: `irods-keycloak-spi`

Deliver:

```text
Request iRODS account
Provision iRODS account
Show iRODS provisioning status
```

Java owns the Keycloak flow/UI hook; Go performs mutation.

---

### Phase 5: Native Auth / Password Plugins

Repo: `irods-keycloak-spi`

Deliver only after earlier phases are stable:

```text
iRODS auth provider
password bridge
```

These are the riskiest pieces because they affect login and passwords.

---

## Group Management Authority Model

Recommended config:

```yaml
groups:
  authority: irods
  keycloak_group_root: /irods
  keycloak_can_create_irods_groups: true
  keycloak_can_delete_irods_groups: true
  keycloak_can_manage_membership: true
  mutation_order: irods_first
```

Meaning:

```text
Keycloak may initiate admin actions.
iRODS must receive the real mutation first.
Keycloak mirror is updated afterward or by reconciler.
```

Periodic sync behavior:

```text
iRODS group exists but Keycloak group missing:
  create Keycloak mirror group

iRODS membership exists but Keycloak membership missing:
  add Keycloak mirror membership

Keycloak membership exists but iRODS membership missing:
  remove Keycloak mirror membership

Keycloak group exists under managed /irods root but iRODS group missing:
  disable/delete Keycloak mirror group, subject to guardrails
```

Do not do this in periodic sync:

```text
Keycloak membership exists but iRODS membership missing:
  add to iRODS
```

That can incorrectly undo legitimate iRODS-side removals.

---

## Minimal AVU Model

Use AVUs only where they prevent ambiguity.

Recommended user AVUs:

```text
kc.managed       true
kc.realm         irods
kc.issuer        https://keycloak.example.org/realms/irods
kc.sub           abc-123
kc.username      alice
kc.auth_mode     oidc-federated | ldap-pam | native-mirror | irods-provider
kc.last_sync     2026-05-20T00:00:00Z
```

Optional group AVUs if acceptable:

```text
kc.managed       true
kc.realm         irods
kc.group_path    /irods/project-alpha
kc.authority     irods
kc.last_sync     2026-05-20T00:00:00Z
```

If group AVUs are avoided, use deterministic naming plus Keycloak group attributes:

```text
iRODS group project-alpha -> Keycloak /irods/project-alpha
```

---

## Dangerous Areas and Guardrails

### Danger 1: Accidental Realm Wipe

Bad config could make the sync think all iRODS users disappeared.

Guardrails:

```text
Never delete unmanaged Keycloak users.
Never hard-delete on first missing observation.
Require max-delete threshold.
Require plan/apply for destructive operations.
Prefer disable/quarantine before delete.
```

Example config:

```yaml
safety:
  max_user_deletes_per_run: 5
  delete_policy: disable_then_delete
  delete_grace_period: 7d
  require_confirm_for_deletes: true
```

---

### Danger 2: Membership Drift

If someone edits managed Keycloak groups directly, Keycloak may drift from iRODS.

Guardrail:

```text
For /irods managed groups, periodic repair makes Keycloak match iRODS.
Keycloak-side UI actions must call iRODS first.
```

Do not infer iRODS membership changes from Keycloak during repair.

---

### Danger 3: Password Drift

Scenario 3 can cause Keycloak and iRODS passwords to diverge.

Guardrail:

```text
iRODS password update must succeed before Keycloak accepts the change,
or the user must be marked password_sync_failed and blocked.
```

Also:

```text
Never enable password bridge for LDAP/OIDC-federated users.
Never log password payloads.
Never run password bridge asynchronously without compensation.
```

---

### Danger 4: Identity Collisions

Different external users may map to the same iRODS username.

Guardrail:

```text
Identity mapper must detect normalized collisions and block provisioning.
```

---

### Danger 5: Stale Token Group Claims

Keycloak token might say user is in a group after iRODS membership changed.

Guardrail:

```text
REST services must enforce iRODS ACLs/group state, not trust token groups as final data authorization.
```

---

### Danger 6: Deleting iRODS Groups With ACLs

Deleting iRODS groups can affect data access broadly.

Guardrails:

```text
Before delete:
  list members
  check ACL references if possible
  require explicit confirmation
  default to disable/hide Keycloak mirror, not delete iRODS group
```

---

### Danger 7: Mixed Authority Confusion

Do not enable contradictory authority modes.

Every deployment profile must declare authority explicitly:

```yaml
groups:
  authority: irods
users:
  creation_sources:
    - irods
    - keycloak-approved
passwords:
  authority: ldap | keycloak-mirror | irods-provider | external-oidc
```

---

## Profile Matrix

| Scenario | Password authority | REST auth | Direct iRODS login | User provisioning | Group management |
|---|---|---|---|---|---|
| 1. OIDC/SAML federation | upstream IdP | Keycloak OIDC | not unless separate auth | Keycloak/self-service -> iRODS | iRODS-first admin tool |
| 2. LDAP + PAM | LDAP | Keycloak OIDC | PAM LDAP | Keycloak/approval -> iRODS PAM user | iRODS-first admin tool |
| 3. Native + KC password self-service | Keycloak+iRODS mirrored | Keycloak OIDC | iRODS native | Keycloak/iRODS | iRODS-first admin tool |
| 4. KC authenticates against iRODS | iRODS native | Keycloak OIDC | iRODS native | iRODS -> Keycloak mirror | iRODS-first admin tool |
| 5. REST-only portal | upstream/Keycloak | Keycloak OIDC | no human direct login | self-service/approval | iRODS-first admin tool |
| 6. Machine clients | Keycloak client creds | Keycloak OIDC | service only | explicit service mapping | explicit only |

---

## Implementation Notes for Keycloak Events

Keycloak event listeners are not generic remote callbacks by default. They are Java SPI providers running inside Keycloak.

Pattern:

```text
Keycloak JVM
  loads Java provider JAR
  invokes EventListenerProvider on events
```

The event listener may then act like a webhook bridge:

```text
EventListenerProvider
        |
        v
HTTP POST to irods-keycloak-admin
        |
        v
iRODS mutation / Keycloak Admin REST mirror update
```

Keep the listener minimal:

- Extract event type.
- Extract user/group identifiers.
- Validate it is a managed realm/group if applicable.
- Call Go admin service.
- Log result.
- Avoid embedding iRODS client and business rules in Java plugin.

---

## Practical Meaning of iRODS-First Group Management

Bad model:

```text
Admin adds alice to /irods/project-alpha in Keycloak
        |
        v
Later sync notices Keycloak has alice but iRODS does not
        |
        v
Sync guesses maybe it should add alice to iRODS
```

Problem:

```text
The sync cannot know whether Alice was added in Keycloak
or removed in iRODS.
```

Good model:

```text
Admin clicks Add alice to project-alpha
        |
        v
Tool/API calls iRODS immediately:
    add alice to iRODS group project-alpha
        |
        v
If iRODS succeeds:
    update Keycloak mirror immediately or wait for sync
```

This means Keycloak is a control surface, not the membership database.

---

## Recommended MVP Scope

Start with:

```text
1. Go admin service/API for Keycloak-facing iRODS user/group operations.
2. Sync/repair CLI from iRODS to Keycloak.
3. Minimal AVU mapping.
4. OIDC middleware for REST services in go-irodsclient-extensions.
5. Plan/apply guardrails.
```

Do not start with:

```text
1. Password bridge.
2. iRODS native auth provider.
3. Complex Keycloak admin UI extension.
4. Bidirectional periodic membership inference.
```

Those can come later after the basic authority and reconciliation model is stable.

---

## Final Standing Recommendation

Build a composable iRODS-Keycloak toolkit with these boundaries:

```text
irods-keycloak-admin       Go
  Main toolkit: admin API, sync, repair, Keycloak-facing group/user operations, and sync CLI.

irods-keycloak-spi         Java
  Thin Keycloak plugins only: event listener, required action, auth provider, password bridge.

go-irodsclient-extensions  Go
  Shared OIDC-to-iRODS identity middleware, request context, and audit helpers for DRS/REST services.

irods-keycloak-deploy      YAML/Shell/Docs
  Docker Compose, realm templates, scenario configs, runbooks.
```

Default operational posture:

```text
Keycloak handles login, federation, OIDC clients, self-service UX, and admin initiation.
iRODS remains authoritative for data users/groups/ACLs.
Group mutations are applied iRODS-first.
Keycloak mirrors managed iRODS groups and users.
Reconciliation is conservative and plan/apply based.
No external identity sync database is required.
```
