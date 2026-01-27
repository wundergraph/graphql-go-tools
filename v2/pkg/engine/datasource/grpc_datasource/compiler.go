package grpcdatasource

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/tidwall/gjson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	// InvalidRef is a constant used to indicate that a reference is invalid.
	InvalidRef = -1
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
	DataTypeBytes   DataType = "bytes"     // Bytes type
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
	"bytes":   DataTypeBytes,
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
	case "ID", "String":
		return DataTypeString
	case "Int":
		// https://spec.graphql.org/October2021/#sec-Int
		// Fields returning the type Int expect to encounter 32-bit integer internal values.
		return DataTypeInt32
	case "Float":
		// https://spec.graphql.org/October2021/#sec-Float
		// Fields returning the type Float expect to encounter double-precision floating-point internal values.
		return DataTypeDouble
	case "Boolean":
		return DataTypeBool
	default:
		// Fallback to bytes for unknown types to handle raw data.
		return DataTypeBytes
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

type NodeKind int

const (
	NodeKindMessage NodeKind = iota + 1
	NodeKindEnum
	NodeKindService
	NodeKindUnknown
)

type node struct {
	ref  int
	kind NodeKind
}

// Document represents a compiled protobuf document with all its services, messages, and methods.
type Document struct {
	nodes    map[uint64]node
	Package  string    // The package name of the protobuf document
	Imports  []string  // The imports of the protobuf document
	Services []Service // All services defined in the document
	Enums    []Enum    // All enums defined in the document
	Messages []Message // All messages defined in the document
	Methods  []Method  // All methods from all services in the document
}

// newNode creates a new node in the document.
func (d *Document) newNode(ref int, name string, kind NodeKind) {
	digest := pool.Hash64.Get()
	defer pool.Hash64.Put(digest)
	_, _ = digest.WriteString(name)

	d.nodes[digest.Sum64()] = node{
		ref:  ref,
		kind: kind,
	}
}

// nodeByName returns a node by its name.
// Returns false if the node does not exist.
func (d *Document) nodeByName(name string) (node, bool) {
	digest := pool.Hash64.Get()
	defer pool.Hash64.Put(digest)
	_, _ = digest.WriteString(name)

	node, exists := d.nodes[digest.Sum64()]
	return node, exists
}

// appendMessage appends a message to the document and returns the reference index.
func (d *Document) appendMessage(message Message) int {
	d.Messages = append(d.Messages, message)
	return len(d.Messages) - 1
}

// appendEnum appends an enum to the document and returns the reference index.
func (d *Document) appendEnum(enum Enum) int {
	d.Enums = append(d.Enums, enum)
	return len(d.Enums) - 1
}

// appendService appends a service to the document and returns the reference index.
func (d *Document) appendService(service Service) int {
	d.Services = append(d.Services, service)
	return len(d.Services) - 1
}

// Service represents a gRPC service with methods.
type Service struct {
	Name        string // The name of the service
	FullName    string // The full name of the service
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
	Fields map[uint64]Field
	Name   string                     // The name of the message
	Desc   protoref.MessageDescriptor // The protobuf descriptor for the message
}

// GetField returns a field by its name.
// Returns nil if no field with the given name exists.
func (m *Message) GetField(name string) *Field {
	digest := pool.Hash64.Get()
	defer pool.Hash64.Put(digest)
	_, _ = digest.WriteString(name)

	field, found := m.Fields[digest.Sum64()]
	if !found {
		return nil
	}

	return &field
}

func (m *Message) SetField(f Field) {
	digest := pool.Hash64.Get()
	defer pool.Hash64.Put(digest)
	_, _ = digest.WriteString(f.Name)

	if m.Fields == nil {
		m.Fields = make(map[uint64]Field)
	}

	m.Fields[digest.Sum64()] = f
}

// AllocFields allocates a new map of fields with the given count.
func (m *Message) AllocFields(count int) {
	m.Fields = make(map[uint64]Field, count)
}

// Field represents a field in a protobuf message.
type Field struct {
	Name       string   // The name of the field
	Type       DataType // The data type of the field
	Number     int32    // The field number in the protobuf message
	Ref        int      // Reference to the field (used for complex types)
	Repeated   bool     // Whether the field is a repeated field (array/list)
	Optional   bool     // Whether the field is optional
	MessageRef int      // If the field is a message type, this points to the message definition
}

func (f *Field) IsMessage() bool {
	return f.Type == DataTypeMessage
}

func (f *Field) ResolveUnderlyingMessage(doc *Document) *Message {
	if f.MessageRef >= 0 {
		return &doc.Messages[f.MessageRef]
	}

	return nil
}

// Enum represents a protobuf enum type with its values.
type Enum struct {
	Name   string      // The name of the enum
	Values []EnumValue // The values in the enum
}

// EnumValue represents a single value in a protobuf enum.
type EnumValue struct {
	Name         string // The name of the enum value
	GraphqlValue string // The target value of the enum value
	Number       int32  // The numeric value of the enum value
}

// RPCCompiler compiles protobuf schema strings into a Document and can
// build protobuf messages from JSON data based on the schema.
type RPCCompiler struct {
	doc      *Document // The compiled Document
	Ancestor []Message
}

// ServiceByName returns a Service by its name.
// Returns an empty Service if no service with the given name exists.
func (d *Document) ServiceByName(name string) *Service {
	node, found := d.nodeByName(name)
	if !found || node.kind != NodeKindService {
		return nil
	}

	return &d.Services[node.ref]
}

// MessageByName returns a Message by its name.
// Returns an empty Message if no message with the given name exists.
// We only expect this function to return false if either the message name was provided incorrectly,
// or the schema and mapping was not properly configured.
func (d *Document) MessageByName(name string) (Message, bool) {
	node, found := d.nodeByName(name)
	if !found || node.kind != NodeKindMessage {
		return Message{}, false
	}

	return d.Messages[node.ref], true
}

// MessageRefByName returns the index of a Message in the Messages slice by its name.
// Returns -1 if no message with the given name exists.
func (d *Document) MessageRefByName(name string) int {
	node, found := d.nodeByName(name)
	if !found || node.kind != NodeKindMessage {
		return InvalidRef
	}
	return node.ref

}

// MessageByRef returns a Message by its reference index.
func (d *Document) MessageByRef(ref int) Message {
	return d.Messages[ref]
}

// EnumByName returns an Enum by its name.
// Returns false if the enum does not exist.
func (d *Document) EnumByName(name string) (Enum, bool) {
	node, found := d.nodeByName(name)
	if !found || node.kind != NodeKindEnum {
		return Enum{}, false
	}

	return d.Enums[node.ref], true
}

// NewProtoCompiler compiles the protobuf schema into a Document structure.
// It extracts information about services, methods, messages, and enums
// from the protobuf schema.
func NewProtoCompiler(schema string, mapping *GRPCMapping) (*RPCCompiler, error) {
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

	schemaFile := fd[0]
	pc := &RPCCompiler{
		doc: &Document{
			nodes:   make(map[uint64]node),
			Package: string(schemaFile.Package()),
		},
	}

	// Extract information from the compiled file descriptor
	pc.doc.Package = string(schemaFile.Package())

	// We potentially have imported other files and need to resolve the types first
	// before we can parse the schema.
	for i := 0; i < schemaFile.Imports().Len(); i++ {
		protoImport := schemaFile.Imports().Get(i)
		pc.doc.Imports = append(pc.doc.Imports, string(protoImport.Path()))
		pc.processFile(protoImport, mapping)
	}

	// Process the schema file
	pc.processFile(schemaFile, mapping)

	return pc, nil
}

func (p *RPCCompiler) processFile(f protoref.FileDescriptor, mapping *GRPCMapping) {
	// Process all enums in the schema
	for i := 0; i < f.Enums().Len(); i++ {
		enum := p.parseEnum(f.Enums().Get(i), mapping)
		ref := p.doc.appendEnum(enum)
		p.doc.newNode(ref, enum.Name, NodeKindEnum)
	}

	// Process all messages in the schema
	messages := p.parseMessageDefinitions(f.Messages())
	for _, message := range messages {
		ref := p.doc.appendMessage(message)
		p.doc.newNode(ref, message.Name, NodeKindMessage)
	}

	// We need to reiterate over the messages to handle recursive types.
	for ref, message := range p.doc.Messages {
		p.enrichMessageData(ref, message.Desc)
	}

	// Process all services in the schema
	for i := 0; i < f.Services().Len(); i++ {
		service := p.parseService(f.Services().Get(i))
		ref := p.doc.appendService(service)
		p.doc.newNode(ref, service.Name, NodeKindService)
	}
}

// ServiceCall represents a single gRPC service call with its input and output messages.
type ServiceCall struct {
	// ServiceName is the name of the gRPC service to call
	ServiceName string
	// MethodName is the name of the method on the service to call
	MethodName string
	// Input is the input message for the gRPC call
	Input protoref.Message
	// Output is the output message for the gRPC call
	Output protoref.Message
	// RPC is the call that was made to the gRPC service
	RPC *RPCCall
}

func (s *ServiceCall) MethodFullName() string {
	var builder strings.Builder

	builder.Grow(len(s.ServiceName) + len(s.MethodName) + 2)
	builder.WriteRune('/')
	builder.WriteString(s.ServiceName)
	builder.WriteRune('/')
	builder.WriteString(s.MethodName)

	return builder.String()
}

// func (p *RPCCompiler) CompileFetches(graph *DependencyGraph, fetches []FetchItem, inputData gjson.Result) ([]Invocation, error) {
// 	invocations := make([]Invocation, 0, len(fetches))

// 	resultChan := make(chan Invocation, len(fetches))
// 	errChan := make(chan error, len(fetches))

// 	wg := sync.WaitGroup{}
// 	wg.Add(len(fetches))

// 	for _, node := range fetches {
// 		go func() {
// 			defer wg.Done()
// 			invocation, err := p.CompileNode(graph, node, inputData)
// 			if err != nil {
// 				errChan <- err
// 				return
// 			}

// 			resultChan <- invocation
// 			node.Invocation = &invocation
// 		}()
// 	}

// 	close(resultChan)
// 	close(errChan)

// 	var joinErr error
// 	for err := range errChan {
// 		joinErr = errors.Join(joinErr, err)
// 	}

// 	if joinErr != nil {
// 		return nil, joinErr
// 	}

// 	for invocation := range resultChan {
// 		invocations = append(invocations, invocation)
// 	}

// 	return invocations, nil
// }

func (p *RPCCompiler) CompileFetches(graph *DependencyGraph, fetches []FetchItem, inputData gjson.Result) ([]ServiceCall, error) {
	serviceCalls := make([]ServiceCall, 0, len(fetches))

	for _, node := range fetches {
		serviceCall, err := p.CompileNode(graph, node, inputData)
		if err != nil {
			return nil, err
		}

		graph.SetFetchData(node.ID, &serviceCall)
		serviceCalls = append(serviceCalls, serviceCall)
	}

	return serviceCalls, nil
}

func (p *RPCCompiler) CompileNode(graph *DependencyGraph, fetch FetchItem, inputData gjson.Result) (ServiceCall, error) {
	call := fetch.Plan

	outputMessage, ok := p.doc.MessageByName(call.Response.Name)
	if !ok {
		return ServiceCall{}, fmt.Errorf("output message %s not found in document", call.Response.Name)
	}

	response, err := p.newEmptyMessage(outputMessage)
	if err != nil {
		return ServiceCall{}, err
	}

	var request protoref.Message
	inputMessage, ok := p.doc.MessageByName(call.Request.Name)
	if !ok {
		return ServiceCall{}, fmt.Errorf("input message %s not found in document", call.Request.Name)
	}

	switch call.Kind {
	case CallKindStandard, CallKindEntity:
		request, err = p.buildProtoMessage(inputMessage, &call.Request, inputData)
		if err != nil {
			return ServiceCall{}, err
		}

	case CallKindResolve:
		context, err := graph.FetchDependencies(&fetch)
		if err != nil {
			return ServiceCall{}, err
		}

		request, err = p.buildProtoMessageWithContext(inputMessage, &call.Request, inputData, context)
		if err != nil {
			return ServiceCall{}, err
		}
	case CallKindRequired:
		request, err = p.buildRequiredFieldsMessage(inputMessage, &call.Request, inputData)
		if err != nil {
			return ServiceCall{}, err
		}
	}

	serviceName, ok := p.resolveServiceName(call)
	if !ok {
		return ServiceCall{}, fmt.Errorf("failed to resolve service name for method %s from the protobuf definition", call.MethodName)
	}

	return ServiceCall{
		ServiceName: serviceName,
		MethodName:  call.MethodName,
		Input:       request,
		Output:      response,
		RPC:         call,
	}, nil

}

func (p *RPCCompiler) resolveServiceName(call *RPCCall) (string, bool) {
	if service := p.doc.ServiceByName(call.ServiceName); service != nil {
		return service.FullName, true
	}

	// Fallback. Try to find the service by the method name.
	for _, service := range p.doc.Services {
		for _, methodRef := range service.MethodsRefs {
			if p.doc.Methods[methodRef].Name == call.MethodName {
				return service.FullName, true
			}
		}
	}

	return "", false
}

// newEmptyMessage creates a new empty dynamicpb.Message from a Message definition.
func (p *RPCCompiler) newEmptyMessage(message Message) (protoref.Message, error) {
	if p.doc.MessageRefByName(message.Name) == InvalidRef {
		return nil, fmt.Errorf("message %s not found in document", message.Name)
	}

	return dynamicpb.NewMessage(message.Desc), nil
}

// buildProtoMessageWithContext builds a protobuf message from an RPCMessage definition
// and JSON data. It handles nested messages and repeated fields.
//
// Example:
//
//	message ResolveCategoryProductCountRequest {
//	  repeated CategoryProductCountContext context = 1;
//	  CategoryProductCountArgs field_args = 2;
//	}
func (p *RPCCompiler) buildProtoMessageWithContext(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result, context []FetchItem) (protoref.Message, error) {
	if rpcMessage == nil {
		return nil, fmt.Errorf("rpc message is nil")
	}

	if len(context) == 0 {
		return nil, fmt.Errorf("context is required for resolve calls")
	}

	if p.doc.MessageRefByName(rpcMessage.Name) == InvalidRef {
		return nil, fmt.Errorf("message %s not found in document", rpcMessage.Name)
	}

	rootMessage := dynamicpb.NewMessage(inputMessage.Desc)

	if len(inputMessage.Fields) != 2 {
		return nil, fmt.Errorf("message %s must have exactly two fields: context and field_args", inputMessage.Name)
	}

	contextSchemaField := inputMessage.GetField("context")
	if contextSchemaField == nil {
		return nil, fmt.Errorf("context field not found in message %s", inputMessage.Name)
	}

	contextRPCField := rpcMessage.Fields.ByName(contextSchemaField.Name)
	if contextRPCField == nil {
		return nil, fmt.Errorf("context field not found in message %s", rpcMessage.Name)
	}

	contextList := p.newEmptyListMessageByName(rootMessage, contextSchemaField.Name)
	contextData := p.resolveContextData(context[0], contextRPCField) // TODO handle multiple contexts (resolver requires another resolver)

	for _, contextElement := range contextData {
		val := contextList.NewElement()
		valMsg := val.Message()
		for fieldName, value := range contextElement {
			if err := p.setMessageValue(valMsg, fieldName, value); err != nil {
				return nil, err
			}
		}

		contextList.Append(val)
	}

	argsSchemaField := inputMessage.GetField("field_args")
	if argsSchemaField == nil {
		return nil, fmt.Errorf("field_args field not found in message %s", inputMessage.Name)
	}

	argsMessage := p.doc.Messages[argsSchemaField.MessageRef]
	argsRPCField := rpcMessage.Fields.ByName("field_args")
	if argsRPCField == nil {
		return nil, fmt.Errorf("field_args field not found in message %s", rpcMessage.Name)
	}

	args, err := p.buildProtoMessage(argsMessage, argsRPCField.Message, data)
	if err != nil {
		return nil, err
	}
	// Set the args field
	if err := p.setMessageValue(rootMessage, argsRPCField.Name, protoref.ValueOfMessage(args)); err != nil {
		return nil, err
	}

	return rootMessage, nil
}

// buildRequiredFieldsMessage builds a protobuf message from an RPCMessage definition
// and JSON data. It handles nested messages and repeated fields.
//
// Example:
//
//	message RequireWarehouseStockHealthScoreByIdRequest {
//		// RequireWarehouseStockHealthScoreByIdContext provides the context for the required fields method RequireWarehouseStockHealthScoreById.
//		repeated RequireWarehouseStockHealthScoreByIdContext context = 1;
//	}
//
//	message RequireWarehouseStockHealthScoreByIdContext {
//	 	LookupWarehouseByIdRequestKey key = 1;
//		RequireWarehouseStockHealthScoreByIdFields fields = 2;
//	}
func (p *RPCCompiler) buildRequiredFieldsMessage(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result) (protoref.Message, error) {
	if rpcMessage == nil {
		return nil, fmt.Errorf("rpc message is nil")
	}

	if p.doc.MessageRefByName(rpcMessage.Name) == InvalidRef {
		return nil, fmt.Errorf("message %s not found in document", rpcMessage.Name)
	}

	rootMessage := dynamicpb.NewMessage(inputMessage.Desc)

	contextSchemaField := inputMessage.GetField("context")
	if contextSchemaField == nil {
		return nil, fmt.Errorf("context field not found in message %s", inputMessage.Name)
	}

	contextRPCField := rpcMessage.Fields.ByName(contextSchemaField.Name)
	if contextRPCField == nil {
		return nil, fmt.Errorf("context field not found in message %s", rpcMessage.Name)
	}

	contextList := p.newEmptyListMessageByName(rootMessage, contextSchemaField.Name)
	contextFieldMessage := contextRPCField.Message

	if contextFieldMessage == nil {
		return nil, fmt.Errorf("context field message not found in message %s", inputMessage.Name)
	}

	keyField := contextFieldMessage.Fields.ByName("key")
	if keyField == nil {
		return nil, fmt.Errorf("key field message not found in message %s", contextFieldMessage.Name)
	}

	keyMessage, ok := p.doc.MessageByName(keyField.Message.Name)
	if !ok {
		return nil, fmt.Errorf("message %s not found in document", keyField.Message.Name)
	}

	requiresSelectionField := contextFieldMessage.Fields.ByName("fields")
	if requiresSelectionField == nil {
		return nil, fmt.Errorf("fields field not found in message %s", contextFieldMessage.Name)
	}

	requiresSelectionMessage, ok := p.doc.MessageByName(requiresSelectionField.Message.Name)
	if !ok {
		return nil, fmt.Errorf("message %s not found in document", requiresSelectionField.Message.Name)
	}

	representations := data.Get("representations").Array()
	for _, representation := range representations {
		element := contextList.NewElement()
		msg := element.Message()

		keyMsg, err := p.buildProtoMessage(keyMessage, keyField.Message, representation)
		if err != nil {
			return nil, err
		}

		reqMsg, err := p.buildProtoMessage(requiresSelectionMessage, requiresSelectionField.Message, representation)
		if err != nil {
			return nil, err
		}

		if err := p.setMessageValue(msg, keyField.Name, protoref.ValueOfMessage(keyMsg)); err != nil {
			return nil, err
		}

		if err := p.setMessageValue(msg, requiresSelectionField.Name, protoref.ValueOfMessage(reqMsg)); err != nil {
			return nil, err
		}

		// build fields message
		contextList.Append(element)
	}

	return rootMessage, nil
}

func (p *RPCCompiler) resolveContextData(context FetchItem, contextField *RPCField) []map[string]protoref.Value {
	if context.ServiceCall == nil || context.ServiceCall.Output == nil {
		return []map[string]protoref.Value{}
	}

	contextValues := make([]map[string]protoref.Value, 0)
	for _, field := range contextField.Message.Fields {
		values := p.resolveContextDataForPath(context.ServiceCall.Output, field.ResolvePath)

		for index, value := range values {
			if index >= len(contextValues) {
				contextValues = append(contextValues, make(map[string]protoref.Value))
			}

			contextValues[index][field.Name] = value
		}

	}

	return contextValues
}

// resolveContextDataForPath resolves the data for a given path in the context message.
func (p *RPCCompiler) resolveContextDataForPath(message protoref.Message, path ast.Path) []protoref.Value {
	if path.Len() == 0 {
		return nil
	}

	segment := path[0]
	path = path[1:]

	msg, fd := p.getMessageField(message, segment.FieldName.String())
	if !msg.IsValid() {
		return nil
	}

	if fd.IsList() {
		return p.resolveListDataForPath(msg.List(), fd, path)
	}

	return p.resolveDataForPath(msg.Message(), path)
}

// resolveListDataForPath resolves the data for a given path in a list message.
func (p *RPCCompiler) resolveListDataForPath(message protoref.List, fd protoref.FieldDescriptor, path ast.Path) []protoref.Value {
	if path.Len() == 0 {
		return nil
	}

	result := make([]protoref.Value, 0, message.Len())

	for i := range message.Len() {
		item := message.Get(i)

		switch fd.Kind() {
		case protoref.MessageKind:
			values := p.resolveDataForPath(item.Message(), path)

			for _, val := range values {
				if list, isList := val.Interface().(protoref.List); isList {
					values := p.resolveListDataForPath(list, fd, path[1:])
					result = append(result, values...)
					continue
				} else {
					result = append(result, val)
				}
			}

		default:
			result = append(result, item)
		}
	}

	return result
}

// resolveDataForPath resolves the data for a given path in a message.
func (p *RPCCompiler) resolveDataForPath(message protoref.Message, path ast.Path) []protoref.Value {
	if path.Len() == 0 {
		return nil
	}

	segment := path[0]

	if fn := segment.FieldName.String(); strings.HasPrefix(fn, "@") {
		list := p.resolveUnderlyingList(message, fn)

		result := make([]protoref.Value, 0, len(list))
		for _, item := range list {
			result = append(result, p.resolveDataForPath(item.Message(), path[1:])...)
		}

		return result
	}

	field, fd := p.getMessageField(message, segment.FieldName.String())
	if !field.IsValid() {
		return nil
	}

	switch fd.Kind() {
	case protoref.MessageKind:
		if fd.IsList() {
			return []protoref.Value{protoref.ValueOfList(field.List())}
		}

		return p.resolveDataForPath(field.Message(), path[1:])
	default:
		return []protoref.Value{field}
	}
}

// getMessageField gets the field from the message by its name.
func (p *RPCCompiler) getMessageField(message protoref.Message, fieldName string) (protoref.Value, protoref.FieldDescriptor) {
	fd := message.Descriptor().Fields().ByName(protoref.Name(fieldName))
	if fd == nil {
		return protoref.Value{}, nil
	}

	return message.Get(fd), fd
}

// resolveUnderlyingList resolves the underlying list message from a nested list message.
//
//	message ListOfFloat {
//	  message List {
//	    repeated double items = 1;
//	  }
//	  List list = 1;
//	}
func (p *RPCCompiler) resolveUnderlyingList(msg protoref.Message, fieldName string) []protoref.Value {
	nestingLevel := 0
	for _, char := range fieldName {
		if char != '@' {
			break
		}
		nestingLevel++
	}

	listFieldValue := msg.Get(msg.Descriptor().Fields().ByName(protoref.Name(fieldName[nestingLevel:])))
	if !listFieldValue.IsValid() {
		return nil
	}

	return p.resolveUnderlyingListItems(listFieldValue, nestingLevel)

}

// resolveUnderlyingListItems resolves the items in a list message.
//
//	message ListOfFloat {
//	  message List {
//	    repeated double items = 1;
//	  }
//	  List list = 1;
//	}
func (p *RPCCompiler) resolveUnderlyingListItems(value protoref.Value, nestingLevel int) []protoref.Value {
	// The field number of the list and items field in the message
	const listAndItemsFieldNumber = 1
	msg := value.Message()
	fd := msg.Descriptor().Fields().ByNumber(listAndItemsFieldNumber)
	if fd == nil {
		return nil
	}

	listMsg := msg.Get(fd)
	if !listMsg.IsValid() {
		return nil
	}

	itemsValue := listMsg.Message().Get(listMsg.Message().Descriptor().Fields().ByNumber(listAndItemsFieldNumber))
	if !itemsValue.IsValid() {
		return nil
	}

	itemsList := itemsValue.List()
	itemsListLen := itemsList.Len()
	if itemsListLen == 0 {
		return nil
	}

	if nestingLevel > 1 {
		items := make([]protoref.Value, 0, itemsListLen)
		for i := 0; i < itemsListLen; i++ {
			items = append(items, p.resolveUnderlyingListItems(itemsList.Get(i), nestingLevel-1)...)
		}

		return items
	}

	result := make([]protoref.Value, itemsListLen)
	for i := 0; i < itemsListLen; i++ {
		result[i] = itemsList.Get(i)
	}

	return result
}

func (p *RPCCompiler) newEmptyListMessageByName(msg protoref.Message, name string) protoref.List {
	return msg.Mutable(msg.Descriptor().Fields().ByName(protoref.Name(name))).List()
}

func (p *RPCCompiler) setMessageValue(message protoref.Message, fieldName string, value protoref.Value) error {
	fd := message.Descriptor().Fields().ByName(protoref.Name(fieldName))
	if fd == nil {
		return fmt.Errorf("field %s not found in message %s", fieldName, message.Descriptor().Name())
	}

	// If we are setting a list value here, we need to create a copy of the list
	// because the field descriptor is included in the type check, so we cannot asign it using `Set` directly.
	if fd.IsList() {
		list := message.Mutable(fd).List()
		source, ok := value.Interface().(protoref.List)
		if !ok {
			return fmt.Errorf("value is not a list")
		}

		p.copyListValues(source, list)
		return nil
	}

	message.Set(message.Descriptor().Fields().ByName(protoref.Name(fieldName)), value)
	return nil
}

func (p *RPCCompiler) copyListValues(source protoref.List, destination protoref.List) {
	for i := range source.Len() {
		destination.Append(source.Get(i))
	}
}

// buildProtoMessage recursively builds a protobuf message from an RPCMessage definition
// and JSON data. It handles nested messages and repeated fields.
func (p *RPCCompiler) buildProtoMessage(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result) (protoref.Message, error) {
	if rpcMessage == nil {
		return nil, errors.New("rpc message is nil")
	}

	inputMessageRef := p.doc.MessageRefByName(inputMessage.Name)
	if inputMessageRef == InvalidRef {
		return nil, fmt.Errorf("message %s not found in document", inputMessage.Name)
	}

	message := dynamicpb.NewMessage(inputMessage.Desc)

	for _, rpcField := range rpcMessage.Fields {
		fd := inputMessage.Desc.Fields()

		// Look up the field in the RPC message definition
		field := inputMessage.GetField(rpcField.Name)
		if field == nil {
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
				switch field.Type {
				case DataTypeMessage:
					// If we handle entity lookups, we get a list of representation variables that need to
					// be applied to the correct type names.
					if !isAllowedForTypename(rpcField.Message, element) {
						continue
					}

					fieldMsg, err := p.buildProtoMessage(p.doc.Messages[field.MessageRef], rpcField.Message, element)
					if err != nil {
						return nil, err
					}

					list.Append(protoref.ValueOfMessage(fieldMsg))
				default:
					list.Append(p.setValueForKind(field.Type, element))
				}
			}

			continue
		}

		// Handle nested message fields
		if field.MessageRef >= 0 {
			var (
				fieldMsg protoref.Message
				err      error
			)

			switch {
			case rpcField.IsListType:
				// Nested and nullable lists are wrapped in a message, therefore we need to handle them differently
				// than repeated fields. We need to do this because protobuf repeated fields are not nullable and cannot be nested.
				//
				// message BlogPost {
				//   ListOfBoolean is_published = 1;
				//   ListOfListOfString related_topics = 2;
				// }
				if !data.Get(rpcField.JSONPath).Exists() {
					if !rpcField.Optional {
						return nil, fmt.Errorf("field %s is required but has no value", rpcField.JSONPath)
					}

					continue
				}

				if rpcField.ListMetadata == nil {
					return nil, fmt.Errorf("list metadata not found for field %s", rpcField.JSONPath)
				}

				fieldMsg, err = p.buildListMessage(inputMessage.Desc, field, &rpcField, data)
				if err != nil {
					return nil, err
				}

				if fieldMsg == nil {
					continue
				}

			case rpcField.IsOptionalScalar():
				// If the field is optional, we are handling a scalar value that is wrapped in a message
				// as protobuf scalar types are not nullable.

				if !data.Get(rpcField.JSONPath).Exists() {
					// If we don't have a value for an optional field, we skip it to provide a null message.
					continue
				}

				// As those optional messages are well known wrapper types, we can convert them to the underlying message definition.
				fieldMsg, err = p.buildProtoMessage(
					p.doc.Messages[field.MessageRef],
					rpcField.ToOptionalTypeMessage(p.doc.Messages[field.MessageRef].Name),
					data,
				)

				if err != nil {
					return nil, err
				}
			default:
				fieldMsg, err = p.buildProtoMessage(p.doc.Messages[field.MessageRef], rpcField.Message, data.Get(rpcField.JSONPath))
				if err != nil {
					return nil, err
				}
			}

			message.Set(inputMessage.Desc.Fields().ByName(protoref.Name(field.Name)), protoref.ValueOfMessage(fieldMsg))
			continue
		}

		if field.Type == DataTypeEnum {
			val, err := p.getEnumValue(rpcField.EnumName, data.Get(rpcField.JSONPath))
			if err != nil {
				return nil, err
			}

			if val != nil {
				message.Set(fd.ByName(protoref.Name(field.Name)), *val)
			}

			continue
		}

		// Handle scalar fields
		value := data.Get(rpcField.JSONPath)
		message.Set(fd.ByName(protoref.Name(field.Name)), p.setValueForKind(field.Type, value))
	}

	return message, nil
}

// buildListMessage creates a new protobuf message, which reflects a wrapper type to work with a list in GraphQL.
// A list wrapper type has an inner message type, which contains a repeated field.
// We need this to make sure we can differentiate between a null list and an empty list, as repeated fields are not nullable.
//
//	message ListOfFloat {
//	  message List {
//	    repeated double items = 1;
//	  }
//	  List list = 1;
//	}
func (p *RPCCompiler) buildListMessage(desc protoref.MessageDescriptor, field *Field, rpcField *RPCField, data gjson.Result) (protoref.Message, error) {
	msg, err := p.traverseList(
		dynamicpb.NewMessage(desc.Fields().ByName(protoref.Name(field.Name)).Message()),
		1,
		field,
		rpcField,
		data.Get(rpcField.JSONPath),
	)

	if err != nil {
		return nil, err
	}
	return msg, nil
}

// traverseList makes sure we can handle nested lists properly.
// A nested list follows the same structure as a regular list, but references the lower nested message list wrapper.
//
//	message ListOfListOfString {
//	  message List {
//	    repeated ListOfString items = 1;
//	  }
//	  List list = 1;
//	}
func (p *RPCCompiler) traverseList(rootMsg protoref.Message, level int, field *Field, rpcField *RPCField, data gjson.Result) (protoref.Message, error) {
	listFieldDesc := rootMsg.Descriptor().Fields().ByNumber(1)
	if listFieldDesc == nil {
		return nil, fmt.Errorf("field with number %d not found in message %s", 1, rootMsg.Descriptor().Name())
	}

	elements := data.Array()
	newListField := rootMsg.NewField(listFieldDesc)
	if len(elements) == 0 {
		if rpcField.ListMetadata.LevelInfo[level-1].Optional {
			return nil, nil
		}

		rootMsg.Set(listFieldDesc, newListField)
		return rootMsg, nil
	}

	// Inside of a List message type we expect a repeated "items" field with field number 1
	itemsFieldMsg := newListField.Message()
	itemsFieldDesc := itemsFieldMsg.Descriptor().Fields().ByNumber(1)
	if itemsFieldDesc == nil {
		return nil, fmt.Errorf("field with number %d not found in message %s", 1, itemsFieldMsg.Descriptor().Name())
	}

	itemsField := itemsFieldMsg.Mutable(itemsFieldDesc).List()

	if level >= rpcField.ListMetadata.NestingLevel {
		switch DataType(rpcField.ProtoTypeName) {
		case DataTypeMessage:
			itemsFieldMsg, ok := p.doc.MessageByName(rpcField.Message.Name)
			if !ok {
				return nil, fmt.Errorf("message %s not found in document", rpcField.Message.Name)
			}

			for _, element := range elements {
				msg, err := p.buildProtoMessage(itemsFieldMsg, rpcField.Message, element)
				if err != nil {
					return nil, err
				}

				if msg != nil {
					itemsField.Append(protoref.ValueOfMessage(msg))
				}
			}
		case DataTypeEnum:
			for _, element := range elements {
				val, err := p.getEnumValue(rpcField.EnumName, element)
				if err != nil {
					return nil, err
				}

				if val != nil {
					itemsField.Append(*val)
				}
			}
		default:
			for _, element := range elements {
				itemsField.Append(p.setValueForKind(DataType(itemsFieldDesc.Kind().String()), element))
			}
		}

		itemsFieldMsg.Set(itemsFieldDesc, protoref.ValueOfList(itemsField))
		rootMsg.Set(listFieldDesc, newListField)
		return rootMsg, nil
	}

	for _, element := range elements {
		newElement := itemsField.NewElement()
		val, err := p.traverseList(newElement.Message(), level+1, field, rpcField, element)
		if err != nil {
			return nil, err
		}

		if val != nil {
			itemsField.Append(protoref.ValueOfMessage(val))
		}
	}

	rootMsg.Set(listFieldDesc, newListField)
	return rootMsg, nil
}

func (p *RPCCompiler) getEnumValue(enumName string, data gjson.Result) (*protoref.Value, error) {
	enum, ok := p.doc.EnumByName(enumName)
	if !ok {
		return nil, fmt.Errorf("enum %s not found in document", enumName)
	}

	for _, enumValue := range enum.Values {
		if enumValue.GraphqlValue == data.String() {
			v := protoref.ValueOfEnum(protoref.EnumNumber(enumValue.Number))
			return &v, nil
		}
	}

	return nil, nil
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
func (p *RPCCompiler) parseEnum(e protoref.EnumDescriptor, mapping *GRPCMapping) Enum {
	var enumValueMappings []EnumValueMapping
	name := string(e.Name())

	if mapping != nil && mapping.EnumValues != nil {
		enumValueMappings = mapping.EnumValues[name]
	}

	values := make([]EnumValue, 0, e.Values().Len())

	for j := 0; j < e.Values().Len(); j++ {
		values = append(values, p.parseEnumValue(e.Values().Get(j), enumValueMappings))
	}

	return Enum{Name: name, Values: values}
}

// parseEnumValue extracts information from a protobuf enum value descriptor.
func (p *RPCCompiler) parseEnumValue(v protoref.EnumValueDescriptor, enumValueMappings []EnumValueMapping) EnumValue {
	name := string(v.Name())
	number := int32(v.Number())

	enumValue := EnumValue{Name: name, Number: number}

	for _, mapping := range enumValueMappings {
		if mapping.TargetValue == name {
			enumValue.GraphqlValue = mapping.Value
			break
		}
	}

	return enumValue
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
		FullName:    string(s.FullName()),
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

// parseMessageDefinitions extracts information from a protobuf message descriptor.
// It returns a slice of Message objects with the name and descriptor.
func (p *RPCCompiler) parseMessageDefinitions(messages protoref.MessageDescriptors) []Message {
	extractedMessages := make([]Message, 0, messages.Len())

	for i := 0; i < messages.Len(); i++ {
		protoMessage := messages.Get(i)

		message := Message{
			Name: p.fullMessageName(protoMessage),
			Desc: protoMessage,
		}

		extractedMessages = append(extractedMessages, message)

		if submessages := protoMessage.Messages(); submessages.Len() > 0 {
			extractedMessages = append(extractedMessages, p.parseMessageDefinitions(submessages)...)
		}

	}

	return extractedMessages
}

// fullMessageName returns the full name of the message omiting the package name.
// In our case don't need the fqn as we only have one package where we need to resolve the messages.
func (p *RPCCompiler) fullMessageName(m protoref.MessageDescriptor) string {
	return strings.TrimLeft(string(m.FullName()), p.doc.Package+".")
}

// enrichMessageData enriches the message data with the field information.
func (p *RPCCompiler) enrichMessageData(ref int, m protoref.MessageDescriptor) {
	fields := make([]Field, m.Fields().Len())
	msg := &p.doc.Messages[ref]
	// Process all fields in the message
	for i := 0; i < m.Fields().Len(); i++ {
		f := m.Fields().Get(i)

		field := p.parseField(f)

		if f.Kind() == protoref.MessageKind {
			// Handle nested messages when they are recursive types
			field.MessageRef = p.doc.MessageRefByName(p.fullMessageName(f.Message()))
		}

		fields[i] = field
	}

	msg.AllocFields(len(fields))

	for i := range fields {
		msg.SetField(fields[i])
	}
}

// parseField extracts information from a protobuf field descriptor.
func (p *RPCCompiler) parseField(f protoref.FieldDescriptor) Field {
	name := string(f.Name())
	typeName := f.Kind().String()

	return Field{
		Name:       name,
		Type:       parseDataType(typeName),
		Number:     int32(f.Number()),
		Repeated:   f.IsList(),
		Optional:   f.Cardinality() == protoref.Optional,
		MessageRef: InvalidRef,
	}
}

func isAllowedForTypename(message *RPCMessage, element gjson.Result) bool {
	if message == nil {
		// We assume that having a nil message expects a null value.
		return true
	}

	// If we don't have a member types, we assume that the message is allowed for all types.
	if message.MemberTypes == nil {
		return true
	}

	typeName := element.Get("__typename")
	if !typeName.Exists() {
		// If we don't have a type name, we assume that the message is allowed for all types.
		return true
	}

	typeString := typeName.String()

	// If we have a type name, we need to check if the message is restricted to a specific type.
	return slices.Contains(message.MemberTypes, typeString)
}
