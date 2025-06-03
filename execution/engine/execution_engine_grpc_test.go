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
}
