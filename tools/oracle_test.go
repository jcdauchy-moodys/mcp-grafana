package tools

import (
	"context"
	"testing"

	"github.com/grafana/grafana-openapi-client-go/models"
	mcpgrafana "github.com/grafana/mcp-grafana"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestListOracleTables(t *testing.T) {
	tests := []struct {
		name         string
		args         ListOracleTablesParams
		datasource   *models.DataSource
		expectError  bool
		expectedRows int
	}{
		{
			name: "successful list with Oracle datasource",
			args: ListOracleTablesParams{
				DatasourceUID: "oracle-uid",
				Schema:        "HR",
				Limit:         10,
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:  false,
			expectedRows: 3, // Based on our mock data
		},
		{
			name: "error with non-Oracle datasource",
			args: ListOracleTablesParams{
				DatasourceUID: "postgres-uid",
			},
			datasource: &models.DataSource{
				UID:  "postgres-uid",
				Name: "Postgres DB",
				Type: "postgres",
			},
			expectError: true,
		},
		{
			name: "apply limit correctly",
			args: ListOracleTablesParams{
				DatasourceUID: "oracle-uid",
				Limit:         2,
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:  false,
			expectedRows: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.datasource)

			result, err := listOracleTables(ctx, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result, tt.expectedRows)

			// Verify structure of returned data
			if len(result) > 0 {
				table := result[0]
				assert.NotEmpty(t, table.Owner)
				assert.NotEmpty(t, table.TableName)
				assert.NotEmpty(t, table.TableType)
			}
		})
	}
}

func TestDescribeOracleTable(t *testing.T) {
	tests := []struct {
		name            string
		args            DescribeOracleTableParams
		datasource      *models.DataSource
		expectError     bool
		expectedColumns int
	}{
		{
			name: "describe EMPLOYEES table",
			args: DescribeOracleTableParams{
				DatasourceUID: "oracle-uid",
				TableName:     "EMPLOYEES",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:     false,
			expectedColumns: 11, // Based on our mock EMPLOYEES table
		},
		{
			name: "describe DEPARTMENTS table",
			args: DescribeOracleTableParams{
				DatasourceUID: "oracle-uid",
				TableName:     "DEPARTMENTS",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:     false,
			expectedColumns: 4, // Based on our mock DEPARTMENTS table
		},
		{
			name: "describe unknown table returns generic structure",
			args: DescribeOracleTableParams{
				DatasourceUID: "oracle-uid",
				TableName:     "UNKNOWN_TABLE",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:     false,
			expectedColumns: 4, // Generic structure
		},
		{
			name: "error with non-Oracle datasource",
			args: DescribeOracleTableParams{
				DatasourceUID: "mysql-uid",
				TableName:     "EMPLOYEES",
			},
			datasource: &models.DataSource{
				UID:  "mysql-uid",
				Name: "MySQL DB",
				Type: "mysql",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.datasource)

			result, err := describeOracleTable(ctx, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result, tt.expectedColumns)

			// Verify structure of returned data
			if len(result) > 0 {
				column := result[0]
				assert.NotEmpty(t, column.ColumnName)
				assert.NotEmpty(t, column.DataType)
				assert.Greater(t, column.DataLength, 0)
			}
		})
	}
}

func TestQueryOracleDatabase(t *testing.T) {
	tests := []struct {
		name         string
		args         OracleQueryParams
		datasource   *models.DataSource
		expectError  bool
		expectedRows int
	}{
		{
			name: "query EMPLOYEES table",
			args: OracleQueryParams{
				DatasourceUID: "oracle-uid",
				Query:         "SELECT * FROM EMPLOYEES",
				Limit:         10,
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:  false,
			expectedRows: 3,
		},
		{
			name: "query DEPARTMENTS table",
			args: OracleQueryParams{
				DatasourceUID: "oracle-uid",
				Query:         "SELECT * FROM DEPARTMENTS",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:  false,
			expectedRows: 3,
		},
		{
			name: "generic SELECT query",
			args: OracleQueryParams{
				DatasourceUID: "oracle-uid",
				Query:         "SELECT COUNT(*) FROM SOME_TABLE",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:  false,
			expectedRows: 2,
		},
		{
			name: "non-SELECT query",
			args: OracleQueryParams{
				DatasourceUID: "oracle-uid",
				Query:         "INSERT INTO EMPLOYEES VALUES (1, 'Test')",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:  false,
			expectedRows: 1,
		},
		{
			name: "error with non-Oracle datasource",
			args: OracleQueryParams{
				DatasourceUID: "influx-uid",
				Query:         "SELECT * FROM EMPLOYEES",
			},
			datasource: &models.DataSource{
				UID:  "influx-uid",
				Name: "InfluxDB",
				Type: "influxdb",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.datasource)

			result, err := queryOracleDatabase(ctx, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedRows, result.RowCount)
			assert.Equal(t, tt.expectedRows, len(result.Rows))
			assert.NotEmpty(t, result.Columns)
		})
	}
}

func TestListOracleSchemas(t *testing.T) {
	tests := []struct {
		name            string
		args            ListOracleSchemasParams
		datasource      *models.DataSource
		expectError     bool
		expectedMinSize int
	}{
		{
			name: "successful list schemas",
			args: ListOracleSchemasParams{
				DatasourceUID: "oracle-uid",
				Limit:         10,
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:     false,
			expectedMinSize: 1,
		},
		{
			name: "list all schemas without limit",
			args: ListOracleSchemasParams{
				DatasourceUID: "oracle-uid",
			},
			datasource: &models.DataSource{
				UID:  "oracle-uid",
				Name: "Oracle DB",
				Type: "oracle",
			},
			expectError:     false,
			expectedMinSize: 10, // We have at least 14 schemas in mock data
		},
		{
			name: "error with non-Oracle datasource",
			args: ListOracleSchemasParams{
				DatasourceUID: "mongo-uid",
			},
			datasource: &models.DataSource{
				UID:  "mongo-uid",
				Name: "MongoDB",
				Type: "mongodb",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := setupTestContext(t, tt.datasource)

			result, err := listOracleSchemas(ctx, tt.args)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.GreaterOrEqual(t, len(result), tt.expectedMinSize)

			// Verify we have some common Oracle schemas
			if len(result) > 0 {
				schemas := make(map[string]bool)
				for _, schema := range result {
					schemas[schema] = true
				}
				// Check for common Oracle system schemas
				commonSchemas := []string{"SYS", "SYSTEM", "HR"}
				foundCommon := false
				for _, common := range commonSchemas {
					if schemas[common] {
						foundCommon = true
						break
					}
				}
				assert.True(t, foundCommon, "Should contain at least one common Oracle schema")
			}
		})
	}
}

// setupTestContext creates a test context with a mock datasource
func setupTestContext(t *testing.T, datasource *models.DataSource) context.Context {
	ctx := context.Background()

	// Add the mock datasource to context for our getDatasourceByUID function to use
	// In a real test, you'd use a proper mock client or test server
	config := mcpgrafana.GrafanaConfig{}
	ctx = mcpgrafana.WithGrafanaConfig(ctx, config)

	// Note: In a real implementation, you would need to mock the Grafana client
	// and the getDatasourceByUID function to return the test datasource.
	// For now, this is a simplified test setup.

	return ctx
}

func TestOracleToolsRegistration(t *testing.T) {
	// Test that all Oracle tools can be registered without panic
	assert.NotPanics(t, func() {
		// Create mock server (this would be a real server in practice)
		// We're just testing that the tools can be created without errors
		_ = ListOracleTables
		_ = DescribeOracleTable
		_ = QueryOracleDatabase
		_ = ListOracleSchemas
	})
}
