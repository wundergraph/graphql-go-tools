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
	"sync"

	"github.com/tidwall/gjson"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
)

// Verify DataSource implements the resolve.DataSource interface
var _ resolve.DataSource = (*DataSource)(nil)

// DataSource implements the resolve.DataSource interface for gRPC services.
// It handles the conversion of GraphQL queries to gRPC requests and
// transforms the responses back to GraphQL format.
type DataSource struct {
	// Invocations is a list of gRPC invocations to be executed
	plan              *RPCExecutionPlan
	cc                grpc.ClientConnInterface
	rc                *RPCCompiler
	mapping           *GRPCMapping
	federationConfigs plan.FederationFieldConfigurations
	disabled          bool
}

type ProtoConfig struct {
	Schema string
}

type DataSourceConfig struct {
	Operation         *ast.Document
	Definition        *ast.Document
	Compiler          *RPCCompiler
	SubgraphName      string
	Mapping           *GRPCMapping
	FederationConfigs plan.FederationFieldConfigurations
	Disabled          bool
}

// NewDataSource creates a new gRPC datasource
func NewDataSource(client grpc.ClientConnInterface, config DataSourceConfig) (*DataSource, error) {
	planner := NewPlanner(config.SubgraphName, config.Mapping, config.FederationConfigs)
	plan, err := planner.PlanOperation(config.Operation, config.Definition)
	if err != nil {
		return nil, err
	}

	return &DataSource{
		plan:              plan,
		cc:                client,
		rc:                config.Compiler,
		mapping:           config.Mapping,
		federationConfigs: config.FederationConfigs,
		disabled:          config.Disabled,
	}, nil
}

// Load implements resolve.DataSource interface.
// It processes the input JSON data to make gRPC calls and writes
// the response to the output buffer.
//
// The input is expected to contain the necessary information to make
// a gRPC call, including service name, method name, and request data.
func (d *DataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	// get variables from input
	variables := gjson.Parse(string(input)).Get("body.variables")
	builder := newJSONBuilder(d.mapping, variables)

	if d.disabled {
		out.Write(builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")))
		return nil
	}

	// get invocations from plan
	invocations, err := d.rc.Compile(d.plan, variables)
	if err != nil {
		return err
	}

	a := astjson.Arena{}

	responses := make([]*astjson.Value, len(invocations))
	errGrp, errGrpCtx := errgroup.WithContext(ctx)

	mu := sync.Mutex{}
	// make gRPC calls
	for index, invocation := range invocations {
		errGrp.Go(func() error {
			// Invoke the gRPC method - this will populate invocation.Output
			methodName := fmt.Sprintf("/%s/%s", invocation.ServiceName, invocation.MethodName)

			err := d.cc.Invoke(errGrpCtx, methodName, invocation.Input, invocation.Output)
			if err != nil {
				return err
			}

			response, err := builder.marshalResponseJSON(&a, &invocation.Call.Response, invocation.Output)
			if err != nil {
				return err
			}

			d.synchronizedSetResponse(&mu, responses, index, response)
			return nil
		})
	}

	if err := errGrp.Wait(); err != nil {
		out.Write(builder.writeErrorBytes(err))
		return nil
	}

	root := a.NewObject()
	for _, response := range responses {
		root, err = builder.mergeValues(root, response)
		if err != nil {
			out.Write(builder.writeErrorBytes(err))
			return err
		}
	}

	data := builder.toDataObject(root)
	out.Write(data.MarshalTo(nil))

	return nil
}

func (d *DataSource) synchronizedSetResponse(mu *sync.Mutex, responses []*astjson.Value, index int, response *astjson.Value) {
	mu.Lock()
	defer mu.Unlock()

	responses[index] = response
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
