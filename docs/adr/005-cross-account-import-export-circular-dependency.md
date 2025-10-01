# ADR-005: Cross-Account Import/Export Circular Dependency

## Status

Superseded by ADR-006 (Unified NKey Resource)

## Context

When two accounts need to exchange data bidirectionally using NATS imports and exports, a circular
dependency emerges in Terraform:

```hcl
resource "nsc_account" "service_a" {
  name          = "service-a"
  operator_seed = nsc_operator.main.seed

  export {
    subject = "requests.>"
    type    = "stream"
  }

  import {
    account = nsc_account.service_b.public_key  # Depends on service_b
    subject = "responses.>"
    type    = "stream"
  }
}

resource "nsc_account" "service_b" {
  name          = "service-b"
  operator_seed = nsc_operator.main.seed

  export {
    subject = "responses.>"
    type    = "stream"
  }

  import {
    account = nsc_account.service_a.public_key  # Depends on service_a
    subject = "requests.>"
    type    = "stream"
  }
}
```

**The Problem:**

- Account A imports from Account B (needs B's public key)
- Account B imports from Account A (needs A's public key)
- Terraform cannot resolve this circular dependency

This is a fundamental issue with bidirectional service communication patterns in NATS.

## Key Observations

1. **Account public keys are deterministic**: Given a seed, the public key is immediately derivable
   without creating the JWT
2. **JWTs contain imports/exports**: The JWT must be signed with all imports/exports included
3. **Imports require exporter's public key**: The importing account JWT must reference the exporting
   account's public key
4. **Two-phase approach won't work**: Unlike the system account case (ADR-003), we can't manage both
   accounts in one resource

## Alternatives Considered

### Alternative 1: Separate Key Generation from JWT Signing

Split each account resource into two separate resources:

```hcl
# Phase 1: Generate keys only
resource "nsc_account_key" "service_a" {
  name = "service-a"
}

resource "nsc_account_key" "service_b" {
  name = "service-b"
}

# Phase 2: Sign JWTs with imports/exports
resource "nsc_account_jwt" "service_a" {
  account_seed  = nsc_account_key.service_a.seed
  operator_seed = nsc_operator.main.seed

  export {
    subject = "requests.>"
    type    = "stream"
  }

  import {
    account = nsc_account_key.service_b.public_key  # No circular dependency!
    subject = "responses.>"
    type    = "stream"
  }
}

resource "nsc_account_jwt" "service_b" {
  account_seed  = nsc_account_key.service_b.seed
  operator_seed = nsc_operator.main.seed

  export {
    subject = "responses.>"
    type    = "stream"
  }

  import {
    account = nsc_account_key.service_a.public_key  # No circular dependency!
    subject = "requests.>"
    type    = "stream"
  }
}
```

**Pros:**

- Breaks circular dependency completely
- Keys available before JWT signing
- Explicit two-phase approach
- More granular control over lifecycle

**Cons:**

- Breaking change requiring migration
- More verbose configuration
- Deviates from typical Terraform patterns
- Users must manage two resources per account

### Alternative 2: Add `seed` Input Attribute to Account Resource

Allow users to optionally provide a pre-generated seed:

```hcl
resource "nsc_account" "uptime_dev" {
  name          = "uptime-com.uptime.dev"
  operator_seed = nsc_operator.uptime.seed
  seed          = "SA..." # Optional: provide pre-generated seed

  import {
    account = nsc_account.up2_monitoring_dev.public_key
    subject = "up.dev.monitoring.*.checkexecutioncompleted"
    type    = "stream"
  }
}
```

Workflow:

1. Generate seeds externally: `nk -gen account`
2. Derive public keys externally: `nk -pubout -inkey seed.txt`
3. Configure accounts with known public keys
4. Apply creates JWTs with correct imports

**Pros:**

- Minimal API change
- No additional resources needed
- Backwards compatible (seed is optional)

**Cons:**

- Requires manual key generation step
- Seeds must be managed outside Terraform initially
- Not fully declarative
- Error-prone (easy to mismatch seed/public key)

### Alternative 3: Use `terraform_data` Resource for Key Pre-generation

Leverage Terraform's `terraform_data` resource with external key generation:

```hcl
resource "terraform_data" "uptime_dev_key" {
  provisioner "local-exec" {
    command = "nk -gen account > /tmp/uptime_dev_seed.txt"
  }
}

data "external" "uptime_dev_pubkey" {
  program = [ "sh", "-c", "nk -pubout -inkey /tmp/uptime_dev_seed.txt | jq -R '{public_key: .}'" ]
  depends_on = [ terraform_data.uptime_dev_key ]
}

resource "nsc_account" "uptime_dev" {
  seed = file("/tmp/uptime_dev_seed.txt")
  operator_seed = nsc_operator.uptime.seed

  import {
    account = data.external.up2_monitoring_dev_pubkey.result.public_key
    subject = "..."
    type    = "stream"
  }
}
```

**Pros:**

- Works with existing resources
- No provider changes needed

**Cons:**

- Extremely hacky and brittle
- Requires external tools (nk)
- Not portable across platforms
- State management nightmare

### Alternative 4: Defer Import Configuration to Separate Resource

Create accounts without imports, then add imports via separate resource:

```hcl
resource "nsc_account" "uptime_dev" {
  name          = "uptime-com.uptime.dev"
  operator_seed = nsc_operator.uptime.seed

  export {
    subject = "up.dev.monitoring.*.checkconfigbatch"
    type    = "stream"
  }
  # No imports yet
}

resource "nsc_account" "up2_monitoring_dev" {
  name          = "uptime-com.up2-monitoring.dev"
  operator_seed = nsc_operator.uptime.seed

  export {
    subject = "up.dev.monitoring.*.checkexecutioncompleted"
    type    = "stream"
  }
  # No imports yet
}

# Add imports after both accounts exist
resource "nsc_account_import" "uptime_dev_imports" {
  account_seed  = nsc_account.uptime_dev.seed
  operator_seed = nsc_operator.uptime.seed

  import {
    account = nsc_account.up2_monitoring_dev.public_key
    subject = "up.dev.monitoring.*.checkexecutioncompleted"
    type    = "stream"
  }
}

resource "nsc_account_import" "up2_monitoring_dev_imports" {
  account_seed  = nsc_account.up2_monitoring_dev.seed
  operator_seed = nsc_operator.uptime.seed

  import {
    account = nsc_account.uptime_dev.public_key
    subject = "up.dev.monitoring.*.checkconfigbatch"
    type    = "stream"
  }
}
```

**Pros:**

- No breaking changes to existing accounts
- Imports added incrementally
- Clear dependency chain

**Cons:**

- Requires JWT regeneration when imports change
- More resources to manage
- Less intuitive - imports separated from account definition

## Recommendation

**Alternative 1: Separate Key Generation from JWT Signing** is the cleanest solution because:

1. **Solves the root cause**: Keys are available before JWT creation
2. **Aligns with NATS architecture**: Key generation and JWT signing are conceptually separate
   operations
3. **Enables advanced patterns**: Users can reference keys without creating JWTs immediately
4. **Explicit lifecycle management**: Clear separation between key creation and JWT issuance

## Proposed Implementation

### New Resources

```hcl
# nsc_account_key - Generates account keypair only
resource "nsc_account_key" "example" {
  name = "my-account"
}

# Outputs:
# - id (public key)
# - public_key
# - seed (sensitive)
```

```hcl
# nsc_account_jwt - Signs account JWT with all claims
resource "nsc_account_jwt" "example" {
  account_seed  = nsc_account_key.example.seed
  operator_seed = nsc_operator.main.seed

  # All existing account attributes (imports, exports, limits, etc.)
  import { ... }
  export { ... }
}

# Outputs:
# - id (public key, derived from seed)
# - public_key (derived from seed)
# - jwt (signed token)
```

### Migration Path

**Option A: Deprecate `nsc_account` gradually**

1. Mark `nsc_account` as deprecated in v0.7.0
2. Add migration guide to docs
3. Remove in v1.0.0

**Option B: Keep `nsc_account` as convenience wrapper**

1. Implement `nsc_account` internally as `nsc_account_key` + `nsc_account_jwt`
2. Provide both interfaces indefinitely
3. Users choose based on their needs (simple vs. complex dependencies)

### Example Migration

**Before:**

```hcl
resource "nsc_account" "app" {
  name          = "app"
  operator_seed = nsc_operator.main.seed

  import {
    account = nsc_account.monitoring.public_key
    subject = "metrics.>"
    type    = "stream"
  }
}
```

**After:**

```hcl
resource "nsc_account_key" "app" {
  name = "app"
}

resource "nsc_account_jwt" "app" {
  account_seed  = nsc_account_key.app.seed
  operator_seed = nsc_operator.main.seed

  import {
    account = nsc_account_key.monitoring.public_key  # No circular dependency
    subject = "metrics.>"
    type    = "stream"
  }
}
```

## Consequences

### Positive

- **Eliminates circular dependencies**: Keys available before JWT signing
- **More flexible**: Keys can be generated, referenced, and used independently
- **Aligns with NATS concepts**: Separation of key material and signed claims
- **Enables advanced patterns**: External key generation, key rotation, etc.

### Negative

- **Breaking change**: Requires migration for existing users
- **More verbose**: Simple cases require two resources instead of one
- **Learning curve**: Users must understand key vs. JWT distinction

### Neutral

- **Matches operator pattern**: Similar to how signing keys work on operator resource
- **State management**: Two resources mean two state entries per account

## Open Questions

1. **Should we keep `nsc_account` as a convenience wrapper?** This would provide backwards
   compatibility.
2. **Should `nsc_account_key` accept an optional seed input?** Enables importing existing keys.
3. **Should we provide utilities for external key generation?** Could include a `nsc_key_generator`
   data source.
4. **What about user resources?** Users don't have circular dependency issues since they're leaf
   nodes in the hierarchy. Splitting for uniformity adds complexity without solving a problem.
   Recommendation: Keep `nsc_user` as-is unless a concrete use case emerges.

## Decision Request

Should we proceed with splitting account resources into `nsc_account_key` and `nsc_account_jwt` to
resolve cross-account circular dependencies?

If yes:

- Should `nsc_account` be deprecated or kept as a convenience wrapper?
- Should this be v0.7.0 (MESO) or v1.0.0 (MACRO)?
