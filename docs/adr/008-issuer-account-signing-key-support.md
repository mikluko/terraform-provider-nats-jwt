# ADR-008: IssuerAccount Field with Signing Keys

## Status

Accepted

## Context

### NATS JWT Signing Architecture

NATS supports signing keys to enhance security by allowing operators and accounts to delegate
signing authority without exposing primary keys. The JWT structure varies depending on whether the
primary key or a signing key is used:

**Operator signing Account JWT:**

- If signed with operator primary key P:
    - `iss` (issuer) = P (operator's subject/primary key)
    - No additional fields needed
- If signed with operator signing key S:
    - `iss` (issuer) = S (signing key's public key)
    - Validation: NATS checks if S is in operator's `signing_keys` list

**Account signing User JWT:**

- If signed with account primary key P:
    - `iss` (issuer) = P (account's subject/primary key)
    - `issuer_account` = P
- If signed with account signing key S:
    - `iss` (issuer) = S (signing key's public key)
    - `issuer_account` = P (account's subject/primary key - NOT the signing key!)

### The Problem

The current implementation in `resource_user.go` derives `IssuerAccount` from `issuer_seed`:

```text
Lines 257-300 (resource_user.go):
  accountSeedStr := config.IssuerSeed.ValueString()
  accountKP, err := nkeys.FromSeed([]byte(accountSeedStr))
  accountPubKey, err := accountKP.PublicKey()
  // ...
  userClaims.IssuerAccount = accountPubKey  // BUG!
```

This works correctly when `issuer_seed` is the account's primary key, but is **incorrect** when
using a signing key:

| Scenario    | issuer_seed          | Desired issuer_account (JWT) | Actual issuer_account | Correct? |
|-------------|----------------------|------------------------------|-----------------------|----------|
| Primary key | Account primary seed | Account primary key P        | P                     | ✅        |
| Signing key | Account signing seed | Account primary key P        | Signing key S         | ❌        |

### Why Account Resources Don't Have This Issue

The `nsc_account` resource does NOT have this problem because:

1. `AccountClaims` has no equivalent `IssuerOperator` field
2. The JWT `issuer` field is automatically set during `Encode()` to the signing key's public key
3. NATS validates by checking if the issuer is the operator's subject OR in its `signing_keys` list

The `issuer_account` JWT field (Go struct field: `UserClaims.IssuerAccount`) exists specifically to
maintain the link back to the account's primary identity when signing keys are used.

## Decision

Modify the `nsc_user` resource schema and implementation to support signing keys properly:

### Option 1: Add `issuer_account` Attribute (RECOMMENDED)

Add a new **optional** `issuer_account` attribute that explicitly specifies the account's primary
public key:

```hcl
resource "nsc_user" "service" {
  name           = "ServiceUser"
  subject        = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account_signing.seed  # Signing key
  issuer_account = nsc_nkey.account.public_key    # Account primary key
}
```

**Logic:**

- If `issuer_account` is provided: use it for the JWT's `issuer_account` field
- If `issuer_seed` derives an account key AND `issuer_account` is not provided: use derived key (
  backwards compatible)
- If `issuer_seed` is not an account key: `issuer_account` is REQUIRED

**Validation:**

```text
if issuer_account is provided:
    validate it starts with 'A'
    use it for userClaims.IssuerAccount
else:
    derive public key from issuer_seed
    if derived key starts with 'A':
        use it for userClaims.IssuerAccount (backwards compatible)
    else:
        error: "issuer_account required when issuer_seed is not an account key"
```

### Option 2: Relax Validation on `issuer_seed`

Allow `issuer_seed` to be ANY operator-compatible key, but always require the user to specify the
account:

```hcl
resource "nsc_user" "service" {
  subject        = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account_signing.seed  # Can be ANY account key
  issuer_account = nsc_nkey.account.public_key    # REQUIRED
}
```

This is more explicit but breaks backwards compatibility.

### Option 3: Auto-detect from Terraform Graph

Attempt to automatically determine the account's primary key by analyzing the resource dependency
graph. This is complex and fragile.

**Recommended: Option 1** - it's backwards compatible and explicit.

## Rationale

### Why Option 1 is Best

1. **Backwards Compatible**: Existing configurations continue to work without changes
2. **Explicit When Needed**: Only requires `issuer_account` when using signing keys
3. **Follows NATS Patterns**: Mirrors how `nsc` CLI handles signing keys
4. **Type Safe**: Validates that `issuer_account` is actually an account public key
5. **Future Proof**: Supports advanced signing key scenarios

### Signing Keys Use Case

Signing keys enable:

- **Key Rotation**: Rotate signing keys without regenerating all user JWTs
- **Separation of Concerns**: Store primary keys in secure vaults, use signing keys in CI/CD
- **Least Privilege**: Distribute signing keys to teams without exposing primary keys
- **Compromise Recovery**: Revoke compromised signing keys without changing account identity

Example with proper signing key support:

```hcl
# Account primary key (stored securely)
resource "nsc_nkey" "account" {
  type = "account"
}

# Account signing key (used in CI/CD)
resource "nsc_nkey" "account_signing" {
  type = "account"
}

# Account JWT includes signing key
resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
  signing_keys = [ nsc_nkey.account_signing.public_key ]
}

# User JWT signed with signing key
resource "nsc_user" "service" {
  name           = "ServiceUser"
  subject        = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account_signing.seed      # Signing key
  issuer_account = nsc_nkey.account.public_key        # Primary key
}
```

## Consequences

### Positive

1. **Correct JWT Structure**: User JWTs will have proper `IssuerAccount` field
2. **Enterprise Ready**: Supports production key management patterns
3. **Security**: Enables key rotation and separation strategies
4. **Backwards Compatible**: Existing configurations unaffected
5. **Documentation**: Forces users to be explicit about signing keys

### Negative

1. **Additional Attribute**: More complexity in the schema
2. **User Education**: Users need to understand signing key concept
3. **Migration Path**: Users with signing keys must update configs (but bug currently makes this
   unusable anyway)

### Implementation Changes

**Schema Changes:**

```go
// Add to UserResourceModel struct
IssuerAccount types.String `tfsdk:"issuer_account"`

// Add to schema
"issuer_account": schema.StringAttribute{
	Optional:            true,
	Computed:            true,
	MarkdownDescription: "Account public key when issuer_seed is a signing key. If not provided, derived from issuer_seed (must be an account key).",
	Validators: []validator.String{
		stringvalidator.RegexMatches(
			regexp.MustCompile(`^A[A-Z2-7]{55}$`),
			"must be a valid account public key starting with 'A'",
		),
	},
}
```

**Create/Update Logic:**

```go
// Determine IssuerAccount field for user JWT
var issuerAccount string
if !data.IssuerAccount.IsNull() {
	// User provided explicit issuer_account
	issuerAccount = data.IssuerAccount.ValueString()
	if !strings.HasPrefix(issuerAccount, "A") {
		resp.Diagnostics.AddError(
			"Invalid issuer_account",
			"issuer_account must be an account public key (starts with 'A')",
		)
		return
	}
} else {
	// Derive from issuer_seed (backwards compatible)
	issuerPubKey := derivePublicKey(issuerSeed)
	if strings.HasPrefix(issuerPubKey, "A") {
		issuerAccount = issuerPubKey
		data.IssuerAccount = types.StringValue(issuerAccount)
	} else {
		resp.Diagnostics.AddError(
			"Missing issuer_account",
			"issuer_account is required when issuer_seed is not an account key",
		)
		return
	}
}

userClaims.IssuerAccount = issuerAccount
```

**Documentation Updates:**

- Update `nsc_user` resource docs with `issuer_account` attribute
- Add example showing signing key usage
- Update migration guide if needed

### Breaking Changes

None - this is a backwards-compatible addition. Existing configurations where `issuer_seed` is the
account's primary key continue to work identically.

## Open Questions

1. Should we add similar `issuer_operator` to `nsc_account`?
    - **Answer**: No, not needed - AccountClaims doesn't have this field and NATS validation doesn't
      require it

2. Should we deprecate the auto-derivation behavior?
    - **Answer**: No, it's convenient for the common case and doesn't harm anything

3. Should this be in v0.13.0 or v1.0.0?
    - **Answer**: Can be in v0.13.0 as it's backwards compatible

## Related ADRs

- **ADR-006**: Unified NKey Resource - established the pattern of separating key generation from JWT
  signing
