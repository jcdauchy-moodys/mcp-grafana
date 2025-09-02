# Cursor Setup for Local MCP Server without JWT authentication

### Example for local MCP server

```json
{
  "mcpServers": {
    "local-grafana": {
      "command": "C:\\code\\grafana-mcp\\mcp-grafana.exe",
      "args": [],
      "env": {
        "GRAFANA_URL": "https://monitoring-cluster-brt-nprd.bankingcloud.moodysanalytics.net/grafana",
        "GRAFANA_API_KEY": "<The grafana API key>"
      }
    }
  }
}
```


# Cursor Setup for Remote MCP Server with JWT Authentication

## Option 2: Direct Configuration

Alternative `.cursor/mcp.json` with hardcoded values (less secure):

```json
{
  "mcpServers": {
    "remote-grafana": {
    "url": "https://monitoring-cluster-brt-nprd.bankingcloud.moodysanalytics.net/grafana-mcp/sse",
    "headers": {
      "Authorization": "Bearer <Your personal JWT Token>"
      }
    }
  }
}
```        
