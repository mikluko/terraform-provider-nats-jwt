# ADR-006: Unified NKey Resource

## Status

Proposed (Supersedes ADR-005)

## Context

ADR-005 proposed splitting account resources into `nsc_account_key` and `nsc_account_jwt` to resolve circular dependencies. However, this approach has limitations:

1. **Type-specific duplication**: We'd need `nsc_operator_key`, `nsc_account_key`, `nsc_user_key`
2. **Inconsistent with operator/user**: Only accounts have the split, creating API inconsistency
3. **System account complexity**: System accounts are still embedded in operator resource (ADR-003)
4. **Not extensible**: Adding new key types requires new resources

## Decision

Create a **single generic `nsc_nkey` resource** for all key generation, and update JWT/credential resources to accept seeds as input:

```hcl
# Generic key generation
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

# JWT generation - only needs issuer's seed and subject's public key
resource "nsc_operator" "main" {
  name        = "MyOperator"
  subject     = nsc_nkey.operator.public_key  # Subject of JWT (self-issued)
  issuer_seed = nsc_nkey.operator.seed        # Self-signs with this
}

resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key  # Subject of JWT
  issuer_seed = nsc_nkey.operator.seed       # Issuer signs with this
}

resource "nsc_user" "service" {
  name        = "ServiceUser"
  subject     = nsc_nkey.user.public_key  # Subject of JWT
  issuer_seed = nsc_nkey.account.seed     # Issuer signs with this
}

# Credentials file combines JWT with user's private seed
data "nsc_creds" "service" {
  jwt  = nsc_user.service.jwt
  seed = nsc_nkey.user.seed
}
```

## Key Changes from Current Implementation

### 1. Single Generic Key Resource

**`nsc_nkey`** - Generates keypairs for any NATS key type:

```hcl
resource "nsc_nkey" "example" {
  type = "operator" # or "account", "user"
}

# Outputs:
# - id (public key)
# - type
# - public_key
# - seed (sensitive)
```

**Validation:**
- `type` is required and must be one of: `operator`, `account`, `user`
- Generated keys are validated to match the specified type

**Import:**
- Import ID format: `<seed>`
- Example: `terraform import nsc_nkey.example SOABC123...`
- The seed is validated to match the resource's declared `type`

### 2. Updated JWT/Credential Resources

All resources accept seeds as inputs instead of generating them:

**`nsc_operator`:**
```hcl
resource "nsc_operator" "main" {
  name        = "MyOperator"
  subject     = nsc_nkey.operator.public_key  # REQUIRED - subject (self-issued)
  issuer_seed = nsc_nkey.operator.seed        # REQUIRED - signs the JWT

  # Optional signing keys (list of public keys)
  # These keys can be used to sign account JWTs instead of the main operator key
  signing_keys = [nsc_nkey.signing.public_key]

  # Optional system account reference
  system_account = nsc_account.sys.public_key
}
```

**Example with signing keys:**
```hcl
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "signing_key" {
  type = "operator"  # Signing keys are also operator-type keys
}

resource "nsc_operator" "main" {
  name         = "MyOperator"
  subject      = nsc_nkey.operator.public_key
  issuer_seed  = nsc_nkey.operator.seed
  signing_keys = [nsc_nkey.signing_key.public_key]
}

# Account can be signed by either the main operator key or a signing key
resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.signing_key.seed  # Use signing key to sign
}
```

**`nsc_account`:**
```hcl
resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key  # REQUIRED - subject of JWT
  issuer_seed = nsc_nkey.operator.seed       # REQUIRED - issuer signs the JWT

  # Optional account signing keys (for signing user JWTs)
  signing_keys = [nsc_nkey.account_signing.public_key]

  # All existing attributes (imports, exports, limits, etc.)
}
```

**Example with account signing keys:**
```hcl
resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "account_signing_key" {
  type = "account"  # Account signing keys are also account-type keys
}

resource "nsc_account" "app" {
  name         = "AppAccount"
  subject      = nsc_nkey.account.public_key
  issuer_seed  = nsc_nkey.operator.seed
  signing_keys = [nsc_nkey.account_signing_key.public_key]
}

# User can be signed by either the main account key or a signing key
resource "nsc_user" "service" {
  name        = "ServiceUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account_signing_key.seed  # Use signing key to sign
}
```

**`nsc_user`:**
```hcl
resource "nsc_user" "service" {
  name        = "ServiceUser"
  subject     = nsc_nkey.user.public_key  # REQUIRED - subject of JWT
  issuer_seed = nsc_nkey.account.seed     # REQUIRED - issuer signs the JWT

  # All existing attributes (permissions, limits, etc.)
}
```

### 3. System Account - Now Explicit

System accounts become regular `nsc_account` resources:

```hcl
# Before (ADR-003): Embedded in operator
resource "nsc_operator" "main" {
  name                  = "MyOperator"
  create_system_account = true
  system_account_name   = "SYS"
}

# After (ADR-006): Explicit account
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "sys_account" {
  type = "account"
}

resource "nsc_account" "sys" {
  name        = "SYS"
  subject     = nsc_nkey.sys_account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_operator" "main" {
  name           = "MyOperator"
  subject        = nsc_nkey.operator.public_key
  issuer_seed    = nsc_nkey.operator.seed
  system_account = nsc_account.sys.public_key  # Reference, not embedded
}
```

**Benefits:**
- System accounts are first-class resources
- Can configure system account limits, permissions, etc.
- No special-case logic in operator resource
- Resolves the circular dependency that motivated ADR-003

## Resolving Circular Dependencies

The unified approach solves all circular dependency scenarios:

### Cross-Account Imports
```hcl
# Phase 1: Generate keys
resource "nsc_nkey" "service_a" { type = "account" }
resource "nsc_nkey" "service_b" { type = "account" }

# Phase 2: Create accounts with cross-references
resource "nsc_account" "service_a" {
  subject     = nsc_nkey.service_a.public_key
  issuer_seed = nsc_nkey.operator.seed

  import {
    account = nsc_nkey.service_b.public_key  # No circular dependency!
    subject = "responses.>"
    type    = "stream"
  }
}

resource "nsc_account" "service_b" {
  subject     = nsc_nkey.service_b.public_key
  issuer_seed = nsc_nkey.operator.seed

  import {
    account = nsc_nkey.service_a.public_key  # No circular dependency!
    subject = "requests.>"
    type    = "stream"
  }
}
```

### System Account (Previously Required ADR-003)
```hcl
resource "nsc_nkey" "operator" { type = "operator" }
resource "nsc_nkey" "sys" { type = "account" }

# These can now be created in any order
resource "nsc_account" "sys" {
  subject     = nsc_nkey.sys.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_operator" "main" {
  subject        = nsc_nkey.operator.public_key
  issuer_seed    = nsc_nkey.operator.seed
  system_account = nsc_account.sys.public_key
}
```

## Migration Strategy

This is a backwards-incompatible breaking change requiring:

1. Add `nsc_nkey` resource
2. Update JWT resources to require `subject` and `issuer_seed` parameters
3. Remove key generation from `nsc_operator`, `nsc_account`, `nsc_user`
4. Remove `create_system_account` from operator (use explicit account)
5. Remove `generate_signing_key` from operator (use explicit nkey)

Users must update their configurations to use the new two-resource pattern (nsc_nkey + JWT resource).

## Implementation Notes

### `nsc_nkey` Resource

```go
type NKeyResourceModel struct {
    ID        types.String `tfsdk:"id"`         // public key
    Type      types.String `tfsdk:"type"`       // required: operator, account, user
    PublicKey types.String `tfsdk:"public_key"` // computed: same as ID
    Seed      types.String `tfsdk:"seed"`       // computed, sensitive
}
```

**Type Validation:**
```go
var nkeyTypes = map[string]func() (nkeys.KeyPair, error){
    "operator": nkeys.CreateOperator,
    "account":  nkeys.CreateAccount,
    "user":     nkeys.CreateUser,
}
```

**Import:**
- ImportState receives seed as import ID
- Validates seed matches the declared `type` in configuration
- Example: `terraform import nsc_nkey.operator SOABC123...`

### Resource Schema Changes

**Before (generates key):**
```go
"seed": schema.StringAttribute{
    Computed:  true,
    Sensitive: true,
}
```

**After (accepts subject and issuer seed):**
```go
// Subject - the public key this JWT is about
"subject": schema.StringAttribute{
    Required: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
}

// Issuer seed - the private key used to sign this JWT
"issuer_seed": schema.StringAttribute{
    Required:  true,
    Sensitive: true,
    PlanModifiers: []planmodifier.String{
        stringplanmodifier.RequiresReplace(),
    },
}
```

## Advantages

1. **Consistent API**: All resources follow same pattern (keys separate from JWTs)
2. **Explicit control**: Users control key lifecycle independently
3. **Flexible**: Can generate keys externally or reference existing ones
4. **Solves all circular deps**: Keys available before JWT creation
5. **System accounts normalized**: No special-case embedded resources
6. **Extensible**: New key types (signing keys, curve keys) use same resource
7. **Testable**: Key generation separate from business logic
8. **Matches NATS concepts**: Keys and JWTs are separate in NATS architecture

## Disadvantages

1. **More verbose**: Simple cases require two resources instead of one
2. **Breaking change**: Requires migration from existing resources
3. **Learning curve**: Users must understand key/JWT separation
4. **State migration**: Existing state must be migrated carefully

## Comparison with ADR-005

| Aspect | ADR-005 | ADR-006 (This) |
|--------|---------|----------------|
| Account keys | `nsc_account_key` | `nsc_nkey` (type="account") |
| Operator keys | Not addressed | `nsc_nkey` (type="operator") |
| User keys | Not addressed | `nsc_nkey` (type="user") |
| Consistency | Only accounts split | All resources consistent |
| System account | Still embedded | Explicit resource |
| Extensibility | New resource per type | Single resource |

## Data Sources

### `nsc_creds` - Generate Credentials File

Creds files are derived outputs that combine JWT and seed. They should be data sources, not resource attributes:

```hcl
resource "nsc_user" "service" {
  name        = "ServiceUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed
}

data "nsc_creds" "service" {
  jwt  = nsc_user.service.jwt
  seed = nsc_nkey.user.seed
}

# Outputs:
# - creds (sensitive) - The full .creds file content
```

**Usage examples:**

```hcl
# Use in Kubernetes secret
resource "kubernetes_secret" "nats_creds" {
  metadata {
    name = "nats-credentials"
  }
  data = {
    "service.creds" = data.nsc_creds.service.creds
  }
}

# Write to local file
resource "local_file" "creds" {
  filename        = "${path.module}/service.creds"
  content         = data.nsc_creds.service.creds
  file_permission = "0600"
}
```

**Why a data source?**
- Creds files are pure functions of JWT + seed (no side effects)
- Read-only derived data fits data source semantics
- No state to manage beyond inputs
- Can be regenerated on demand
- Generic - works with any JWT + seed combination

## Open Questions

1. **Should we support `nsc_nkey` for signing keys?** Yes, use `type = "operator"` or `type = "account"` - they're just additional keys.

## Recommendation

Adopt this unified approach as a backwards-incompatible breaking change. This provides a clean, consistent API that solves all circular dependency issues while maintaining explicit control over key lifecycle.
