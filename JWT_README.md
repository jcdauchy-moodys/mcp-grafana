# JWT Authentication for Grafana MCP Server

This document describes the JWT authentication feature added to the Grafana MCP server.

## Overview

The JWT authentication feature provides token-based authentication using RSA256 signatures. When enabled, the server will validate JWT tokens from HTTP requests before processing them.

## Configuration

JWT authentication is configured through command-line flags:

- `--jwt-enabled`: Enable JWT token validation (default: false)
- `--jwt-rsa-key-file`: Path to RSA public key file for JWT validation (required when JWT is enabled)
- `--jwt-header`: HTTP header name to extract JWT from (default: "Authorization")  
- `--jwt-prefix`: Token prefix in the header (default: "Bearer ")

## Usage Examples

### Enable JWT validation with default settings:
```bash
./mcp-grafana --jwt-enabled --jwt-rsa-key-file=/path/to/public_key.pem --transport=sse
```

### Custom header configuration:
```bash
./mcp-grafana --jwt-enabled --jwt-rsa-key-file=/path/to/public_key.pem --jwt-header="X-Auth-Token" --jwt-prefix="" --transport=sse
```

## RSA Public Key Format

The JWT feature supports standard PEM-encoded RSA public keys in two formats:

1. PKCS#1 format (RSA PUBLIC KEY):
```
-----BEGIN RSA PUBLIC KEY-----
... key data ...
-----END RSA PUBLIC KEY-----
```

2. PKIX format (PUBLIC KEY):
```
-----BEGIN PUBLIC KEY-----
... key data ...
-----END PUBLIC KEY-----
```

## How It Works

1. When JWT validation is enabled, the server creates a JWT validator during startup
2. For HTTP-based transports (SSE and streamable-http), incoming requests are validated
3. Valid JWT tokens are stored in the request context and can be accessed by tools
4. Invalid tokens result in a warning log and error stored in context
5. The stdio transport does not perform JWT validation (no HTTP headers available)

## Integration Points

The JWT validation is integrated into the existing context chain with minimal impact:

- New `ComposedSSEContextFuncWithJWT()` and `ComposedHTTPContextFuncWithJWT()` functions
- JWT validation occurs before Grafana client creation
- Context functions: `JWTFromContext()` and `JWTErrorFromContext()` for accessing tokens/errors

## Security Notes

- Only RSA256 algorithm is supported for JWT signature validation
- Invalid tokens are logged but don't stop request processing (configurable behavior)
- JWT validation only applies to HTTP-based transports (SSE and streamable-http)
- The public key file is read once during server startup
