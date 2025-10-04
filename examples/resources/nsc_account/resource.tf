# Generate keys
resource "nsc_nkey" "operator" {
  type = "operator"
}

resource "nsc_nkey" "account" {
  type = "account"
}

# Create operator
resource "nsc_operator" "main" {
  name        = "MyOperator"
  subject     = nsc_nkey.operator.public_key
  issuer_seed = nsc_nkey.operator.seed
}

# Basic account with permissions and limits
resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed

  # Default permissions for users in this account
  allow_pub = ["app.>", "_INBOX.>"]
  allow_sub = ["app.>", "_INBOX.>"]

  # Account limits
  max_connections = 1000
  max_payload     = 1048576 # 1MB max message size
}

# Access the account JWT
output "account_jwt" {
  value = nsc_account.app.jwt
}

# Account public key
output "account_public_key" {
  value = nsc_account.app.public_key
}
