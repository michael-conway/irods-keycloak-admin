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
- synchronization when actions are initiated either through `iadmin` or through
  Keycloak-facing workflow surfaces

Tools and framework themes:

- Keycloak LDAP federation
- iRODS PAM LDAP
- iRODS administration through `iadmin` and service tooling
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
- synchronization must tolerate changes that originate through either `iadmin`
  or Keycloak workflow surfaces

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
- synchronization when actions are initiated either through `iadmin` or through
  Keycloak-facing workflow surfaces
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
- synchronization must tolerate changes that originate through either `iadmin`
  or Keycloak workflow surfaces

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

### Scenario 5: Keycloak-only REST, No Human iCommands

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
