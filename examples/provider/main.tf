terraform {
  required_providers {
    nsc = {
      source  = "mikluko/nsc"
      version = "~> 0.1"
    }
  }
}

provider "nsc" {
  # This provider has no configuration options
  # All JWT tokens and keys are managed through resources
}
