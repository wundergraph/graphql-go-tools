// Package grpcdatasource provides a GraphQL datasource implementation for gRPC services.
// It allows GraphQL servers to connect to gRPC backends and execute RPC calls
// as part of GraphQL query resolution.
//
// The package includes tools for parsing Protobuf definitions, building execution plans,
// and converting GraphQL queries into gRPC requests.
package grpcdatasource

import (
	"context"
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"

	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

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
	cc                grpc.ClientConnInterface
	rc                *RPCCompiler
	mapping           *GRPCMapping
	federationConfigs plan.FederationFieldConfigurations
	disabled          bool

	pool *arena.Pool
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
	planner, err := NewPlanner(config.SubgraphName, config.Mapping, config.FederationConfigs)
	if err != nil {
		return nil, err
	}
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
		pool:              arena.NewArenaPool(),
	}, nil
}

// Load implements resolve.DataSource interface.
// It processes the input JSON data to make gRPC calls and returns
// the response data.
//
// Headers are converted to gRPC metadata and part of gRPC calls.
//
// The input is expected to contain the necessary information to make
// a gRPC call, including service name, method name, and request data.
func (d *DataSource) Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
	// get variables from input
	variables := gjson.Parse(unsafebytes.BytesToString(input)).Get("body.variables")

	var (
		poolItems []*arena.PoolItem
	)
	defer func() {
		d.pool.ReleaseMany(poolItems)
	}()

	item := d.acquirePoolItem(input, 0)
	poolItems = append(poolItems, item)
	builder := newJSONBuilder(item.Arena, d.mapping, variables)

	if d.disabled {
		return builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")), nil
	}

	// convert headers to grpc metadata and attach to ctx
	if len(headers) > 0 {
		md := make(metadata.MD, len(headers))
		for k, v := range headers {
			md.Set(strings.ToLower(k), v...)
		}
		ctx = metadata.NewOutgoingContext(ctx, md)
	}

	graph := NewDependencyGraph(d.plan)

	root := astjson.ObjectValue(nil)

	if err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
		serviceCalls, err := d.rc.CompileFetches(graph, nodes, variables)
		if err != nil {
			return err
		}

		results := make([]resultData, len(serviceCalls))
		errGrp, errGrpCtx := errgroup.WithContext(ctx)

		// make gRPC calls
		for index, serviceCall := range serviceCalls {
			item := d.acquirePoolItem(input, index)
			poolItems = append(poolItems, item)
			builder := newJSONBuilder(item.Arena, d.mapping, variables)
			errGrp.Go(func() error {
				// Invoke the gRPC method - this will populate serviceCall.Output

				err := d.cc.Invoke(errGrpCtx, serviceCall.MethodFullName(), serviceCall.Input, serviceCall.Output)
				if err != nil {
					return err
				}

				response, err := builder.marshalResponseJSON(&serviceCall.RPC.Response, serviceCall.Output)
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
			return err
		}

		for _, result := range results {
			switch result.kind {
			case CallKindResolve:
				err = builder.mergeWithPath(root, result.response, result.responsePath)
			default:
				root, err = builder.mergeValues(root, result.response)
			}
			if err != nil {
				return err
			}
		}

		return nil
	}); err != nil {
		return builder.writeErrorBytes(err), nil
	}

	value := builder.toDataObject(root)
	return value.MarshalTo(nil), err
}

func (d *DataSource) acquirePoolItem(input []byte, index int) *arena.PoolItem {
	keyGen := xxhash.New()
	_, _ = keyGen.Write(input)
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(index))
	_, _ = keyGen.Write(b[:])
	key := keyGen.Sum64()
	item := d.pool.Acquire(key)
	return item
}

// LoadWithFiles implements resolve.DataSource interface.
// Similar to Load, but handles file uploads if needed.
//
// Note: File uploads are typically not part of gRPC, so this method
// might not be applicable for most gRPC use cases.
//
// Currently unimplemented.
func (d *DataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (data []byte, err error) {
	panic("unimplemented")
}
