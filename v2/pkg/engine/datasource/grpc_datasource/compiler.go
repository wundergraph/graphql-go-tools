package grpcdatasource

import (
	"context"
	"fmt"
	"strings"

	"github.com/bufbuild/protocompile"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

const (
	// InvalidRef is a constant used to indicate that a reference is invalid.
	InvalidRef = -1
)

func fromGraphQLType(s []byte) DataType {
	switch string(s) {
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
func parseDataType(kind protoref.Kind) DataType {
	if _, ok := dataTypeMap[kind]; !ok {
		return DataTypeUnknown
	}

	return dataTypeMap[kind]
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

// ResolveUnderlyingMessage returns the Message that this field points to via
// MessageRef, or nil if the field is not a message type.
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

// RPCCompiler compiles protobuf schema strings into a Document and runtime schema.
type RPCCompiler struct {
	doc     *Document      // The compiled Document
	runtime *runtimeSchema // The compiled runtime schema
}

// MessageByName returns a Message by its name.
// Returns false if no message with the given name exists.
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

	runtime, err := newSchemaRuntime(pc.doc)
	if err != nil {
		return nil, err
	}

	pc.runtime = runtime

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
	return strings.TrimPrefix(string(m.FullName()), p.doc.Package+".")
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

	return Field{
		Name:       name,
		Type:       parseDataType(f.Kind()),
		Number:     int32(f.Number()),
		Repeated:   f.IsList(),
		Optional:   f.Cardinality() == protoref.Optional,
		MessageRef: InvalidRef,
	}
}
