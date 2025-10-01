# ADR-001: System Account Creation Strategy

## Status

Superseded by ADR-003

## Note

This ADR has been superseded by ADR-003 due to the discovery of a circular dependency issue between operator and system account resources in Terraform.

## Context

NATS JWT authentication in operator mode requires a system account for cluster monitoring and
management operations. The system account provides access to system-level subjects like `$SYS.>` for
monitoring server health, connections, and other operational metrics.

When implementing a Terraform provider for NATS JWT management, we need to decide how to handle
system account creation:

1. Should it be created implicitly when creating an operator?
2. Should it be an explicit, separate resource?

### Current Behavior in nsc

The `nsc` tool provides a `--sys` flag when creating operators:

```bash
nsc add operator --name MyOp --sys # Creates operator with system account

```

This creates both the operator and a system account named "SYS" in a single operation.

## Decision

We will require **explicit creation** of the system account as a separate `nsc_account` resource
with an `is_system` flag.

```hcl
# Explicit system account creation
resource "nsc_account" "system" {
  name          = "SYS"
  operator_seed = nsc_operator.main.seed
  is_system     = true
}
```

## Rationale

### Terraform Best Practices

Terraform strongly favors explicit resource management over implicit creation:

- Each infrastructure component should be a distinct resource
- Resources should have clear ownership and lifecycle
- Side effects and hidden resources should be avoided

### Flexibility and Customization

Explicit creation allows full control over the system account:

- Custom naming (not forced to use "SYS")
- Configure permissions, limits, and exports
- Add imports from other accounts
- Set expiry and validity periods
- Future support for account-specific features

### Separation of Concerns

The operator resource should focus solely on operator-level configuration:

- Signing keys
- Operator claims
- Operator metadata

Account management, including system accounts, belongs in account resources.

### Import and State Management

Explicit resources simplify Terraform operations:

- Clean import: `terraform import nsc_account.system "SYS/SA.../SO..."`
- Clear state representation
- Predictable destroy behavior
- No hidden dependencies

### Alternative Considered: Implicit Creation

We considered creating the system account automatically with the operator:

```hcl
# Alternative: Implicit approach (rejected)
resource "nsc_operator" "main" {
  name               = "MyOperator"
  create_sys_account = true  # Would create system account implicitly
}
```

This was rejected because:

- Violates Terraform's explicit resource principle
- Complicates the operator resource with account logic
- Makes it difficult to customize the system account
- Creates confusion about resource ownership and lifecycle

## Consequences

### Positive

- **Explicit control**: Users have full control over system account configuration
- **Terraform idiomatic**: Follows established Terraform patterns
- **Future-proof**: Easy to add features like exports, imports, limits
- **Clean architecture**: Clear separation between operator and account concerns
- **Standard workflows**: Works with normal Terraform import, plan, apply, destroy

### Negative

- **Extra step**: Users must explicitly create the system account
- **Documentation burden**: Need clear documentation about system account requirement
- **Potential for mistakes**: Users might forget to create system account

### Mitigation

To address the negatives:

1. Provide clear examples in documentation showing system account creation
2. Add validation to warn if no system account exists
3. Include system account in all example configurations
4. Consider adding a data source to validate operator configuration completeness

## Implementation Notes

### Validation Rules

- Only one account per operator can have `is_system = true`
- System account should typically have monitoring exports (future feature)
- Warn if operator has no associated system account (optional validation)

### Future Enhancements

When we add support for exports and imports:

- System account should export `$SYS.>` subjects by default
- Provide templates for common system account configurations
- Support for account JWT push to NATS servers

## References

- [NATS System Account Documentation](https://docs.nats.io/running-a-nats-service/nats_admin/sys_accounts)
- [Terraform Best Practices - Explicit vs Implicit](https://www.terraform.io/docs/extend/best-practices/design-principles.html)
- [nsc Operator Creation](https://docs.nats.io/using-nats/nats-tools/nsc/nsc)