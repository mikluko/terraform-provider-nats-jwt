# User with rolling 30-day expiry (recomputes on every change)
resource "nsc_user" "rolling_expiry" {
  name        = "RollingExpiryUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  # Rolling expiry - JWT expires 30 days from when it's issued
  # NOTE: This recomputes on EVERY resource change, issuing a new JWT
  # with a new 30-day expiry window
  expires_in = "720h" # 30 days

  allow_pub = ["app.>"]
  allow_sub = ["app.>"]
}

output "rolling_expiry_jwt" {
  value = nsc_user.rolling_expiry.jwt
}

output "rolling_expiry_time" {
  value       = nsc_user.rolling_expiry.expires_at
  description = "Actual expiry timestamp (changes on every update)"
}
