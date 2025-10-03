# Generate keys
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "bearer_user" {
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

# Bearer token user (JWT is the credential - no private key required)
resource "nsc_user" "api_client" {
  name        = "APIClient"
  subject     = nsc_nkey.bearer_user.public_key
  issuer_seed = nsc_nkey.account.seed

  # Enable bearer mode - JWT becomes a secret
  bearer = true

  # Permissions
  allow_pub = ["api.>"]
  allow_sub = ["api.responses.>"]

  # Limits
  max_subscriptions = 10
  max_payload       = 65536 # 64KB

  # Expiry (recommended for bearer tokens)
  expires_in = "720h" # 30 days
}

# IMPORTANT: Use jwt_sensitive for bearer tokens
# The jwt attribute will be null when bearer = true
output "bearer_token" {
  value       = nsc_user.api_client.jwt_sensitive
  sensitive   = true
  description = "Bearer token - treat as secret credential"
}

# Note: Bearer tokens don't need a seed for authentication
# The JWT alone is sufficient to connect
