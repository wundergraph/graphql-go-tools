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
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafebytes"
)

type resultData struct {
	kind         CallKind
	response     *astjson.Value
	responsePath ast.Path
}

// Verify DataSource implements the resolve.DataSource interface
var _ resolve.DataSource = (*DataSource)(nil)

// DataSource implements the resolve.DataSource interface for gRPC services.
// It handles the conversion of GraphQL queries to gRPC requests and
// transforms the responses back to GraphQL format.
type DataSource struct {
	plan              *RPCExecutionPlan
	graph             *DependencyGraph
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
		graph:             NewDependencyGraph(plan),
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
	variables := gjson.Parse(unsafebytes.BytesToString(input)).Get("body.variables")
	builder := newJSONBuilder(d.mapping, variables)

	if d.disabled {
		out.Write(builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")))
		return nil
	}

	arena := astjson.Arena{}
	root := arena.NewObject()

	failed := false

	if err := d.graph.TopologicalSortResolve(func(nodes []FetchItem) error {
		serviceCalls, err := d.rc.CompileFetches(d.graph, nodes, variables)
		if err != nil {
			return err
		}

		results := make([]resultData, len(serviceCalls))
		errGrp, errGrpCtx := errgroup.WithContext(ctx)

		// make gRPC calls
		for index, serviceCall := range serviceCalls {
			errGrp.Go(func() error {
				a := astjson.Arena{}
				// Invoke the gRPC method - this will populate serviceCall.Output
				methodName := fmt.Sprintf("/%s/%s", serviceCall.ServiceName, serviceCall.MethodName)

				err := d.cc.Invoke(errGrpCtx, methodName, serviceCall.Input, serviceCall.Output)
				if err != nil {
					return err
				}

				response, err := builder.marshalResponseJSON(&a, &serviceCall.RPC.Response, serviceCall.Output)
				if err != nil {
					return err
				}

				// In case of a federated response, we need to ensure that the response is valid.
				// The number of entities per type must match the number of lookup keys in the variablese
				if serviceCall.RPC.Kind == CallKindEntity {
					err = builder.validateFederatedResponse(response)
					if err != nil {
						return err
					}
				}

				results[index] = resultData{
					kind:         serviceCall.RPC.Kind,
					response:     response,
					responsePath: serviceCall.RPC.ResponsePath,
				}

				return nil
			})
		}

		if err := errGrp.Wait(); err != nil {
			out.Write(builder.writeErrorBytes(err))
			failed = true
			return nil
		}

		for _, result := range results {
			switch result.kind {
			case CallKindResolve:
				err = builder.mergeWithPath(root, result.response, result.responsePath)
			default:
				root, err = builder.mergeValues(root, result.response)
			}
			if err != nil {
				out.Write(builder.writeErrorBytes(err))
				return err
			}
		}

		return nil
	}); err != nil || failed {
		return err
	}

	data := builder.toDataObject(root)
	out.Write(data.MarshalTo(nil))
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
