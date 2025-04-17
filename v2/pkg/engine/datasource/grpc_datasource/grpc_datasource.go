// Package grpcdatasource provides a GraphQL datasource implementation for gRPC services.
// It allows GraphQL servers to connect to gRPC backends and execute RPC calls
// as part of GraphQL query resolution.
//
// The package includes tools for parsing Protobuf definitions, building execution plans,
// and converting GraphQL queries into gRPC requests.
package grpcdatasource

import (
	"bytes"
	"context"
	"fmt"

	"github.com/tidwall/gjson"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/graphql_datasource"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

// Verify DataSource implements the resolve.DataSource interface
var _ resolve.DataSource = (*DataSource)(nil)

// DataSource implements the resolve.DataSource interface for gRPC services.
// It handles the conversion of GraphQL queries to gRPC requests and
// transforms the responses back to GraphQL format.
type DataSource struct {
	// Invocations is a list of gRPC invocations to be executed
	Plan *RPCExecutionPlan
	cc   grpc.ClientConnInterface
	rc   *RPCCompiler
}

type ProtoConfig struct {
	Schema string
}

type DataSourceConfig struct {
	Operation    *ast.Document
	Definition   *ast.Document
	ProtoSchema  string
	SubgraphName string
	Mapping      *graphql_datasource.GRPCMapping
}

// NewDataSource creates a new gRPC datasource
func NewDataSource(client grpc.ClientConnInterface, config DataSourceConfig) (*DataSource, error) {
	compiler, err := NewProtoCompiler(config.ProtoSchema)
	if err != nil {
		return nil, err
	}

	planner := NewPlanner(config.SubgraphName, config.Mapping)
	plan, err := planner.PlanOperation(config.Operation, config.Definition)
	if err != nil {
		return nil, err
	}

	return &DataSource{
		Plan: plan,
		cc:   client,
		rc:   compiler,
	}, nil
}

// Load implements resolve.DataSource interface.
// It processes the input JSON data to make gRPC calls and writes
// the response to the output buffer.
//
// The input is expected to contain the necessary information to make
// a gRPC call, including service name, method name, and request data.
// TODO Implement this
func (d *DataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	// get variables from input
	variables := gjson.Parse(string(input)).Get("variables")

	// get invocations from plan
	invocations, err := d.rc.Compile(d.Plan, variables)
	if err != nil {
		return err
	}

	invocationGroups := make(map[int][]Invocation)

	for _, invocation := range invocations {
		invocationGroups[invocation.GroupIndex] = append(invocationGroups[invocation.GroupIndex], invocation)
	}

	for i := 0; i < len(invocationGroups); i++ {
		group := invocationGroups[i]

		// make gRPC calls
		for _, invocation := range group {
			// Invoke the gRPC method - this will populate invocation.Output
			methodName := fmt.Sprintf("/%s/%s", invocation.ServiceName, invocation.MethodName)
			err := d.cc.Invoke(ctx, methodName, invocation.Input, invocation.Output)
			if err != nil {
				a := astjson.Arena{}
				errorRoot := a.NewObject()
				errorArray := a.NewArray()
				errorRoot.Set("errors", errorArray)

				errorItem := a.NewObject()
				errorItem.Set("message", a.NewString(err.Error()))
				errorArray.SetArrayItem(0, errorItem)

				out.Write(errorRoot.MarshalTo(nil))
				return nil
			}

			// Marshal the populated output message to JSON
			outputBytes, err := protojson.Marshal(invocation.Output)
			if err != nil {
				return err
			}

			a := astjson.Arena{}
			root := a.NewObject()
			root.Set("data", astjson.MustParseBytes(outputBytes))

			// write output to out
			out.Write(root.MarshalTo(nil))
		}
	}

	return nil
}

// LoadWithFiles implements resolve.DataSource interface.
// Similar to Load, but handles file uploads if needed.
//
// Note: File uploads are typically not part of gRPC, so this method
// might not be applicable for most gRPC use cases.
//
// Currently unimplemented.
func (d *DataSource) LoadWithFiles(ctx context.Context, input []byte, files []*httpclient.FileUpload, out *bytes.Buffer) (err error) {
	panic("unimplemented")
}
