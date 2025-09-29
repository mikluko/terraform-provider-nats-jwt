terraform {
  required_providers {
    natsjwt = {
      source  = "mikluko/nats-jwt"
      version = "~> 0.1"
    }
  }
}

provider "natsjwt" {
  # This provider has no configuration options
  # All JWT tokens and keys are managed through resources
}