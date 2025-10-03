# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Terraform provider for managing NATS JWT authentication tokens and related resources. Named after the [nsc](https://docs.nats.io/using-nats/nats-tools/nsc) (NATS Security and Configuration) CLI tool.

The provider generates JWT tokens for NATS operators, accounts, and users, with all keys and JWTs stored in Terraform state.

## Development Commands

### Build
Use GoReleaser for building binaries (never use `go build` directly):

```bash
# Build snapshot (development build)
goreleaser build --snapshot --clean --single-target

# Full release build
goreleaser release --snapshot --clean
```

### Test
```bash
# Unit tests
go test ./...

# Integration tests
go test ./internal/provider -v

# Run specific test
go test ./internal/provider -v -run TestIntegration_ProviderPush

# Run single test with timeout
go test ./internal/provider -v -count=1 -run TestAccUserResource_basic -timeout 30s
```

### Generate Test Operator Seed
```bash
# Using nkeys tool
go install github.com/nats-io/nkeys/nk@latest
nk -gen operator

# Using nsc
brew install nats-io/nats-tools/nsc
nsc generate nkey --operator

# Copy seed (starts with "SO") to .envrc
cp .envrc.example .envrc
# Edit .envrc and add: export NATSJWT_TEST_OPERATOR_SEED="SO..."
```

### Generate Documentation
```bash
go generate ./...
```

### Lint
```bash
golangci-lint run
```

### Release Process

Complete release workflow:

```bash
# 1. Regenerate documentation
go generate ./...

# 2. Run go fmt
go fmt ./...

# 3. Run full test suite
go test ./internal/provider -v -count=1 -timeout 60s

# 4. Commit any documentation updates
git add docs/
git commit -m "docs: update documentation"

# 5. Create release tag with changelog
# Use EffVer versioning: MACRO.MESO.MICRO
# - MACRO: Significant effort (major breaking changes)
# - MESO: Some effort (new features, minor breaking changes)
# - MICRO: No effort (bug fixes, backward-compatible)
git tag -a v0.x.y -m "Release message with changelog"

# 6. Push commits and tags
git push origin main
git push origin v0.x.y

# 7. Run GoReleaser (builds and publishes to GitHub)
goreleaser release --clean
```

## Architecture

### Resource Hierarchy
NATS JWT authentication follows a three-tier hierarchy:
- **Operator**: Root of trust, signs account JWTs
- **Account**: Tenant/namespace, signs user JWTs
- **User**: Individual authentication credentials

All use NKeys (Ed25519-based public-key signature system) for signing.

### Provider Structure
- `internal/provider/provider.go` - Main provider configuration (no config required)
- `internal/provider/resource_operator_nsc.go` - Operator resource
- `internal/provider/resource_account_nsc.go` - Account resource
- `internal/provider/resource_user_nsc.go` - User resource
- `internal/provider/provider_test.go` - Test utilities and provider tests
- `examples/` - Example Terraform configurations

### Key Architecture Decisions

#### System Account Management (ADR-003)
System accounts are **integrated into the operator resource** to avoid circular dependencies. Use `create_system_account = true` on the operator resource instead of creating a separate account resource.

This design:
- Prevents circular dependency issues (operator needs system account public key, system account needs operator seed)
- Matches `nsc` CLI behavior (`nsc add operator --sys`)
- Allows single-apply deployments

System account attributes are exposed as computed outputs:
- `system_account` - public key
- `system_account_jwt` - JWT token
- `system_account_seed` - private seed

#### State-Only Storage
The provider stores all keys and JWTs exclusively in Terraform state. There is no external API or storage backend. Resources implement Read/Update/Delete as state-only operations.

#### Seed Format Validation
Seeds have predictable prefixes:
- Operator seeds: `SO*`
- Account seeds: `SA*`
- User seeds: `SU*`

Public keys have matching prefixes:
- Operator keys: `O*`
- Account keys: `A*`
- User keys: `U*`

### Versioning

This project uses [EffVer (Intended Effort Versioning)](https://jacobtomlinson.dev/effver/):
- **MACRO**: Significant effort required (major breaking changes)
- **MESO**: Some effort required (minor breaking changes, larger features)
- **MICRO**: No effort required (small fixes, backward-compatible features)

## Architecture Decision Records

ADRs are located in `docs/adr/`. **Always check existing ADRs before making significant design changes.**

### Working with ADRs

1. Check existing ADRs to understand current design decisions
2. Create new ADRs for significant architectural decisions using sequential numbering
3. Never approve your own ADRs - always present Proposed ADRs to the user
4. Update status when ADRs are superseded
5. Reference ADRs in code comments when implementing their decisions

### Current ADRs
- **ADR-003** (Accepted): System account integrated into operator resource to resolve circular dependencies
- **ADR-004** (Accepted): Provider named "nsc" to match the NATS Security CLI tool
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

## NATS JWT Security Concepts

### Authentication Hierarchy
- **Operator**: Root of trust, contains system account reference, signs all account JWTs
- **Account**: Namespace/tenant, can have resource limits and default permissions, signs user JWTs
- **User**: Individual credentials, inherits account permissions, can have additional limits
- **System Account**: Special account with elevated privileges for monitoring and administration

### Signing Keys
- Optional signing keys enable key rotation without regenerating all JWTs
- Operator can have signing keys to sign account JWTs
- Accounts can have signing keys to sign user JWTs
- Currently implemented for operators (`generate_signing_key = true`)

### Permissions Model
- Allow/deny lists for publish and subscribe operations
- Subject-based authorization using NATS subject patterns
- Response permissions for request-reply patterns
- User permissions inherit from account defaults

### Resource Limits
Accounts support:
- Connection limits (max connections, leaf nodes)
- Data limits (max data, payload size, subscriptions)
- Import/export limits
- JetStream limits (storage, streams, consumers)

Users support:
- Connection limits (max subscriptions, max data, max payload)
- Connection type restrictions (STANDARD, WEBSOCKET, MQTT, etc.)

### Import Format
Resources support import using the format:
- Operator: `name/seed` or `name/seed/signing_key_seed`
- Account: `name/seed` or `name/seed/operator_seed`
- User: `name/seed` or `name/seed/account_seed`

Names containing `/` should be encoded as `//` or `%2F`.
