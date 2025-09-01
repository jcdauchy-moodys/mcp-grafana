# Grafana MCP Server Tools

This document provides comprehensive documentation for all available tools in the Grafana MCP Server. The server provides access to your Grafana instance and the surrounding ecosystem.

## Tool Categories

The MCP server organizes tools into the following categories:

- [Search Tools](#search-tools) - Search for Grafana resources
- [Datasource Tools](#datasource-tools) - Manage and query datasources
- [Dashboard Tools](#dashboard-tools) - Manage dashboards and extract panel information
- [Prometheus Tools](#prometheus-tools) - Query Prometheus metrics and metadata
- [Loki Tools](#loki-tools) - Query Loki logs and retrieve log metadata
- [Alerting Tools](#alerting-tools) - Manage alert rules and contact points
- [Incident Tools](#incident-tools) - Manage incidents and activities
- [OnCall Tools](#oncall-tools) - Manage on-call schedules and users
- [Pyroscope Tools](#pyroscope-tools) - Query profiling data
- [Sift Tools](#sift-tools) - Perform automated investigations
- [Asserts Tools](#asserts-tools) - Query Asserts data
- [Admin Tools](#admin-tools) - Administrative operations
- [Navigation Tools](#navigation-tools) - Generate deeplinks to Grafana resources

---

## Search Tools

### search_dashboards

Search for Grafana dashboards by a query string. Returns a list of matching dashboards with details like title, UID, folder, tags, and URL.

**Parameters:**
- `query` (string): The query to search for

**Returns:** List of dashboard search results with metadata

---

## Datasource Tools

### list_datasources

List available Grafana datasources. Optionally filter by datasource type (e.g., 'prometheus', 'loki'). Returns a summary list including ID, UID, name, type, and default status.

**Parameters:**
- `type` (string, optional): The type of datasources to search for (e.g., 'prometheus', 'loki', 'tempo')

**Returns:** Array of datasource summaries

### get_datasource_by_uid

Retrieves detailed information about a specific datasource using its UID. Returns the full datasource model, including name, type, URL, access settings, JSON data, and secure JSON field status.

**Parameters:**
- `uid` (string, required): The UID of the datasource

**Returns:** Full datasource configuration

### get_datasource_by_name

Retrieves detailed information about a specific datasource using its name. Returns the full datasource model, including UID, type, URL, access settings, JSON data, and secure JSON field status.

**Parameters:**
- `name` (string, required): The name of the datasource

**Returns:** Full datasource configuration

---

## Dashboard Tools

### get_dashboard_by_uid

Retrieves the complete dashboard, including panels, variables, and settings, for a specific dashboard identified by its UID. **WARNING:** Large dashboards can consume significant context window space. Consider using `get_dashboard_summary` or `get_dashboard_property` instead.

**Parameters:**
- `uid` (string, required): The UID of the dashboard

**Returns:** Complete dashboard JSON

### get_dashboard_summary

Get a compact summary of a dashboard including title, panel count, panel types, variables, and other metadata without the full JSON. Use this for dashboard overview and planning modifications without consuming large context windows.

**Parameters:**
- `uid` (string, required): The UID of the dashboard

**Returns:** Dashboard summary with metadata

### get_dashboard_property

Get specific parts of a dashboard using JSONPath expressions to minimize context window usage. Common paths: '$.title' (title), '$.panels[*].title' (all panel titles), '$.panels[0]' (first panel), '$.templating.list' (variables), '$.tags' (tags), '$.panels[*].targets[*].expr' (all queries).

**Parameters:**
- `uid` (string, required): The UID of the dashboard
- `jsonPath` (string, required): JSONPath expression to extract specific data

**Returns:** Extracted dashboard data based on JSONPath

### get_dashboard_panel_queries

Use this tool to retrieve panel queries and information from a Grafana dashboard. Returns an array of objects, each representing a panel, with fields: title, query, and datasource (an object with uid and type).

**Parameters:**
- `uid` (string, required): The UID of the dashboard

**Returns:** Array of panel queries with datasource information

### update_dashboard

Create or update a dashboard using either full JSON or efficient patch operations. For new dashboards, provide the 'dashboard' field. For updating existing dashboards, use 'uid' + 'operations' for better context window efficiency. Supports complex JSONPaths and array operations.

**Parameters:**
- `dashboard` (object, optional): The full dashboard JSON for new dashboards
- `uid` (string, optional): UID of existing dashboard to update (required for patch operations)
- `operations` (array, optional): Array of patch operations for targeted updates
- `folderUid` (string, optional): The UID of the dashboard's folder
- `message` (string, optional): Commit message for version history
- `overwrite` (boolean, optional): Overwrite if exists
- `userId` (integer, optional): ID of the user making the change

**Returns:** Dashboard save response

---

## Prometheus Tools

### query_prometheus

Query Prometheus using a PromQL expression. Supports both instant queries (at a single point in time) and range queries (over a time range). Time can be specified in RFC3339 format or as relative expressions like 'now', 'now-1h', etc.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `expr` (string, required): The PromQL expression to query
- `startTime` (string, required): Start time (RFC3339 or relative)
- `endTime` (string, optional): End time (required for range queries)
- `stepSeconds` (integer, optional): Step size in seconds (required for range queries)
- `queryType` (string, optional): 'range' or 'instant' (defaults to 'range')

**Returns:** Prometheus query results

### list_prometheus_metric_names

List metric names in a Prometheus datasource. Retrieves all metric names and filters them using the provided regex. Supports pagination.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `regex` (string, optional): Regex to match against metric names
- `limit` (integer, optional): Maximum number of results
- `page` (integer, optional): Page number

**Returns:** Array of metric names

### list_prometheus_label_names

List label names in a Prometheus datasource. Allows filtering by series selectors and time range.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `matches` (array, optional): Label matchers to filter results
- `startRfc3339` (string, optional): Start time in RFC3339 format
- `endRfc3339` (string, optional): End time in RFC3339 format
- `limit` (integer, optional): Maximum number of results

**Returns:** Array of label names

### list_prometheus_label_values

Get the values for a specific label name in Prometheus. Allows filtering by series selectors and time range.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `labelName` (string, required): The name of the label to query
- `matches` (array, optional): Selectors to filter results
- `startRfc3339` (string, optional): Start time
- `endRfc3339` (string, optional): End time
- `limit` (integer, optional): Maximum number of results

**Returns:** Array of label values

### list_prometheus_metric_metadata

List Prometheus metric metadata. Returns metadata about metrics currently scraped from targets. **Note:** This endpoint is experimental.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `limit` (integer, optional): Maximum number of metrics to return
- `limitPerMetric` (integer, optional): Maximum number of entries per metric
- `metric` (string, optional): Specific metric to query

**Returns:** Metric metadata

---

## Loki Tools

### query_loki_logs

Executes a LogQL query against a Loki datasource to retrieve log entries or metric values. Returns results with timestamps, labels, and either log lines or numeric values. Supports full LogQL syntax.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `logql` (string, required): The LogQL query to execute
- `startRfc3339` (string, optional): Start time in RFC3339 format
- `endRfc3339` (string, optional): End time in RFC3339 format
- `limit` (integer, optional): Maximum number of log lines (default: 10, max: 100)
- `direction` (string, optional): 'forward' or 'backward' (default: 'backward')

**Returns:** Array of log entries with timestamps and labels

### query_loki_stats

Retrieves statistics about log streams matching a LogQL selector. Returns stream, chunk, entry, and byte counts. The LogQL parameter must be a simple label selector only.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `logql` (string, required): LogQL label selector (simple matchers only)
- `startRfc3339` (string, optional): Start time in RFC3339 format
- `endRfc3339` (string, optional): End time in RFC3339 format

**Returns:** Statistics object with counts

### list_loki_label_names

Lists all available label names (keys) found in logs within a Loki datasource and time range. Returns unique label strings.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `startRfc3339` (string, optional): Start time (defaults to 1 hour ago)
- `endRfc3339` (string, optional): End time (defaults to now)

**Returns:** Array of label names

### list_loki_label_values

Retrieves all unique values for a specific label name within a Loki datasource and time range. Useful for discovering filter options.

**Parameters:**
- `datasourceUid` (string, required): The UID of the datasource to query
- `labelName` (string, required): The label name to retrieve values for
- `startRfc3339` (string, optional): Start time (defaults to 1 hour ago)
- `endRfc3339` (string, optional): End time (defaults to now)

**Returns:** Array of label values

---

## Alerting Tools

### list_alert_rules

Lists Grafana alert rules, returning a summary including UID, title, current state (e.g., 'pending', 'firing', 'inactive'), and labels. Supports filtering and pagination. 'Inactive' state means normal, not firing.

**Parameters:**
- `limit` (integer, optional): Maximum number of results (default: 100)
- `page` (integer, optional): Page number
- `label_selectors` (array, optional): Label matchers to filter rules

**Returns:** Array of alert rule summaries

### get_alert_rule_by_uid

Retrieves the full configuration and detailed status of a specific alert rule by UID. Includes title, condition, query data, folder UID, rule group, state settings, evaluation interval, annotations, and labels.

**Parameters:**
- `uid` (string, required): The UID of the alert rule

**Returns:** Complete alert rule configuration

### list_contact_points

Lists Grafana notification contact points, returning UID, name, and type. Supports filtering by name (exact match) and limiting results.

**Parameters:**
- `limit` (integer, optional): Maximum number of results (default: 100)
- `name` (string, optional): Filter by contact point name

**Returns:** Array of contact point summaries

---

## Incident Tools

### list_incidents

List Grafana incidents. Allows filtering by status ('active', 'resolved') and optionally including drill incidents. Returns preview list with basic details.

**Parameters:**
- `limit` (integer, optional): Maximum number of incidents
- `drill` (boolean, optional): Whether to include drill incidents
- `status` (string, optional): Status filter ('active', 'resolved')

**Returns:** Array of incident previews

### get_incident

Get a single incident by ID. Returns full incident details including title, status, severity, labels, timestamps, and metadata.

**Parameters:**
- `id` (string, required): The ID of the incident

**Returns:** Complete incident details

### create_incident

Create a new Grafana incident. Requires title, severity, and room prefix. **This tool should be used judiciously and only after user confirmation, as it may notify or alarm many people.**

**Parameters:**
- `title` (string, required): The title of the incident
- `severity` (string, required): The severity level
- `roomPrefix` (string, required): The room prefix to create
- `isDrill` (boolean, optional): Whether this is a drill incident
- `status` (string, optional): Initial status
- `attachCaption` (string, optional): Caption for attachments
- `attachUrl` (string, optional): URL for attachments
- `labels` (array, optional): Labels to add

**Returns:** Created incident details

### add_activity_to_incident

Add a note (userNote activity) to an existing incident's timeline. The note body can include URLs which will be attached as context.

**Parameters:**
- `incidentId` (string, required): The ID of the incident
- `body` (string, required): The activity body (URLs will be parsed)
- `eventTime` (string, optional): When the activity occurred

**Returns:** Created activity item

---

## OnCall Tools

### list_oncall_schedules

List Grafana OnCall schedules, optionally filtering by team ID. If a specific schedule ID is provided, retrieves details for only that schedule. Returns schedule summaries including ID, name, team ID, timezone, and shift IDs.

**Parameters:**
- `teamId` (string, optional): Filter by team ID
- `scheduleId` (string, optional): Get specific schedule details
- `page` (integer, optional): Page number (1-based)

**Returns:** Array of schedule summaries

### get_oncall_shift

Get detailed information for a specific OnCall shift using its ID. A shift represents a designated time period within a schedule when users are actively on-call.

**Parameters:**
- `shiftId` (string, required): The ID of the shift

**Returns:** Complete shift details

### get_current_oncall_users

Get the list of users currently on-call for a specific schedule. Returns the schedule ID, name, and detailed user objects for those currently on call.

**Parameters:**
- `scheduleId` (string, required): The ID of the schedule

**Returns:** Schedule info with current on-call users

### list_oncall_teams

List teams configured in Grafana OnCall. Returns team objects with details. Supports pagination.

**Parameters:**
- `page` (integer, optional): Page number

**Returns:** Array of team objects

### list_oncall_users

List users from Grafana OnCall. Can retrieve all users, a specific user by ID, or filter by username. Supports pagination.

**Parameters:**
- `userId` (string, optional): Get specific user by ID
- `username` (string, optional): Filter by username
- `page` (integer, optional): Page number

**Returns:** Array of user objects

---

## Pyroscope Tools

### list_pyroscope_profile_types

Lists all available profile types in a Pyroscope datasource. Returns profile types in format: `<name>:<sample type>:<sample unit>:<period type>:<period unit>`. Not all types are available for every service.

**Parameters:**
- `data_source_uid` (string, required): The UID of the datasource
- `start_rfc_3339` (string, optional): Start time (defaults to 1 hour ago)
- `end_rfc_3339` (string, optional): End time (defaults to now)

**Returns:** Array of profile type strings

### list_pyroscope_label_names

Lists all available label names found in profiles within a Pyroscope datasource. Label matchers are typically used to qualify a service name. Returns unique label strings. Labels with double underscores are internal.

**Parameters:**
- `data_source_uid` (string, required): The UID of the datasource
- `matchers` (string, optional): Prometheus-style matchers (defaults to {})
- `start_rfc_3339` (string, optional): Start time (defaults to 1 hour ago)
- `end_rfc_3339` (string, optional): End time (defaults to now)

**Returns:** Array of label names

### list_pyroscope_label_values

Lists all available label values for a specific label name in profiles within a Pyroscope datasource. Label matchers can be used to filter results.

**Parameters:**
- `data_source_uid` (string, required): The UID of the datasource
- `name` (string, required): The label name to query
- `matchers` (string, optional): Prometheus-style matchers (defaults to {})
- `start_rfc_3339` (string, optional): Start time (defaults to 1 hour ago)
- `end_rfc_3339` (string, optional): End time (defaults to now)

**Returns:** Array of label values

### fetch_pyroscope_profile

Fetches a profile from a Pyroscope datasource. The profile type is required (use `list_pyroscope_profile_types` to see available types). Matchers are recommended to select applications. Returns profile in DOT format.

**Parameters:**
- `data_source_uid` (string, required): The UID of the datasource
- `profile_type` (string, required): The profile type to fetch
- `matchers` (string, optional): Prometheus-style matchers (defaults to {})
- `max_node_depth` (integer, optional): Max depth of nodes (default: 100, -1 for unbounded)
- `start_rfc_3339` (string, optional): Start time (defaults to 1 hour ago)
- `end_rfc_3339` (string, optional): End time (defaults to now)

**Returns:** Profile in DOT format

---

## Sift Tools

### list_sift_investigations

Retrieves a list of Sift investigations with an optional limit. If no limit is specified, defaults to 10 investigations.

**Parameters:**
- `limit` (integer, optional): Maximum number of investigations (default: 10)

**Returns:** Array of investigation objects

### get_sift_investigation

Retrieves an existing Sift investigation by its UUID. The ID should be provided as a string in UUID format.

**Parameters:**
- `id` (string, required): The UUID of the investigation

**Returns:** Complete investigation details

### get_sift_analysis

Retrieves a specific analysis from an investigation by its UUID. Both investigation ID and analysis ID should be provided as UUID strings.

**Parameters:**
- `investigationId` (string, required): The UUID of the investigation
- `analysisId` (string, required): The UUID of the analysis

**Returns:** Complete analysis details

### find_error_pattern_logs

Searches Loki logs for elevated error patterns compared to the last day's average, waits for the analysis to complete, and returns the results including any patterns found.

**Parameters:**
- `name` (string, required): The name of the investigation
- `labels` (object, required): Labels to scope the analysis
- `start` (string, optional): Start time (defaults to 30 minutes ago)
- `end` (string, optional): End time (defaults to now)

**Returns:** Analysis with error patterns found

### find_slow_requests

Searches relevant Tempo datasources for slow requests, waits for the analysis to complete, and returns the results.

**Parameters:**
- `name` (string, required): The name of the investigation
- `labels` (object, required): Labels to scope the analysis
- `start` (string, optional): Start time (defaults to 30 minutes ago)
- `end` (string, optional): End time (defaults to now)

**Returns:** Analysis with slow request results

---

## Asserts Tools

### get_assertions

Get assertion summary for a given entity with its type, name, env, site, namespace, and a time range.

**Parameters:**
- `startTime` (string, required): Start time in RFC3339 format
- `endTime` (string, required): End time in RFC3339 format
- `entityType` (string, optional): Entity type (e.g., Service, Node, Pod)
- `entityName` (string, optional): Entity name
- `env` (string, optional): Environment
- `site` (string, optional): Site
- `namespace` (string, optional): Namespace

**Returns:** Assertion summary data

---

## Admin Tools

### list_teams

Search for Grafana teams by a query string. Returns matching teams with details like name, ID, and URL.

**Parameters:**
- `query` (string, optional): Search query (can be empty to fetch all teams)

**Returns:** Array of team objects

### list_users_by_org

List users by organization. Returns users with details like userid, email, role, etc.

**Parameters:**
- `random_string` (string, required): Dummy parameter for no-parameter tools

**Returns:** Array of user objects

---

## Navigation Tools

### generate_deeplink

Generate deeplink URLs for Grafana resources. Supports dashboards (requires dashboardUid), panels (requires dashboardUid and panelId), and Explore queries (requires datasourceUid). Optionally accepts time range and additional query parameters.

**Parameters:**
- `resourceType` (string, required): Type of resource ('dashboard', 'panel', 'explore')
- `dashboardUid` (string, optional): Dashboard UID (required for dashboard and panel types)
- `datasourceUid` (string, optional): Datasource UID (required for explore type)
- `panelId` (integer, optional): Panel ID (required for panel type)
- `queryParams` (object, optional): Additional query parameters
- `timeRange` (object, optional): Time range with 'from' and 'to' fields

**Returns:** Generated deeplink URL

---

## Configuration

The MCP server supports enabling/disabling tool categories using command-line flags:

- `--enabled-tools`: Comma-separated list of enabled tool categories (default: all)
- `--disable-<category>`: Disable specific tool categories (e.g., `--disable-search`)

Available categories: search, datasource, incident, prometheus, loki, alerting, dashboard, oncall, asserts, sift, admin, pyroscope, navigation

## Authentication

The server supports multiple authentication methods:

- **API Key**: Set `GRAFANA_API_KEY` environment variable or `X-Grafana-API-Key` header
- **Basic Auth**: Set `GRAFANA_USERNAME` and `GRAFANA_PASSWORD` environment variables
- **On-behalf-of Auth**: For Grafana Cloud with access tokens and ID tokens
- **JWT**: Support for JWT-based authentication with configuration files

## Transport Modes

The server supports three transport modes:

1. **stdio**: Standard input/output (default)
2. **sse**: Server-Sent Events over HTTP
3. **streamable-http**: HTTP-based streaming transport

## Environment Variables

- `GRAFANA_URL`: Grafana instance URL (default: http://localhost:3000)
- `GRAFANA_API_KEY`: API key for authentication
- `GRAFANA_USERNAME`: Username for basic auth
- `GRAFANA_PASSWORD`: Password for basic auth

## TLS Configuration

The server supports custom TLS configuration:

- `--tls-cert-file`: Client certificate file
- `--tls-key-file`: Client private key file  
- `--tls-ca-file`: CA certificate file for server verification
- `--tls-skip-verify`: Skip TLS certificate verification (insecure)
