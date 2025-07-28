//go:build !windows
// +build !windows

package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/mapping"
	"google.golang.org/grpc"
)

// mockPlugin is the plugin implementation for the test
type mockPlugin struct {
	plugin.Plugin
}

func (p *mockPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (p *mockPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (any, error) {
	return c, nil
}

// Helper function to find or build the plugin binary
// Returns the path to the plugin binary and an error if any
func findOrBuildPluginBinary(t *testing.T) (string, error) {
	// Locate the plugin directory
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller")

	currentDir := filepath.Dir(filename)
	pluginDir := filepath.Join(currentDir, "..", "..", "v2", "pkg", "grpctest", "plugin")

	// Create a temporary directory for the plugin binary using testing's built-in helper
	// This directory will be automatically cleaned up when the test completes
	tempDir := t.TempDir()

	// Use the temp directory for the plugin binary
	pluginPath := filepath.Join(tempDir, "plugin_service")

	// Build the plugin
	t.Logf("Building plugin binary at %s", pluginPath)
	cmd := exec.Command("go", "build", "-o", pluginPath, "plugin_service.go")
	cmd.Dir = pluginDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to build plugin: %w", err)
	}

	// Verify plugin exists after build
	if _, err := os.Stat(pluginPath); err != nil {
		return "", fmt.Errorf("plugin binary not found after build: %w", err)
	}

	// Return path to the plugin binary in the temporary directory
	return pluginPath, nil
}

func setupGRPCTestGoPluginServer(t *testing.T) grpc.ClientConnInterface {
	// Skip if not in CI environment to avoid plugin compilation issues
	if os.Getenv("CI") == "" && testing.Short() {
		t.Skip("Skipping plugin test in short mode and non-CI environment")
	}

	// Find the plugin binary path
	pluginPath, err := findOrBuildPluginBinary(t)
	if err != nil {
		t.Fatalf("failed to find or build plugin binary: %v", err)
	}

	t.Logf("Using plugin binary: %s", pluginPath)

	// Start the plugin
	handshakeConfig := plugin.HandshakeConfig{
		ProtocolVersion:  1,
		MagicCookieKey:   "GRPC_DATASOURCE_PLUGIN",
		MagicCookieValue: "Foobar",
	}

	// Create the client
	client := plugin.NewClient(&plugin.ClientConfig{
		HandshakeConfig:  handshakeConfig,
		Plugins:          map[string]plugin.Plugin{"grpc_datasource": &mockPlugin{}},
		Cmd:              exec.Command(pluginPath),
		AllowedProtocols: []plugin.Protocol{plugin.ProtocolGRPC},
	})
	t.Cleanup(client.Kill)

	// Connect to the plugin
	rpcClient, err := client.Client()
	require.NoError(t, err)

	// Request the plugin
	raw, err := rpcClient.Dispense("grpc_datasource")
	require.NoError(t, err)

	// Convert to gRPC client connection
	conn, ok := raw.(*grpc.ClientConn)
	require.True(t, ok, "expected *grpc.ClientConn")

	return conn
}

type executeOpts struct {
	grpcMapping *grpcdatasource.GRPCMapping
}

func withGRPCMapping(mapping *grpcdatasource.GRPCMapping) func(*executeOpts) {
	return func(opts *executeOpts) {
		opts.grpcMapping = mapping
	}
}

func executeOperation(t *testing.T, grpcClient grpc.ClientConnInterface, operation graphql.Request, execOpts ...func(*executeOpts)) (string, error) {
	t.Helper()

	executeOpts := &executeOpts{
		grpcMapping: &grpcdatasource.GRPCMapping{},
	}
	for _, opt := range execOpts {
		opt(executeOpts)
	}

	factory, err := graphql_datasource.NewFactoryGRPC(context.Background(), grpcClient)
	if err != nil {
		return "", fmt.Errorf("failed to create factory: %w", err)
	}

	schema, err := grpctest.GraphQLSchemaWithoutBaseDefinitions()
	if err != nil {
		return "", fmt.Errorf("failed to create schema: %w", err)
	}

	protoSchema, err := grpctest.ProtoSchema()
	if err != nil {
		return "", fmt.Errorf("failed to create proto schema: %w", err)
	}

	compiler, err := grpcdatasource.NewProtoCompiler(protoSchema, executeOpts.grpcMapping)
	if err != nil {
		return "", fmt.Errorf("failed to create proto compiler: %w", err)
	}

	cfg, err := graphql_datasource.NewConfiguration(graphql_datasource.ConfigurationInput{
		GRPC: &grpcdatasource.GRPCConfiguration{
			Mapping:  executeOpts.grpcMapping,
			Compiler: compiler,
		},
		SchemaConfiguration: mustSchemaConfig(
			t,
			nil,
			string(schema.Input.RawBytes),
		),
	})
	if err != nil {
		return "", fmt.Errorf("failed to create configuration: %w", err)
	}

	dsCfg, err := plan.NewDataSourceConfiguration(
		"id",
		factory,
		grpctest.GetDataSourceMetadata(),
		cfg,
	)
	if err != nil {
		return "", fmt.Errorf("failed to create data source configuration: %w", err)
	}

	inputSchema, err := graphql.NewSchemaFromBytes(schema.Input.RawBytes)
	if err != nil {
		return "", fmt.Errorf("failed to create schema: %w", err)
	}

	engineConf := NewConfiguration(inputSchema)
	engineConf.SetDataSources([]plan.DataSource{dsCfg})
	engineConf.SetFieldConfigurations(grpctest.GetFieldConfigurations())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var opts _executionTestOptions
	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{
		MaxConcurrency:               1024,
		ResolvableOptions:            opts.resolvableOptions,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: resolve.SubgraphErrorPropagationModeWrapped,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create execution engine: %w", err)
	}

	resultWriter := graphql.NewEngineResultWriter()

	execCtx, execCtxCancel := context.WithCancel(context.Background())
	defer execCtxCancel()

	err = engine.Execute(execCtx, &operation, &resultWriter)
	if err != nil {
		return "", fmt.Errorf("failed to execute operation: %w", err)
	}

	response := resultWriter.String()

	return response, nil
}

func TestGRPCSubgraphExecution(t *testing.T) {
	conn := setupGRPCTestGoPluginServer(t)

	t.Run("running simple query should work", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "UserQuery",
			Variables:     nil,
			Query:         "query UserQuery { users { id name } }",
		}

		response, err := executeOperation(t, conn, operation)
		require.NoError(t, err)
		require.Equal(t, `{"data":{"users":[{"id":"user-1","name":"User 1"},{"id":"user-2","name":"User 2"},{"id":"user-3","name":"User 3"}]}}`, response)
	})

	t.Run("should run query with variable", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "UserQuery",
			Variables: stringify(map[string]any{
				"id": "1",
			}),
			Query: `
				query UserQuery($id: ID!) {
					user(id: $id) {
						id
						name
					}
				}
			`,
		}

		response, err := executeOperation(t, conn, operation)
		require.NoError(t, err)
		require.Equal(t, `{"data":{"user":{"id":"1","name":"User 1"}}}`, response)
	})

	t.Run("should run complex query", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "ComplexFilterTypeQuery",
			Variables: stringify(map[string]any{
				"filter": map[string]any{
					"filter": map[string]any{
						"name":         "test",
						"filterField1": "test",
						"filterField2": "test",
					},
				},
			}),
			Query: `
				query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) {
					complexFilterType(filter: $filter) {
						id
						name
					}
				}
			`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))
		require.NoError(t, err)
		require.Equal(t, `{"data":{"complexFilterType":[{"id":"test-id-123","name":"test"}]}}`, response)
	})

	t.Run("should run query with two arguments and no variables and mapping for field names", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "QueryWithTwoArguments",
			Query:         `query QueryWithTwoArguments { typeFilterWithArguments(filterField1: "test1", filterField2: "test2") { id name filterField1 filterField2 } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))
		require.NoError(t, err)
		require.Equal(t, `{"data":{"typeFilterWithArguments":[{"id":"multi-filter-1","name":"MultiFilter 1","filterField1":"test1","filterField2":"test2"},{"id":"multi-filter-2","name":"MultiFilter 2","filterField1":"test1","filterField2":"test2"}]}}`, response)
	})

	t.Run("should run query with a complex input type and no variables and mapping for field names", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "ComplexFilterTypeQuery",
			Query:         `query ComplexFilterTypeQuery { complexFilterType(filter: { filter: { name: "test", filterField1: "test1", filterField2: "test2", pagination: { page: 1, perPage: 10 } } }) { id name } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))
		require.NoError(t, err)
		require.Equal(t, `{"data":{"complexFilterType":[{"id":"test-id-123","name":"test"}]}}`, response)
	})

	t.Run("should run query with a complex input type and variables with different name", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "ComplexFilterTypeQuery",
			Variables: stringify(map[string]any{
				"foobar": map[string]any{
					"filter": map[string]any{
						"name":         "test",
						"filterField1": "test",
						"filterField2": "test",
					},
				},
			}),
			Query: `query ComplexFilterTypeQuery($foobar: ComplexFilterTypeInput!) { complexFilterType(filter: $foobar) { id name } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))
		require.NoError(t, err)
		require.Equal(t, `{"data":{"complexFilterType":[{"id":"test-id-123","name":"test"}]}}`, response)
	})

	t.Run("should run query with a type filter with arguments and variables", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "TypeWithMultipleFilterFieldsQuery",
			Variables: stringify(map[string]any{
				"filter": map[string]any{
					"filterField1": "test",
					"filterField2": "test",
				},
			}),
			Query: `query TypeWithMultipleFilterFieldsQuery($filter: FilterTypeInput!) { typeWithMultipleFilterFields(filter: $filter) { id name } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))
		require.NoError(t, err)
		require.Equal(t, `{"data":{"typeWithMultipleFilterFields":[{"id":"filtered-1","name":"Filter: 1"},{"id":"filtered-2","name":"Filter: 2"}]}}`, response)
	})

	t.Run("should run query with a nested type", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NestedTypeQuery",
			Query:         `query NestedTypeQuery { nestedType { id name b { id name c { id name } } } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))
		require.NoError(t, err)
		require.Equal(t, `{"data":{"nestedType":[{"id":"nested-a-1","name":"Nested A 1","b":{"id":"nested-b-1","name":"Nested B 1","c":{"id":"nested-c-1","name":"Nested C 1"}}},{"id":"nested-a-2","name":"Nested A 2","b":{"id":"nested-b-2","name":"Nested B 2","c":{"id":"nested-c-2","name":"Nested C 2"}}}]}}`, response)
	})

	t.Run("should run query with a recursive type", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "RecursiveTypeQuery",
			Query:         `query RecursiveTypeQuery { recursiveType { id name recursiveType { id recursiveType { id name } name } } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"recursiveType":{"id":"recursive-1","name":"Level 1","recursiveType":{"id":"recursive-2","recursiveType":{"id":"recursive-3","name":"Level 3"},"name":"Level 2"}}}}`, response)
	})

	t.Run("should stop when no mapping is found for the operation request", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "UserQuery",
			Query:         `query UserQuery { user(id: "1") { id name } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(
			&grpcdatasource.GRPCMapping{
				QueryRPCs: map[string]grpcdatasource.RPCConfig{
					"user": {
						RPC:      "QueryUser",
						Request:  "",
						Response: "QueryUserResponse",
					},
				},
			},
		))

		require.Empty(t, response)
		require.Error(t, err)
	})

	// Category tests to verify enum handling
	t.Run("should correctly handle query for all categories with enum values", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "CategoriesQuery",
			Query:         `query CategoriesQuery { categories { id name kind } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		// Verify response contains category data with enum values properly mapped
		require.Contains(t, response, `"kind":"BOOK"`)
		require.Contains(t, response, `"kind":"ELECTRONICS"`)
		require.Contains(t, response, `"kind":"FURNITURE"`)
		require.Contains(t, response, `"kind":"OTHER"`)
	})

	t.Run("should correctly handle query for categories by specific enum kind", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "CategoriesByKindQuery",
			Variables: stringify(map[string]any{
				"kind": "BOOK",
			}),
			Query: `query CategoriesByKindQuery($kind: CategoryKind!) { 
				categoriesByKind(kind: $kind) { 
					id 
					name 
					kind 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		// Verify all returned categories have the requested kind
		require.NotContains(t, response, `"kind":"ELECTRONICS"`)
		require.NotContains(t, response, `"kind":"FURNITURE"`)
		require.NotContains(t, response, `"kind":"OTHER"`)
		require.Contains(t, response, `"kind":"BOOK"`)
	})

	t.Run("should correctly handle filter categories with enum and pagination", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "FilterCategoriesQuery",
			Variables: stringify(map[string]any{
				"filter": map[string]any{
					"category": "ELECTRONICS",
					"pagination": map[string]any{
						"page":    1,
						"perPage": 2,
					},
				},
			}),
			Query: `query FilterCategoriesQuery($filter: CategoryFilter!) { 
				filterCategories(filter: $filter) { 
					id 
					name 
					kind 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		// Verify only ELECTRONICS categories are returned
		require.NotContains(t, response, `"kind":"BOOK"`)
		require.NotContains(t, response, `"kind":"FURNITURE"`)
		require.NotContains(t, response, `"kind":"OTHER"`)
		require.Contains(t, response, `"kind":"ELECTRONICS"`)
	})

	t.Run("should handle all enum values with explicit mapping", func(t *testing.T) {
		// Test each enum value explicitly
		enumValues := []string{"BOOK", "ELECTRONICS", "FURNITURE", "OTHER"}

		for _, enumValue := range enumValues {
			t.Run(fmt.Sprintf("Test with enum value %s", enumValue), func(t *testing.T) {
				operation := graphql.Request{
					OperationName: "CategoriesByKindQuery",
					Variables: stringify(map[string]any{
						"kind": enumValue,
					}),
					Query: `query CategoriesByKindQuery($kind: CategoryKind!) { 
						categoriesByKind(kind: $kind) { 
							id 
							name 
							kind 
						} 
					}`,
				}

				response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

				require.NoError(t, err)
				// Verify all returned categories have the requested kind
				require.Contains(t, response, fmt.Sprintf(`"kind":"%s"`, enumValue))

				// Verify no other enum values are present
				for _, otherEnum := range enumValues {
					if otherEnum != enumValue {
						require.NotContains(t, response, fmt.Sprintf(`"kind":"%s"`, otherEnum))
					}
				}
			})
		}
	})

	t.Run("should handle nullable fields", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NullableFieldsTypeQuery",
			Query:         `query NullableFieldsTypeQuery { nullableFieldsType { id optionalString optionalInt optionalFloat optionalBoolean requiredString requiredInt } }`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"nullableFieldsType":{"id":"nullable-default","optionalString":"Default optional string","optionalInt":777,"optionalFloat":null,"optionalBoolean":true,"requiredString":"Default required string","requiredInt":999}}}`, response)
	})

	t.Run("should handle nullable fields query by ID with full data", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NullableFieldsTypeByIdQuery",
			Variables: stringify(map[string]any{
				"id": "full-data",
			}),
			Query: `query NullableFieldsTypeByIdQuery($id: ID!) { 
				nullableFieldsTypeById(id: $id) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"nullableFieldsTypeById":{"id":"full-data","name":"Full Data by ID","optionalString":"All fields populated","optionalInt":123,"optionalFloat":12.34,"optionalBoolean":false,"requiredString":"Required by ID","requiredInt":456}}}`, response)
	})

	t.Run("should handle nullable fields query by ID with partial data", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NullableFieldsTypeByIdQuery",
			Variables: stringify(map[string]any{
				"id": "partial-data",
			}),
			Query: `query NullableFieldsTypeByIdQuery($id: ID!) { 
				nullableFieldsTypeById(id: $id) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"nullableFieldsTypeById":{"id":"partial-data","name":"Partial Data by ID","optionalString":null,"optionalInt":789,"optionalFloat":null,"optionalBoolean":true,"requiredString":"Partial required by ID","requiredInt":321}}}`, response)
	})

	t.Run("should handle nullable fields query by ID with minimal data", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NullableFieldsTypeByIdQuery",
			Variables: stringify(map[string]any{
				"id": "minimal-data",
			}),
			Query: `query NullableFieldsTypeByIdQuery($id: ID!) { 
				nullableFieldsTypeById(id: $id) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"nullableFieldsTypeById":{"id":"minimal-data","name":"Minimal Data by ID","optionalString":null,"optionalInt":null,"optionalFloat":null,"optionalBoolean":null,"requiredString":"Only required fields","requiredInt":111}}}`, response)
	})

	t.Run("should handle nullable fields query by ID returning null for not found", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NullableFieldsTypeByIdQuery",
			Variables: stringify(map[string]any{
				"id": "not-found",
			}),
			Query: `query NullableFieldsTypeByIdQuery($id: ID!) { 
				nullableFieldsTypeById(id: $id) { 
					id 
					name 
					optionalString 
					requiredString 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"nullableFieldsTypeById":null}}`, response)
	})

	t.Run("should handle query for all nullable fields types", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AllNullableFieldsTypesQuery",
			Query: `query AllNullableFieldsTypesQuery { 
				allNullableFieldsTypes { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"allNullableFieldsTypes":[{"id":"nullable-1","name":"Full Data Entry","optionalString":"Optional String Value","optionalInt":42,"optionalFloat":3.14,"optionalBoolean":true,"requiredString":"Required String 1","requiredInt":100},{"id":"nullable-2","name":"Partial Data Entry","optionalString":"Only string is set","optionalInt":null,"optionalFloat":null,"optionalBoolean":false,"requiredString":"Required String 2","requiredInt":200},{"id":"nullable-3","name":"Minimal Data Entry","optionalString":null,"optionalInt":null,"optionalFloat":null,"optionalBoolean":null,"requiredString":"Required String 3","requiredInt":300}]}}`, response)
	})

	t.Run("should handle nullable fields query with filter", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "NullableFieldsTypeWithFilterQuery",
			Variables: stringify(map[string]any{
				"filter": map[string]any{
					"name":           "TestFilter",
					"optionalString": "FilteredString",
					"includeNulls":   true,
				},
			}),
			Query: `query NullableFieldsTypeWithFilterQuery($filter: NullableFieldsFilter!) { 
				nullableFieldsTypeWithFilter(filter: $filter) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"id":"filtered-1"`)
		require.Contains(t, response, `"name":"TestFilter - 1"`)
		require.Contains(t, response, `"optionalString":"FilteredString"`)
		require.Contains(t, response, `"requiredString":"Required filtered 1"`)
		require.Contains(t, response, `"requiredInt":1000`)
		require.Contains(t, response, `"id":"filtered-2"`)
		require.Contains(t, response, `"id":"filtered-3"`)
	})

	t.Run("should handle create nullable fields type mutation", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "CreateNullableFieldsTypeMutation",
			Variables: stringify(map[string]any{
				"input": map[string]any{
					"name":            "Created Type",
					"optionalString":  "Optional Value",
					"optionalInt":     42,
					"optionalFloat":   3.14,
					"optionalBoolean": true,
					"requiredString":  "Required Value",
					"requiredInt":     100,
				},
			}),
			Query: `mutation CreateNullableFieldsTypeMutation($input: NullableFieldsInput!) { 
				createNullableFieldsType(input: $input) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"name":"Created Type"`)
		require.Contains(t, response, `"optionalString":"Optional Value"`)
		require.Contains(t, response, `"optionalInt":42`)
		require.Contains(t, response, `"optionalFloat":3.14`)
		require.Contains(t, response, `"optionalBoolean":true`)
		require.Contains(t, response, `"requiredString":"Required Value"`)
		require.Contains(t, response, `"requiredInt":100`)
		// Verify ID contains "nullable-" prefix
		require.Contains(t, response, `"id":"nullable-`)
	})

	t.Run("should handle create nullable fields type mutation with minimal input", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "CreateNullableFieldsTypeMutation",
			Variables: stringify(map[string]any{
				"input": map[string]any{
					"name":           "Minimal Type",
					"requiredString": "Only Required",
					"requiredInt":    200,
				},
			}),
			Query: `mutation CreateNullableFieldsTypeMutation($input: NullableFieldsInput!) { 
				createNullableFieldsType(input: $input) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"name":"Minimal Type"`)
		require.Contains(t, response, `"optionalString":null`)
		require.Contains(t, response, `"optionalInt":null`)
		require.Contains(t, response, `"optionalFloat":null`)
		require.Contains(t, response, `"optionalBoolean":null`)
		require.Contains(t, response, `"requiredString":"Only Required"`)
		require.Contains(t, response, `"requiredInt":200`)
		// Verify ID contains "nullable-" prefix
		require.Contains(t, response, `"id":"nullable-`)
	})

	t.Run("should handle update nullable fields type mutation", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "UpdateNullableFieldsTypeMutation",
			Variables: stringify(map[string]any{
				"id": "test-update",
				"input": map[string]any{
					"name":           "Updated Type",
					"optionalString": "Updated Optional",
					"optionalInt":    999,
					"requiredString": "Updated Required",
					"requiredInt":    500,
				},
			}),
			Query: `mutation UpdateNullableFieldsTypeMutation($id: ID!, $input: NullableFieldsInput!) { 
				updateNullableFieldsType(id: $id, input: $input) { 
					id 
					name 
					optionalString 
					optionalInt 
					optionalFloat 
					optionalBoolean 
					requiredString 
					requiredInt 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"updateNullableFieldsType":{"id":"test-update","name":"Updated Type","optionalString":"Updated Optional","optionalInt":999,"optionalFloat":null,"optionalBoolean":null,"requiredString":"Updated Required","requiredInt":500}}}`, response)
	})

	t.Run("should handle update nullable fields type mutation returning null for non-existent ID", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "UpdateNullableFieldsTypeMutation",
			Variables: stringify(map[string]any{
				"id": "non-existent",
				"input": map[string]any{
					"name":           "Should Not Exist",
					"requiredString": "Not Created",
					"requiredInt":    0,
				},
			}),
			Query: `mutation UpdateNullableFieldsTypeMutation($id: ID!, $input: NullableFieldsInput!) { 
				updateNullableFieldsType(id: $id, input: $input) { 
					id 
					name 
					optionalString 
					requiredString 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"updateNullableFieldsType":null}}`, response)
	})

	// BlogPost and Author list tests
	t.Run("should handle BlogPost query with scalar lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "BlogPostScalarListsQuery",
			Query: `query BlogPostScalarListsQuery { 
				blogPost { 
					id 
					title 
					content 
					tags 
					optionalTags 
					categories 
					keywords 
					viewCounts 
					ratings 
					isPublished 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"tags":`)
		require.Contains(t, response, `"optionalTags":`)
		require.Contains(t, response, `"categories":`)
		require.Contains(t, response, `"keywords":`)
		require.Contains(t, response, `"viewCounts":`)
		require.Contains(t, response, `"ratings":`)
		require.Contains(t, response, `"isPublished":`)
	})

	t.Run("should handle BlogPost query with nested scalar lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "BlogPostNestedScalarListsQuery",
			Query: `query BlogPostNestedScalarListsQuery { 
				blogPost { 
					id 
					title 
					tagGroups 
					relatedTopics 
					commentThreads 
					suggestions 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"blogPost":{"id":"blog-default","title":"Default Blog Post","tagGroups":[["tech","programming"],["golang","backend"]],"relatedTopics":[["microservices","api"],["databases","performance"]],"commentThreads":[["Great post!","Very helpful"],["Could use more examples","Thanks for sharing"]],"suggestions":[["Add code examples","Include diagrams"]]}}}`, response)
	})

	t.Run("should handle BlogPost query with complex lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "BlogPostComplexListsQuery",
			Query: `query BlogPostComplexListsQuery { 
				blogPost { 
					id 
					title 
					relatedCategories { 
						id 
						name 
						kind 
					} 
					contributors { 
						id 
						name 
					} 
					mentionedProducts { 
						id 
						name 
						price 
					} 
					mentionedUsers { 
						id 
						name 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"relatedCategories":`)
		require.Contains(t, response, `"contributors":`)
		require.Contains(t, response, `"mentionedProducts":`)
		require.Contains(t, response, `"mentionedUsers":`)
		// Verify complex objects within lists
		require.Contains(t, response, `"kind":`)
		require.Contains(t, response, `"price":`)
	})

	t.Run("should handle BlogPost query with nested complex lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "BlogPostNestedComplexListsQuery",
			Query: `query BlogPostNestedComplexListsQuery { 
				blogPost { 
					id 
					title 
					categoryGroups { 
						id 
						name 
						kind 
					} 
					contributorTeams { 
						id 
						name 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"categoryGroups":`)
		require.Contains(t, response, `"contributorTeams":`)
		// Verify nested complex objects
		require.Contains(t, response, `"kind":`)
	})

	t.Run("should handle BlogPost query by ID", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "BlogPostByIdQuery",
			Variables: stringify(map[string]any{
				"id": "test-blog-1",
			}),
			Query: `query BlogPostByIdQuery($id: ID!) { 
				blogPostById(id: $id) { 
					id 
					title 
					content 
					tags 
					tagGroups 
					relatedCategories { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"id":"test-blog-1"`)
		require.Contains(t, response, `"title":"Blog Post test-blog-1"`)
		require.Contains(t, response, `"tags":`)
		require.Contains(t, response, `"tagGroups":`)
		require.Contains(t, response, `"relatedCategories":`)
	})

	t.Run("should handle BlogPost filtered query", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "BlogPostFilteredQuery",
			Variables: stringify(map[string]any{
				"filter": map[string]any{
					"title":         "Test",
					"hasCategories": true,
					"minTags":       2,
				},
			}),
			Query: `query BlogPostFilteredQuery($filter: BlogPostFilter!) { 
				blogPostsWithFilter(filter: $filter) { 
					id 
					title 
					tags 
					categories 
					tagGroups 
					relatedCategories { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"blogPostsWithFilter":`)
		require.Contains(t, response, `"tags":`)
		require.Contains(t, response, `"categories":`)
		require.Contains(t, response, `"tagGroups":`)
		require.Contains(t, response, `"relatedCategories":`)
	})

	t.Run("should handle Author query with scalar lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AuthorScalarListsQuery",
			Query: `query AuthorScalarListsQuery { 
				author { 
					id 
					name 
					email 
					skills 
					languages 
					socialLinks 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"skills":`)
		require.Contains(t, response, `"languages":`)
		require.Contains(t, response, `"socialLinks":`)
	})

	t.Run("should handle Author query with nested scalar lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AuthorNestedScalarListsQuery",
			Query: `query AuthorNestedScalarListsQuery { 
				author { 
					id 
					name 
					teamsByProject 
					collaborations 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Equal(t, `{"data":{"author":{"id":"author-default","name":"Default Author","teamsByProject":[["Alice","Bob","Charlie"],["David","Eve"]],"collaborations":[["Open Source Project A","Research Paper B"],["Conference Talk C"]]}}}`, response)
	})

	t.Run("should handle Author query with complex lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AuthorComplexListsQuery",
			Query: `query AuthorComplexListsQuery { 
				author { 
					id 
					name 
					writtenPosts { 
						id 
						title 
						content 
					} 
					favoriteCategories { 
						id 
						name 
						kind 
					} 
					relatedAuthors { 
						id 
						name 
					} 
					productReviews { 
						id 
						name 
						price 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"writtenPosts":`)
		require.Contains(t, response, `"favoriteCategories":`)
		require.Contains(t, response, `"relatedAuthors":`)
		require.Contains(t, response, `"productReviews":`)
		// Verify complex objects within lists
		require.Contains(t, response, `"title":`)
		require.Contains(t, response, `"kind":`)
		require.Contains(t, response, `"price":`)
	})

	t.Run("should handle Author query with nested complex lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AuthorNestedComplexListsQuery",
			Query: `query AuthorNestedComplexListsQuery { 
				author { 
					id 
					name 
					authorGroups { 
						id 
						name 
					} 
					categoryPreferences { 
						id 
						name 
						kind 
					} 
					projectTeams { 
						id 
						name 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"authorGroups":`)
		require.Contains(t, response, `"categoryPreferences":`)
		require.Contains(t, response, `"projectTeams":`)
		// Verify nested complex objects
		require.Contains(t, response, `"kind":`)
	})

	t.Run("should handle Author query by ID", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AuthorByIdQuery",
			Variables: stringify(map[string]any{
				"id": "test-author-1",
			}),
			Query: `query AuthorByIdQuery($id: ID!) { 
				authorById(id: $id) { 
					id 
					name 
					skills 
					teamsByProject 
					favoriteCategories { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"id":"test-author-1"`)
		require.Contains(t, response, `"name":"Author test-author-1"`)
		require.Contains(t, response, `"skills":`)
		require.Contains(t, response, `"teamsByProject":`)
		require.Contains(t, response, `"favoriteCategories":`)
	})

	t.Run("should handle Author filtered query", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AuthorFilteredQuery",
			Variables: stringify(map[string]any{
				"filter": map[string]any{
					"name":       "Test",
					"hasTeams":   true,
					"skillCount": 3,
				},
			}),
			Query: `query AuthorFilteredQuery($filter: AuthorFilter!) { 
				authorsWithFilter(filter: $filter) { 
					id 
					name 
					skills 
					teamsByProject 
					favoriteCategories { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"authorsWithFilter":`)
		require.Contains(t, response, `"skills":`)
		require.Contains(t, response, `"teamsByProject":`)
		require.Contains(t, response, `"favoriteCategories":`)
	})

	t.Run("should handle BlogPost creation mutation with complex input lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "CreateBlogPostMutation",
			Variables: stringify(map[string]any{
				"input": map[string]any{
					"title":        "Complex Lists Blog Post",
					"content":      "Testing complex input lists",
					"tags":         []string{"graphql", "grpc", "lists"},
					"optionalTags": []string{"optional1", "optional2"},
					"categories":   []string{"Technology", "Programming"},
					"keywords":     []string{"nested", "complex", "types"},
					"viewCounts":   []int{150, 250, 350},
					"ratings":      []float64{4.2, 4.8, 3.9},
					"isPublished":  []bool{true, false, true},
					"tagGroups": [][]string{
						{"graphql", "schema"},
						{"grpc", "protobuf"},
						{"lists", "arrays"},
					},
					"relatedTopics": [][]string{
						{"backend", "api"},
						{"frontend", "ui"},
					},
					"commentThreads": [][]string{
						{"Great post!", "Thanks for sharing"},
						{"Very helpful", "Keep it up"},
					},
					"suggestions": [][]string{
						{"Add examples"},
						{"More details", "Better formatting"},
					},
					"relatedCategories": []map[string]any{
						{"name": "Web Development", "kind": "ELECTRONICS"},
						{"name": "API Design", "kind": "OTHER"},
					},
					"contributors": []map[string]any{
						{"name": "Alice Developer"},
						{"name": "Bob Engineer"},
					},
					"categoryGroups": [][]map[string]any{
						{
							{"name": "Backend", "kind": "ELECTRONICS"},
							{"name": "Database", "kind": "OTHER"},
						},
						{
							{"name": "Frontend", "kind": "ELECTRONICS"},
						},
					},
				},
			}),
			Query: `mutation CreateBlogPostMutation($input: BlogPostInput!) { 
				createBlogPost(input: $input) { 
					id 
					title 
					content 
					tags 
					optionalTags 
					categories 
					keywords 
					viewCounts 
					ratings 
					isPublished 
					tagGroups 
					relatedTopics 
					commentThreads 
					suggestions 
					relatedCategories { 
						id 
						name 
						kind 
					} 
					contributors { 
						id 
						name 
					} 
					categoryGroups { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"title":"Complex Lists Blog Post"`)
		require.Contains(t, response, `"content":"Testing complex input lists"`)
		require.Contains(t, response, `"tags":["graphql","grpc","lists"]`)
		require.Contains(t, response, `"optionalTags":["optional1","optional2"]`)
		require.Contains(t, response, `"categories":["Technology","Programming"]`)
		require.Contains(t, response, `"keywords":["nested","complex","types"]`)
		require.Contains(t, response, `"viewCounts":[150,250,350]`)
		require.Contains(t, response, `"ratings":[4.2,4.8,3.9]`)
		require.Contains(t, response, `"isPublished":[true,false,true]`)
		require.Contains(t, response, `"tagGroups":[["graphql","schema"],["grpc","protobuf"],["lists","arrays"]]`)
		require.Contains(t, response, `"relatedTopics":[["backend","api"],["frontend","ui"]]`)
		require.Contains(t, response, `"relatedCategories":`)
		require.Contains(t, response, `"contributors":`)
		require.Contains(t, response, `"categoryGroups":`)
		require.Contains(t, response, `"name":"Web Development"`)
		require.Contains(t, response, `"name":"Alice Developer"`)
		require.Contains(t, response, `"name":"Backend"`)
	})

	t.Run("should handle Author creation mutation with complex input lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "CreateAuthorMutation",
			Variables: stringify(map[string]any{
				"input": map[string]any{
					"name":        "New Author",
					"email":       "author@example.com",
					"skills":      []string{"Go", "GraphQL", "gRPC"},
					"languages":   []string{"English", "Spanish"},
					"socialLinks": []string{"twitter.com/author", "github.com/author"},
					"teamsByProject": [][]string{
						{"Alice", "Bob"},
						{"Charlie", "David", "Eve"},
					},
					"collaborations": [][]string{
						{"Project1", "Project2"},
						{"Project3"},
					},
					"favoriteCategories": []map[string]any{
						{"name": "Backend Development", "kind": "ELECTRONICS"},
						{"name": "API Design", "kind": "OTHER"},
					},
					"authorGroups": [][]map[string]any{
						{
							{"name": "Go Team"},
							{"name": "GraphQL Team"},
						},
					},
					"projectTeams": [][]map[string]any{
						{
							{"name": "Team Lead"},
							{"name": "Senior Dev"},
						},
						{
							{"name": "Junior Dev"},
						},
					},
				},
			}),
			Query: `mutation CreateAuthorMutation($input: AuthorInput!) { 
				createAuthor(input: $input) { 
					id 
					name 
					email 
					skills 
					languages 
					socialLinks 
					teamsByProject 
					collaborations 
					favoriteCategories { 
						id 
						name 
						kind 
					} 
					authorGroups { 
						id 
						name 
					} 
					projectTeams { 
						id 
						name 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"name":"New Author"`)
		require.Contains(t, response, `"email":"author@example.com"`)
		require.Contains(t, response, `"skills":["Go","GraphQL","gRPC"]`)
		require.Contains(t, response, `"languages":["English","Spanish"]`)
		require.Contains(t, response, `"socialLinks":["twitter.com/author","github.com/author"]`)
		require.Contains(t, response, `"teamsByProject":[["Alice","Bob"],["Charlie","David","Eve"]]`)
		require.Contains(t, response, `"collaborations":[["Project1","Project2"],["Project3"]]`)
		require.Contains(t, response, `"favoriteCategories":`)
		require.Contains(t, response, `"authorGroups":`)
		require.Contains(t, response, `"projectTeams":`)
		require.Contains(t, response, `"name":"Backend Development"`)
		require.Contains(t, response, `"name":"Go Team"`)
	})

	t.Run("should handle all BlogPosts query with lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AllBlogPostsQuery",
			Query: `query AllBlogPostsQuery { 
				allBlogPosts { 
					id 
					title 
					tags 
					tagGroups 
					viewCounts 
					ratings 
					relatedCategories { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"allBlogPosts":`)
		require.Contains(t, response, `"tags":`)
		require.Contains(t, response, `"tagGroups":`)
		require.Contains(t, response, `"viewCounts":`)
		require.Contains(t, response, `"ratings":`)
		require.Contains(t, response, `"relatedCategories":`)
	})

	t.Run("should handle all Authors query with lists", func(t *testing.T) {
		operation := graphql.Request{
			OperationName: "AllAuthorsQuery",
			Query: `query AllAuthorsQuery { 
				allAuthors { 
					id 
					name 
					skills 
					teamsByProject 
					favoriteCategories { 
						id 
						name 
						kind 
					} 
				} 
			}`,
		}

		response, err := executeOperation(t, conn, operation, withGRPCMapping(mapping.DefaultGRPCMapping()))

		require.NoError(t, err)
		require.Contains(t, response, `"allAuthors":`)
		require.Contains(t, response, `"skills":`)
		require.Contains(t, response, `"teamsByProject":`)
		require.Contains(t, response, `"favoriteCategories":`)
	})
}
