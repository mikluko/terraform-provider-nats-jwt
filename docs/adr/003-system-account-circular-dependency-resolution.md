# ADR-003: System Account Circular Dependency Resolution

## Status

Accepted

## Context

Following ADR-001 and ADR-002, we discovered a critical circular dependency issue with the explicit
system account approach:

1. The operator JWT must contain the system account's public key in its `SystemAccount` field
2. The system account JWT must be signed by the operator's key
3. Terraform cannot handle this circular dependency cleanly

This creates an impossible situation:

- Can't create operator without system account public key
- Can't create system account without operator seed
- The "two-pass" approach in ADR-002 requires operator JWT regeneration, which changes the operator
  resource on every apply

## Decision

**Manage the system account as part of the operator resource** with an optional
`create_system_account` flag.

```hcl
resource "nsc_operator" "main" {
  name = "MyOperator"
  generate_signing_key = true

  # Optional: Create and manage system account within operator
  create_system_account = true
  system_account_name   = "SYS"  # Optional, defaults to "SYS"
}

# Regular accounts still created separately
resource "nsc_account" "app" {
  name          = "AppAccount"
  operator_seed = nsc_operator.main.seed
  # ...
}
```

## Alternatives Considered

### Alternative 1: Keep Explicit (Current Implementation)

- **Problem**: Circular dependency requires constant JWT regeneration
- **Impact**: Operator shows changes on every `terraform apply`

### Alternative 2: System Account Data Source

- **Idea**: Create account, then use data source to update operator
- **Problem**: Still requires two applies and JWT regeneration

### Alternative 3: Deferred System Account Reference

- **Idea**: Allow operator without system account initially
- **Problem**: Not compliant with NATS best practices; operator mode requires system account

## Rationale

### Solves Circular Dependency

- Operator resource creates both operator and system account atomically
- System account public key is known at operator creation time
- Single resource manages the complete operator configuration

### Aligns with nsc Behavior

```bash
nsc add operator --name MyOp --sys # Creates both operator and system account
```

This matches how `nsc` handles the relationship.

### Maintains Terraform Principles

- Still explicit: User must opt-in with `create_system_account = true`
- Resource represents a complete, functional unit (operator with its system account)
- No hidden side effects: Flag clearly indicates system account creation

### Backwards Compatible

- Operators without system accounts can still be created (set flag to false)
- Existing operators can be imported without system accounts
- Migration path available for existing deployments

## Consequences

### Positive

- **No circular dependencies**: Single resource manages both entities
- **Stable state**: No constant JWT regeneration
- **Matches nsc**: Familiar pattern for NATS users
- **Single apply**: Everything works on first `terraform apply`

### Negative

- **Less flexible**: System account configuration limited to operator resource attributes
- **Coupling**: Operator and system account lifecycle are tied together
- **Resource complexity**: Operator resource becomes more complex

### Mitigation

To address the negatives:

1. Provide essential system account configuration options in operator resource
2. Document that advanced system account features require separate management
3. Consider a future `nsc_system_account` resource for advanced use cases

## Implementation Details

### Operator Resource Attributes

```hcl
resource "nsc_operator" "main" {
  # Existing attributes
  name                 = "MyOperator"
  generate_signing_key = true
  expiry               = "8760h"
  start = "0s"

  # System account management
  create_system_account = true        # Create system account
  system_account_name = "SYS"       # Optional, defaults to "SYS"

  # Future: Basic system account configuration
  # system_account_exports = [...]    # For future implementation
}
```

### Computed Outputs

```hcl
# Operator outputs
output "operator_jwt" { value = nsc_operator.main.jwt }
output "operator_seed" { value = nsc_operator.main.seed }

# System account outputs (when created)
output "system_jwt" { value = nsc_operator.main.system_account_jwt }
output "system_seed" { value = nsc_operator.main.system_account_seed }
output "system_public_key" { value = nsc_operator.main.system_account }
```

### Migration Strategy

For users of the previous approach:

1. Remove separate `nsc_account` resource for system account
2. Set `create_system_account = true` on operator
3. Run `terraform apply` to migrate

## Decision Request

This ADR proposes reversing ADR-001 and ADR-002 based on the discovered circular dependency issue.
The proposed solution manages the system account as part of the operator resource, similar to how
`nsc` works.

**Do you approve this approach?**