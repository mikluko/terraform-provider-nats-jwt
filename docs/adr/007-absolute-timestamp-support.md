# ADR-007: Absolute Timestamp Support for JWT Validity

## Status

Accepted

## Context

Currently, JWT resources (operator, account, user) use relative durations for expiry and start times:

```hcl
resource "nsc_user" "example" {
  expiry = "720h"  # 30 days from now
  start  = "0s"    # immediately
}
```

### The Problem: Time Drift

The provider computes absolute timestamps on every Create/Update operation:

```go
userClaims.Expires = time.Now().Add(duration).Unix()
userClaims.NotBefore = time.Now().Add(duration).Unix()
```

This causes **persistent drift**:

1. **Day 1**: Create user with `expiry = "720h"` → JWT expires at `2025-11-01 00:00:00`
2. **Day 2**: Update user permissions → JWT regenerated → expires at `2025-11-02 00:00:00`
3. **Day 3**: Run `terraform apply` with no changes → JWT regenerated → expires at `2025-11-03 00:00:00`

The JWT changes constantly even though the configuration hasn't changed, because `time.Now()` advances.

### Why This Matters

- **Unnecessary JWT rotation**: JWTs get new signatures on every apply
- **Client disruption**: Clients may cache JWTs, causing authentication issues
- **Audit trail noise**: Every apply shows a change even when nothing actually changed
- **Bearer token rotation**: Bearer tokens become new secrets on every apply

## Decision

Introduce new attributes supporting both relative and absolute timestamps, with the absolute value stored in state to prevent drift:

### New Schema

For all JWT resources (operator, account, user):

**Input attributes (mutually exclusive):**
- `expires_in` (Optional, duration) - Relative duration (e.g., `"720h"`)
- `expires_at` (Optional, RFC3339 timestamp) - Absolute timestamp (e.g., `"2025-11-01T00:00:00Z"`)
- `starts_in` (Optional, duration) - Relative duration for NotBefore
- `starts_at` (Optional, RFC3339 timestamp) - Absolute timestamp for NotBefore

**Computed outputs:**
- `expires_at` (Optional + Computed, RFC3339) - Actual expiry timestamp used in JWT
- `starts_at` (Optional + Computed, RFC3339) - Actual start timestamp used in JWT

Note: `expires_at` and `starts_at` are both Optional (user can provide) and Computed (provider computes from `_in` variants).

### Behavior

**On Create:**
1. If `expires_in` provided: compute `expires_at = now + duration`, store in state
2. If `expires_at` provided: use directly, store in state
3. Validate `expires_in` and `expires_at` are mutually exclusive
4. Same logic for `starts_in` / `starts_at`

**On Update:**
1. If `expires_in` provided: recompute `expires_at = now + duration` (rolling expiry)
2. If `expires_at` provided: use the provided value (fixed deadline)
3. Same logic for `starts_in` / `starts_at`

**Important: Rolling vs Fixed Expiry**

- **Rolling Expiry (`expires_in`)**: Timestamps recompute on **every** JWT regeneration (any resource change). This is acceptable because we're issuing a new JWT anyway. If users want stable expiry dates, they should use `expires_at`.

  Example: User with `expires_in = "720h"` updated on Day 1 → JWT expires on Day 31. Same user updated on Day 5 → JWT expires on Day 35 (new 30-day window).

- **Fixed Deadline (`expires_at`)**: Timestamp never changes, even when other attributes change.

  Example: User with `expires_at = "2026-01-01T00:00:00Z"` → JWT always expires on 2026-01-01, regardless of updates.

**Special cases:**
- `expires_in = "0s"` or `expires_at = null` → no expiry (JWT never expires)
- `starts_in = "0s"` or `starts_at = null` → immediate validity

### Migration Path

**Deprecate (but keep) existing attributes:**
- `expiry` → `expires_in` (same semantics, better naming)
- `start` → `starts_in` (same semantics, better naming)

Mark `expiry` and `start` as deprecated in documentation:

> **Deprecated**: Use `expires_in` and `expires_at` instead. The `expiry` attribute will be removed in v1.0.

**Validation logic:**
```go
// Mutually exclusive groups:
// Group 1: expiry, expires_in, expires_at (max one)
// Group 2: start, starts_in, starts_at (max one)
```

This allows gradual migration without breaking existing configurations.

## Rationale

### Why Both Relative and Absolute?

**Relative (`expires_in`):**
- **Common use case**: "I want tokens valid for 30 days"
- **Dynamic environments**: Token lifetime relative to deployment time
- **Simpler mental model**: "30 days from now" vs calculating exact date

**Absolute (`expires_at`):**
- **Fixed deadlines**: "Service access ends on 2025-12-31"
- **Compliance requirements**: "User access must expire on contract end date"
- **External coordination**: Aligning with other systems using absolute timestamps

### Why Store Computed Value?

Without storing `expires_at` in state:
```
Day 1: expiry = "720h" → Expires = 2025-11-01
Day 2: terraform plan → Expires = 2025-11-02 (DRIFT!)
```

With stored `expires_at`:
```
Day 1: expires_in = "720h" → expires_at = "2025-11-01T00:00:00Z" (stored)
Day 2: terraform plan → expires_at = "2025-11-01T00:00:00Z" (reused, no drift)
```

### Why Optional + Computed?

Making `expires_at` both Optional and Computed allows it to serve dual purposes:
1. **Input**: User can directly specify absolute timestamp
2. **Output**: Provider computes from `expires_in` when provided

This reduces API surface (no separate `expires_time` input attribute).

### Alternative Considered: Time Anchoring

Instead of new attributes, anchor `expiry`/`start` to a fixed point:

```go
// On Create
data.ExpiryAnchor = types.StringValue(time.Now().Format(time.RFC3339))

// On Update
anchor, _ := time.Parse(time.RFC3339, state.ExpiryAnchor.ValueString())
userClaims.Expires = anchor.Add(duration).Unix()
```

**Rejected because:**
- Confusing: User sees `expiry = "720h"` but actual expiry depends on hidden anchor
- Inflexible: Can't specify absolute timestamps
- Opaque: Computed value not visible in state/outputs

## Consequences

### Benefits

**Prevents drift:**
- JWTs only regenerate when configuration actually changes
- Predictable behavior for `terraform plan`

**Flexibility:**
- Relative durations for common cases
- Absolute timestamps for specific requirements

**Better UX:**
- Computed `expires_at` visible in outputs
- Users can see actual expiry timestamp

**Backward compatible:**
- Existing `expiry`/`start` still work (deprecated)
- Gradual migration path to v1.0

### Migration Impact

**For existing users:**
- No immediate action required
- Will see deprecation warnings in docs
- Can migrate at their own pace

**Migration example:**

Before:
```hcl
resource "nsc_user" "example" {
  expiry = "720h"
  start  = "0s"
}
```

After:
```hcl
resource "nsc_user" "example" {
  expires_in = "720h"  # or expires_at = "2025-11-01T00:00:00Z"
  starts_in  = "0s"    # or starts_at = null
}
```

### Implementation Complexity

**Medium complexity:**
- Add 4 new attributes per resource (12 total)
- Custom validation for mutual exclusivity
- State migration logic (reuse old values if present)
- Update documentation and examples

**Estimated effort:**
- Schema changes: 1-2 hours
- Logic implementation: 2-3 hours
- Testing: 2-3 hours
- Documentation: 1-2 hours
- **Total: 6-10 hours**

### Future Considerations

**v1.0 Cleanup:**
- Remove deprecated `expiry` and `start` attributes
- Simplify validation (only `_in` vs `_at`)

**Potential enhancements:**
- Add `renew_before` duration to trigger renewal before expiry
- Support cron-like schedules for periodic rotation
