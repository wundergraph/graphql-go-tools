// Package events is a small gRPC test service for events.
// Its proto file uses proto3 optional fields for the nullable scalars.
package events

import (
	"context"
	"embed"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/grpctest/events/eventv1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

//go:embed testdata/events.graphqls event.proto
var files embed.FS

// MustGraphQLSchema returns the validated GraphQL schema of the service.
func MustGraphQLSchema(t testing.TB) ast.Document {
	schema, err := files.ReadFile("testdata/events.graphqls")
	require.NoError(t, err)

	doc := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(string(schema))

	report := &operationreport.Report{}
	astvalidation.DefaultDefinitionValidator().Validate(&doc, report)
	if report.HasErrors() {
		t.Fatalf("failed to validate schema: %s", report.Error())
	}

	return doc
}

// MustProtoSchema returns the proto file of the service.
func MustProtoSchema(t testing.TB) string {
	schema, err := files.ReadFile("event.proto")
	require.NoError(t, err)
	return string(schema)
}

// NewClientConn starts the mock service on an in-memory listener.
// It returns a client connection to the service. The test cleanup stops the service.
func NewClientConn(t testing.TB) *grpc.ClientConn {
	t.Helper()

	listener := bufconn.Listen(1024 * 1024)
	server := grpc.NewServer()
	eventv1.RegisterEventServiceServer(server, &MockService{})

	go func() {
		if err := server.Serve(listener); err != nil {
			t.Errorf("failed to serve: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		_ = conn.Close()
		server.Stop()
		_ = listener.Close()
	})

	return conn
}
