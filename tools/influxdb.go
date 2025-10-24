package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	mcpgrafana "github.com/grafana/mcp-grafana"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const (
	// Default query timeout for InfluxDB queries
	DefaultInfluxDBQueryTimeout = "30s"
)

// InfluxDBClient represents a client for querying InfluxDB through Grafana's datasource proxy
type InfluxDBClient struct {
	httpClient *http.Client
	baseURL    string
}

// InfluxDBSeries represents a time series data structure returned by InfluxDB
type InfluxDBSeries struct {
	Name    string            `json:"name"`
	Tags    map[string]string `json:"tags,omitempty"`
	Columns []string          `json:"columns"`
	Values  [][]interface{}   `json:"values"`
}

// InfluxDBResult represents a result set from InfluxDB
type InfluxDBResult struct {
	StatementID int              `json:"statement_id,omitempty"`
	Series      []InfluxDBSeries `json:"series,omitempty"`
	Messages    []interface{}    `json:"messages,omitempty"`
	Partial     bool             `json:"partial,omitempty"`
	Error       string           `json:"error,omitempty"`
}

// InfluxDBResponse represents the complete response from InfluxDB
type InfluxDBResponse struct {
	Results []InfluxDBResult `json:"results"`
	Error   string           `json:"error,omitempty"`
}

// FluxTable represents a Flux table result
type FluxTable struct {
	Columns []FluxColumn `json:"columns"`
	Records []FluxRecord `json:"records"`
}

// FluxColumn represents a Flux column definition
type FluxColumn struct {
	Label    string `json:"label"`
	DataType string `json:"dataType"`
	Group    bool   `json:"group,omitempty"`
	Default  string `json:"default,omitempty"`
}

// FluxRecord represents a single record in a Flux table
type FluxRecord map[string]interface{}

// newInfluxDBClient creates a new InfluxDB client for the given datasource UID
func newInfluxDBClient(ctx context.Context, uid string) (*InfluxDBClient, error) {
	// First check if the datasource exists
	_, err := getDatasourceByUID(ctx, GetDatasourceByUIDParams{UID: uid})
	if err != nil {
		return nil, err
	}

	cfg := mcpgrafana.GrafanaConfigFromContext(ctx)
	url := fmt.Sprintf("%s/api/datasources/proxy/uid/%s", strings.TrimRight(cfg.URL, "/"), uid)

	// Create custom transport with TLS configuration if available
	var transport = http.DefaultTransport
	if tlsConfig := cfg.TLSConfig; tlsConfig != nil {
		var err error
		transport, err = tlsConfig.HTTPTransport(transport.(*http.Transport))
		if err != nil {
			return nil, fmt.Errorf("failed to create custom transport: %w", err)
		}
	}

	authTransport := &influxDBAuthRoundTripper{
		accessToken: cfg.AccessToken,
		idToken:     cfg.IDToken,
		apiKey:      cfg.APIKey,
		basicAuth:   cfg.BasicAuth,
		underlying:  transport,
	}

	client := &http.Client{
		Transport: mcpgrafana.NewUserAgentTransport(authTransport),
	}

	return &InfluxDBClient{
		httpClient: client,
		baseURL:    url,
	}, nil
}

// influxDBAuthRoundTripper handles authentication for InfluxDB requests
type influxDBAuthRoundTripper struct {
	accessToken string
	idToken     string
	apiKey      string
	basicAuth   *url.Userinfo
	underlying  http.RoundTripper
}

func (rt *influxDBAuthRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if rt.accessToken != "" && rt.idToken != "" {
		req.Header.Set("X-Access-Token", rt.accessToken)
		req.Header.Set("X-Grafana-Id", rt.idToken)
	} else if rt.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+rt.apiKey)
	} else if rt.basicAuth != nil {
		password, _ := rt.basicAuth.Password()
		req.SetBasicAuth(rt.basicAuth.Username(), password)
	}

	resp, err := rt.underlying.RoundTrip(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// buildURL constructs a full URL for an InfluxDB API endpoint
func (c *InfluxDBClient) buildURL(urlPath string) string {
	fullURL := c.baseURL
	if !strings.HasSuffix(fullURL, "/") && !strings.HasPrefix(urlPath, "/") {
		fullURL += "/"
	} else if strings.HasSuffix(fullURL, "/") && strings.HasPrefix(urlPath, "/") {
		// Remove the leading slash from urlPath to avoid double slash
		urlPath = strings.TrimPrefix(urlPath, "/")
	}
	return fullURL + urlPath
}

// makeRequest makes an HTTP request to the InfluxDB API and returns the response body
func (c *InfluxDBClient) makeRequest(ctx context.Context, method, urlPath string, params url.Values, body []byte) ([]byte, error) {
	fullURL := c.buildURL(urlPath)

	u, err := url.Parse(fullURL)
	if err != nil {
		return nil, fmt.Errorf("parsing URL: %w", err)
	}

	if params != nil {
		u.RawQuery = params.Encode()
	}

	var reqBody io.Reader
	if body != nil {
		reqBody = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reqBody)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	if method == "POST" && body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Check for non-200 status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("InfluxDB API returned status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	// Read the response body with a limit to prevent memory issues
	respBody := io.LimitReader(resp.Body, 1024*1024*48)
	bodyBytes, err := io.ReadAll(respBody)
	if err != nil {
		return nil, fmt.Errorf("reading response body: %w", err)
	}

	// Check if the response is empty
	if len(bodyBytes) == 0 {
		return nil, fmt.Errorf("empty response from InfluxDB API")
	}

	// Trim any whitespace that might cause JSON parsing issues
	return bytes.TrimSpace(bodyBytes), nil
}

// QueryInfluxQLParams defines the parameters for executing InfluxQL queries
type QueryInfluxQLParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Query         string `json:"query" jsonschema:"required,description=The InfluxQL query to execute. This is the traditional SQL-like query language for InfluxDB 1.x. Example: 'SELECT mean(value) FROM measurement WHERE time > now() - 1h GROUP BY time(5m)'"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to query (optional\\, may be configured in the datasource)"`
	Epoch         string `json:"epoch,omitempty" jsonschema:"description=The time precision for timestamps in the response. Valid values: 'ns'\\, 'u'\\, 'ms'\\, 's'\\, 'm'\\, 'h'. If not specified\\, RFC3339 format is used."`
	ChunkedSize   int    `json:"chunkedSize,omitempty" jsonschema:"description=If specified\\, responses will be chunked by series or by time intervals of this size"`
}

// queryInfluxQL executes an InfluxQL query against an InfluxDB datasource
func queryInfluxQL(ctx context.Context, args QueryInfluxQLParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	params := url.Values{}
	params.Add("q", args.Query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	if args.Epoch != "" {
		params.Add("epoch", args.Epoch)
	}

	if args.ChunkedSize > 0 {
		params.Add("chunked", "true")
		params.Add("chunk_size", fmt.Sprintf("%d", args.ChunkedSize))
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling InfluxQL response (content: %s): %w", string(bodyBytes), err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// QueryInfluxQL is a tool for executing InfluxQL queries against InfluxDB
var QueryInfluxQL = mcpgrafana.MustTool(
	"query_influxdb_influxql",
	"Execute an InfluxQL query against an InfluxDB datasource. InfluxQL is the SQL-like query language for InfluxDB 1.x. Supports SELECT statements with WHERE clauses, GROUP BY, ORDER BY, and various functions like MEAN, COUNT, SUM, etc. Returns time series data with measurements, tags, fields, and timestamps.",
	queryInfluxQL,
	mcp.WithTitleAnnotation("Query InfluxDB with InfluxQL"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// QueryFluxParams defines the parameters for executing Flux queries
type QueryFluxParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Query         string `json:"query" jsonschema:"required,description=The Flux query to execute. Flux is the functional query language for InfluxDB 2.x that supports complex data transformations. Example: 'from(bucket: \\\"mybucket\\\") |> range(start: -1h) |> filter(fn: (r) => r._measurement == \\\"temperature\\\") |> mean()'"`
	Timeout       string `json:"timeout,omitempty" jsonschema:"description=Query timeout duration (e.g. '30s'\\, '5m'\\, '1h'). Defaults to 30s if not specified."`
}

// queryFlux executes a Flux query against an InfluxDB datasource
func queryFlux(ctx context.Context, args QueryFluxParams) ([]FluxTable, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	timeout := args.Timeout
	if timeout == "" {
		timeout = DefaultInfluxDBQueryTimeout
	}

	// Create the request body for Flux query
	requestBody := map[string]interface{}{
		"query":   args.Query,
		"timeout": timeout,
	}

	bodyBytes, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling Flux query request: %w", err)
	}

	respBytes, err := client.makeRequest(ctx, "POST", "/api/v2/query", nil, bodyBytes)
	if err != nil {
		return nil, err
	}

	// Flux queries can return CSV format or JSON format
	// Try to parse as JSON first, then fall back to CSV parsing if needed
	var tables []FluxTable

	// Check if the response starts with JSON
	if strings.HasPrefix(strings.TrimSpace(string(respBytes)), "{") || strings.HasPrefix(strings.TrimSpace(string(respBytes)), "[") {
		err = json.Unmarshal(respBytes, &tables)
		if err != nil {
			return nil, fmt.Errorf("unmarshalling Flux JSON response: %w", err)
		}
	} else {
		// Parse CSV format response (which is the default for Flux)
		tables, err = parseFluxCSV(string(respBytes))
		if err != nil {
			return nil, fmt.Errorf("parsing Flux CSV response: %w", err)
		}
	}

	return tables, nil
}

// parseFluxCSV parses Flux CSV response into structured data
func parseFluxCSV(csvData string) ([]FluxTable, error) {
	lines := strings.Split(strings.TrimSpace(csvData), "\n")
	if len(lines) == 0 {
		return []FluxTable{}, nil
	}

	var tables []FluxTable
	var currentTable *FluxTable
	var headerParsed bool

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check if this is an annotation line (starts with #)
		if strings.HasPrefix(line, "#") {
			if strings.Contains(line, "datatype") {
				// This is a datatype annotation line
				// Parse column datatypes
				parts := strings.Split(line[1:], ",") // Remove the # prefix
				if len(parts) > 0 && !headerParsed {
					currentTable = &FluxTable{
						Columns: make([]FluxColumn, len(parts)),
						Records: []FluxRecord{},
					}
					for i, dataType := range parts {
						currentTable.Columns[i] = FluxColumn{
							DataType: strings.TrimSpace(dataType),
						}
					}
				}
			} else if strings.Contains(line, "group") {
				// This is a group annotation line
				parts := strings.Split(line[1:], ",") // Remove the # prefix
				if currentTable != nil && len(parts) == len(currentTable.Columns) {
					for i, group := range parts {
						if i < len(currentTable.Columns) {
							currentTable.Columns[i].Group = strings.TrimSpace(group) == "true"
						}
					}
				}
			} else if strings.Contains(line, "default") {
				// This is a default annotation line
				parts := strings.Split(line[1:], ",") // Remove the # prefix
				if currentTable != nil && len(parts) == len(currentTable.Columns) {
					for i, defaultVal := range parts {
						if i < len(currentTable.Columns) {
							currentTable.Columns[i].Default = strings.TrimSpace(defaultVal)
						}
					}
				}
			}
			continue
		}

		// Parse header line (column names)
		if !headerParsed && currentTable != nil {
			parts := strings.Split(line, ",")
			if len(parts) == len(currentTable.Columns) {
				for i, label := range parts {
					currentTable.Columns[i].Label = strings.TrimSpace(label)
				}
				headerParsed = true
			}
			continue
		}

		// Parse data line
		if headerParsed && currentTable != nil {
			parts := strings.Split(line, ",")
			if len(parts) == len(currentTable.Columns) {
				record := make(FluxRecord)
				for i, value := range parts {
					columnLabel := currentTable.Columns[i].Label
					trimmedValue := strings.TrimSpace(value)

					// Convert value based on datatype
					switch currentTable.Columns[i].DataType {
					case "long":
						if trimmedValue != "" {
							record[columnLabel] = trimmedValue // Keep as string for now
						}
					case "double":
						if trimmedValue != "" {
							record[columnLabel] = trimmedValue // Keep as string for now
						}
					case "boolean":
						record[columnLabel] = trimmedValue == "true"
					case "dateTime:RFC3339":
						record[columnLabel] = trimmedValue
					default:
						record[columnLabel] = trimmedValue
					}
				}
				currentTable.Records = append(currentTable.Records, record)
			}
		}

		// Check for table separators (empty line usually indicates new table)
		if line == "" && currentTable != nil {
			tables = append(tables, *currentTable)
			currentTable = nil
			headerParsed = false
		}
	}

	// Add the last table if it exists
	if currentTable != nil {
		tables = append(tables, *currentTable)
	}

	return tables, nil
}

// QueryFlux is a tool for executing Flux queries against InfluxDB
var QueryFlux = mcpgrafana.MustTool(
	"query_influxdb_flux",
	"Execute a Flux query against an InfluxDB datasource. Flux is the functional query language for InfluxDB 2.x that enables complex data transformations, joins, and analysis. Supports operations like filtering, aggregation, windowing, and mathematical operations on time series data. Returns structured tabular data with typed columns.",
	queryFlux,
	mcp.WithTitleAnnotation("Query InfluxDB with Flux"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBDatabasesParams defines parameters for listing databases in InfluxDB 1.x
type ShowInfluxDBDatabasesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
}

// showInfluxDBDatabases lists available databases in InfluxDB 1.x
func showInfluxDBDatabases(ctx context.Context, args ShowInfluxDBDatabasesParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	params := url.Values{}
	params.Add("q", "SHOW DATABASES")

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBDatabases is a tool for listing databases in InfluxDB 1.x
var ShowInfluxDBDatabases = mcpgrafana.MustTool(
	"show_influxdb_databases",
	"List all available databases in an InfluxDB 1.x instance. This uses the 'SHOW DATABASES' InfluxQL command to retrieve database names. Only works with InfluxDB 1.x datasources.",
	showInfluxDBDatabases,
	mcp.WithTitleAnnotation("Show InfluxDB databases"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBMeasurementsParams defines parameters for listing measurements
type ShowInfluxDBMeasurementsParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to query measurements from (optional if configured in datasource)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Limit the number of measurements returned"`
	Offset        int    `json:"offset,omitempty" jsonschema:"description=Offset for pagination"`
}

// showInfluxDBMeasurements lists available measurements in InfluxDB 1.x
func showInfluxDBMeasurements(ctx context.Context, args ShowInfluxDBMeasurementsParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW MEASUREMENTS"

	if args.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", args.Limit)
	}

	if args.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", args.Offset)
	}

	params := url.Values{}
	params.Add("q", query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBMeasurements is a tool for listing measurements in InfluxDB 1.x
var ShowInfluxDBMeasurements = mcpgrafana.MustTool(
	"show_influxdb_measurements",
	"List all available measurements (similar to tables in SQL) in an InfluxDB 1.x database. This uses the 'SHOW MEASUREMENTS' InfluxQL command. Supports pagination with limit and offset parameters.",
	showInfluxDBMeasurements,
	mcp.WithTitleAnnotation("Show InfluxDB measurements"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBTagKeysParams defines parameters for listing tag keys
type ShowInfluxDBTagKeysParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to query tag keys from (optional if configured in datasource)"`
	Measurement   string `json:"measurement,omitempty" jsonschema:"description=The measurement name to filter tag keys by (optional)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Limit the number of tag keys returned"`
	Offset        int    `json:"offset,omitempty" jsonschema:"description=Offset for pagination"`
}

// showInfluxDBTagKeys lists available tag keys in InfluxDB 1.x
func showInfluxDBTagKeys(ctx context.Context, args ShowInfluxDBTagKeysParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW TAG KEYS"

	if args.Measurement != "" {
		query += fmt.Sprintf(" FROM %s", args.Measurement)
	}

	if args.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", args.Limit)
	}

	if args.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", args.Offset)
	}

	params := url.Values{}
	params.Add("q", query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBTagKeys is a tool for listing tag keys in InfluxDB 1.x
var ShowInfluxDBTagKeys = mcpgrafana.MustTool(
	"show_influxdb_tag_keys",
	"List all available tag keys (labels/dimensions) in an InfluxDB 1.x database. Tag keys are the names of indexed metadata fields. Optionally filter by measurement name and supports pagination.",
	showInfluxDBTagKeys,
	mcp.WithTitleAnnotation("Show InfluxDB tag keys"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBTagValuesParams defines parameters for listing tag values
type ShowInfluxDBTagValuesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	TagKey        string `json:"tagKey" jsonschema:"required,description=The tag key to retrieve values for"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to query tag values from (optional if configured in datasource)"`
	Measurement   string `json:"measurement,omitempty" jsonschema:"description=The measurement name to filter tag values by (optional)"`
	WhereClause   string `json:"whereClause,omitempty" jsonschema:"description=Additional WHERE clause to filter tag values (optional)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Limit the number of tag values returned"`
	Offset        int    `json:"offset,omitempty" jsonschema:"description=Offset for pagination"`
}

// showInfluxDBTagValues lists available tag values for a specific tag key in InfluxDB 1.x
func showInfluxDBTagValues(ctx context.Context, args ShowInfluxDBTagValuesParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := fmt.Sprintf("SHOW TAG VALUES WITH KEY = %s", args.TagKey)

	if args.Measurement != "" {
		query += fmt.Sprintf(" FROM %s", args.Measurement)
	}

	if args.WhereClause != "" {
		query += fmt.Sprintf(" WHERE %s", args.WhereClause)
	}

	if args.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", args.Limit)
	}

	if args.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", args.Offset)
	}

	params := url.Values{}
	params.Add("q", query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBTagValues is a tool for listing tag values in InfluxDB 1.x
var ShowInfluxDBTagValues = mcpgrafana.MustTool(
	"show_influxdb_tag_values",
	"List all available values for a specific tag key in an InfluxDB 1.x database. Tag values are the actual indexed metadata values. Supports filtering by measurement, additional WHERE clauses, and pagination.",
	showInfluxDBTagValues,
	mcp.WithTitleAnnotation("Show InfluxDB tag values"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBFieldKeysParams defines parameters for listing field keys
type ShowInfluxDBFieldKeysParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to query field keys from (optional if configured in datasource)"`
	Measurement   string `json:"measurement,omitempty" jsonschema:"description=The measurement name to filter field keys by (optional)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Limit the number of field keys returned"`
	Offset        int    `json:"offset,omitempty" jsonschema:"description=Offset for pagination"`
}

// showInfluxDBFieldKeys lists available field keys in InfluxDB 1.x
func showInfluxDBFieldKeys(ctx context.Context, args ShowInfluxDBFieldKeysParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW FIELD KEYS"

	if args.Measurement != "" {
		query += fmt.Sprintf(" FROM %s", args.Measurement)
	}

	if args.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", args.Limit)
	}

	if args.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", args.Offset)
	}

	params := url.Values{}
	params.Add("q", query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBFieldKeys is a tool for listing field keys in InfluxDB 1.x
var ShowInfluxDBFieldKeys = mcpgrafana.MustTool(
	"show_influxdb_field_keys",
	"List all available field keys (metrics/values) in an InfluxDB 1.x database. Field keys are the names of non-indexed data columns that store actual metric values and their data types. Optionally filter by measurement name and supports pagination.",
	showInfluxDBFieldKeys,
	mcp.WithTitleAnnotation("Show InfluxDB field keys"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBSeriesParams defines parameters for listing series
type ShowInfluxDBSeriesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to query series from (optional if configured in datasource)"`
	Measurement   string `json:"measurement,omitempty" jsonschema:"description=The measurement name to filter series by (optional)"`
	WhereClause   string `json:"whereClause,omitempty" jsonschema:"description=Additional WHERE clause to filter series (optional)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Limit the number of series returned"`
	Offset        int    `json:"offset,omitempty" jsonschema:"description=Offset for pagination"`
}

// showInfluxDBSeries lists available series in InfluxDB 1.x
func showInfluxDBSeries(ctx context.Context, args ShowInfluxDBSeriesParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW SERIES"

	if args.Measurement != "" {
		query += fmt.Sprintf(" FROM %s", args.Measurement)
	}

	if args.WhereClause != "" {
		query += fmt.Sprintf(" WHERE %s", args.WhereClause)
	}

	if args.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", args.Limit)
	}

	if args.Offset > 0 {
		query += fmt.Sprintf(" OFFSET %d", args.Offset)
	}

	params := url.Values{}
	params.Add("q", query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBSeries is a tool for listing series in InfluxDB 1.x
var ShowInfluxDBSeries = mcpgrafana.MustTool(
	"show_influxdb_series",
	"List all available series (unique combinations of measurement and tag sets) in an InfluxDB 1.x database. Series represent distinct time series with their measurement name and tag combinations. Supports filtering by measurement, WHERE clauses, and pagination.",
	showInfluxDBSeries,
	mcp.WithTitleAnnotation("Show InfluxDB series"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBRetentionPoliciesParams defines parameters for listing retention policies
type ShowInfluxDBRetentionPoliciesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database,omitempty" jsonschema:"description=The database name to show retention policies for (optional if configured in datasource)"`
}

// showInfluxDBRetentionPolicies lists available retention policies in InfluxDB 1.x
func showInfluxDBRetentionPolicies(ctx context.Context, args ShowInfluxDBRetentionPoliciesParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW RETENTION POLICIES"

	if args.Database != "" {
		query += fmt.Sprintf(" ON %s", args.Database)
	}

	params := url.Values{}
	params.Add("q", query)

	if args.Database != "" {
		params.Add("db", args.Database)
	}

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBRetentionPolicies is a tool for listing retention policies in InfluxDB 1.x
var ShowInfluxDBRetentionPolicies = mcpgrafana.MustTool(
	"show_influxdb_retention_policies",
	"List all retention policies for a database in InfluxDB 1.x. Retention policies define how long data is kept and the replication factor. Shows policy name, duration, shard group duration, replication factor, and whether it's the default policy.",
	showInfluxDBRetentionPolicies,
	mcp.WithTitleAnnotation("Show InfluxDB retention policies"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// CreateInfluxDBRetentionPolicyParams defines parameters for creating retention policies
type CreateInfluxDBRetentionPolicyParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name to create the retention policy in"`
	PolicyName    string `json:"policyName" jsonschema:"required,description=The name of the retention policy to create"`
	Duration      string `json:"duration" jsonschema:"required,description=How long data is kept (e.g. '30d'\\, '1w'\\, '1h'\\, 'INF' for infinite)"`
	Replication   int    `json:"replication,omitempty" jsonschema:"description=Replication factor (default: 1)"`
	ShardDuration string `json:"shardDuration,omitempty" jsonschema:"description=Shard group duration (optional\\, e.g. '1d'\\, '1w')"`
	SetAsDefault  bool   `json:"setAsDefault,omitempty" jsonschema:"description=Whether to set this as the default retention policy for the database"`
}

// createInfluxDBRetentionPolicy creates a new retention policy in InfluxDB 1.x
func createInfluxDBRetentionPolicy(ctx context.Context, args CreateInfluxDBRetentionPolicyParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	replication := args.Replication
	if replication == 0 {
		replication = 1
	}

	query := fmt.Sprintf("CREATE RETENTION POLICY %s ON %s DURATION %s REPLICATION %d",
		args.PolicyName, args.Database, args.Duration, replication)

	if args.ShardDuration != "" {
		query += fmt.Sprintf(" SHARD DURATION %s", args.ShardDuration)
	}

	if args.SetAsDefault {
		query += " DEFAULT"
	}

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// CreateInfluxDBRetentionPolicy is a tool for creating retention policies in InfluxDB 1.x
var CreateInfluxDBRetentionPolicy = mcpgrafana.MustTool(
	"create_influxdb_retention_policy",
	"Create a new retention policy in an InfluxDB 1.x database. Retention policies control data retention duration and replication. Use with caution in production environments. Duration examples: '30d' (30 days), '1w' (1 week), '1h' (1 hour), 'INF' (infinite).",
	createInfluxDBRetentionPolicy,
	mcp.WithTitleAnnotation("Create InfluxDB retention policy"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// AlterInfluxDBRetentionPolicyParams defines parameters for altering retention policies
type AlterInfluxDBRetentionPolicyParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name containing the retention policy"`
	PolicyName    string `json:"policyName" jsonschema:"required,description=The name of the retention policy to alter"`
	Duration      string `json:"duration,omitempty" jsonschema:"description=New duration for data retention (e.g. '30d'\\, '1w'\\, '1h'\\, 'INF')"`
	Replication   int    `json:"replication,omitempty" jsonschema:"description=New replication factor"`
	ShardDuration string `json:"shardDuration,omitempty" jsonschema:"description=New shard group duration (e.g. '1d'\\, '1w')"`
	SetAsDefault  bool   `json:"setAsDefault,omitempty" jsonschema:"description=Whether to set this as the default retention policy"`
}

// alterInfluxDBRetentionPolicy modifies an existing retention policy in InfluxDB 1.x
func alterInfluxDBRetentionPolicy(ctx context.Context, args AlterInfluxDBRetentionPolicyParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := fmt.Sprintf("ALTER RETENTION POLICY %s ON %s", args.PolicyName, args.Database)

	modifications := []string{}

	if args.Duration != "" {
		modifications = append(modifications, fmt.Sprintf("DURATION %s", args.Duration))
	}

	if args.Replication > 0 {
		modifications = append(modifications, fmt.Sprintf("REPLICATION %d", args.Replication))
	}

	if args.ShardDuration != "" {
		modifications = append(modifications, fmt.Sprintf("SHARD DURATION %s", args.ShardDuration))
	}

	if args.SetAsDefault {
		modifications = append(modifications, "DEFAULT")
	}

	if len(modifications) == 0 {
		return nil, fmt.Errorf("at least one modification must be specified (duration, replication, shardDuration, or setAsDefault)")
	}

	query += " " + strings.Join(modifications, " ")

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// AlterInfluxDBRetentionPolicy is a tool for modifying retention policies in InfluxDB 1.x
var AlterInfluxDBRetentionPolicy = mcpgrafana.MustTool(
	"alter_influxdb_retention_policy",
	"Modify an existing retention policy in an InfluxDB 1.x database. Can change duration, replication factor, shard duration, or set as default. Use with caution in production environments as this affects data retention behavior.",
	alterInfluxDBRetentionPolicy,
	mcp.WithTitleAnnotation("Alter InfluxDB retention policy"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// DropInfluxDBRetentionPolicyParams defines parameters for dropping retention policies
type DropInfluxDBRetentionPolicyParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name containing the retention policy"`
	PolicyName    string `json:"policyName" jsonschema:"required,description=The name of the retention policy to drop"`
}

// dropInfluxDBRetentionPolicy removes a retention policy from InfluxDB 1.x
func dropInfluxDBRetentionPolicy(ctx context.Context, args DropInfluxDBRetentionPolicyParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := fmt.Sprintf("DROP RETENTION POLICY %s ON %s", args.PolicyName, args.Database)

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// DropInfluxDBRetentionPolicy is a tool for removing retention policies in InfluxDB 1.x
var DropInfluxDBRetentionPolicy = mcpgrafana.MustTool(
	"drop_influxdb_retention_policy",
	"Remove a retention policy from an InfluxDB 1.x database. WARNING: This is a destructive operation that cannot be undone. Data associated with the retention policy may become inaccessible. Use with extreme caution in production environments.",
	dropInfluxDBRetentionPolicy,
	mcp.WithTitleAnnotation("Drop InfluxDB retention policy"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// DropInfluxDBSeriesParams defines parameters for dropping series
type DropInfluxDBSeriesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name to drop series from"`
	Measurement   string `json:"measurement,omitempty" jsonschema:"description=The measurement name to drop series from (optional)"`
	WhereClause   string `json:"whereClause,omitempty" jsonschema:"description=WHERE clause to filter which series to drop (optional but recommended for safety)"`
}

// dropInfluxDBSeries removes series from InfluxDB 1.x
func dropInfluxDBSeries(ctx context.Context, args DropInfluxDBSeriesParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "DROP SERIES"

	if args.Measurement != "" {
		query += fmt.Sprintf(" FROM %s", args.Measurement)
	}

	if args.WhereClause != "" {
		query += fmt.Sprintf(" WHERE %s", args.WhereClause)
	}

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// DropInfluxDBSeries is a tool for removing series from InfluxDB 1.x
var DropInfluxDBSeries = mcpgrafana.MustTool(
	"drop_influxdb_series",
	"Remove series (time series data) from an InfluxDB 1.x database. WARNING: This is a destructive operation that permanently deletes data and cannot be undone. Always use WHERE clauses to limit scope. Use 'show_influxdb_series' first to preview what will be deleted.",
	dropInfluxDBSeries,
	mcp.WithTitleAnnotation("Drop InfluxDB series"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// DropInfluxDBMeasurementParams defines parameters for dropping measurements
type DropInfluxDBMeasurementParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name to drop the measurement from"`
	Measurement   string `json:"measurement" jsonschema:"required,description=The measurement name to drop"`
}

// dropInfluxDBMeasurement removes an entire measurement from InfluxDB 1.x
func dropInfluxDBMeasurement(ctx context.Context, args DropInfluxDBMeasurementParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := fmt.Sprintf("DROP MEASUREMENT %s", args.Measurement)

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// DropInfluxDBMeasurement is a tool for removing measurements from InfluxDB 1.x
var DropInfluxDBMeasurement = mcpgrafana.MustTool(
	"drop_influxdb_measurement",
	"Remove an entire measurement (table equivalent) from an InfluxDB 1.x database. WARNING: This is a highly destructive operation that permanently deletes ALL data in the measurement and cannot be undone. Use with extreme caution in production environments.",
	dropInfluxDBMeasurement,
	mcp.WithTitleAnnotation("Drop InfluxDB measurement"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// DeleteInfluxDBDataParams defines parameters for deleting specific data points
type DeleteInfluxDBDataParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name to delete data from"`
	Measurement   string `json:"measurement,omitempty" jsonschema:"description=The measurement name to delete data from (optional)"`
	WhereClause   string `json:"whereClause" jsonschema:"required,description=WHERE clause to specify which data points to delete (REQUIRED for safety)"`
}

// deleteInfluxDBData removes specific data points from InfluxDB 1.x
func deleteInfluxDBData(ctx context.Context, args DeleteInfluxDBDataParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	if args.WhereClause == "" {
		return nil, fmt.Errorf("WHERE clause is required for DELETE operations for safety reasons")
	}

	query := "DELETE"

	if args.Measurement != "" {
		query += fmt.Sprintf(" FROM %s", args.Measurement)
	}

	query += fmt.Sprintf(" WHERE %s", args.WhereClause)

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// DeleteInfluxDBData is a tool for removing specific data points from InfluxDB 1.x
var DeleteInfluxDBData = mcpgrafana.MustTool(
	"delete_influxdb_data",
	"Delete specific data points from an InfluxDB 1.x database using WHERE conditions. WARNING: This permanently deletes data and cannot be undone. A WHERE clause is mandatory for safety. Use SELECT queries first to verify what data will be affected.",
	deleteInfluxDBData,
	mcp.WithTitleAnnotation("Delete InfluxDB data"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// ShowInfluxDBShardsParams defines parameters for listing shards
type ShowInfluxDBShardsParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
}

// showInfluxDBShards lists shard information in InfluxDB 1.x
func showInfluxDBShards(ctx context.Context, args ShowInfluxDBShardsParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW SHARDS"

	params := url.Values{}
	params.Add("q", query)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBShards is a tool for listing shards in InfluxDB 1.x
var ShowInfluxDBShards = mcpgrafana.MustTool(
	"show_influxdb_shards",
	"List all shards in an InfluxDB 1.x instance. Shards are the physical storage units for time series data, organized by database, retention policy, shard group, and time range. Shows shard ID, database, retention policy, shard group, start/end times, expiry time, and owners.",
	showInfluxDBShards,
	mcp.WithTitleAnnotation("Show InfluxDB shards"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBShardGroupsParams defines parameters for listing shard groups
type ShowInfluxDBShardGroupsParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
}

// showInfluxDBShardGroups lists shard group information in InfluxDB 1.x
func showInfluxDBShardGroups(ctx context.Context, args ShowInfluxDBShardGroupsParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW SHARD GROUPS"

	params := url.Values{}
	params.Add("q", query)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBShardGroups is a tool for listing shard groups in InfluxDB 1.x
var ShowInfluxDBShardGroups = mcpgrafana.MustTool(
	"show_influxdb_shard_groups",
	"List all shard groups in an InfluxDB 1.x instance. Shard groups are collections of shards that cover the same time range across different series. Shows shard group ID, database, retention policy, start/end times, expiry time, and associated shards.",
	showInfluxDBShardGroups,
	mcp.WithTitleAnnotation("Show InfluxDB shard groups"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// ShowInfluxDBContinuousQueriesParams defines parameters for listing continuous queries
type ShowInfluxDBContinuousQueriesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
}

// showInfluxDBContinuousQueries lists continuous queries in InfluxDB 1.x
func showInfluxDBContinuousQueries(ctx context.Context, args ShowInfluxDBContinuousQueriesParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := "SHOW CONTINUOUS QUERIES"

	params := url.Values{}
	params.Add("q", query)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// ShowInfluxDBContinuousQueries is a tool for listing continuous queries in InfluxDB 1.x
var ShowInfluxDBContinuousQueries = mcpgrafana.MustTool(
	"show_influxdb_continuous_queries",
	"List all continuous queries across all databases in an InfluxDB 1.x instance. Continuous queries are automatically executed queries that downsample data or create aggregations. Shows database name, continuous query name, and the full query definition.",
	showInfluxDBContinuousQueries,
	mcp.WithTitleAnnotation("Show InfluxDB continuous queries"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// CreateInfluxDBContinuousQueryParams defines parameters for creating continuous queries
type CreateInfluxDBContinuousQueryParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name to create the continuous query in"`
	QueryName     string `json:"queryName" jsonschema:"required,description=The name of the continuous query to create"`
	SelectQuery   string `json:"selectQuery" jsonschema:"required,description=The SELECT portion of the continuous query (e.g. 'SELECT mean(value) INTO average FROM measurement GROUP BY time(1h)')"`
}

// createInfluxDBContinuousQuery creates a new continuous query in InfluxDB 1.x
func createInfluxDBContinuousQuery(ctx context.Context, args CreateInfluxDBContinuousQueryParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := fmt.Sprintf("CREATE CONTINUOUS QUERY %s ON %s BEGIN %s END",
		args.QueryName, args.Database, args.SelectQuery)

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// CreateInfluxDBContinuousQuery is a tool for creating continuous queries in InfluxDB 1.x
var CreateInfluxDBContinuousQuery = mcpgrafana.MustTool(
	"create_influxdb_continuous_query",
	"Create a new continuous query in an InfluxDB 1.x database. Continuous queries automatically execute at regular intervals to downsample data or create aggregations. Use with caution as they consume system resources and run continuously. Example selectQuery: 'SELECT mean(temperature) INTO average_temp FROM weather GROUP BY time(1h)'",
	createInfluxDBContinuousQuery,
	mcp.WithTitleAnnotation("Create InfluxDB continuous query"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// DropInfluxDBContinuousQueryParams defines parameters for dropping continuous queries
type DropInfluxDBContinuousQueryParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the InfluxDB datasource to query"`
	Database      string `json:"database" jsonschema:"required,description=The database name containing the continuous query"`
	QueryName     string `json:"queryName" jsonschema:"required,description=The name of the continuous query to drop"`
}

// dropInfluxDBContinuousQuery removes a continuous query from InfluxDB 1.x
func dropInfluxDBContinuousQuery(ctx context.Context, args DropInfluxDBContinuousQueryParams) (*InfluxDBResponse, error) {
	client, err := newInfluxDBClient(ctx, args.DatasourceUID)
	if err != nil {
		return nil, fmt.Errorf("creating InfluxDB client: %w", err)
	}

	query := fmt.Sprintf("DROP CONTINUOUS QUERY %s ON %s", args.QueryName, args.Database)

	params := url.Values{}
	params.Add("q", query)
	params.Add("db", args.Database)

	bodyBytes, err := client.makeRequest(ctx, "GET", "/query", params, nil)
	if err != nil {
		return nil, err
	}

	var response InfluxDBResponse
	err = json.Unmarshal(bodyBytes, &response)
	if err != nil {
		return nil, fmt.Errorf("unmarshalling response: %w", err)
	}

	// Check for errors in the response
	if response.Error != "" {
		return nil, fmt.Errorf("InfluxDB query error: %s", response.Error)
	}

	for i, result := range response.Results {
		if result.Error != "" {
			return nil, fmt.Errorf("InfluxDB query error in result %d: %s", i, result.Error)
		}
	}

	return &response, nil
}

// DropInfluxDBContinuousQuery is a tool for removing continuous queries from InfluxDB 1.x
var DropInfluxDBContinuousQuery = mcpgrafana.MustTool(
	"drop_influxdb_continuous_query",
	"Remove a continuous query from an InfluxDB 1.x database. WARNING: This stops the automatic execution of the continuous query and cannot be undone. Any data previously generated by the continuous query will remain, but no new data will be generated.",
	dropInfluxDBContinuousQuery,
	mcp.WithTitleAnnotation("Drop InfluxDB continuous query"),
	mcp.WithIdempotentHintAnnotation(false),
	mcp.WithReadOnlyHintAnnotation(false),
)

// AddInfluxDBTools registers all InfluxDB tools with the MCP server
func AddInfluxDBTools(mcp *server.MCPServer) {
	// Query tools
	QueryInfluxQL.Register(mcp)
	QueryFlux.Register(mcp)

	// Schema introspection tools (read-only)
	ShowInfluxDBDatabases.Register(mcp)
	ShowInfluxDBMeasurements.Register(mcp)
	ShowInfluxDBTagKeys.Register(mcp)
	ShowInfluxDBTagValues.Register(mcp)
	ShowInfluxDBFieldKeys.Register(mcp)
	ShowInfluxDBSeries.Register(mcp)

	// Storage introspection tools (read-only)
	ShowInfluxDBShards.Register(mcp)
	ShowInfluxDBShardGroups.Register(mcp)

	// Retention policy tools
	ShowInfluxDBRetentionPolicies.Register(mcp)
	CreateInfluxDBRetentionPolicy.Register(mcp)
	AlterInfluxDBRetentionPolicy.Register(mcp)
	DropInfluxDBRetentionPolicy.Register(mcp)

	// Continuous query tools
	ShowInfluxDBContinuousQueries.Register(mcp)
	CreateInfluxDBContinuousQuery.Register(mcp)
	DropInfluxDBContinuousQuery.Register(mcp)

	// Data management tools (destructive operations)
	DropInfluxDBSeries.Register(mcp)
	DropInfluxDBMeasurement.Register(mcp)
	DeleteInfluxDBData.Register(mcp)
}
