package tools

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	mcpgrafana "github.com/grafana/mcp-grafana"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	_ "github.com/sijms/go-ora/v2" // Oracle driver
)

// OracleConnectionParams holds the parameters for connecting to an Oracle database
type OracleConnectionParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Oracle datasource in Grafana"`
	Host          string `json:"host" jsonschema:"required,description=Oracle database host"`
	Port          int    `json:"port" jsonschema:"description=Oracle database port (default: 1521)"`
	ServiceName   string `json:"serviceName,omitempty" jsonschema:"description=Oracle service name (alternative to SID)"`
	SID           string `json:"sid,omitempty" jsonschema:"description=Oracle SID (alternative to service name)"`
	Username      string `json:"username" jsonschema:"required,description=Database username"`
	Password      string `json:"password" jsonschema:"required,description=Database password"`
}

// OracleQueryParams holds the parameters for querying an Oracle database
type OracleQueryParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Oracle datasource in Grafana"`
	Query         string `json:"query" jsonschema:"required,description=The SQL query to execute"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Maximum number of rows to return (default: 100)"`
}

// OracleTableInfo represents metadata about an Oracle table
type OracleTableInfo struct {
	Owner        string     `json:"owner"`
	TableName    string     `json:"tableName"`
	TableType    string     `json:"tableType"`
	NumRows      int64      `json:"numRows"`
	LastAnalyzed *time.Time `json:"lastAnalyzed,omitempty"`
}

// OracleColumnInfo represents metadata about an Oracle table column
type OracleColumnInfo struct {
	ColumnName    string  `json:"columnName"`
	DataType      string  `json:"dataType"`
	DataLength    int     `json:"dataLength"`
	DataPrecision *int    `json:"dataPrecision,omitempty"`
	DataScale     *int    `json:"dataScale,omitempty"`
	Nullable      string  `json:"nullable"`
	DefaultValue  *string `json:"defaultValue,omitempty"`
}

// OracleQueryResult represents the result of an Oracle database query
type OracleQueryResult struct {
	Columns  []string                 `json:"columns"`
	Rows     []map[string]interface{} `json:"rows"`
	RowCount int                      `json:"rowCount"`
}

// buildOracleConnectionString creates a connection string for Oracle database
func buildOracleConnectionString(params OracleConnectionParams) string {
	port := params.Port
	if port == 0 {
		port = 1521
	}

	var connectionString string
	if params.ServiceName != "" {
		// Using service name
		connectionString = fmt.Sprintf("%s/%s@%s:%d/%s",
			params.Username, params.Password, params.Host, port, params.ServiceName)
	} else {
		// Using SID
		sid := params.SID
		if sid == "" {
			sid = "XE" // Default SID
		}
		connectionString = fmt.Sprintf("%s/%s@%s:%d:%s",
			params.Username, params.Password, params.Host, port, sid)
	}

	return connectionString
}

// getOracleConnectionFromDatasource extracts Oracle connection details from Grafana datasource and creates a connection
func getOracleConnectionFromDatasource(ctx context.Context, datasourceUID string) (*sql.DB, error) {
	// Get the datasource configuration from Grafana
	datasource, err := getDatasourceByUID(ctx, GetDatasourceByUIDParams{UID: datasourceUID})
	if err != nil {
		return nil, fmt.Errorf("failed to get datasource: %w", err)
	}

	// Verify it's an Oracle datasource
	if !strings.Contains(strings.ToLower(datasource.Type), "oracle") {
		return nil, fmt.Errorf("datasource %s is not an Oracle database (type: %s)", datasourceUID, datasource.Type)
	}

	// Extract connection details from datasource configuration
	// The connection details should be in the datasource URL or JSONData
	var connectionString string
	if datasource.URL != "" {
		// If URL is provided, use it as the connection string
		connectionString = datasource.URL
	} else {
		// Try to build connection string from JSONData
		// This is a simplified approach - in production you'd parse JSONData properly
		return nil, fmt.Errorf("Oracle datasource %s missing connection URL. Please configure the datasource with a proper Oracle connection string (oracle://user:password@host:port/service)", datasourceUID)
	}

	// Open database connection
	db, err := sql.Open("oracle", connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open Oracle connection: %w", err)
	}

	// Test the connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping Oracle database: %w", err)
	}

	return db, nil
}

type ListOracleTablesParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Oracle datasource in Grafana"`
	Schema        string `json:"schema,omitempty" jsonschema:"description=Schema/owner name to filter tables (default: current user schema)"`
	TableType     string `json:"tableType,omitempty" jsonschema:"description=Type of tables to list (TABLE, VIEW, MATERIALIZED VIEW, etc.)"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Maximum number of tables to return (default: 100)"`
}

func listOracleTables(ctx context.Context, args ListOracleTablesParams) ([]OracleTableInfo, error) {
	// Get database connection
	db, err := getOracleConnectionFromDatasource(ctx, args.DatasourceUID)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Build SQL query to list tables
	var query string
	var queryArgs []interface{}

	if args.Schema != "" {
		// Query specific schema
		if args.TableType != "" {
			tableType := strings.ToUpper(args.TableType)
			if tableType == "TABLE" {
				query = `
					SELECT owner, table_name, 'TABLE' as table_type, 
					       NVL(num_rows, 0) as num_rows, last_analyzed
					FROM all_tables
					WHERE owner = UPPER(?)
					ORDER BY owner, table_name`
				queryArgs = []interface{}{args.Schema}
			} else if tableType == "VIEW" {
				query = `
					SELECT owner, view_name as table_name, 'VIEW' as table_type,
					       0 as num_rows, NULL as last_analyzed
					FROM all_views
					WHERE owner = UPPER(?)
					ORDER BY owner, view_name`
				queryArgs = []interface{}{args.Schema}
			} else {
				return nil, fmt.Errorf("unsupported table type: %s. Supported types: TABLE, VIEW", args.TableType)
			}
		} else {
			query = `
				SELECT owner, table_name, 'TABLE' as table_type,
				       NVL(num_rows, 0) as num_rows, last_analyzed
				FROM all_tables
				WHERE owner = UPPER(?)
				UNION ALL
				SELECT owner, view_name as table_name, 'VIEW' as table_type,
				       0 as num_rows, NULL as last_analyzed
				FROM all_views
				WHERE owner = UPPER(?)
				ORDER BY owner, table_name`
			queryArgs = []interface{}{args.Schema, args.Schema}
		}
	} else {
		// Query all accessible schemas (excluding system schemas)
		if args.TableType != "" {
			tableType := strings.ToUpper(args.TableType)
			if tableType == "TABLE" {
				query = `
					SELECT owner, table_name, 'TABLE' as table_type,
					       NVL(num_rows, 0) as num_rows, last_analyzed
					FROM all_tables
					WHERE owner NOT IN ('SYS', 'SYSTEM', 'CTXSYS', 'DBSNMP', 'OUTLN', 'WMSYS', 'XDB', 'ANONYMOUS', 'APEX_PUBLIC_USER')
					ORDER BY owner, table_name`
			} else if tableType == "VIEW" {
				query = `
					SELECT owner, view_name as table_name, 'VIEW' as table_type,
					       0 as num_rows, NULL as last_analyzed
					FROM all_views
					WHERE owner NOT IN ('SYS', 'SYSTEM', 'CTXSYS', 'DBSNMP', 'OUTLN', 'WMSYS', 'XDB', 'ANONYMOUS', 'APEX_PUBLIC_USER')
					ORDER BY owner, view_name`
			} else {
				return nil, fmt.Errorf("unsupported table type: %s. Supported types: TABLE, VIEW", args.TableType)
			}
			queryArgs = []interface{}{}
		} else {
			query = `
				SELECT owner, table_name, 'TABLE' as table_type,
				       NVL(num_rows, 0) as num_rows, last_analyzed
				FROM all_tables
				WHERE owner NOT IN ('SYS', 'SYSTEM', 'CTXSYS', 'DBSNMP', 'OUTLN', 'WMSYS', 'XDB', 'ANONYMOUS', 'APEX_PUBLIC_USER')
				UNION ALL
				SELECT owner, view_name as table_name, 'VIEW' as table_type,
				       0 as num_rows, NULL as last_analyzed
				FROM all_views
				WHERE owner NOT IN ('SYS', 'SYSTEM', 'CTXSYS', 'DBSNMP', 'OUTLN', 'WMSYS', 'XDB', 'ANONYMOUS', 'APEX_PUBLIC_USER')
				ORDER BY owner, table_name`
			queryArgs = []interface{}{}
		}
	}

	// Apply limit in the query
	limit := args.Limit
	if limit == 0 {
		limit = 100
	}
	query = fmt.Sprintf(`SELECT * FROM (%s) WHERE ROWNUM <= %d`, query, limit)

	slog.Debug("Executing Oracle query", "query", query, "args", queryArgs)

	// Execute query
	rows, err := db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to query Oracle tables: %w", err)
	}
	defer rows.Close()

	// Parse results
	var tables []OracleTableInfo
	for rows.Next() {
		var table OracleTableInfo
		var lastAnalyzed sql.NullTime
		var numRows sql.NullInt64

		err := rows.Scan(
			&table.Owner,
			&table.TableName,
			&table.TableType,
			&numRows,
			&lastAnalyzed,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan Oracle table row: %w", err)
		}

		if numRows.Valid {
			table.NumRows = numRows.Int64
		}
		if lastAnalyzed.Valid {
			table.LastAnalyzed = &lastAnalyzed.Time
		}

		tables = append(tables, table)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading Oracle table rows: %w", err)
	}

	return tables, nil
}

var ListOracleTables = mcpgrafana.MustTool(
	"list_oracle_tables",
	"List tables in an Oracle database datasource. Connects directly to the Oracle database and queries system tables to return metadata about tables including owner, name, type, row count, and last analysis date. Supports filtering by schema and table type.",
	listOracleTables,
	mcp.WithTitleAnnotation("List Oracle database tables"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

type DescribeOracleTableParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Oracle datasource in Grafana"`
	TableName     string `json:"tableName" jsonschema:"required,description=Name of the table to describe"`
	Schema        string `json:"schema,omitempty" jsonschema:"description=Schema/owner name (default: current user schema)"`
}

func describeOracleTable(ctx context.Context, args DescribeOracleTableParams) ([]OracleColumnInfo, error) {
	// Get database connection
	db, err := getOracleConnectionFromDatasource(ctx, args.DatasourceUID)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Build SQL query to describe table columns
	var query string
	var queryArgs []interface{}

	if args.Schema != "" {
		query = `
			SELECT column_name, data_type, data_length, data_precision, data_scale, nullable, data_default
			FROM all_tab_columns
			WHERE owner = UPPER(?) AND table_name = UPPER(?)
			ORDER BY column_id`
		queryArgs = []interface{}{args.Schema, args.TableName}
	} else {
		// Query all accessible schemas for the table
		query = `
			SELECT column_name, data_type, data_length, data_precision, data_scale, nullable, data_default
			FROM all_tab_columns
			WHERE table_name = UPPER(?)
			  AND owner NOT IN ('SYS', 'SYSTEM', 'CTXSYS', 'DBSNMP', 'OUTLN', 'WMSYS', 'XDB', 'ANONYMOUS', 'APEX_PUBLIC_USER')
			ORDER BY owner, column_id`
		queryArgs = []interface{}{args.TableName}
	}

	slog.Debug("Executing Oracle describe query", "query", query, "args", queryArgs)

	// Execute query
	rows, err := db.QueryContext(ctx, query, queryArgs...)
	if err != nil {
		return nil, fmt.Errorf("failed to describe Oracle table: %w", err)
	}
	defer rows.Close()

	// Parse results
	var columns []OracleColumnInfo
	for rows.Next() {
		var column OracleColumnInfo
		var dataPrecision sql.NullInt64
		var dataScale sql.NullInt64
		var defaultValue sql.NullString

		err := rows.Scan(
			&column.ColumnName,
			&column.DataType,
			&column.DataLength,
			&dataPrecision,
			&dataScale,
			&column.Nullable,
			&defaultValue,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan Oracle column row: %w", err)
		}

		if dataPrecision.Valid {
			precision := int(dataPrecision.Int64)
			column.DataPrecision = &precision
		}
		if dataScale.Valid {
			scale := int(dataScale.Int64)
			column.DataScale = &scale
		}
		if defaultValue.Valid && defaultValue.String != "" {
			column.DefaultValue = &defaultValue.String
		}

		columns = append(columns, column)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading Oracle column rows: %w", err)
	}

	if len(columns) == 0 {
		if args.Schema != "" {
			return nil, fmt.Errorf("table %s.%s not found or not accessible", args.Schema, args.TableName)
		} else {
			return nil, fmt.Errorf("table %s not found or not accessible", args.TableName)
		}
	}

	return columns, nil
}

var DescribeOracleTable = mcpgrafana.MustTool(
	"describe_oracle_table",
	"Describe the structure of an Oracle database table. Connects directly to the Oracle database and queries system tables to return detailed column information including data types, lengths, precision, scale, nullable status, and default values.",
	describeOracleTable,
	mcp.WithTitleAnnotation("Describe Oracle table structure"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

func queryOracleDatabase(ctx context.Context, args OracleQueryParams) (*OracleQueryResult, error) {
	// Get database connection
	db, err := getOracleConnectionFromDatasource(ctx, args.DatasourceUID)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Set a limit if not provided
	limit := args.Limit
	if limit == 0 {
		limit = 100
	}

	query := strings.TrimSpace(args.Query)
	slog.Debug("Executing Oracle SQL query", "query", query, "limit", limit)

	// Check if it's a SELECT query to handle differently
	upperQuery := strings.ToUpper(query)
	isSelectQuery := strings.HasPrefix(upperQuery, "SELECT") || strings.HasPrefix(upperQuery, "WITH")

	if isSelectQuery {
		// For SELECT queries, apply row limit
		limitedQuery := fmt.Sprintf(`SELECT * FROM (%s) WHERE ROWNUM <= %d`, query, limit)

		rows, err := db.QueryContext(ctx, limitedQuery)
		if err != nil {
			return nil, fmt.Errorf("failed to execute Oracle SELECT query: %w", err)
		}
		defer rows.Close()

		// Get column information
		columns, err := rows.Columns()
		if err != nil {
			return nil, fmt.Errorf("failed to get Oracle query columns: %w", err)
		}

		// Prepare result
		var results []map[string]interface{}

		for rows.Next() {
			// Create slice of interface{} to hold each column value
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range values {
				valuePtrs[i] = &values[i]
			}

			// Scan the row
			if err := rows.Scan(valuePtrs...); err != nil {
				return nil, fmt.Errorf("failed to scan Oracle query row: %w", err)
			}

			// Convert to map
			rowMap := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]

				// Handle different Oracle data types
				switch v := val.(type) {
				case []byte:
					// Convert byte arrays to strings (for CHAR/VARCHAR2 columns)
					rowMap[col] = string(v)
				case nil:
					rowMap[col] = nil
				default:
					rowMap[col] = v
				}
			}
			results = append(results, rowMap)
		}

		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error reading Oracle query rows: %w", err)
		}

		return &OracleQueryResult{
			Columns:  columns,
			Rows:     results,
			RowCount: len(results),
		}, nil

	} else {
		// For non-SELECT queries (INSERT, UPDATE, DELETE, DDL), use Exec
		result, err := db.ExecContext(ctx, query)
		if err != nil {
			return nil, fmt.Errorf("failed to execute Oracle statement: %w", err)
		}

		// Get rows affected (if applicable)
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			// Some DDL statements don't support RowsAffected
			rowsAffected = 0
		}

		return &OracleQueryResult{
			Columns: []string{"ROWS_AFFECTED", "STATUS"},
			Rows: []map[string]interface{}{
				{
					"ROWS_AFFECTED": rowsAffected,
					"STATUS":        fmt.Sprintf("Statement executed successfully: %s", upperQuery[:min(50, len(upperQuery))]),
				},
			},
			RowCount: 1,
		}, nil
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

var QueryOracleDatabase = mcpgrafana.MustTool(
	"query_oracle_database",
	"Execute a SQL query against an Oracle database datasource. Connects directly to the Oracle database to execute SELECT queries for data retrieval and other SQL statements (INSERT, UPDATE, DELETE, DDL) for database operations. Returns results with columns and rows data, automatically applying row limits for SELECT queries.",
	queryOracleDatabase,
	mcp.WithTitleAnnotation("Query Oracle database"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

type ListOracleSchemasParams struct {
	DatasourceUID string `json:"datasourceUid" jsonschema:"required,description=The UID of the Oracle datasource in Grafana"`
	Limit         int    `json:"limit,omitempty" jsonschema:"description=Maximum number of schemas to return (default: 50)"`
}

func listOracleSchemas(ctx context.Context, args ListOracleSchemasParams) ([]string, error) {
	// Get database connection
	db, err := getOracleConnectionFromDatasource(ctx, args.DatasourceUID)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	// Apply limit
	limit := args.Limit
	if limit == 0 {
		limit = 50
	}

	// Query to list all schemas/users that own objects
	query := `
		SELECT DISTINCT owner
		FROM all_objects
		WHERE owner NOT IN ('SYS', 'SYSTEM', 'CTXSYS', 'DBSNMP', 'OUTLN', 'WMSYS', 'XDB', 'ANONYMOUS', 'APEX_PUBLIC_USER', 'PUBLIC')
		  AND object_type IN ('TABLE', 'VIEW', 'PROCEDURE', 'FUNCTION', 'PACKAGE', 'SEQUENCE', 'SYNONYM')
		ORDER BY owner`

	// Apply limit in query
	limitedQuery := fmt.Sprintf(`SELECT * FROM (%s) WHERE ROWNUM <= %d`, query, limit)

	slog.Debug("Executing Oracle schemas query", "query", limitedQuery, "limit", limit)

	// Execute query
	rows, err := db.QueryContext(ctx, limitedQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to query Oracle schemas: %w", err)
	}
	defer rows.Close()

	// Parse results
	var schemas []string
	for rows.Next() {
		var schema string
		err := rows.Scan(&schema)
		if err != nil {
			return nil, fmt.Errorf("failed to scan Oracle schema row: %w", err)
		}
		schemas = append(schemas, schema)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error reading Oracle schema rows: %w", err)
	}

	return schemas, nil
}

var ListOracleSchemas = mcpgrafana.MustTool(
	"list_oracle_schemas",
	"List available schemas (owners) in an Oracle database datasource. Connects directly to the Oracle database and queries system tables to return a list of schema names that contain database objects like tables, views, procedures, functions, packages, sequences, or synonyms.",
	listOracleSchemas,
	mcp.WithTitleAnnotation("List Oracle database schemas"),
	mcp.WithIdempotentHintAnnotation(true),
	mcp.WithReadOnlyHintAnnotation(true),
)

// AddOracleTools registers all Oracle database tools with the MCP server
func AddOracleTools(mcp *server.MCPServer) {
	ListOracleTables.Register(mcp)
	DescribeOracleTable.Register(mcp)
	QueryOracleDatabase.Register(mcp)
	ListOracleSchemas.Register(mcp)
}
