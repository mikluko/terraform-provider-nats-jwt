terraform {
  required_providers {
    nsc = {
      source = "mikluko/nsc"
    }
  }
}

provider "nsc" {}

# Generate keys
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "operator_signing" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

resource "nsc_nkey" "user" {
  type = "user"
}

# Create operator JWT
resource "nsc_operator" "main" {
  name        = "MyOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed

  # Optional: signing key for signing account JWTs
  signing_keys = [nsc_nkey.operator_signing.public_key]

  # Optional: JWT validity
  expiry = "8760h" # 1 year
  start  = "0s"    # valid immediately
}

# Create account JWT
resource "nsc_account" "app" {
  name        = "ApplicationAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed

  # Default permissions for all users in this account
  allow_pub = ["app.>"]
  allow_sub = ["app.>", "metrics.>"]
  deny_pub  = ["app.admin.>"]
  deny_sub  = ["app.secrets.>"]

  # Allow publishing to reply subjects (for services)
  allow_pub_response = 1
  response_ttl       = "5s"

  # JWT validity
  expiry = "8760h" # 1 year
  start  = "0s"    # valid immediately
}

# Create user JWT
resource "nsc_user" "service" {
  name        = "backend-service"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed

  # User-specific permissions (overrides account defaults)
  allow_pub = ["app.events.>", "app.requests.>"]
  allow_sub = ["app.>", "metrics.>"]
  deny_pub  = ["app.admin.>"]
  deny_sub  = ["app.secrets.>"]

  # Allow this user to publish responses
  allow_pub_response = 5 # Can publish up to 5 responses
  response_ttl       = "10s"

  # Optional: tags for organizational purposes
  tag = ["backend", "service"]

  # Optional: restrict connections by source network
  source_network = ["192.168.1.0/24"]

  # JWT validity
  expiry = "720h" # 30 days
  start  = "0s"   # valid immediately

  # Optional: bearer token (no connect challenge required)
  bearer = false
}

# Generate credentials file
data "nsc_creds" "service" {
  jwt  = nsc_user.service.jwt
  seed = nsc_nkey.user.seed
}

# Output the generated JWTs and keys
output "operator_jwt" {
  value     = nsc_operator.main.jwt
  sensitive = true
}

output "operator_public_key" {
  value = nsc_operator.main.public_key
}

output "operator_signing_keys" {
  value     = nsc_operator.main.signing_keys
  sensitive = true
}

output "account_jwt" {
  value     = nsc_account.app.jwt
  sensitive = true
}

output "account_public_key" {
  value = nsc_account.app.public_key
}

output "user_jwt" {
  value     = nsc_user.service.jwt
  sensitive = true
}

output "user_creds" {
  value       = data.nsc_creds.service.creds
  sensitive   = true
  description = "Credentials string containing JWT and seed for NATS client connection"
}
