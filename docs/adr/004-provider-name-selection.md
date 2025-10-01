# ADR-004: Provider Name Selection

## Status

Accepted

## Context

We need to decide on the naming convention for our Terraform provider. Currently:
- Repository name: `terraform-provider-nats-jwt` (with hyphen)
- Provider type in code: `natsjwt` (no hyphen)
- Resource prefix: `nsc_*` (no hyphen, underscore separator)

This creates an inconsistency on the Terraform Registry where the registry URL shows `nats-jwt` but the actual provider type and resource names use `natsjwt`.

The provider is not yet published, so we have full freedom to rename without breaking existing users.

Existing NATS ecosystem providers:
- `terraform-provider-jetstream` by nats-io (official, manages JetStream streams/consumers)

## Decision

**Rename the provider to `nsc`** (NATS Security and Configuration tool)

- Repository: `terraform-provider-nsc`
- Provider type: `nsc`
- Resources: `nsc_operator`, `nsc_account`, `nsc_user`

## Options Considered

### Option 1: Keep Current (nats-jwt repo, natsjwt provider)

**Pros**: Human-readable repo, avoids hyphen issues
**Cons**: Inconsistent between registry display and actual usage

### Option 2: Rename to natsjwt everywhere

**Pros**: Perfect consistency
**Cons**: Less readable, loses semantic clarity

### Option 3: Name it after nsc tool (CHOSEN)

**Pros**:
- Familiar to NATS users - nsc is the official NATS security tool
- Shorter, cleaner resource names
- Clear association with NATS security/JWT functionality
- Perfect consistency across repo/registry/code
- Aligns with NATS ecosystem conventions (like jetstream provider)

**Cons**:
- May require explanation for non-NATS users
- Three-letter acronym less searchable

### Option 4: Use natsauth or natsaccess

**Pros**: More descriptive
**Cons**: Longer names, doesn't align with existing NATS tooling

## Rationale

1. **Ecosystem alignment**: nsc is the canonical NATS security tool. Terraform users familiar with NATS will immediately understand the provider's purpose.

2. **Conciseness**: Short, clean resource names:
   - `nsc_user` vs `nsc_user`
   - `nsc_account` vs `nsc_account`
   - `nsc_operator` vs `nsc_operator`

3. **Consistency**: Perfect alignment between repository name, registry listing, and code usage.

4. **Precedent**: The official NATS provider is named `jetstream` (after the tool), not `nats-jetstream`. Following this pattern: `nsc` not `nats-nsc`.

5. **No conflicts**: No existing `terraform-provider-nsc` in the registry.

6. **Semantic clarity**: "NSC for Terraform" clearly indicates this is the Terraform equivalent of the nsc CLI tool.

## Consequences

### Positive

- Clean, short resource names
- Immediately recognizable to NATS users (target audience)
- Perfect naming consistency eliminates confusion
- Aligns with NATS ecosystem naming conventions
- No Terraform hyphen-related issues

### Negative

- Three-letter acronym may be less discoverable in general searches
- Requires explanation for users unfamiliar with NATS ecosystem
- More extensive refactoring required (but no users to break yet)

### Mitigation

- Comprehensive documentation explaining NSC relationship
- README clearly states: "Terraform provider for NATS JWT authentication (nsc)"
- Registry description mentions both "NATS" and "JWT" for searchability
- Examples show equivalence with nsc CLI commands

## Implementation

Changes required:
1. Rename GitHub repository to `terraform-provider-nsc`
2. Update provider type name in `internal/provider/provider.go`
3. Rename all resource files and types
4. Update `go.mod` module path
5. Update all documentation, examples, and templates
6. Update generated documentation
7. Re-tag release
