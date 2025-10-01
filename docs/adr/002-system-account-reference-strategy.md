# ADR-002: System Account Reference Strategy

## Status

Superseded by ADR-003

## Note

This ADR has been superseded by ADR-003. The two-pass approach described here was found to cause perpetual drift in Terraform state due to the circular dependency between operator and system account.

## Context

Following ADR-001's decision for explicit system account creation, we need to determine how to link
the system account to the operator in the JWT claims.

The NATS JWT `OperatorClaims` structure includes a `SystemAccount` field that should contain the
public key of the system account. This creates a circular dependency challenge in Terraform:

- The operator needs to know the system account's public key
- The account needs the operator's seed to sign its JWT
- Both resources need to exist for the references to work

## Decision

Use a **two-pass approach** where the system account reference is optional on the operator and can
be set after account creation:

```hcl
# Step 1: Create operator
resource "nsc_operator" "main" {
  name = "MyOperator"
  # system_account is optional, can be set later
}

# Step 2: Create system account
resource "nsc_account" "system" {
  name          = "SYS"
  operator_seed = nsc_operator.main.seed
  is_system     = true
}

# Step 3: Update operator with system account reference
resource "nsc_operator" "main" {
  name           = "MyOperator"
  system_account = nsc_account.system.public_key
}
```

### Alternative: Computed System Account

We could also make the operator automatically detect and use any account with `is_system = true`,
but this would:

- Create implicit behavior
- Make the operator dependent on all accounts
- Complicate the provider logic

## Rationale

### Terraform Dependency Management

The two-pass approach works well with Terraform's dependency graph:

- Initial operator creation doesn't require system account
- System account creation uses operator's seed
- Operator update adds the system account reference
- Clean dependency chain: operator → account → operator update

### Matches Real-World Usage

This mirrors how operators are actually deployed:

1. Create operator first
2. Create accounts (including system)
3. Configure operator with system account

### Flexibility

- Operators can function without system accounts initially
- System account can be added/changed later
- No forced ordering of resource creation

## Consequences

### Positive

- No circular dependencies
- Clean Terraform apply on first run
- Flexible deployment options
- Can add system account to existing operators

### Negative

- Operator JWT needs regeneration when system account is added
- Requires understanding of the two-pass pattern
- System account reference could be inconsistent

### Mitigation

- Document the pattern clearly in examples
- Add validation to ensure referenced account has `is_system = true`
- Consider adding a warning if operator has no system account

## Implementation Notes

### Validation

When `system_account` is set on operator:

1. Verify it's a valid account public key (starts with 'A')
2. Optionally verify the account exists and has `is_system = true`

### JWT Regeneration

The operator JWT must be regenerated whenever the system account changes, as it's embedded in the
claims.

## Examples

### Complete Configuration

```hcl
resource "nsc_operator" "main" {
  name                 = "production"
  system_account       = nsc_account.system.public_key
  generate_signing_key = true
}

resource "nsc_account" "system" {
  name          = "SYS"
  operator_seed = nsc_operator.main.seed
  is_system     = true
}

resource "nsc_account" "app" {
  name          = "application"
  operator_seed = nsc_operator.main.seed

  allow_pub = [ "app.>" ]
  allow_sub = [ "app.>", "_INBOX.>" ]
}
```

### Migration Path

For existing operators without system accounts:

```hcl
# Add system account to existing operator
resource "nsc_account" "system" {
  name          = "SYS"
  operator_seed = data.nsc_operator.existing.seed
  is_system     = true
}

# Update operator configuration
resource "nsc_operator" "existing" {
  # ... existing config ...
  system_account = nsc_account.system.public_key
}
```