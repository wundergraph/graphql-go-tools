package grpcdatasource

import (
	"bytes"
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/grpc_datasource/testdata"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/dynamicpb"
)

type mockInterface struct {
}

func (m mockInterface) Invoke(ctx context.Context, method string, args any, reply any, opts ...grpc.CallOption) error {
	fmt.Println(method, args, reply)

	msg, ok := reply.(*dynamicpb.Message)
	if !ok {
		return fmt.Errorf("reply is not a dynamicpb.Message")
	}

	// fd := msg.Descriptor().Fields()
	// Based on the method name, populate the response with appropriate test data
	if method == "QueryComplexFilterType" {

		responseJSON := []byte(`{"typeWithComplexFilterInput":[{"id":"test-id-123", "name":"Test Product"}]}`)

		err := protojson.Unmarshal(responseJSON, msg)
		if err != nil {
			return err
		}

		// // Get the descriptor of the response message
		// resultsField := fd.ByName("typeWithComplexFilterInput")

		// if resultsField != nil {
		// 	// Create a list to hold items
		// 	list := msg.Mutable(resultsField).List()

		// 	// Create a new item message
		// 	typeDesc := resultsField.Message()
		// 	item := dynamicpb.NewMessage(typeDesc)

		// 	// Set fields on the item
		// 	idField := typeDesc.Fields().ByName("id")
		// 	if idField != nil {
		// 		item.Set(idField, protoreflect.ValueOfString("test-id-123"))
		// 	}

		// 	nameField := typeDesc.Fields().ByName("name")
		// 	if nameField != nil {
		// 		item.Set(nameField, protoreflect.ValueOfString("Test Product"))
		// 	}

		// 	// Add the item to the list
		// 	list.Append(protoreflect.ValueOfMessage(item.ProtoReflect()))
		// }
	}

	// fmt.Println(reply)

	return nil
}

func (m mockInterface) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	//TODO implement me
	panic("implement me")
}

var _ grpc.ClientConnInterface = (*mockInterface)(nil)

func Test_DataSource_Load(t *testing.T) {

	query := `query ComplexFilterTypeQuery($filter: ComplexFilterTypeInput!) { complexFilterType(filter: $filter) { id name } }`
	variables := `{"variables":{"filter":{"name":"test","filterField1":"test","filterField2":"test"}}}`

	report := &operationreport.Report{}
	// Parse the GraphQL schema
	schemaDoc := ast.NewDocument()
	schemaDoc.Input.ResetInputString(testdata.UpstreamSchema)
	astparser.NewParser().Parse(schemaDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse schema: %s", report.Error())
	}

	// Parse the GraphQL query
	queryDoc := ast.NewDocument()
	queryDoc.Input.ResetInputString(query)
	astparser.NewParser().Parse(queryDoc, report)
	if report.HasErrors() {
		t.Fatalf("failed to parse query: %s", report.Error())
	}
	// Transform the GraphQL ASTs
	err := asttransform.MergeDefinitionWithBaseSchema(schemaDoc)
	if err != nil {
		t.Fatalf("failed to merge schema with base: %s", err)
	}

	mi := mockInterface{}
	ds, err := NewDataSource(mi, DataSourceConfig{
		Operation:    queryDoc,
		Definition:   schemaDoc,
		ProtoSchema:  testdata.ProtoSchema(t),
		SubgraphName: "Products",
	})

	require.NoError(t, err)

	output := new(bytes.Buffer)

	err = ds.Load(context.Background(), []byte(`{"query":"`+query+`","variables":`+variables+`}`), output)
	require.NoError(t, err)

	fmt.Println(output.String())
}
