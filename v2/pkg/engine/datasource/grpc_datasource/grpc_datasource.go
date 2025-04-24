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
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"google.golang.org/grpc"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
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
	Mapping      *GRPCMapping
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
			a := astjson.Arena{}
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

			responseJSON := marshalResponseJSON(&a, &invocation.Call.Response, invocation.Output)

			// TODO we need to build the response based on the mapping rules
			root := a.NewObject()
			root.Set("data", responseJSON)

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

func marshalResponseJSON(arena *astjson.Arena, structure *RPCMessage, data protoref.Message) *astjson.Value {
	root := arena.NewObject()

	fmt.Println("data", data)

	for _, field := range structure.Fields {
		fd := data.Descriptor().Fields().ByNumber(protoref.FieldNumber(field.Index + 1))
		if fd == nil {
			continue
		}

		if fd.IsList() {
			arr := arena.NewArray()
			root.Set(field.JSONPath, arr)
			list := data.Get(fd).List()
			for i := 0; i < list.Len(); i++ {
				message := list.Get(i).Message()
				value := marshalResponseJSON(arena, field.Message, message)
				arr.SetArrayItem(i, value)
			}

			continue
		}

		if fd.Kind() == protoref.MessageKind {
			message := data.Get(fd).Message()
			value := marshalResponseJSON(arena, field.Message, message)
			root.Set(field.JSONPath, value)

			continue
		}

		setJSONValue(arena, root, field.JSONPath, data, fd)
	}

	return root
}

func setJSONValue(arena *astjson.Arena, root *astjson.Value, name string, data protoref.Message, fd protoref.FieldDescriptor) {
	switch fd.Kind() {
	case protoref.BoolKind:
		boolValue := data.Get(fd).Bool()
		if boolValue {
			root.Set(name, arena.NewTrue())
		} else {
			root.Set(name, arena.NewFalse())
		}
	case protoref.StringKind:
		root.Set(name, arena.NewString(data.Get(fd).String()))
	case protoref.Int32Kind, protoref.Int64Kind:
		root.Set(name, arena.NewNumberInt(int(data.Get(fd).Int())))
	case protoref.Uint32Kind, protoref.Uint64Kind:
		root.Set(name, arena.NewNumberString(fmt.Sprintf("%d", data.Get(fd).Uint())))
	case protoref.FloatKind, protoref.DoubleKind:
		root.Set(name, arena.NewNumberFloat64(data.Get(fd).Float()))
	case protoref.BytesKind:
		root.Set(name, arena.NewStringBytes(data.Get(fd).Bytes()))
	case protoref.EnumKind:
		enumDesc := fd.Enum()
		enumNumber := data.Get(fd).Enum()
		enumValueDesc := enumDesc.Values().ByNumber(enumNumber)
		if enumValueDesc == nil {
			root.Set(name, arena.NewNull())
			return
		}

		root.Set(name, arena.NewString(string(enumValueDesc.Name())))
	}
}
