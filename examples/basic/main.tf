terraform {
  required_providers {
    natsjwt = {
      source = "registry.terraform.io/mikluko/natsjwt"
    }
  }
}

provider "natsjwt" {
  # Provider configuration for JWT management
  # Could optionally include paths for keystore, config dir, etc.
}

# Create an operator - the root of the JWT hierarchy
resource "natsjwt_operator" "main" {
  name = "MyOperator"

  # Optional: generate a signing key with the operator
  generate_signing_key = true

  # Optional: JWT validity
  expiry = "8760h"  # 1 year
  start  = "0"      # valid immediately
}

# Create an account under the operator
resource "natsjwt_account" "app" {
  name          = "ApplicationAccount"
  operator_seed = natsjwt_operator.main.seed

  # Default permissions for all users in this account
  allow_pub    = ["app.>"]
  allow_sub    = ["app.>", "metrics.>"]
  deny_pub     = ["app.admin.>"]
  deny_sub     = ["app.secrets.>"]

  # Allow publishing to reply subjects (for services)
  allow_pub_response = 1
  response_ttl       = "5s"

  # JWT validity
  expiry = "8760h"  # 1 year
  start  = "0"      # valid immediately
}

# Create a user under the account
resource "natsjwt_user" "service" {
  name         = "backend-service"
  account_seed = natsjwt_account.app.seed

  # User-specific permissions (overrides account defaults)
  allow_pub    = ["app.events.>", "app.requests.>"]
  allow_sub    = ["app.>", "metrics.>"]
  deny_pub     = ["app.admin.>"]
  deny_sub     = ["app.secrets.>"]

  # Allow this user to publish responses
  allow_pub_response = 5  # Can publish up to 5 responses
  response_ttl       = "10s"

  # Optional: tags for organizational purposes
  tag = ["backend", "service"]

  # Optional: restrict connections by source network
  source_network = ["192.168.1.0/24"]

  # JWT validity
  expiry = "720h"  # 30 days
  start  = "0"     # valid immediately

  # Optional: bearer token (no connect challenge required)
  bearer = false
}

# Output the generated JWTs and keys
output "operator_jwt" {
  value     = natsjwt_operator.main.jwt
  sensitive = true
}

output "operator_public_key" {
  value = natsjwt_operator.main.public_key
}

output "operator_signing_key" {
  value     = natsjwt_operator.main.signing_key
  sensitive = true
}

output "account_jwt" {
  value     = natsjwt_account.app.jwt
  sensitive = true
}

output "account_public_key" {
  value = natsjwt_account.app.public_key
}

output "user_jwt" {
  value     = natsjwt_user.service.jwt
  sensitive = true
}

output "user_seed" {
  value     = natsjwt_user.service.seed
  sensitive = true
}

output "user_creds" {
  value       = natsjwt_user.service.creds
  sensitive   = true
  description = "Credentials string containing JWT and seed for NATS client connection"
}