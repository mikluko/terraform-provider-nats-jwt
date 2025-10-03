# User with fixed expiry date (never changes)
resource "nsc_user" "fixed_expiry" {
  name        = "FixedExpiryUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  # Fixed deadline - JWT always expires at this specific time
  # This timestamp stays constant even when other attributes change
  expires_at = "2026-01-01T00:00:00Z"

  allow_pub = ["app.>"]
  allow_sub = ["app.>"]
}

output "fixed_expiry_jwt" {
  value = nsc_user.fixed_expiry.jwt
}

output "fixed_expiry_time" {
  value       = nsc_user.fixed_expiry.expires_at
  description = "Fixed expiry timestamp (never changes)"
}
