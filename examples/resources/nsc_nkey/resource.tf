# Generate an operator key
resource "nsc_nkey" "operator" {
  type = "operator"
}

# Generate an account key
resource "nsc_nkey" "account" {
  type = "account"
}

# Generate a user key
resource "nsc_nkey" "user" {
  type = "user"
}

# Generate a signing key for the operator
resource "nsc_nkey" "operator_signing" {
  type = "operator"
}

# Use the generated keys with JWT resources
resource "nsc_operator" "main" {
  name         = "MyOperator"
  subject      = nsc_nkey.operator.public_key
  issuer_seed  = nsc_nkey.operator.seed
  signing_keys = [nsc_nkey.operator_signing.public_key]
}

resource "nsc_account" "app" {
  name        = "AppAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed
}

resource "nsc_user" "service" {
  name        = "ServiceUser"
  subject     = nsc_nkey.user.public_key
  issuer_seed = nsc_nkey.account.seed
}
