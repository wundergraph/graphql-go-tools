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
	"github.com/tidwall/gjson"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type fetchData struct {
	kind           CallKind
	response       *astjson.Value
	responsePath   ast.Path
	entityIndexMap entityIndexMap
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
	program, err := compileProgram(plan, config.Compiler.runtime)
	if err != nil {
		return nil, err
	}

	return &DataSource{
		plan:              plan,
		cc:                client,
		rc:                config.Compiler,
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
	var poolItems []*arena.PoolItem
	defer func() {
		d.pool.ReleaseMany(poolItems)
	}()

	item := d.acquirePoolItem(input, 0)
	poolItems = append(poolItems, item)
	// get variables from input
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

	variables := gjson.ParseBytes(input).Get("body.variables")

	builder := newJSONBuilder(item.Arena, d.mapping, variables)

	if d.disabled {
		return builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")), nil
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

	root := astjson.ObjectValue(nil)

	callMap := make(map[int]fetchData)

	representations := getRepresentations(astJsonVariables)
	for _, stage := range d.program.stages {
		results := make([]fetchData, 0, len(stage.fetches))

		for _, fetch := range stage.fetches {
			// TODO: unmarshal with our own codec logic
			responseMessage := dynamicpb.NewMessage(fetch.response.responseType.desc)

			requestVariables := astJsonVariables
			if fetch.requestedEntityType != "" {
				requestVariables = filterRepresentations(item.Arena, requestVariables, fetch.requestedEntityType)
			}

			// if fetch.dependentCall != nil {
			// 	requestVariables = astjson.DeepCopy(item.Arena, astJsonVariables)

			// 	call, found := callMap[fetch.dependentCall.ID]
			// 	if !found {
			// 		return nil, fmt.Errorf("dependent call %d not found", fetch.dependentCall.ID)
			// 	}

			// 	contextField := fetch.request.rpcMessage.Fields.ByName(contextFieldName)
			// 	if contextField == nil {
			// 		return nil, fmt.Errorf("context field not found in dependent call %d", fetch.dependentCall.ID)
			// 	}

			// 	contextValue := call.response.Get(contextField.JSONPath)
			// 	if !contextValue.Exists() {
			// 		return nil, fmt.Errorf("context value not found in dependent call %d", fetch.dependentCall.ID)
			// 	}

			// 	var contextData []*astjson.Value
			// 	if contextValue.Type() == astjson.TypeArray {
			// 		contextData = contextValue.GetArray()
			// 	} else {
			// 		contextData = []*astjson.Value{contextValue}
			// 	}

			// 	ov := astjson.ObjectValue(item.Arena)
			// 	contextArr := astjson.ArrayValue(item.Arena)
			// 	for i, data := range contextData {
			// 		contextArr.SetArrayItem(item.Arena, i, data)
			// 	}
			// 	ov.Set(item.Arena, contextField.JSONPath, contextArr)

			// 	requestVariables, _, err = astjson.MergeValues(item.Arena, requestVariables, ov)
			// 	if err != nil {
			// 		return nil, err
			// 	}
			// }

			buffer, err := fetch.request.createProtoWire(requestVariables)
			if err != nil {
				return nil, err
			}

			err = d.cc.Invoke(ctx, fetch.methodFullName, NewPreWiredInputMessage(buffer), responseMessage)
			if err != nil {
				return builder.writeErrorBytes(err), nil
			}

			responseJson, err := builder.marshalResponseJSON(&fetch.response.rpcMessage, responseMessage)
			if err != nil {
				return builder.writeErrorBytes(err), nil
			}

			fetchResult := fetchData{
				kind:         fetch.kind,
				response:     responseJson,
				responsePath: fetch.responsePath,
			}

			// In case of a federated response, we need to ensure that the response is valid.
			// The number of entities per type must match the number of lookup keys in the variables.
			// On success we build the index map used by mergeEntities to place each response
			// entity at the correct position in the final _entities array.
			if fetch.kind == CallKindEntity {
				if err := validateEntityResponse(responseJson, fetch.requestedEntityType, representations); err != nil {
					return builder.writeErrorBytes(err), nil
				}

				fetchResult.entityIndexMap = newEntityIndexMap(fetch.requestedEntityType, representations)
			}

			results = append(results, fetchResult)
			callMap[fetch.id] = fetchResult
		}

		for _, result := range results {
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

// func (d *DataSource) LoadOld(ctx context.Context, headers http.Header, input []byte) (data []byte, err error) {
// 	// get variables from input
// 	variables := gjson.Parse(unsafebytes.BytesToString(input)).Get("body.variables")

// 	var poolItems []*arena.PoolItem
// 	defer func() {
// 		d.pool.ReleaseMany(poolItems)
// 	}()

// 	item := d.acquirePoolItem(input, 0)
// 	poolItems = append(poolItems, item)

// 	builder := newJSONBuilder(item.Arena, d.mapping, variables)

// 	if d.disabled {
// 		return builder.writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")), nil
// 	}

// 	// convert headers to grpc metadata and attach to ctx
// 	if len(headers) > 0 {
// 		// assume that each header has exactly one value for default pairs size
// 		pairs := make([]string, 0, len(headers)*2)
// 		for headerName, headerValues := range headers {
// 			headerName = strings.ToLower(headerName)
// 			for _, v := range headerValues {
// 				pairs = append(pairs, headerName, v)
// 			}
// 		}
// 		ctx = metadata.AppendToOutgoingContext(ctx, pairs...)
// 	}

// 	graph := NewDependencyGraph(d.plan)

// 	root := astjson.ObjectValue(nil)

// 	representations := getRepresentations(variables)
// 	if err := graph.TopologicalSortResolve(func(nodes []FetchItem) error {
// 		// TODO: Compile fetches should be converted to a program.
// 		// The program defines all the fetches that need to be executed in parallel for a given query.

// 		serviceCalls, err := d.rc.CompileFetches(graph, nodes, variables)
// 		if err != nil {
// 			return err
// 		}

// 		results := make([]resultData, len(serviceCalls))
// 		errGrp, errGrpCtx := errgroup.WithContext(ctx)

// 		// make gRPC calls
// 		for index, serviceCall := range serviceCalls {
// 			item := d.acquirePoolItem(input, index)
// 			poolItems = append(poolItems, item)

// 			builder := newJSONBuilder(item.Arena, d.mapping, variables)
// 			errGrp.Go(func() error {
// 				// Invoke the gRPC method - this will populate serviceCall.Output
// 				err := d.cc.Invoke(errGrpCtx, serviceCall.MethodFullName(), serviceCall.Input, serviceCall.Output)
// 				if err != nil {
// 					return err
// 				}

// 				response, err := builder.marshalResponseJSON(&serviceCall.RPC.Response, serviceCall.Output)
// 				if err != nil {
// 					return err
// 				}

// 				results[index] = resultData{
// 					kind:         serviceCall.RPC.Kind,
// 					response:     response,
// 					responsePath: serviceCall.RPC.ResponsePath,
// 				}

// 				// In case of a federated response, we need to ensure that the response is valid.
// 				// The number of entities per type must match the number of lookup keys in the variables.
// 				// On success we build the index map used by mergeEntities to place each response
// 				// entity at the correct position in the final _entities array.
// 				if serviceCall.RPC.Kind == CallKindEntity {
// 					if err := validateEntityResponse(response, serviceCall.RPC.RequestedEntityType, representations); err != nil {
// 						return err
// 					}

// 					results[index].entityIndexMap = newEntityIndexMap(serviceCall.RPC.RequestedEntityType, representations)
// 				}

// 				return nil
// 			})
// 		}

// 		if err := errGrp.Wait(); err != nil {
// 			return err
// 		}

// 		for _, result := range results {
// 			switch result.kind {
// 			case CallKindResolve, CallKindRequired:
// 				err = builder.mergeWithPath(root, result.response, result.responsePath)
// 			default:
// 				root, err = builder.mergeValues(root, result)
// 			}
// 			if err != nil {
// 				return err
// 			}
// 		}

// 		return nil
// 	}); err != nil {
// 		return builder.writeErrorBytes(err), nil
// 	}

// 	value := builder.toDataObject(root)
// 	return value.MarshalTo(nil), err
// }
