# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Terraform provider for managing NATS JWT authentication tokens and related resources.

## Development Commands

### Build
```bash
go build -o terraform-provider-natsjwt
```

### Test
```bash
go test ./...
go test -v ./...  # verbose output
go test ./internal/provider -run TestSpecificTest  # run specific test
```

### Lint
```bash
golangci-lint run
go fmt ./...
go vet ./...
```

### Terraform Provider Development
```bash
# Generate documentation
go generate ./...

# Install provider locally for testing
go install .

# Run acceptance tests (requires NATS server)
TF_ACC=1 go test ./... -v
```

## Architecture

### Provider Structure
- `internal/provider/` - Provider implementation and resources
- `internal/provider/provider.go` - Main provider configuration
- `internal/provider/*_resource.go` - Individual resource implementations
- `internal/provider/*_data_source.go` - Data source implementations
- `examples/` - Example Terraform configurations
- `docs/` - Generated documentation

### Key Components
- Provider will interact with NATS JWT library (`github.com/nats-io/jwt/v2`)
- Resources will manage JWT tokens for operators, accounts, and users
- Provider configuration will handle JWT signing keys and NATS server connections

## NATS JWT Specifics

- JWT tokens follow a hierarchy: Operator -> Account -> User
- Each level has its own signing key
- Tokens are signed using Ed25519 keys
- Resources should validate JWT claims and expiry times
- System accounts should be created explicitly, not implicitly via operator flags

## NSC Command Reference

### Operator Creation (`nsc create operator --help`)
```
Flags:
  --expiry string              valid until ('0' is always, '2M' is two months)
  --generate-signing-key       generate a signing key with the operator
  -n, --name string            operator name
  --start string               valid from ('0' is always, '3d' is three days)
  --sys                        [EXCLUDED - use explicit account creation instead]
```

### Account Creation (`nsc create account --help`)
```
Flags:
  --allow-pub strings          add publish default permissions
  --allow-pub-response int     default permissions to limit reply publishes
  --allow-pubsub strings       add publish and subscribe default permissions
  --allow-sub strings          add subscribe default permissions
  --deny-pub strings           add deny publish default permissions
  --deny-pubsub strings        add deny publish and subscribe default permissions
  --deny-sub strings           add deny subscribe default permissions
  --expiry string              valid until ('0' is always, '2M' is two months)
  -n, --name string            account name
  -k, --public-key string      public key identifying the account
  --response-ttl string        time limit for default permissions
  --start string               valid from ('0' is always, '3d' is three days)
```

### User Creation (`nsc create user --help`)
```
Flags:
  -a, --account string         account name
  --allow-pub strings          add publish permissions
  --allow-pub-response int     permissions to limit reply publishes
  --allow-pubsub strings       add publish and subscribe permissions
  --allow-sub strings          add subscribe permissions
  --bearer                     no connect challenge required for user
  --deny-pub strings           add deny publish permissions
  --deny-pubsub strings        add deny publish and subscribe permissions
  --deny-sub strings           add deny subscribe permissions
  --expiry string              valid until ('0' is always, '2M' is two months)
  -n, --name string            name to assign the user
  -k, --public-key string      public key identifying the user
  --response-ttl string        time limit for permissions
  --source-network strings     source network for connection
  --start string               valid from ('0' is always, '3d' is three days)
  --tag strings                tags for user
```