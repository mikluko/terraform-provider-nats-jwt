# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

This is a Terraform provider for managing NATS JWT authentication tokens and related resources.

## Architecture Decision Records (ADRs)

This project uses ADRs to document important architectural decisions. They are located in `docs/adr/`.

### Working with ADRs

1. **Always check existing ADRs** before making significant design changes
2. **ADRs have status**: Proposed, Accepted, Superseded, Deprecated
3. **Never approve your own ADRs** - always present Proposed ADRs to the user for approval
4. **Create new ADRs** for significant design decisions:
   - Use sequential numbering (001, 002, 003...)
   - Follow the existing format (Status, Context, Decision, Rationale, Consequences)
   - Link to related ADRs when superseding or referencing
5. **Update status** when ADRs are superseded by new decisions
6. **Reference ADRs** in code comments when implementing their decisions

### Current ADRs

- ADR-003 (Active): System account is managed as part of operator resource to avoid circular dependencies
- ADR-001 & ADR-002: Superseded by ADR-003

### ADR Template

```markdown
# ADR-XXX: Title

## Status
Proposed / Accepted / Superseded / Deprecated

## Context
What is the issue that we're seeing that is motivating this decision?

## Decision
What is the change that we're proposing and/or doing?

## Rationale
Why is this the right decision?

## Consequences
What becomes easier or more difficult because of this change?
```

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

## NATS JWT Security Model

### Authentication Hierarchy
- **Operator**: Root of trust, signs account JWTs
- **Account**: Tenant/namespace, signs user JWTs
- **User**: Individual authentication credentials

### Key Security Concepts
- Uses NKeys (Ed25519-based public-key signature system)
- JWT tokens are digitally signed by private keys
- Supports decentralized authentication and authorization
- Each level can have signing keys for easier key rotation

### System Account
- Special account with elevated privileges for monitoring and administration
- REQUIRED in operator mode for system operations
- Must be explicitly created and configured in server config
- Used for:
  - Server monitoring ($SYS subjects)
  - Account management operations
  - System-level administration

### Resolver Configuration
- Memory resolver: Preload JWTs in server config (good for small deployments)
- File resolver: Store JWTs on disk (better for larger deployments)
- Provider currently targets memory resolver with preloaded accounts

### Best Practices
- Always use signing keys for account and user token management
- Implement granular permissions at account and user levels
- System account should have unrestricted permissions
- Regular accounts should have appropriate limits and restrictions
- Use allow/deny lists for fine-grained subject-based authorization

### Current Provider Limitations
- No support for account imports/exports (cross-account communication)
- No support for account limits (connection, payload, etc.)
- No support for signing key rotation
- No support for JWT revocation
- Basic permission model only (allow/deny pub/sub)

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