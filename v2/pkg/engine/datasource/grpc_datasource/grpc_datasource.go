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
	"encoding/binary"
	"fmt"
	"net/http"
	"strings"

	"github.com/cespare/xxhash/v2"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type fetchResult struct {
	kind            CallKind
	responseMessage *dynamicpb.Message
	response        *astjson.Value
	responsePath    ast.Path
	entityIndexMap  entityIndexMap
	skipped         bool
}

// Verify DataSource implements the resolve.DataSource interface
var _ resolve.DataSource = (*DataSource)(nil)

// DataSource implements the resolve.DataSource interface for gRPC services.
// It handles the conversion of GraphQL queries to gRPC requests and
// transforms the responses back to GraphQL format.
type DataSource struct {
	plan              *RPCExecutionPlan
	transport         RPCTransport
	mapping           *GRPCMapping
	federationConfigs plan.FederationFieldConfigurations
	definition        *ast.Document
	disabled          bool

	pool     *arena.Pool
	program  *program
	codecOpt grpc.CallOption
	wireBuf  bytes.Buffer
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

// NewDataSource creates a new datasource with the given RPCTransport.
func NewDataSource(transport RPCTransport, config DataSourceConfig) (*DataSource, error) {
	planner, err := NewPlanner(config.SubgraphName, config.Mapping, config.FederationConfigs)
	if err != nil {
		return nil, err
	}
	plan, err := planner.PlanOperation(config.Operation, config.Definition)
	if err != nil {
		return nil, err
	}
	program, err := compileProgram(plan, config.Compiler.runtime)
	if err != nil {
		return nil, err
	}

	return &DataSource{
		plan:              plan,
		transport:         transport,
		mapping:           config.Mapping,
		definition:        config.Definition,
		federationConfigs: config.FederationConfigs,
		disabled:          config.Disabled,
		pool:              arena.NewArenaPool(),
		program:           program,
		codecOpt:          grpc.ForceCodecV2(&connectCodec{}),
	}, nil
}

// Load implements resolve.DataSource interface.
// It processes the input JSON data to make gRPC calls and returns
// the response data.
//
// Headers are converted to gRPC metadata and are part of gRPC calls.
//
// The input is expected to contain the necessary information to make
// a gRPC call, including service name, method name, and request data.
func (d *DataSource) Load(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
	// convert headers to grpc metadata and attach to ctx
	// TODO: ConnectRPC will have to handle headers differently when using a http client.
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

	switch d.transport.(type) {
	case *grpcTransport:
		return d.loadWithGRPC(ctx, input)
	default:
		return d.loadWithConnect(ctx, input)
	}
}

func (d *DataSource) loadWithGRPC(ctx context.Context, input []byte) (data []byte, err error) {
	return d.execute(ctx, input, wireRequestBuilder)
}

// loadWithConnect is the Connect-style load path. It walks the precompiled
// program stages (same as loadWithGRPC) but builds real protoref.Message values
// instead of raw wire bytes, so transports that need proto.Marshal /
// protojson.Marshal (e.g. the Connect client) can serialize them directly.
func (d *DataSource) loadWithConnect(ctx context.Context, input []byte) (data []byte, err error) {
	return d.execute(ctx, input, messageRequestBuilder)
}

// execute walks the precompiled program stages, running each stage's fetches in
// parallel via runFetch and merging their responses into a single result object.
// The only thing that varies between transports is buildRequest.
func (d *DataSource) execute(ctx context.Context, input []byte, buildRequest requestBuilder) (data []byte, err error) {
	var poolItems []*arena.PoolItem
	defer func() {
		d.pool.ReleaseMany(poolItems)
	}()

	item := d.acquirePoolItem(input, 0)
	poolItems = append(poolItems, item)

	value, err := astjson.ParseBytesWithArena(item.Arena, input)
	if err != nil {
		return nil, err
	}

	if value.Exists("body") {
		value = value.Get("body")
	}

	astJsonVariables := value.Get("variables")
	if !value.Exists() {
		return nil, fmt.Errorf("variables are required")
	}

	builder := newJSONBuilder(item.Arena, d.mapping)

	if d.disabled {
		return builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")), nil
	}

	root := astjson.ObjectValue(nil)
	callMap := make(map[int]fetchResult)
	representations := getRepresentations(astJsonVariables)

	for _, stage := range d.program.stages {
		results := make([]fetchResult, len(stage.fetches))

		errGrp, errCtx := errgroup.WithContext(ctx)

		for i, fetch := range stage.fetches {
			// Each fetch gets its own arena so the request builder and
			// marshalResponseJSON never share allocator state across goroutines.
			fetchItem := d.acquirePoolItem(input, fetch.id+1)
			poolItems = append(poolItems, fetchItem)
			fetchBuilder := newJSONBuilder(fetchItem.Arena, d.mapping)

			errGrp.Go(func() error {
				result, err := d.runFetch(
					errCtx,
					&fetch,
					fetchItem.Arena,
					fetchBuilder,
					callMap,
					astJsonVariables,
					representations,
					buildRequest,
				)
				if err != nil {
					return err
				}
				results[i] = result
				return nil
			})
		}

		if err := errGrp.Wait(); err != nil {
			return builder.writeErrorBytes(err), nil
		}

		// Populate callMap and merge into root serially — no concurrent map access.
		for i, result := range results {
			callMap[stage.fetches[i].id] = result

			if result.skipped {
				continue
			}

			switch result.kind {
			case CallKindResolve, CallKindRequired:
				err = builder.mergeWithPath(root, result.response, result.responsePath)
			default:
				root, err = builder.mergeValues(root, result)
			}
			if err != nil {
				return builder.writeErrorBytes(err), nil
			}
		}
	}

	resultValue := builder.toDataObject(root)
	return resultValue.MarshalTo(nil), err
}

// runFetch executes one fetch: build request → invoke transport → marshal response → validate entity.
// The caller owns the per-fetch arena and builder so parallel callers do not share allocator state.
func (d *DataSource) runFetch(
	ctx context.Context,
	fetch *fetchProgram,
	a arena.Arena,
	fetchBuilder *jsonBuilder,
	callMap map[int]fetchResult,
	astJsonVariables *astjson.Value,
	representations []*astjson.Value,
	buildRequest requestBuilder,
) (fetchResult, error) {
	request, skip, err := buildRequest(a, fetch, callMap, astJsonVariables)
	if err != nil {
		return fetchResult{}, err
	}

	if skip {
		return fetchResult{
			kind:         fetch.kind,
			responsePath: fetch.responsePath,
			skipped:      true,
		}, nil
	}

	responseMessage := dynamicpb.NewMessage(fetch.response.responseType.desc)
	if err := d.transport.Invoke(ctx, fetch.methodFullName, request, responseMessage); err != nil {
		return fetchResult{}, err
	}

	responseJson, err := fetchBuilder.marshalResponseJSON(&fetch.response.rpcMessage, responseMessage)
	if err != nil {
		return fetchResult{}, err
	}

	result := fetchResult{
		kind:            fetch.kind,
		response:        responseJson,
		responseMessage: responseMessage,
		responsePath:    fetch.responsePath,
	}

	// In case of a federated response, we need to ensure that the response is valid.
	// The number of entities per type must match the number of lookup keys in the variables.
	// On success we build the index map used by mergeEntities to place each response
	// entity at the correct position in the final _entities array.
	if fetch.kind == CallKindEntity {
		if err := validateEntityResponse(responseJson, fetch.requestedEntityType, representations); err != nil {
			return fetchResult{}, err
		}
		result.entityIndexMap = newEntityIndexMap(fetch.requestedEntityType, representations)
	}

	return result, nil
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

// requestBuilder produces the per-fetch request value passed to RPCTransport.Invoke.
// loadWithGRPC supplies wireRequestBuilder (pre-encoded bytes wrapped in PreWiredInputMessage);
// loadWithConnect supplies messageRequestBuilder (a populated protoref.Message).
type requestBuilder func(a arena.Arena, fetch *fetchProgram, callMap map[int]fetchResult, requestVariables *astjson.Value) (any, bool, error)

// wireRequestBuilder builds a pre-encoded gRPC request for the fetch, ready to be
// handed to RPCTransport.Invoke via PreWiredInputMessage.
func wireRequestBuilder(a arena.Arena, fetch *fetchProgram, callMap map[int]fetchResult, requestVariables *astjson.Value) (any, bool, error) {
	var buffer []byte
	var err error

	switch fetch.kind {
	case CallKindStandard:
		buffer, err = fetch.request.createProtoWire(requestVariables)
		if err != nil {
			return nil, false, err
		}
	case CallKindEntity, CallKindRequired:
		if fetch.requestedEntityType != "" {
			requestVariables = filterRepresentations(a, requestVariables, fetch.requestedEntityType)
		}

		buffer, err = fetch.request.createProtoWire(requestVariables)
		if err != nil {
			return nil, false, err
		}
	case CallKindResolve:
		contextFetch, found := callMap[fetch.dependentCall.ID]
		if !found {
			return nil, false, fmt.Errorf("context fetch not found for dependent call %d", fetch.dependentCall.ID)
		}

		if contextFetch.responseMessage == nil || contextFetch.skipped {
			return nil, true, nil
		}

		buffer, err = fetch.request.createProtoWireWithContext(a, requestVariables, contextFetch.responseMessage)
		if err != nil {
			if err == errShouldSkip {
				return nil, true, nil
			}

			return nil, false, err
		}
	}

	return NewPreWiredInputMessage(buffer), false, nil
}

// messageRequestBuilder is the proto-message counterpart of wireRequestBuilder. It
// dispatches on fetch.kind and returns a populated dynamicpb message ready to
// be marshaled by a Connect-style transport.
func messageRequestBuilder(a arena.Arena, fetch *fetchProgram, callMap map[int]fetchResult, requestVariables *astjson.Value) (any, bool, error) {
	var message protoref.Message
	var err error

	switch fetch.kind {
	case CallKindStandard:
		message, err = fetch.request.createProtoMessage(requestVariables)
		if err != nil {
			return nil, false, err
		}
	case CallKindEntity, CallKindRequired:
		if fetch.requestedEntityType != "" {
			requestVariables = filterRepresentations(a, requestVariables, fetch.requestedEntityType)
		}

		message, err = fetch.request.createProtoMessage(requestVariables)
		if err != nil {
			return nil, false, err
		}
	case CallKindResolve:
		contextFetch, found := callMap[fetch.dependentCall.ID]
		if !found {
			return nil, false, fmt.Errorf("context fetch not found for dependent call %d", fetch.dependentCall.ID)
		}

		if contextFetch.responseMessage == nil || contextFetch.skipped {
			return nil, true, nil
		}

		message, err = fetch.request.createProtoMessageWithContext(a, requestVariables, contextFetch.responseMessage)
		if err != nil {
			if err == errShouldSkip {
				return nil, true, nil
			}
			return nil, false, err
		}
	}

	return message, false, nil
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
