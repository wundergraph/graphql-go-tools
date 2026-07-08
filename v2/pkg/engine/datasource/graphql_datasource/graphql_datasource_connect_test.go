package graphql_datasource

import (
	"context"
	"fmt"
	"testing"

	"github.com/jensneuse/abstractlogger"
	"github.com/stretchr/testify/require"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	grpcdatasource "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource"
)

func TestNewFactoryRPCTransport_NilCtx(t *testing.T) {
	var nilCtx context.Context
	_, err := NewFactoryRPCTransport(nilCtx, &stubRPCTransport{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "execution context is required")
}

func TestNewFactoryRPCTransport_NilTransport(t *testing.T) {
	_, err := NewFactoryRPCTransport(context.Background(), nil)
	require.Error(t, err)
	require.Contains(t, err.Error(), "rpc transport is required")
}

// TestNewFactoryRPCTransport_PropagatesTransport verifies that the RPC
// transport supplied to NewFactoryRPCTransport flows through to the Planner
// instances the Factory produces, and that PlanningBehavior reflects the
// RPC-backed Factory.
func TestNewFactoryRPCTransport_PropagatesTransport(t *testing.T) {
	transport := &stubRPCTransport{}
	f, err := NewFactoryRPCTransport(context.Background(), transport)
	require.NoError(t, err)
	require.NotNil(t, f)
	require.Same(t, transport, f.rpcTransport)

	planner, ok := f.Planner(abstractlogger.NoopLogger).(*Planner[Configuration])
	require.True(t, ok, "Planner returned unexpected type")
	require.Same(t, transport, planner.rpcTransport)

	require.True(t, f.PlanningBehavior().AlwaysFlattenFragments,
		"AlwaysFlattenFragments must be true for RPC-backed factories so the planner emits inline fields")
}

// TestNewConfiguration_ConnectRequiresGRPC asserts the invariant that a
// Connect configuration cannot stand alone: it reuses the gRPC mapping and
// compiler, so the GRPC field on ConfigurationInput must also be populated.
func TestNewConfiguration_ConnectRequiresGRPC(t *testing.T) {
	schema, err := NewSchemaConfiguration(`type Query { hello: String }`, nil)
	require.NoError(t, err)

	_, err = NewConfiguration(ConfigurationInput{
		SchemaConfiguration: schema,
		Connect: &grpcdatasource.ConnectConfiguration{
			BaseURL:  "http://localhost:8080",
			Encoding: grpcdatasource.ConnectEncodingProtobuf,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "grpc configuration")
	require.Contains(t, err.Error(), "connect")
}

// TestNewConfiguration_ConnectWithGRPCSucceeds verifies the happy path:
// when both GRPC (mapping/compiler) and Connect (transport endpoint) are
// supplied, the configuration is accepted and reports IsConnect/IsGRPC.
func TestNewConfiguration_ConnectWithGRPCSucceeds(t *testing.T) {
	schema, err := NewSchemaConfiguration(`type Query { hello: String }`, nil)
	require.NoError(t, err)

	cfg, err := NewConfiguration(ConfigurationInput{
		SchemaConfiguration: schema,
		GRPC:                &grpcdatasource.GRPCConfiguration{},
		Connect: &grpcdatasource.ConnectConfiguration{
			BaseURL:  "http://localhost:8080",
			Encoding: grpcdatasource.ConnectEncodingProtobuf,
		},
	})
	require.NoError(t, err)
	require.True(t, cfg.IsGRPC(), "GRPC mapping/compiler is still in play under Connect")
	require.True(t, cfg.IsConnect())
}

// TestNewConfiguration_EmptyConfigMentionsConnect makes sure the error
// surfaced when no transport-bearing configuration is supplied calls out
// the connect option, so callers do not assume only fetch/subscription/grpc
// are valid.
func TestNewConfiguration_EmptyConfigMentionsConnect(t *testing.T) {
	schema, err := NewSchemaConfiguration(`type Query { hello: String }`, nil)
	require.NoError(t, err)

	_, err = NewConfiguration(ConfigurationInput{SchemaConfiguration: schema})
	require.Error(t, err)
	require.Contains(t, err.Error(), "connect")
}

// stubRPCTransport implements grpcdatasource.RPCTransport for tests that
// only need a non-nil RPCTransport reference; the Invoke body is never
// reached by these unit tests because they do not drive a planner end to
// end (that path is exercised in pkg/engine/datasource/grpc_datasource).
type stubRPCTransport struct{}

func (*stubRPCTransport) Invoke(ctx context.Context, methodFullName string, input, output protoref.Message) error {
	return fmt.Errorf("stub: %s", methodFullName)
}
