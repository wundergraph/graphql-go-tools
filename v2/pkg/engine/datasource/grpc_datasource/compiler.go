package grpcdatasource

import (
	"context"
	"fmt"

	"github.com/bufbuild/protocompile"
	"github.com/tidwall/gjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

// DataType represents the different types of data that can be stored in a protobuf field.
type DataType string

// Protobuf data types supported by the compiler.
const (
	DataTypeString  DataType = "string"    // String type
	DataTypeInt32   DataType = "int32"     // 32-bit integer type
	DataTypeInt64   DataType = "int64"     // 64-bit integer type
	DataTypeUint32  DataType = "uint32"    // 32-bit unsigned integer type
	DataTypeUint64  DataType = "uint64"    // 64-bit unsigned integer type
	DataTypeFloat   DataType = "float"     // 32-bit floating point type
	DataTypeDouble  DataType = "double"    // 64-bit floating point type
	DataTypeBool    DataType = "bool"      // Boolean type
	DataTypeEnum    DataType = "enum"      // Enumeration type
	DataTypeMessage DataType = "message"   // Nested message type
	DataTypeUnknown DataType = "<unknown>" // Represents an unknown or unsupported type
)

// dataTypeMap maps string representation of protobuf types to DataType constants.
var dataTypeMap = map[string]DataType{
	"string":  DataTypeString,
	"int32":   DataTypeInt32,
	"int64":   DataTypeInt64,
	"uint32":  DataTypeUint32,
	"uint64":  DataTypeUint64,
	"float":   DataTypeFloat,
	"double":  DataTypeDouble,
	"bool":    DataTypeBool,
	"enum":    DataTypeEnum,
	"message": DataTypeMessage,
}

// String returns the string representation of the DataType.
func (d DataType) String() string {
	return string(d)
}

// IsValid checks if the DataType is a valid protobuf type.
func (d DataType) IsValid() bool {
	_, ok := dataTypeMap[string(d)]
	return ok
}

func fromGraphQLType(s string) DataType {
	switch s {
	case "ID", "String", "Date", "DateTime", "Time", "Timestamp":
		return DataTypeString
	case "Int":
		return DataTypeInt32
	case "Float":
		return DataTypeFloat
	case "Boolean":
		return DataTypeBool
	default:
		return DataTypeUnknown
	}
}

// parseDataType converts a string type name to a DataType constant.
// Returns DataTypeUnknown if the type is not recognized.
func parseDataType(name string) DataType {
	if !dataTypeMap[name].IsValid() {
		return DataTypeUnknown
	}

	return dataTypeMap[name]
}

// Document represents a compiled protobuf document with all its services, messages, and methods.
type Document struct {
	Package  string    // The package name of the protobuf document
	Services []Service // All services defined in the document
	Enums    []Enum    // All enums defined in the document
	Messages []Message // All messages defined in the document
	Methods  []Method  // All methods from all services in the document
}

// Service represents a gRPC service with methods.
type Service struct {
	Name        string // The name of the service
	MethodsRefs []int  // References to methods in the Document.Methods slice
}

// Method represents a gRPC method with input and output message types.
type Method struct {
	Name       string // The name of the method
	InputName  string // The name of the input message type
	InputRef   int    // Reference to the input message in the Document.Messages slice
	OutputName string // The name of the output message type
	OutputRef  int    // Reference to the output message in the Document.Messages slice
}

// Message represents a protobuf message type with its fields.
type Message struct {
	Name   string                     // The name of the message
	Fields []Field                    // The fields in the message
	Desc   protoref.MessageDescriptor // The protobuf descriptor for the message
}

// Field represents a field in a protobuf message.
type Field struct {
	Name     string   // The name of the field
	Type     DataType // The data type of the field
	Number   int32    // The field number in the protobuf message
	Ref      int      // Reference to the field (used for complex types)
	Repeated bool     // Whether the field is a repeated field (array/list)
	Optional bool     // Whether the field is optional
	Message  *Message // If the field is a message type, this points to the message definition
}

// Enum represents a protobuf enum type with its values.
type Enum struct {
	Name   string      // The name of the enum
	Values []EnumValue // The values in the enum
}

// EnumValue represents a single value in a protobuf enum.
type EnumValue struct {
	Name   string // The name of the enum value
	Number int32  // The numeric value of the enum value
}

// RPCCompiler compiles protobuf schema strings into a Document and can
// build protobuf messages from JSON data based on the schema.
type RPCCompiler struct {
	doc *Document // The compiled Document
}

// ServiceByName returns a Service by its name.
// Returns an empty Service if no service with the given name exists.
func (d *Document) ServiceByName(name string) Service {
	for _, s := range d.Services {
		if s.Name == name {
			return s
		}
	}

	return Service{}
}

// MethodByName returns a Method by its name.
// Returns an empty Method if no method with the given name exists.
func (d *Document) MethodByName(name string) Method {
	for _, m := range d.Methods {
		if m.Name == name {
			return m
		}
	}

	return Method{}
}

// MethodRefByName returns the index of a Method in the Methods slice by its name.
// Returns -1 if no method with the given name exists.
func (d *Document) MethodRefByName(name string) int {
	for i, m := range d.Methods {
		if m.Name == name {
			return i
		}
	}

	return -1
}

// MethodByRef returns a Method by its reference index.
func (d *Document) MethodByRef(ref int) Method {
	return d.Methods[ref]
}

// ServiceByRef returns a Service by its reference index.
func (d *Document) ServiceByRef(ref int) Service {
	return d.Services[ref]
}

// MessageByName returns a Message by its name.
// Returns an empty Message if no message with the given name exists.
func (d *Document) MessageByName(name string) Message {
	for _, m := range d.Messages {
		if m.Name == name {
			return m
		}
	}

	return Message{}
}

// MessageRefByName returns the index of a Message in the Messages slice by its name.
// Returns -1 if no message with the given name exists.
func (d *Document) MessageRefByName(name string) int {
	for i, m := range d.Messages {
		if m.Name == name {
			return i
		}
	}

	return -1
}

// MessageByRef returns a Message by its reference index.
func (d *Document) MessageByRef(ref int) Message {
	return d.Messages[ref]
}

// NewProtoCompiler compiles the protobuf schema into a Document structure.
// It extracts information about services, methods, messages, and enums
// from the protobuf schema.
func NewProtoCompiler(schema string) (*RPCCompiler, error) {
	// Create a protocompile compiler with standard imports
	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(map[string]string{
				"": schema,
			}),
		}),
	}

	// Compile the schema
	fd, err := c.Compile(context.Background(), "")
	if err != nil {
		return nil, err
	}

	if len(fd) == 0 {
		return nil, fmt.Errorf("no files compiled")
	}

	f := fd[0]
	pc := &RPCCompiler{
		doc: &Document{
			Package: string(f.Package()),
		},
	}

	// Extract information from the compiled file descriptor
	pc.doc.Package = string(f.Package())

	// Process all enums in the schema
	for i := 0; i < f.Enums().Len(); i++ {
		pc.doc.Enums = append(pc.doc.Enums, pc.parseEnum(f.Enums().Get(i)))
	}

	// Process all messages in the schema
	for i := 0; i < f.Messages().Len(); i++ {
		pc.doc.Messages = append(pc.doc.Messages, pc.parseMessage(f.Messages().Get(i)))
	}

	// Process all services in the schema
	for i := 0; i < f.Services().Len(); i++ {
		pc.doc.Services = append(pc.doc.Services, pc.parseService(f.Services().Get(i)))
	}

	return pc, nil
}

// ConstructExecutionPlan constructs an RPCExecutionPlan from a parsed GraphQL operation and schema.
// It will return an error if the operation does not match the protobuf definition provided to the compiler.
func (p *RPCCompiler) ConstructExecutionPlan(operation, schema *ast.Document) (*RPCExecutionPlan, error) {
	return nil, nil
}

// Invocation represents a single gRPC invocation with its input and output messages.
type Invocation struct {
	ServiceName string
	MethodName  string
	Input       *dynamicpb.Message
	Output      *dynamicpb.Message
	Plan        *RPCExecutionPlan
}

// Compile processes an RPCExecutionPlan and builds protobuf messages from JSON data
// based on the compiled schema.
func (p *RPCCompiler) Compile(executionPlan *RPCExecutionPlan, inputData gjson.Result) ([]Invocation, error) {
	invocations := make([]Invocation, 0, len(executionPlan.Calls))

	for _, call := range executionPlan.Calls {
		// service := p.doc.ServiceByName(call.ServiceName)
		// method := p.doc.MethodByName(call.MethodName)
		inputMessage := p.doc.MessageByName(call.Request.Name)
		outputMessage := p.doc.MessageByName(call.Response.Name)

		request := p.buildProtoMessage(inputMessage, &call.Request, inputData)
		response := p.newEmptyMessage(outputMessage)

		invocations = append(invocations, Invocation{
			ServiceName: call.ServiceName,
			MethodName:  call.MethodName,
			Input:       request,
			Output:      response,
			Plan:        executionPlan,
		})
	}

	return invocations, nil
}

// newEmptyMessage creates a new empty dynamicpb.Message from a Message definition.
func (p *RPCCompiler) newEmptyMessage(message Message) *dynamicpb.Message {
	return dynamicpb.NewMessage(message.Desc)
}

// buildProtoMessage recursively builds a protobuf message from an RPCMessage definition
// and JSON data. It handles nested messages and repeated fields.
// TODO provide a way to have data
func (p *RPCCompiler) buildProtoMessage(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result) *dynamicpb.Message {
	if rpcMessage == nil {
		return nil
	}

	message := dynamicpb.NewMessage(inputMessage.Desc)

	for _, field := range inputMessage.Fields {
		fd := inputMessage.Desc.Fields()

		// Look up the field in the RPC message definition
		rpcField := rpcMessage.Fields.ByName(field.Name)
		if rpcField == nil {
			continue
		}

		// Handle repeated fields (arrays/lists)
		if field.Repeated {
			// Get a mutable reference to the list field
			list := message.Mutable(fd.ByName(protoref.Name(field.Name))).List()

			// Extract the array elements from the JSON data
			elements := data.Get(rpcField.JSONPath).Array()
			if len(elements) == 0 {
				continue
			}

			// Process each element and append to the list
			for _, element := range elements {
				fieldMsg := p.buildProtoMessage(*field.Message, rpcField.Message, element)
				list.Append(protoref.ValueOfMessage(fieldMsg))
			}

			continue
		}

		// Handle nested message fields
		if field.Message != nil {
			fieldMsg := p.buildProtoMessage(*field.Message, rpcField.Message, data)
			message.Set(inputMessage.Desc.Fields().ByName(protoref.Name(field.Name)), protoref.ValueOfMessage(fieldMsg))

			continue
		}

		// Handle scalar fields
		value := data.Get(rpcField.JSONPath)
		message.Set(fd.ByName(protoref.Name(field.Name)), p.setValueForKind(field.Type, value))
	}

	return message
}

// setValueForKind converts a gjson.Result value to the appropriate protobuf value
// based on its kind/type.
func (p *RPCCompiler) setValueForKind(kind DataType, data gjson.Result) protoref.Value {
	switch kind {
	case DataTypeString:
		return protoref.ValueOfString(data.String())
	case DataTypeInt32:
		return protoref.ValueOfInt32(int32(data.Int()))
	case DataTypeInt64:
		return protoref.ValueOfInt64(data.Int())
	case DataTypeUint32:
		return protoref.ValueOfUint32(uint32(data.Int()))
	case DataTypeUint64:
		return protoref.ValueOfUint64(uint64(data.Int()))
	case DataTypeFloat:
		return protoref.ValueOfFloat32(float32(data.Float()))
	case DataTypeDouble:
		return protoref.ValueOfFloat64(data.Float())
	case DataTypeBool:
		return protoref.ValueOfBool(data.Bool())
	}

	return protoref.Value{}
}

// parseEnum extracts information from a protobuf enum descriptor.
func (p *RPCCompiler) parseEnum(e protoref.EnumDescriptor) Enum {
	name := string(e.Name())

	values := make([]EnumValue, 0, e.Values().Len())

	for j := 0; j < e.Values().Len(); j++ {
		values = append(values, p.parseEnumValue(e.Values().Get(j)))
	}

	return Enum{Name: name, Values: values}
}

// parseEnumValue extracts information from a protobuf enum value descriptor.
func (p *RPCCompiler) parseEnumValue(v protoref.EnumValueDescriptor) EnumValue {
	name := string(v.Name())
	number := int32(v.Number())

	return EnumValue{Name: name, Number: number}
}

// parseService extracts information from a protobuf service descriptor,
// including all its methods.
func (p *RPCCompiler) parseService(s protoref.ServiceDescriptor) Service {
	name := string(s.Name())
	m := s.Methods()

	methods := make([]Method, 0, m.Len())
	methodsRefs := make([]int, 0, m.Len())

	for j := 0; j < m.Len(); j++ {
		methods = append(methods, p.parseMethod(m.Get(j)))
		methodsRefs = append(methodsRefs, j)
	}

	// Add the methods to the Document
	p.doc.Methods = append(p.doc.Methods, methods...)

	return Service{
		Name:        name,
		MethodsRefs: methodsRefs,
	}
}

// parseMethod extracts information from a protobuf method descriptor,
// including its input and output message types.
func (p *RPCCompiler) parseMethod(m protoref.MethodDescriptor) Method {
	name := string(m.Name())
	input, output := m.Input(), m.Output()

	return Method{
		Name:       name,
		InputName:  string(input.Name()),
		InputRef:   p.doc.MessageRefByName(string(input.Name())),
		OutputName: string(output.Name()),
		OutputRef:  p.doc.MessageRefByName(string(output.Name())),
	}
}

// parseMessage recursively extracts information from a protobuf message descriptor,
// including all its fields and nested message types.
func (p *RPCCompiler) parseMessage(m protoref.MessageDescriptor) Message {
	name := string(m.Name())

	fields := []Field{}

	// Process all fields in the message
	for i := 0; i < m.Fields().Len(); i++ {
		f := m.Fields().Get(i)

		field := p.parseField(f)

		// If the field is a message type, recursively parse the nested message
		if f.Kind() == protoref.MessageKind {
			message := p.parseMessage(f.Message())
			field.Message = &message
		}

		fields = append(fields, field)
	}

	return Message{
		Name:   name,
		Fields: fields,
		Desc:   m,
	}
}

// parseField extracts information from a protobuf field descriptor.
func (p *RPCCompiler) parseField(f protoref.FieldDescriptor) Field {
	name := string(f.Name())
	typeName := f.Kind().String()

	return Field{
		Name:     name,
		Type:     parseDataType(typeName),
		Number:   int32(f.Number()),
		Repeated: f.IsList(),
		Optional: f.Cardinality() == protoref.Optional,
	}
}
