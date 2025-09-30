# Complete NATS Server Setup with JWT Authentication

This example demonstrates a complete NATS server setup with JWT-based authentication using the `natsjwt` Terraform provider, automatically deploying a NATS server in a Docker container.

## Features

- **Operator** with JWT authentication and integrated system account
- **System Account** (SYS) created automatically with the operator
- **Application Account** with customizable permissions
- **Multiple Users**:
  - System admin user with full system account access
  - Application admin user with full application account access
  - Application user with limited permissions
- **Docker Container** automatically configured and started with the generated configuration

## Generated Files

After running `terraform apply`, the following files will be created:

- `conf/nats-server.conf` - NATS server configuration with JWT authentication
- `creds/app_user.creds` - Credentials for the limited application user
- `creds/app_admin.creds` - Credentials for the application admin
- `creds/sys_admin.creds` - Credentials for the system admin

## Prerequisites

- Docker must be installed and running
- Terraform must be installed
- NATS CLI tool (`nats`) for testing connections

## Usage

### 1. Initialize and Apply Terraform

```bash
terraform init
terraform apply
```

This will:
- Generate JWT tokens for operator, accounts, and users
- Create credential files for each user
- Generate NATS server configuration
- Pull the NATS Docker image
- Start a NATS server container with JWT authentication enabled

### 2. Verify Server is Running

The NATS server starts automatically in a Docker container. Check its status:

```bash
docker ps | grep nats-jwt-example
docker logs nats-jwt-example
```

### 3. Test Connections

Connect with different users using the NATS CLI:

```bash
# Subscribe as app_user (limited permissions)
nats --creds creds/app_user.creds --server nats://localhost:4222 sub 'app.>'

# Publish as app_admin (full permissions in app account)
nats --creds creds/app_admin.creds --server nats://localhost:4222 pub app.test "Hello World"

# Monitor everything as sys_admin
nats --creds creds/sys_admin.creds --server nats://localhost:4222 sub '>'
```

### 4. Test Permissions

Try operations that should be denied:

```bash
# This should fail - app_user cannot publish to app.admin.>
nats --creds creds/app_user.creds --server nats://localhost:4222 pub app.admin.config "test"

# This should fail - app_user cannot subscribe to app.admin.>
nats --creds creds/app_user.creds --server nats://localhost:4222 sub 'app.admin.>'
```

### 5. Monitor Server

Check server status via HTTP monitoring endpoint:

```bash
curl http://localhost:8222/varz
curl http://localhost:8222/connz
curl http://localhost:8222/accountz
```

## Account Structure

```
MyOperator (with integrated system account)
├── SYS (System Account - created with operator)
│   └── Users: sys_admin
└── Application (Application Account)
    ├── Permissions: allow/deny for pub/sub
    ├── Response permissions: configurable
    └── Users: app_admin, app_user
```

## Security Notes

- All credential files are created with restrictive permissions (0600)
- Seeds are included in credential files - keep them secure
- The server config uses memory resolver with preloaded accounts
- No external resolver service is needed

## Customization

You can modify the example to:

- Add more accounts
- Create service-to-service communication patterns
- Add JetStream configuration
- Implement more complex permission schemes
- Add account expiration times
- Configure connection limits and rate limiting

## Clean Up

To stop the server and remove all resources:

```bash
terraform destroy
```

This will:
- Stop and remove the Docker container
- Remove all generated credential files
- Remove the server configuration file

## Manual Server Start (without Docker)

If you prefer to run NATS server directly without Docker:

```bash
# Comment out or remove the docker resources in main.tf
# Then run:
terraform apply
nats-server -c conf/nats-server.conf
```