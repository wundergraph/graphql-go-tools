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
	"errors"
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
)

// Verify DataSource implements the resolve.DataSource interface
var _ resolve.DataSource = (*DataSource)(nil)

// DataSource implements the resolve.DataSource interface for gRPC services.
// It handles the conversion of GraphQL queries to gRPC requests and
// transforms the responses back to GraphQL format.
type DataSource struct {
	// Invocations is a list of gRPC invocations to be executed
	plan     *RPCExecutionPlan
	cc       grpc.ClientConnInterface
	rc       *RPCCompiler
	mapping  *GRPCMapping
	disabled bool
}

type ProtoConfig struct {
	Schema string
}

type DataSourceConfig struct {
	Operation    *ast.Document
	Definition   *ast.Document
	Compiler     *RPCCompiler
	SubgraphName string
	Mapping      *GRPCMapping
	Disabled     bool
}

// NewDataSource creates a new gRPC datasource
func NewDataSource(client grpc.ClientConnInterface, config DataSourceConfig) (*DataSource, error) {
	planner := NewPlanner(config.SubgraphName, config.Mapping)
	plan, err := planner.PlanOperation(config.Operation, config.Definition)
	if err != nil {
		return nil, err
	}

	return &DataSource{
		plan:     plan,
		cc:       client,
		rc:       config.Compiler,
		mapping:  config.Mapping,
		disabled: config.Disabled,
	}, nil
}

// Load implements resolve.DataSource interface.
// It processes the input JSON data to make gRPC calls and writes
// the response to the output buffer.
//
// The input is expected to contain the necessary information to make
// a gRPC call, including service name, method name, and request data.
func (d *DataSource) Load(ctx context.Context, input []byte, out *bytes.Buffer) (err error) {
	if d.disabled {
		out.Write(writeErrorBytes(fmt.Errorf("gRPC datasource needs to be enabled to be used")))
		return nil
	}

	// get variables from input
	variables := gjson.Parse(string(input)).Get("body.variables")

	// get invocations from plan
	invocations, err := d.rc.Compile(d.plan, variables)
	if err != nil {
		return err
	}

	a := astjson.Arena{}
	root := a.NewObject()

	// make gRPC calls
	for _, invocation := range invocations {
		// Invoke the gRPC method - this will populate invocation.Output
		methodName := fmt.Sprintf("/%s/%s", invocation.ServiceName, invocation.MethodName)
		err := d.cc.Invoke(ctx, methodName, invocation.Input, invocation.Output)
		if err != nil {
			out.Write(writeErrorBytes(err))
			return nil
		}

		responseJSON, err := d.marshalResponseJSON(&a, &invocation.Call.Response, invocation.Output)
		if err != nil {
			return err
		}

		root, _, err = astjson.MergeValues(root, responseJSON)
		if err != nil {
			out.Write(writeErrorBytes(err))
			return nil
		}
	}

	data := a.NewObject()
	data.Set("data", root)
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

func (d *DataSource) marshalResponseJSON(arena *astjson.Arena, message *RPCMessage, data protoref.Message) (*astjson.Value, error) {
	if message == nil {
		return arena.NewNull(), nil
	}

	root := arena.NewObject()

	if message.IsOneOf() {
		oneof := data.Descriptor().Oneofs().ByName(protoref.Name(message.OneOfType.FieldName()))
		if oneof == nil {
			return nil, fmt.Errorf("unable to build response JSON: oneof %s not found in message %s", message.OneOfType.FieldName(), message.Name)
		}

		oneofDescriptor := data.WhichOneof(oneof)
		if oneofDescriptor == nil {
			return nil, fmt.Errorf("unable to build response JSON: oneof %s not found in message %s", message.OneOfType.FieldName(), message.Name)
		}

		if oneofDescriptor.Kind() == protoref.MessageKind {
			data = data.Get(oneofDescriptor).Message()
		}
	}

	validFields := message.Fields
	if message.IsOneOf() {
		validFields = append(validFields, message.FieldSelectionSet.SelectFieldsForTypes(message.SelectValidTypes(string(data.Type().Descriptor().Name())))...)
	}

	for _, field := range validFields {
		if field.StaticValue != "" {
			if len(message.MemberTypes) == 0 {
				root.Set(field.AliasOrPath(), arena.NewString(field.StaticValue))
				continue
			}

			for _, memberTypes := range message.MemberTypes {
				if memberTypes == string(data.Type().Descriptor().Name()) {
					root.Set(field.AliasOrPath(), arena.NewString(memberTypes))
					break
				}
			}

			continue
		}

		fd := data.Descriptor().Fields().ByName(protoref.Name(field.Name))
		if fd == nil {
			continue
		}

		if fd.IsList() {
			list := data.Get(fd).List()
			arr := arena.NewArray()
			root.Set(field.AliasOrPath(), arr)

			if !list.IsValid() {
				continue
			}

			for i := 0; i < list.Len(); i++ {

				switch fd.Kind() {
				case protoref.MessageKind:
					message := list.Get(i).Message()
					value, err := d.marshalResponseJSON(arena, field.Message, message)
					if err != nil {
						return nil, err
					}

					arr.SetArrayItem(i, value)
				default:
					d.setArrayItem(i, arena, arr, list.Get(i), fd)
				}

			}

			continue
		}

		if fd.Kind() == protoref.MessageKind {
			msg := data.Get(fd).Message()
			if !msg.IsValid() {
				root.Set(field.AliasOrPath(), arena.NewNull())
				continue
			}

			if field.IsListType {
				arr, err := d.flattenListStructure(arena, field.ListMetadata, msg, field.Message)
				if err != nil {
					return nil, fmt.Errorf("unable to flatten list structure for field %q: %w", field.AliasOrPath(), err)
				}

				root.Set(field.AliasOrPath(), arr)
				continue
			}

			if field.IsOptionalScalar() {
				err := d.resolveOptionalField(arena, root, field.JSONPath, msg)
				if err != nil {
					return nil, err
				}

				continue
			}

			value, err := d.marshalResponseJSON(arena, field.Message, msg)
			if err != nil {
				return nil, err
			}

			if field.JSONPath == "" {
				root, _, err = astjson.MergeValues(root, value)
				if err != nil {
					return nil, err
				}
			} else {
				root.Set(field.AliasOrPath(), value)
			}

			continue
		}

		d.setJSONValue(arena, root, field.AliasOrPath(), data, fd)
	}

	return root, nil
}

func (d *DataSource) flattenListStructure(arena *astjson.Arena, md *ListMetadata, data protoref.Message, message *RPCMessage) (*astjson.Value, error) {
	if md == nil {
		return arena.NewNull(), errors.New("list metadata not found")
	}

	if len(md.LevelInfo) < md.NestingLevel {
		return arena.NewNull(), errors.New("nesting level data does not match the number of levels in the list metadata")
	}

	if !data.IsValid() {
		if md.LevelInfo[0].Optional {
			return arena.NewNull(), nil
		}

		return arena.NewNull(), errors.New("cannot add null item to response for non nullable list")
	}

	root := arena.NewArray()
	return d.traverseList(0, arena, root, md, data, message)
}

func (d *DataSource) traverseList(level int, arena *astjson.Arena, current *astjson.Value, md *ListMetadata, data protoref.Message, message *RPCMessage) (*astjson.Value, error) {
	if level > md.NestingLevel {
		return current, nil
	}

	// List wrappers always use field number 1
	fd := data.Descriptor().Fields().ByNumber(1)
	if fd == nil {
		return arena.NewNull(), fmt.Errorf("field with number %d not found in message %q", 1, data.Descriptor().Name())
	}

	if fd.Kind() != protoref.MessageKind {
		return arena.NewNull(), fmt.Errorf("field %q is not a message", fd.Name())
	}

	msg := data.Get(fd).Message()
	if !msg.IsValid() {
		// If the message is not valid we can either return null if the list is nullable or an error if it is non nullable.
		if md.LevelInfo[level].Optional {
			return arena.NewNull(), nil
		}

		return arena.NewArray(), fmt.Errorf("cannot add null item to response for non nullable list")
	}

	fd = msg.Descriptor().Fields().ByNumber(1)
	if !fd.IsList() {
		return arena.NewNull(), fmt.Errorf("field %q is not a list", fd.Name())
	}

	if level < md.NestingLevel-1 {
		list := msg.Get(fd).List()
		for i := 0; i < list.Len(); i++ {
			next := arena.NewArray()
			val, err := d.traverseList(level+1, arena, next, md, list.Get(i).Message(), message)
			if err != nil {
				return nil, err
			}

			current.SetArrayItem(i, val)
		}

		return current, nil
	}

	list := msg.Get(fd).List()
	if !list.IsValid() {
		// If the list is not valid, we return an empty array here as the
		// nullabilty is checked on the outer List wrapper type.
		return arena.NewArray(), nil
	}

	for i := 0; i < list.Len(); i++ {
		if message != nil {
			val, err := d.marshalResponseJSON(arena, message, list.Get(i).Message())
			if err != nil {
				return nil, err
			}

			current.SetArrayItem(i, val)
		} else {
			d.setArrayItem(i, arena, current, list.Get(i), fd)
		}
	}

	return current, nil
}

func (d *DataSource) resolveOptionalField(arena *astjson.Arena, root *astjson.Value, name string, data protoref.Message) error {
	fd := data.Descriptor().Fields().ByName(protoref.Name("value"))
	if fd == nil {
		return fmt.Errorf("unable to resolve optional field: field %q not found in message %s", "value", data.Descriptor().Name())
	}

	d.setJSONValue(arena, root, name, data, fd)
	return nil
}

func (d *DataSource) setJSONValue(arena *astjson.Arena, root *astjson.Value, name string, data protoref.Message, fd protoref.FieldDescriptor) {
	if !data.IsValid() {
		root.Set(name, arena.NewNull())
		return
	}

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
		root.Set(name, arena.NewNumberString(strconv.FormatUint(data.Get(fd).Uint(), 10)))
	case protoref.FloatKind, protoref.DoubleKind:
		root.Set(name, arena.NewNumberFloat64(data.Get(fd).Float()))
	case protoref.BytesKind:
		root.Set(name, arena.NewStringBytes(data.Get(fd).Bytes()))
	case protoref.EnumKind:
		enumDesc := fd.Enum()
		enumValueDesc := enumDesc.Values().ByNumber(data.Get(fd).Enum())
		if enumValueDesc == nil {
			root.Set(name, arena.NewNull())
			return
		}

		graphqlValue, ok := d.mapping.ResolveEnumValue(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			root.Set(name, arena.NewNull())
			return
		}

		root.Set(name, arena.NewString(graphqlValue))
	}
}

func (d *DataSource) setArrayItem(index int, arena *astjson.Arena, array *astjson.Value, data protoref.Value, fd protoref.FieldDescriptor) {
	if !data.IsValid() {
		array.SetArrayItem(index, arena.NewNull())
		return
	}

	switch fd.Kind() {
	case protoref.BoolKind:
		boolValue := data.Bool()
		if boolValue {
			array.SetArrayItem(index, arena.NewTrue())
		} else {
			array.SetArrayItem(index, arena.NewFalse())
		}
	case protoref.StringKind:
		array.SetArrayItem(index, arena.NewString(data.String()))
	case protoref.Int32Kind, protoref.Int64Kind:
		array.SetArrayItem(index, arena.NewNumberInt(int(data.Int())))
	case protoref.Uint32Kind, protoref.Uint64Kind:
		array.SetArrayItem(index, arena.NewNumberString(strconv.FormatUint(data.Uint(), 10)))
	case protoref.FloatKind, protoref.DoubleKind:
		array.SetArrayItem(index, arena.NewNumberFloat64(data.Float()))
	case protoref.BytesKind:
		array.SetArrayItem(index, arena.NewStringBytes(data.Bytes()))
	case protoref.EnumKind:
		enumDesc := fd.Enum()
		enumValueDesc := enumDesc.Values().ByNumber(data.Enum())
		if enumValueDesc == nil {
			array.SetArrayItem(index, arena.NewNull())
			return
		}

		graphqlValue, ok := d.mapping.ResolveEnumValue(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			array.SetArrayItem(index, arena.NewNull())
			return
		}

		array.SetArrayItem(index, arena.NewString(graphqlValue))
	}
}

func writeErrorBytes(err error) []byte {
	a := astjson.Arena{}
	errorRoot := a.NewObject()
	errorArray := a.NewArray()
	errorRoot.Set("errors", errorArray)

	errorItem := a.NewObject()
	errorItem.Set("message", a.NewString(err.Error()))

	extensions := a.NewObject()
	if st, ok := status.FromError(err); ok {
		extensions.Set("code", a.NewString(st.Code().String()))
	} else {
		extensions.Set("code", a.NewString(codes.Internal.String()))
	}

	errorItem.Set("extensions", extensions)
	errorArray.SetArrayItem(0, errorItem)

	return errorRoot.MarshalTo(nil)
}
