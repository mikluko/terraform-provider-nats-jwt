# Generate keys
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

# Create operator and account
resource "nsc_operator" "main" {
  name        = "MyOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

# Standard user with two-factor authentication
resource "nsc_user" "service" {
  name        = "ServiceUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  # Permissions
  allow_pub = ["app.events.>", "app.requests.>"]
  allow_sub = ["app.responses.>", "_INBOX.>"]

  # Limits
  max_subscriptions = 100
  max_payload       = 1048576 # 1MB
}

# Access the JWT (safe to use in logs since bearer = false)
output "user_jwt" {
  value = nsc_user.service.jwt
}

# Or use jwt_sensitive if you prefer
output "user_jwt_sensitive" {
  value     = nsc_user.service.jwt_sensitive
  sensitive = true
}

# Generate credentials file for nats CLI
data "nsc_creds" "service" {
  jwt  = nsc_user.service.jwt
  seed = nsc_nkey.user.seed
}

output "user_creds" {
  value     = data.nsc_creds.service.creds
  sensitive = true
}
