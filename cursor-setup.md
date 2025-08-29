# Cursor Setup for Remote MCP Server with JWT Authentication

## Option 1: Environment Variables (Recommended)

Users set these environment variables before starting Cursor:

### Windows (PowerShell)
```powershell
$env:JWT_TOKEN = "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
$env:USERNAME = "john.doe@company.com"
```

### Linux/macOS (Bash)
```bash
export JWT_TOKEN="eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9..."
export USERNAME="john.doe@company.com"
```

### .env file (per project)
Create a `.env` file in your project root:
```env
JWT_TOKEN=eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...
USERNAME=john.doe@company.com
```

## Option 2: Direct Configuration

Alternative `.cursor/mcp.json` with hardcoded values (less secure):

```json
{
  "mcpServers": {
    "remote-grafana": {
      "transport": {
        "type": "sse",
        "url": "https://mcp.yourdomain.com/mcp/sse",
        "headers": {
          "Authorization": "Bearer eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9...",
          "X-User": "john.doe@company.com"
        }
      }
    }
  }
}
```

## Option 3: Proxy/Wrapper Script

If Cursor doesn't support remote MCP servers directly, create a local wrapper:

### mcp-proxy.js (Node.js example)
```javascript
#!/usr/bin/env node

const WebSocket = require('ws');
const fetch = require('node-fetch');

const JWT_TOKEN = process.env.JWT_TOKEN;
const USERNAME = process.env.USERNAME;
const REMOTE_URL = 'https://mcp.yourdomain.com';

// Create a local proxy that forwards to remote server with authentication
const server = new WebSocket.Server({ port: 8080 });

server.on('connection', (ws) => {
  // Forward messages to remote server with JWT headers
  const remoteWs = new WebSocket(REMOTE_URL + '/mcp/sse', {
    headers: {
      'Authorization': `Bearer ${JWT_TOKEN}`,
      'X-User': USERNAME
    }
  });
  
  // Pipe messages between local and remote
  ws.on('message', (data) => remoteWs.send(data));
  remoteWs.on('message', (data) => ws.send(data));
});
```

Then configure Cursor to use the local proxy:
```json
{
  "mcpServers": {
    "grafana-proxy": {
      "command": "node",
      "args": ["mcp-proxy.js"],
      "env": {
        "JWT_TOKEN": "${JWT_TOKEN}",
        "USERNAME": "${USERNAME}"
      }
    }
  }
}
```

## JWT Token Structure Expected

Your JWT should include these claims for proper user identification:

```json
{
  "sub": "john.doe@company.com",           // Username (primary)
  "email": "john.doe@company.com",         // Email
  "uid": "12345",                          // User ID
  "roles": ["user", "developer"],         // Roles
  "exp": 1735689600,                       // Expiration
  "iat": 1735603200                        // Issued at
}
```

## Getting JWT Tokens

### From your authentication provider:
```bash
# Example with curl to get JWT from auth service
curl -X POST https://auth.yourdomain.com/token \
  -H "Content-Type: application/json" \
  -d '{
    "username": "john.doe@company.com",
    "password": "your_password"
  }'
```

### Generate test tokens:
```bash
# You can also create tokens programmatically for testing
go run examples/generate_jwt.go --user="john.doe@company.com"
```
