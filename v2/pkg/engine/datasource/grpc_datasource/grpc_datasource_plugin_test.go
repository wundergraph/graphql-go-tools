package grpcdatasource

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/hashicorp/go-plugin"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata/productv1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"google.golang.org/grpc"
)

// mockPlugin is the plugin implementation for the test
type mockPlugin struct {
	plugin.Plugin
}

func (p *mockPlugin) GRPCServer(broker *plugin.GRPCBroker, s *grpc.Server) error {
	return nil
}

func (p *mockPlugin) GRPCClient(ctx context.Context, broker *plugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return c, nil
}

// Test_GRPCDataSourcePlugin tests the grpc datasource using a plugin
func Test_GRPCDataSourcePlugin(t *testing.T) {
	// Skip if not in CI environment to avoid plugin compilation issues
	if os.Getenv("CI") == "" && testing.Short() {
		t.Skip("Skipping plugin test in short mode and non-CI environment")
	}

	// 1. Find the plugin binary path
	pluginPath, err := findOrBuildPluginBinary(t)
	if err != nil {
		t.Fatalf("failed to find or build plugin binary: %v", err)
	}

	t.Logf("Using plugin binary: %s", pluginPath)

	// 2. Start the plugin
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

	// 3. Set up GraphQL query and variables
	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"filter":{"name":"Test Plugin Product","filterField1":"filterField1","filterField2":"filterField2"}}}}`

	// 4. Parse GraphQL schema
	report := &operationreport.Report{}
	schemaDoc := ast.NewDocument()
	schemaDoc.Input.ResetInputString(testdata.UpstreamSchema)
	astparser.NewParser().Parse(schemaDoc, report)
	require.False(t, report.HasErrors(), "failed to parse schema: %s", report.Error())

	// 5. Parse GraphQL query
	queryDoc := ast.NewDocument()
	queryDoc.Input.ResetInputString(query)
	astparser.NewParser().Parse(queryDoc, report)
	require.False(t, report.HasErrors(), "failed to parse query: %s", report.Error())

	// 6. Transform ASTs
	err = asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
	require.NoError(t, err, "failed to merge schema with base")

	// 7. Create data source with plugin client connection
	ds, err := NewDataSource(conn, DataSourceConfig{
		Operation:    queryDoc,
		Definition:   schemaDoc,
		ProtoSchema:  testdata.ProtoSchema(t),
		SubgraphName: "Products",
	})
	require.NoError(t, err)

	// 8. Execute query
	output := new(bytes.Buffer)
	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","variables":`+variables+`}`), output)
	require.NoError(t, err)

	t.Logf("Response: %s", output.String())

	// 9. Verify response
	type response struct {
		TypeWithComplexFilterInput []struct {
			Id   string `json:"id"`
			Name string `json:"name"`
		} `json:"typeWithComplexFilterInput"`
	}

	var resp response
	err = json.Unmarshal(output.Bytes(), &resp)
	require.NoError(t, err)

	require.Len(t, resp.TypeWithComplexFilterInput, 1)
	require.Equal(t, "test-id-123", resp.TypeWithComplexFilterInput[0].Id)
	require.Equal(t, "Test Plugin Product", resp.TypeWithComplexFilterInput[0].Name)

	// 10. Test with direct client
	productClient := productv1.NewProductServiceClient(conn)
	productResp, err := productClient.QueryComplexFilterType(context.Background(), &productv1.QueryComplexFilterTypeRequest{
		Filter: &productv1.ComplexFilterTypeInput{
			Filter: &productv1.FilterType{
				Name:         "Direct Client Test",
				FilterField1: "test1",
				FilterField2: "test2",
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, productResp.TypeWithComplexFilterInput, 1)
	require.Equal(t, "test-id-123", productResp.TypeWithComplexFilterInput[0].Id)
	require.Equal(t, "Direct Client Test", productResp.TypeWithComplexFilterInput[0].Name)
}

// Helper function to find or build the plugin binary
// Returns the path to the plugin binary and an error if any
func findOrBuildPluginBinary(t *testing.T) (string, error) {
	// Locate the plugin directory
	_, filename, _, ok := runtime.Caller(0)
	require.True(t, ok, "failed to get caller")

	currentDir := filepath.Dir(filename)
	pluginDir := filepath.Join(currentDir, "testdata", "plugin")

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
