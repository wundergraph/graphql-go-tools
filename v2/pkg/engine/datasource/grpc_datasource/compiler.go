package grpcdatasource

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bufbuild/protocompile"
	"github.com/tidwall/gjson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
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
	Name   string                     // The name of the message
	Fields []Field                    // The fields in the message
	Desc   protoref.MessageDescriptor // The protobuf descriptor for the message
}

// FieldByName returns a field by its name.
// Returns nil if no field with the given name exists.
func (m *Message) FieldByName(name string) *Field {
	for index, field := range m.Fields {
		if field.Name == name {
			return &m.Fields[index]
		}
	}

	return nil
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
	report   operationreport.Report
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
// We only expect this function to return false if either the message name was provided incorrectly,
// or the schema and mapping was not properly configured.
func (d *Document) MessageByName(name string) (Message, bool) {
	for _, m := range d.Messages {
		if m.Name == name {
			return m, true
		}
	}

	return Message{}, false
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
	for _, e := range d.Enums {
		if e.Name == name {
			return e, true
		}
	}

	return Enum{}, false
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
		report: operationreport.Report{},
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

// ConstructExecutionPlan constructs an RPCExecutionPlan from a parsed GraphQL operation and schema.
// It will return an error if the operation does not match the protobuf definition provided to the compiler.
func (p *RPCCompiler) ConstructExecutionPlan(operation, schema *ast.Document) (*RPCExecutionPlan, error) {
	return nil, nil
}

// ServiceCall represents a single gRPC service call with its input and output messages.
type ServiceCall struct {
	// ServiceName is the name of the gRPC service to call
	ServiceName string
	// MethodName is the name of the method on the service to call
	MethodName string
	// Input is the input message for the gRPC call
	Input *dynamicpb.Message
	// Output is the output message for the gRPC call
	Output *dynamicpb.Message
	// RPC is the call that was made to the gRPC service
	RPC *RPCCall
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

	inputMessage, ok := p.doc.MessageByName(call.Request.Name)
	if !ok {
		return ServiceCall{}, fmt.Errorf("input message %s not found in document", call.Request.Name)
	}

	outputMessage, ok := p.doc.MessageByName(call.Response.Name)
	if !ok {
		return ServiceCall{}, fmt.Errorf("output message %s not found in document", call.Response.Name)
	}

	request, response := p.newEmptyMessage(inputMessage), p.newEmptyMessage(outputMessage)

	switch call.Kind {
	case CallKindStandard, CallKindEntity:
		request = p.buildProtoMessage(inputMessage, &call.Request, inputData)
	case CallKindResolve:
		context, err := graph.FetchDependencies(&fetch)
		if err != nil {
			return ServiceCall{}, err
		}

		if len(context) == 0 {
			return ServiceCall{}, fmt.Errorf("context is required for resolve calls")
		}

		request = p.buildProtoMessageWithContext(inputMessage, &call.Request, inputData, context)
	}

	serviceName, ok := p.resolveServiceName(call.MethodName)
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

// Compile processes an RPCExecutionPlan and builds protobuf messages from JSON data
// based on the compiled schema.
// Deprecated: Use CompileFetches instead.
func (p *RPCCompiler) Compile(executionPlan *RPCExecutionPlan, inputData gjson.Result) ([]ServiceCall, error) {
	serviceCalls := make([]ServiceCall, 0, len(executionPlan.Calls))

	for _, call := range executionPlan.Calls {
		inputMessage, ok := p.doc.MessageByName(call.Request.Name)
		if !ok {
			return nil, fmt.Errorf("input message %s not found in document", call.Request.Name)
		}

		outputMessage, ok := p.doc.MessageByName(call.Response.Name)
		if !ok {
			return nil, fmt.Errorf("output message %s not found in document", call.Response.Name)
		}

		request := p.buildProtoMessage(inputMessage, &call.Request, inputData)
		response := p.newEmptyMessage(outputMessage)

		if p.report.HasErrors() {
			return nil, fmt.Errorf("failed to compile invocation: %w", p.report)
		}

		serviceName, ok := p.resolveServiceName(call.MethodName)
		if !ok {
			return nil, fmt.Errorf("failed to resolve service name for method %s from the protobuf definition", call.MethodName)
		}

		serviceCalls = append(serviceCalls, ServiceCall{
			ServiceName: serviceName,
			MethodName:  call.MethodName,
			Input:       request,
			Output:      response,
			RPC:         &call,
		})
	}

	return serviceCalls, nil
}

func (p *RPCCompiler) resolveServiceName(methodName string) (string, bool) {
	for _, service := range p.doc.Services {
		for _, methodRef := range service.MethodsRefs {
			if p.doc.Methods[methodRef].Name == methodName {
				return service.FullName, true
			}
		}
	}

	return "", false
}

// newEmptyMessage creates a new empty dynamicpb.Message from a Message definition.
func (p *RPCCompiler) newEmptyMessage(message Message) *dynamicpb.Message {
	if p.doc.MessageRefByName(message.Name) == InvalidRef {
		p.report.AddInternalError(fmt.Errorf("message %s not found in document", message.Name))
		return nil
	}

	return dynamicpb.NewMessage(message.Desc)
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
func (p *RPCCompiler) buildProtoMessageWithContext(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result, context []FetchItem) *dynamicpb.Message {
	if rpcMessage == nil {
		return nil
	}

	if p.doc.MessageRefByName(rpcMessage.Name) == InvalidRef {
		p.report.AddInternalError(fmt.Errorf("message %s not found in document", rpcMessage.Name))
		return nil
	}

	rootMessage := dynamicpb.NewMessage(inputMessage.Desc)

	if len(inputMessage.Fields) != 2 {
		p.report.AddInternalError(fmt.Errorf("message %s must have exactly two fields: context and field_args", inputMessage.Name))
		return nil
	}

	contextSchemaField := inputMessage.FieldByName("context")
	if contextSchemaField == nil {
		p.report.AddInternalError(fmt.Errorf("context field not found in message %s", inputMessage.Name))
		return nil
	}

	contextRPCField := rpcMessage.Fields.ByName(contextSchemaField.Name)
	if contextRPCField == nil {
		p.report.AddInternalError(fmt.Errorf("context field not found in message %s", rpcMessage.Name))
		return nil
	}

	contextField := rootMessage.Descriptor().Fields().ByNumber(protoref.FieldNumber(contextSchemaField.Number))
	if contextField == nil {
		p.report.AddInternalError(fmt.Errorf("context field not found in message %s", inputMessage.Name))
		return nil
	}

	contextList := p.newEmptyListMessageByName(rootMessage, contextSchemaField.Name)
	contextData := p.resolveContextData(context[0], contextRPCField) // TODO handle multiple contexts (resolver requires another resolver)

	for _, data := range contextData {
		val := contextList.NewElement()
		valMsg := val.Message()
		for fieldName, value := range data {
			p.setMessageValue(valMsg, fieldName, value)
		}

		contextList.Append(val)
	}

	argsSchemaField := inputMessage.FieldByName("field_args")
	if argsSchemaField == nil {
		p.report.AddInternalError(fmt.Errorf("field_args field not found in message %s", inputMessage.Name))
		return nil
	}

	argsMessage := p.doc.Messages[argsSchemaField.MessageRef]
	argsRPCField := rpcMessage.Fields.ByName("field_args")
	if argsRPCField == nil {
		p.report.AddInternalError(fmt.Errorf("field_args field not found in message %s", rpcMessage.Name))
		return nil
	}

	args := p.buildProtoMessage(argsMessage, argsRPCField.Message, data)

	// // Set the key list
	p.setMessageValue(rootMessage, contextSchemaField.Name, protoref.ValueOfList(contextList))
	p.setMessageValue(rootMessage, argsRPCField.Name, protoref.ValueOfMessage(args))

	return rootMessage
}

func (p *RPCCompiler) resolveContextData(context FetchItem, contextField *RPCField) []map[string]protoref.Value {
	if context.ServiceCall == nil || context.ServiceCall.Output == nil {
		return []map[string]protoref.Value{}
	}

	contextValues := make([]map[string]protoref.Value, 0)
	outputMessage := context.ServiceCall.Output
	for _, field := range contextField.Message.Fields {
		resolvePath := field.ResolvePath
		values := p.resolveContextDataForPath(outputMessage, resolvePath)

		for index, value := range values {
			if index >= len(contextValues) {
				contextValues = append(contextValues, make(map[string]protoref.Value))
			}

			contextValues[index][field.Name] = value
		}

	}

	return contextValues
}

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
					values := p.resolveListDataForPath(list, fd, path)
					result = append(result, values...)
					continue
				}
			}

			result = append(result, values...)
		default:
			result = append(result, item)
		}
	}

	return result
}

func (p *RPCCompiler) resolveDataForPath(messsage protoref.Message, path ast.Path) []protoref.Value {
	if path.Len() == 0 {
		return nil
	}

	segment := path[0]

	if fn := segment.FieldName.String(); strings.HasPrefix(fn, "@") {
		list := p.resolveUnderlyingList(messsage, fn)

		result := make([]protoref.Value, 0, len(list))
		for _, item := range list {
			result = append(result, p.resolveDataForPath(item.Message(), path[1:])...)
		}

		return result
	}

	field, fd := p.getMessageField(messsage, segment.FieldName.String())
	if !field.IsValid() {
		return nil
	}

	switch fd.Kind() {
	case protoref.MessageKind:
		if fd.IsList() {
			return []protoref.Value{field}
		}

		return p.resolveDataForPath(field.Message(), path[1:])
	default:
		return []protoref.Value{field}
	}
}

// getMessageField gets the field from the message by its name.
// It also handles nested lists and nullable lists.
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

func (p *RPCCompiler) resolveUnderlyingListItems(value protoref.Value, nestingLevel int) []protoref.Value {
	msg := value.Message()
	fd := msg.Descriptor().Fields().ByNumber(1)
	if fd == nil {
		return nil
	}

	listMsg := msg.Get(fd)
	if !listMsg.IsValid() {
		return nil
	}

	itemsValue := listMsg.Message().Get(listMsg.Message().Descriptor().Fields().ByNumber(1))
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

func (p *RPCCompiler) newEmptyListMessageByName(msg *dynamicpb.Message, name string) protoref.List {
	return msg.Mutable(msg.Descriptor().Fields().ByName(protoref.Name(name))).List()
}

func (p *RPCCompiler) setMessageValue(message protoref.Message, fieldName string, value protoref.Value) {
	message.Set(message.Descriptor().Fields().ByName(protoref.Name(fieldName)), value)
}

// buildProtoMessage recursively builds a protobuf message from an RPCMessage definition
// and JSON data. It handles nested messages and repeated fields.
func (p *RPCCompiler) buildProtoMessage(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result) *dynamicpb.Message {
	if rpcMessage == nil {
		return nil
	}

	inputMessageRef := p.doc.MessageRefByName(inputMessage.Name)
	if inputMessageRef == InvalidRef {
		p.report.AddInternalError(fmt.Errorf("message %s not found in document", inputMessage.Name))
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
				switch field.Type {
				case DataTypeMessage:
					// If we handle entity lookups, we get a list of representation variables that need to
					// be applied to the correct type names.
					if !isAllowedForTypename(rpcField.Message, element) {
						continue
					}

					fieldMsg := p.buildProtoMessage(p.doc.Messages[field.MessageRef], rpcField.Message, element)
					list.Append(protoref.ValueOfMessage(fieldMsg))
				default:
					list.Append(p.setValueForKind(field.Type, element))
				}
			}

			continue
		}

		// Handle nested message fields
		if field.MessageRef >= 0 {
			var fieldMsg *dynamicpb.Message

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
						p.report.AddInternalError(fmt.Errorf("field %s is required but has no value", rpcField.JSONPath))
					}

					continue
				}

				if rpcField.ListMetadata == nil {
					p.report.AddInternalError(fmt.Errorf("list metadata not found for field %s", rpcField.JSONPath))
					continue
				}

				fieldMsg = p.buildListMessage(inputMessage.Desc, field, rpcField, data)
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
				fieldMsg = p.buildProtoMessage(
					p.doc.Messages[field.MessageRef],
					rpcField.ToOptionalTypeMessage(p.doc.Messages[field.MessageRef].Name),
					data,
				)
			default:
				fieldMsg = p.buildProtoMessage(p.doc.Messages[field.MessageRef], rpcField.Message, data.Get(rpcField.JSONPath))
			}

			message.Set(inputMessage.Desc.Fields().ByName(protoref.Name(field.Name)), protoref.ValueOfMessage(fieldMsg))
			continue
		}

		if field.Type == DataTypeEnum {
			if val := p.getEnumValue(rpcField.EnumName, data.Get(rpcField.JSONPath)); val != nil {
				message.Set(
					fd.ByName(protoref.Name(field.Name)),
					*val,
				)
			}

			continue
		}

		// Handle scalar fields
		value := data.Get(rpcField.JSONPath)
		message.Set(fd.ByName(protoref.Name(field.Name)), p.setValueForKind(field.Type, value))
	}

	return message
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
func (p *RPCCompiler) buildListMessage(desc protoref.MessageDescriptor, field Field, rpcField *RPCField, data gjson.Result) *dynamicpb.Message {
	rootMsg := dynamicpb.NewMessage(desc.Fields().ByName(protoref.Name(field.Name)).Message())
	p.traverseList(rootMsg, 1, field, rpcField, data.Get(rpcField.JSONPath))
	return rootMsg
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
func (p *RPCCompiler) traverseList(rootMsg protoref.Message, level int, field Field, rpcField *RPCField, data gjson.Result) protoref.Message {
	listFieldDesc := rootMsg.Descriptor().Fields().ByNumber(1)
	if listFieldDesc == nil {
		p.report.AddInternalError(fmt.Errorf("field with number %d not found in message %s", 1, rootMsg.Descriptor().Name()))
		return nil
	}

	elements := data.Array()
	newListField := rootMsg.NewField(listFieldDesc)
	if len(elements) == 0 {
		if rpcField.ListMetadata.LevelInfo[level-1].Optional {
			return nil
		}

		rootMsg.Set(listFieldDesc, newListField)
		return rootMsg
	}

	// Inside of a List message type we expect a repeated "items" field with field number 1
	itemsFieldMsg := newListField.Message()
	itemsFieldDesc := itemsFieldMsg.Descriptor().Fields().ByNumber(1)
	if itemsFieldDesc == nil {
		p.report.AddInternalError(fmt.Errorf("field with number %d not found in message %s", 1, itemsFieldMsg.Descriptor().Name()))
		return nil
	}

	itemsField := itemsFieldMsg.Mutable(itemsFieldDesc).List()

	if level >= rpcField.ListMetadata.NestingLevel {
		switch DataType(rpcField.TypeName) {
		case DataTypeMessage:
			itemsFieldMsg, ok := p.doc.MessageByName(rpcField.Message.Name)
			if !ok {
				p.report.AddInternalError(fmt.Errorf("message %s not found in document", rpcField.Message.Name))
				return nil
			}

			for _, element := range elements {
				if msg := p.buildProtoMessage(itemsFieldMsg, rpcField.Message, element); msg != nil {
					itemsField.Append(protoref.ValueOfMessage(msg))
				}
			}
		case DataTypeEnum:
			for _, element := range elements {
				if val := p.getEnumValue(rpcField.EnumName, element); val != nil {
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
		return rootMsg
	}

	for _, element := range elements {
		newElement := itemsField.NewElement()
		if val := p.traverseList(newElement.Message(), level+1, field, rpcField, element); val != nil {
			itemsField.Append(protoref.ValueOfMessage(val))
		}
	}

	rootMsg.Set(listFieldDesc, newListField)
	return rootMsg
}

func (p *RPCCompiler) getEnumValue(enumName string, data gjson.Result) *protoref.Value {
	enum, ok := p.doc.EnumByName(enumName)
	if !ok {
		p.report.AddInternalError(fmt.Errorf("enum %s not found in document", enumName))
		return nil
	}

	for _, enumValue := range enum.Values {
		if enumValue.GraphqlValue == data.String() {
			v := protoref.ValueOfEnum(protoref.EnumNumber(enumValue.Number))
			return &v
		}
	}

	return nil
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
			Name: string(protoMessage.Name()),
			Desc: protoMessage,
		}

		extractedMessages = append(extractedMessages, message)
	}

	return extractedMessages
}

// enrichMessageData enriches the message data with the field information.
func (p *RPCCompiler) enrichMessageData(ref int, m protoref.MessageDescriptor) {
	fields := []Field{}

	msg := p.doc.Messages[ref]
	// Process all fields in the message
	for i := 0; i < m.Fields().Len(); i++ {
		f := m.Fields().Get(i)

		field := p.parseField(f)

		if f.Kind() == protoref.MessageKind {
			// Handle nested messages when they are recursive types
			field.MessageRef = p.doc.MessageRefByName(string(f.Message().Name()))
		}

		fields = append(fields, field)
	}

	msg.Fields = fields
	p.doc.Messages[ref] = msg
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
