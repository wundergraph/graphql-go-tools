package engine

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/execution/graphql"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/starwars"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

// mockGRPCProductsClient provides a simple implementation of grpc.ClientConnInterface for testing
type mockGRPCProductsClient struct {
	t *testing.T
}

func (m mockGRPCProductsClient) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	m.t.Log(method, args, reply)

	msg, ok := reply.(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("reply is not a dynamicpb.Message")
	}

	// Based on the method name, populate the response with appropriate test data
	if strings.HasSuffix(method, "QueryUsers") {
		// Populate the response with test data using protojson.Unmarshal
		responseJSON := []byte(`{"users":[{"id":"test-id-123", "name":"Test Product"}]}`)
		err := protojson.Unmarshal(responseJSON, msg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (m mockGRPCProductsClient) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	panic("implement me")
}

func newTestGRPCClient(t *testing.T) *mockGRPCProductsClient {
	return &mockGRPCProductsClient{t: t}
}

var _ grpc.ClientConnInterface = (*mockGRPCProductsClient)(nil)

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

type GRPCTestCase struct {
	dataSources      []plan.DataSource
	operation        graphql.Request
	schema           *graphql.Schema
	expectedResponse string
}

func runGRPCTestCase(t *testing.T, tc GRPCTestCase) {
	t.Helper()

	engineConf := NewConfiguration(tc.schema)
	engineConf.SetDataSources(tc.dataSources)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var opts _executionTestOptions
	engine, err := NewExecutionEngine(ctx, abstractlogger.Noop{}, engineConf, resolve.ResolverOptions{
		MaxConcurrency:               1024,
		ResolvableOptions:            opts.resolvableOptions,
		PropagateSubgraphErrors:      true,
		SubgraphErrorPropagationMode: resolve.SubgraphErrorPropagationModeWrapped,
	})
	require.NoError(t, err)

	resultWriter := graphql.NewEngineResultWriter()

	execCtx, execCtxCancel := context.WithCancel(context.Background())
	defer execCtxCancel()

	err = engine.Execute(execCtx, &tc.operation, &resultWriter)

	actualResponse := resultWriter.String()

	assert.Equal(t, tc.expectedResponse, actualResponse)
}

func TestExecutionEngineGRPC(t *testing.T) {
	t.Run("grpc bad invocation compile mismatched schema", func(t *testing.T) {
		grpcClient, err := grpc.NewClient("lalala", grpc.WithTransportCredentials(insecure.NewCredentials()))
		require.NoError(t, err)

		protoSchema := grpctest.ProtoSchema(t)
		graphqlSchema := graphql.StarwarsSchema(t)

		tc := GRPCTestCase{
			schema: graphqlSchema,
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactoryGRPC(t,
						grpcClient,
					),
					&plan.DataSourceMetadata{
						RootNodes: []plan.TypeField{
							{
								TypeName:   "Query",
								FieldNames: []string{"hero"},
							},
						},
						ChildNodes: []plan.TypeField{
							{
								TypeName:   "Character",
								FieldNames: []string{"name"},
							},
						},
					},
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						GRPC: &grpcdatasource.GRPCConfiguration{
							ProtoSchema:  protoSchema,
							Mapping:      &grpcdatasource.GRPCMapping{},
							SubgraphName: "Starwars",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphqlSchema.RawSchema()),
						),
					}),
				),
			},
			operation:        graphql.LoadStarWarsQuery(starwars.FileSimpleHeroQuery, nil)(t),
			expectedResponse: `{"errors":[{"message":"Failed to fetch from Subgraph 'id'.","extensions":{"errors":[{"message":"failed to compile invocation: internal: message  not found in document\ninternal: message  not found in document"}]}}],"data":{"hero":null}}`,
		}

		runGRPCTestCase(t, tc)
	})

	t.Run("grpc working with mocked grpc client", func(t *testing.T) {
		protoSchema := grpctest.ProtoSchema(t)
		graphqlSchema := grpctest.GraphQLSchema(t)

		operation := graphql.Request{
			OperationName: "UserQuery",
			Variables:     nil,
			Query:         "query UserQuery { users { id name } }",
		}

		tc := GRPCTestCase{
			schema:           graphqlSchema,
			operation:        operation,
			expectedResponse: `{"data":{"users":[{"id":"test-id-123","name":"Test Product"}]}}`,
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactoryGRPC(t, newTestGRPCClient(t)),
					grpctest.DataSourceMetadata,
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						GRPC: &grpcdatasource.GRPCConfiguration{
							ProtoSchema:  protoSchema,
							Mapping:      &grpcdatasource.GRPCMapping{},
							SubgraphName: "Products",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphqlSchema.RawSchema()),
						),
					}),
				),
			},
		}

		runGRPCTestCase(t, tc)
	})

	t.Run("with real plugin", func(t *testing.T) {
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
		defer client.Kill()

		// Connect to the plugin
		rpcClient, err := client.Client()
		require.NoError(t, err)

		// Request the plugin
		raw, err := rpcClient.Dispense("grpc_datasource")
		require.NoError(t, err)

		// Convert to gRPC client connection
		conn, ok := raw.(grpc.ClientConnInterface)
		require.True(t, ok, "expected grpc.ClientConnInterface")

		protoSchema := grpctest.ProtoSchema(t)
		graphqlSchema := grpctest.GraphQLSchema(t)

		operation := graphql.Request{
			OperationName: "UserQuery",
			Variables:     nil,
			Query:         "query UserQuery { users { id name } }",
		}

		tc := GRPCTestCase{
			schema:           graphqlSchema,
			operation:        operation,
			expectedResponse: `{"data":{"users":[{"id":"user-1","name":"User 1"},{"id":"user-2","name":"User 2"},{"id":"user-3","name":"User 3"}]}}`,
			dataSources: []plan.DataSource{
				mustGraphqlDataSourceConfiguration(t,
					"id",
					mustFactoryGRPC(t, conn),
					grpctest.DataSourceMetadata,
					mustConfiguration(t, graphql_datasource.ConfigurationInput{
						GRPC: &grpcdatasource.GRPCConfiguration{
							ProtoSchema:  protoSchema,
							Mapping:      &grpcdatasource.GRPCMapping{},
							SubgraphName: "Products",
						},
						SchemaConfiguration: mustSchemaConfig(
							t,
							nil,
							string(graphqlSchema.RawSchema()),
						),
					}),
				),
			},
		}

		runGRPCTestCase(t, tc)
	})
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
