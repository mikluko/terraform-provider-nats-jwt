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

# Account with JetStream enabled
resource "nsc_account" "jetstream" {
  name        = "JetStreamAccount"
  subject     = nsc_nkey.account.public_key
  issuer_seed = nsc_nkey.operator.seed

  # Basic permissions
  allow_pub = ["app.>", "_INBOX.>"]
  allow_sub = ["app.>", "_INBOX.>"]

  # JetStream limits (setting these enables JetStream for this account)
  max_memory_storage = 1073741824  # 1GB memory storage
  max_disk_storage   = 10737418240 # 10GB disk storage
  max_streams        = 10          # Maximum 10 streams
  max_consumers      = 100         # Maximum 100 consumers

  # Optional: Additional JetStream stream limits
  max_ack_pending         = 1000  # Max unacknowledged messages per consumer
  max_memory_stream_bytes = -1    # Unlimited memory per stream
  max_disk_stream_bytes   = -1    # Unlimited disk per stream
  max_bytes_required      = false # Don't require max_bytes on streams
}
