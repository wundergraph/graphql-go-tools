// Package grpcdatasource provides a GraphQL datasource implementation for gRPC services.
// It allows GraphQL servers to connect to gRPC backends and execute RPC calls
// as part of GraphQL query resolution.
//
// The package includes tools for parsing Protobuf definitions, building execution plans,
// and converting GraphQL queries into gRPC requests.
package grpcdatasource

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"

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

	// graphPool recycles DependencyGraph instances built from d.plan. Plan is immutable
	// so the graph's nodes+fetches slices are reusable; only per-request ServiceCall
	// pointers are cleared on put via resetForReuse.
	graphPool sync.Pool

	// methodFullNames caches "/ServiceName/MethodName" strings per RPCCall.ID.
	// Avoids the per-Load strings.Builder allocation in ServiceCall.MethodFullName.
	// Built eagerly at NewDataSource; plan.Calls is immutable so the cache is safe
	// to share across all requests without locking.
	methodFullNames map[int]string
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

	ds := &DataSource{
		plan:              plan,
		cc:                client,
		rc:                config.Compiler,
		mapping:           config.Mapping,
		federationConfigs: config.FederationConfigs,
		disabled:          config.Disabled,
		pool:              arena.NewArenaPool(),
	}
	ds.graphPool = sync.Pool{
		New: func() any { return NewDependencyGraph(ds.plan) },
	}

	// Precompute gRPC method full names for every RPC call in the plan. Requires a
	// compiler lookup to resolve the full service name, so it's done once here rather
	// than on every Load. The built values are stable for the plan's lifetime.
	ds.methodFullNames = make(map[int]string, len(plan.Calls))
	for i := range plan.Calls {
		call := &plan.Calls[i]
		serviceName := call.ServiceName
		if svc := config.Compiler.doc.ServiceByName(call.ServiceName); svc != nil {
			serviceName = svc.FullName
		} else {
			// Fallback matching resolveServiceName's secondary lookup by method name.
			for _, svc := range config.Compiler.doc.Services {
				for _, methodRef := range svc.MethodsRefs {
					if config.Compiler.doc.Methods[methodRef].Name == call.MethodName {
						serviceName = svc.FullName
						break
					}
				}
			}
		}
		ds.methodFullNames[call.ID] = "/" + serviceName + "/" + call.MethodName
	}

	// V1 is the correctness fallback (simple dynamicpb only).
	// V2 (DataSourceV2) is the performance path.
	return ds, nil
}

// acquirePoolItemForIndex mixes the precomputed base hash with an index to pick a pool arena.
// Using a stateless splitmix-style mix avoids allocating an *xxhash.Digest per call site.
func (d *DataSource) acquirePoolItemForIndex(baseKey uint64, index int) *arena.PoolItem {
	const golden = 0x9E3779B97F4A7C15 // 2^64 / phi, standard mixing constant
	key := baseKey ^ (uint64(index)+1)*golden
	return d.pool.Acquire(key)
}

// loadScratch carries per-Load scratch slices that are reused via sync.Pool.
// Keeps the poolItems slice out of the Load stack frame so the underlying array
// can be reused across requests.
type loadScratch struct {
	poolItems []*arena.PoolItem
}

var loadScratchPool = sync.Pool{
	New: func() any {
		return &loadScratch{poolItems: make([]*arena.PoolItem, 0, 8)}
	},
}

// Load implements resolve.DataSource interface.
// It processes the input JSON data to make gRPC calls and returns
// the response data.
//
// Headers are converted to gRPC metadata and part of gRPC calls.
//
// The input is expected to contain the necessary information to make
// a gRPC call, including service name, method name, and request data.
func (d *DataSource) Load(ctx context.Context, headers http.Header, input []byte) (*astjson.Value, func(), error) {
	// get variables from input
	variables := gjson.Parse(unsafebytes.BytesToString(input)).Get("body.variables")

	// Precompute the federation entity index map once per request. Previously this was
	// recomputed inside every newJSONBuilder call (once per service call), even though
	// the representations are identical across calls in a single Load.
	im := createRepresentationIndexMap(variables)

	// Compute the arena-pool base key once; per-call keys are derived by index mixing.
	baseKey := xxhash.Sum64(input)

	// Resources acquired during Load and held until the loader finishes with the
	// returned astjson.Value. The cleanup func returned from Load releases them.
	// On error paths we invoke cleanup directly before returning.
	scratch := loadScratchPool.Get().(*loadScratch)
	var graph *DependencyGraph
	var rootBuilder *jsonBuilder

	cleanup := func() {
		d.pool.ReleaseMany(scratch.poolItems)
		scratch.poolItems = scratch.poolItems[:0]
		loadScratchPool.Put(scratch)
		if graph != nil {
			graph.resetForReuse()
			d.graphPool.Put(graph)
		}
		if rootBuilder != nil {
			releaseJSONBuilder(rootBuilder)
		}
	}
	// failCleanup runs cleanup on any failure path before returning so we don't
	// leak pooled resources back to callers that never saw a value.
	done := false
	defer func() {
		if !done {
			cleanup()
		}
	}()

	if d.disabled {
		item := d.acquirePoolItemForIndex(baseKey, 0)
		scratch.poolItems = append(scratch.poolItems, item)
		builder := acquireJSONBuilderWithIndexMap(item.Arena, d.mapping, variables, im)
		errBytes := builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used"))
		releaseJSONBuilder(builder)
		v, parseErr := astjson.ParseBytesWithArena(item.Arena, errBytes)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		done = true
		return v, cleanup, nil
	}

	// convert headers to grpc metadata and attach to ctx
	if len(headers) > 0 {
		// assume that each header has exactly one value for default pairs size
		pairs := make([]string, 0, len(headers)*2)
		for headerName, headerValues := range headers {
			headerName = strings.ToLower(headerName)
			for _, v := range headerValues {
				pairs = append(pairs, headerName, v)
			}
		}
		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
	}

	graph = d.graphPool.Get().(*DependencyGraph)

	// Arena used for top-level assembly (root object, final merge). Separate from the
	// per-service-call arenas so large per-call allocations can be reclaimed independently.
	rootItem := d.acquirePoolItemForIndex(baseKey, 0)
	scratch.poolItems = append(scratch.poolItems, rootItem)
	rootBuilder = acquireJSONBuilderWithIndexMap(rootItem.Arena, d.mapping, variables, im)
	// root is the in-progress merged response tree. Rooted on rootItem.Arena.
	// The final data envelope `{"data": root}` is constructed via rootBuilder.toDataObject.
	root := astjson.ObjectValue(rootItem.Arena)

	// Shared invokeOne helper used by both single-call and multi-call paths. Writes
	// the decoded response into *out. For single call, *out is a stack-allocated
	// resultData; for multi call it's an element of the per-batch results slice.
	invokeOne := func(serviceCall ServiceCall, callCtx context.Context, builder *jsonBuilder, out *resultData) error {
		if err := d.cc.Invoke(callCtx, d.methodFullNames[serviceCall.RPC.ID], serviceCall.Input, serviceCall.Output); err != nil {
			return err
		}

		response, err := builder.marshalResponseJSON(&serviceCall.RPC.Response, serviceCall.Output)
		if err != nil {
			return err
		}

		if serviceCall.RPC.Kind == CallKindEntity {
			if err := builder.validateFederatedResponse(response); err != nil {
				return err
			}
		}

		*out = resultData{
			kind:         serviceCall.RPC.Kind,
			response:     response,
			responsePath: serviceCall.RPC.ResponsePath,
		}
		return nil
	}

	mergeResult := func(r *resultData) error {
		var err error
		switch r.kind {
		case CallKindResolve, CallKindRequired:
			err = rootBuilder.mergeWithPath(root, r.response, r.responsePath)
		default:
			root, err = rootBuilder.mergeValues(root, r.response)
		}
		return err
	}

	if err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
		serviceCalls, err := d.rc.CompileFetches(graph, nodes, variables)
		if err != nil {
			return err
		}

		// Single-call fast path. Avoids: errgroup, goroutine, per-batch slice allocations
		// (results, callBuilders). The entire batch runs inline.
		// This is the overwhelmingly common case for simple queries.
		if len(serviceCalls) == 1 {
			sc := &serviceCalls[0]
			item := d.acquirePoolItemForIndex(baseKey, 1)
			scratch.poolItems = append(scratch.poolItems, item)
			builder := acquireJSONBuilderWithIndexMap(item.Arena, d.mapping, variables, im)
			defer releaseJSONBuilder(builder)

			var result resultData
			if err := invokeOne(*sc, ctx, builder, &result); err != nil {
				return err
			}
			return mergeResult(&result)
		}

		results := make([]resultData, len(serviceCalls))
		callBuilders := make([]*jsonBuilder, 0, len(serviceCalls))
		defer func() {
			for _, b := range callBuilders {
				releaseJSONBuilder(b)
			}
		}()

		errGrp, errGrpCtx := errgroup.WithContext(ctx)
		for index, serviceCall := range serviceCalls {
			item := d.acquirePoolItemForIndex(baseKey, index+1)
			scratch.poolItems = append(scratch.poolItems, item)
			builder := acquireJSONBuilderWithIndexMap(item.Arena, d.mapping, variables, im)
			callBuilders = append(callBuilders, builder)
			errGrp.Go(func() error {
				return invokeOne(serviceCall, errGrpCtx, builder, &results[index])
			})
		}
		if err := errGrp.Wait(); err != nil {
			return err
		}

		for i := range results {
			if err := mergeResult(&results[i]); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		errBytes := rootBuilder.writeErrorBytes(err)
		v, parseErr := astjson.ParseBytesWithArena(rootItem.Arena, errBytes)
		if parseErr != nil {
			return nil, nil, parseErr
		}
		done = true
		return v, cleanup, nil
	}

	// Build the {"data": root} envelope on our arena and hand ownership to the
	// caller. Cleanup releases the pool items / builders / shareds after the
	// loader has finished reading the Value.
	value := rootBuilder.toDataObject(root)
	done = true
	return value, cleanup, nil
}

// LoadWithFiles implements resolve.DataSource interface.
// Similar to Load, but handles file uploads if needed.
//
// Note: File uploads are typically not part of gRPC, so this method
// might not be applicable for most gRPC use cases.
//
// Currently unimplemented.
func (d *DataSource) LoadWithFiles(ctx context.Context, headers http.Header, input []byte, files []*httpclient.FileUpload) (*astjson.Value, func(), error) {
	panic("unimplemented")
}

// SkipsHTTPResponseContext implements resolve.ContextSkippingDataSource. gRPC
// responses don't produce HTTP response contexts (status codes, response objects)
// so the loader can skip the per-fetch httpclient.InjectResponseContext call
// and its associated context.WithValue allocation.
func (d *DataSource) SkipsHTTPResponseContext() {}
