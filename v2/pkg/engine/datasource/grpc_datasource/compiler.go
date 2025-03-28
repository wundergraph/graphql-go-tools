package grpcdatasource

import (
	"context"
	"fmt"

	"github.com/bufbuild/protocompile"
	"github.com/tidwall/gjson"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type DataType string

const (
	DataTypeString  DataType = "string"
	DataTypeInt32   DataType = "int32"
	DataTypeInt64   DataType = "int64"
	DataTypeUint32  DataType = "uint32"
	DataTypeUint64  DataType = "uint64"
	DataTypeFloat   DataType = "float"
	DataTypeDouble  DataType = "double"
	DataTypeBool    DataType = "bool"
	DataTypeEnum    DataType = "enum"
	DataTypeMessage DataType = "message"
	DataTypeGroup   DataType = "group"
	DataTypeUnknown DataType = "<unknown>"
)

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
	"group":   DataTypeGroup,
}

func (d DataType) String() string {
	return string(d)
}

func (d DataType) IsValid() bool {
	_, ok := dataTypeMap[string(d)]
	return ok
}

func parseDataType(name string) DataType {
	if !dataTypeMap[name].IsValid() {
		return DataTypeUnknown
	}

	return dataTypeMap[name]
}

type Document struct {
	Package  string
	Services []Service
	Enums    []Enum
	Messages []Message
	Methods  []Method
}

type Service struct {
	Name        string
	MethodsRefs []int
}

type Method struct {
	Name       string
	InputName  string
	InputRef   int
	OutputName string
	OutputRef  int
}

type Message struct {
	Name   string
	Fields []Field
	Desc   protoref.MessageDescriptor
}

type Field struct {
	Name     string
	Type     DataType
	Number   int32
	Ref      int
	Repeated bool
	Optional bool
	Message  *Message
}

type Enum struct {
	Name   string
	Values []EnumValue
}

type EnumValue struct {
	Name   string
	Number int32
}

type ProtoCompiler struct {
	schema string
	doc    *Document
}

func (d *Document) ServiceByName(name string) Service {
	for _, s := range d.Services {
		if s.Name == name {
			return s
		}
	}

	return Service{}
}

func (d *Document) MethodByName(name string) Method {
	for _, m := range d.Methods {
		if m.Name == name {
			return m
		}
	}

	return Method{}
}

func (d *Document) MethodRefByName(name string) int {
	for i, m := range d.Methods {
		if m.Name == name {
			return i
		}
	}

	return -1
}

func (d *Document) MethodByRef(ref int) Method {
	return d.Methods[ref]
}

func (d *Document) ServiceByRef(ref int) Service {
	return d.Services[ref]
}

func (d *Document) MessageByName(name string) Message {
	for _, m := range d.Messages {
		if m.Name == name {
			return m
		}
	}

	return Message{}
}

func (d *Document) MessageRefByName(name string) int {
	for i, m := range d.Messages {
		if m.Name == name {
			return i
		}
	}

	return -1
}

func (d *Document) MessageByRef(ref int) Message {
	return d.Messages[ref]
}

func NewProtoCompiler(schema string) *ProtoCompiler {
	return &ProtoCompiler{schema: schema, doc: &Document{}}
}

func (p *ProtoCompiler) Compile(executionPlan *RPCExecutionPlan, data gjson.Result) error {
	for _, call := range executionPlan.Calls {
		// service := p.doc.ServiceByName(call.ServiceName)
		// method := p.doc.MethodByName(call.MethodName)
		inputMessage := p.doc.MessageByName(call.Request.Name)
		outputMessage := p.doc.MessageByName(call.Response.Name)

		request := p.buildProtoMessage(inputMessage, &call.Request, data)
		response := p.newEmptyMessage(outputMessage)

		fmt.Println(request.String(), response.String())
	}
	return nil
}

func (p *ProtoCompiler) newEmptyMessage(message Message) *dynamicpb.Message {
	return dynamicpb.NewMessage(message.Desc)
}

// TODO provide a way to have data
func (p *ProtoCompiler) buildProtoMessage(inputMessage Message, rpcMessage *RPCMessage, data gjson.Result) *dynamicpb.Message {
	if rpcMessage == nil {
		return nil
	}

	message := dynamicpb.NewMessage(inputMessage.Desc)

	for _, field := range inputMessage.Fields {
		fd := inputMessage.Desc.Fields()

		rpcField := rpcMessage.Fields.ByName(field.Name)
		if rpcField == nil {
			continue
		}

		if field.Repeated {
			list := message.Mutable(fd.ByName(protoref.Name(field.Name))).List()

			elements := data.Get(rpcField.JSONPath).Array()
			if len(elements) == 0 {
				continue
			}

			for _, element := range elements {
				fieldMsg := p.buildProtoMessage(*field.Message, rpcField.Message, element)
				list.Append(protoref.ValueOfMessage(fieldMsg))
			}

			continue
		}

		if field.Message != nil {
			fieldMsg := p.buildProtoMessage(*field.Message, rpcField.Message, data)
			message.Set(inputMessage.Desc.Fields().ByName(protoref.Name(field.Name)), protoref.ValueOfMessage(fieldMsg))

			continue
		}

		message.Set(fd.ByName(protoref.Name(field.Name)), p.setValueForKind(field.Type, data))
	}

	return message
}

func (p *ProtoCompiler) setValueForKind(kind DataType, data gjson.Result) protoref.Value {
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

func (p *ProtoCompiler) Parse() error {
	c := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			Accessor: protocompile.SourceAccessorFromMap(map[string]string{
				"": p.schema,
			}),
		}),
	}

	fd, err := c.Compile(context.Background(), "")
	if err != nil {
		return err
	}

	if len(fd) == 0 {
		return fmt.Errorf("no files compiled")
	}

	f := fd[0]
	p.doc.Package = string(f.Package())

	for i := 0; i < f.Enums().Len(); i++ {
		p.doc.Enums = append(p.doc.Enums, p.parseEnum(f.Enums().Get(i)))
	}

	for i := 0; i < f.Messages().Len(); i++ {
		p.doc.Messages = append(p.doc.Messages, p.parseMessage(f.Messages().Get(i)))
	}

	for i := 0; i < f.Services().Len(); i++ {
		p.doc.Services = append(p.doc.Services, p.parseService(f.Services().Get(i)))
	}

	return nil
}

func (p *ProtoCompiler) parseEnum(e protoref.EnumDescriptor) Enum {
	name := string(e.Name())

	values := make([]EnumValue, 0, e.Values().Len())

	for j := 0; j < e.Values().Len(); j++ {
		values = append(values, p.parseEnumValue(e.Values().Get(j)))
	}

	return Enum{Name: name, Values: values}
}

func (p *ProtoCompiler) parseEnumValue(v protoref.EnumValueDescriptor) EnumValue {
	name := string(v.Name())
	number := int32(v.Number())

	return EnumValue{Name: name, Number: number}
}

func (p *ProtoCompiler) parseService(s protoref.ServiceDescriptor) Service {
	name := string(s.Name())
	m := s.Methods()

	methods := make([]Method, 0, m.Len())
	methodsRefs := make([]int, 0, m.Len())

	for j := 0; j < m.Len(); j++ {
		methods = append(methods, p.parseMethod(m.Get(j)))
		methodsRefs = append(methodsRefs, j)
	}

	p.doc.Methods = append(p.doc.Methods, methods...)

	return Service{
		Name:        name,
		MethodsRefs: methodsRefs,
	}
}

func (p *ProtoCompiler) parseMethod(m protoref.MethodDescriptor) Method {
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

func (p *ProtoCompiler) parseMessage(m protoref.MessageDescriptor) Message {
	name := string(m.Name())

	fields := []Field{}

	for i := 0; i < m.Fields().Len(); i++ {
		f := m.Fields().Get(i)

		field := p.parseField(f)

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

func (p *ProtoCompiler) parseField(f protoref.FieldDescriptor) Field {
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
