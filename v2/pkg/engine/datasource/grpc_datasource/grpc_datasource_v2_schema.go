package grpcdatasource

import (
	"fmt"

	"buf.build/go/hyperpb"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

type v2SchemaRuntime struct {
	messageByName        map[string]*v2MessageRuntime
	messageByFullName    map[string]*v2MessageRuntime
	methodByName         map[string]*Method
	serviceNamesByMethod map[string]string
}

type v2MessageRuntime struct {
	name          string
	desc          protoref.MessageDescriptor
	generatedDesc protoref.MessageDescriptor
	dynamicType   protoref.MessageType
	generatedType protoref.MessageType
	hyperType     *hyperpb.MessageType
	fieldsByName  map[string]*v2FieldRuntime
}

type v2FieldRuntime struct {
	name      string
	owner     *v2MessageRuntime
	desc      protoref.FieldDescriptor
	genDesc   protoref.FieldDescriptor
	dataType  DataType
	message   *v2MessageRuntime
	repeated  bool
	optional  bool
	isMessage bool
}

func newV2SchemaRuntime(compiler *RPCCompiler) (*v2SchemaRuntime, error) {
	runtime := &v2SchemaRuntime{
		messageByName:        make(map[string]*v2MessageRuntime, len(compiler.doc.Messages)),
		messageByFullName:    make(map[string]*v2MessageRuntime, len(compiler.doc.Messages)),
		methodByName:         make(map[string]*Method, len(compiler.doc.Methods)),
		serviceNamesByMethod: make(map[string]string, len(compiler.doc.Methods)),
	}

	for i := range compiler.doc.Messages {
		message := compiler.doc.Messages[i]
		generatedType, _ := protoregistry.GlobalTypes.FindMessageByName(message.Desc.FullName())
		v2Message := &v2MessageRuntime{
			name:          message.Name,
			desc:          message.Desc,
			dynamicType:   dynamicpb.NewMessageType(message.Desc),
			generatedType: generatedType,
			hyperType:     hyperpb.CompileMessageDescriptor(message.Desc),
			fieldsByName:  make(map[string]*v2FieldRuntime, message.Desc.Fields().Len()),
		}
		if generatedType != nil {
			v2Message.generatedDesc = generatedType.Descriptor()
		}
		runtime.messageByName[message.Name] = v2Message
		runtime.messageByFullName[string(message.Desc.FullName())] = v2Message
	}

	for _, message := range runtime.messageByName {
		for i := 0; i < message.desc.Fields().Len(); i++ {
			fd := message.desc.Fields().Get(i)
			field := &v2FieldRuntime{
				owner:     message,
				name:      string(fd.Name()),
				desc:      fd,
				dataType:  parseDataType(fd.Kind()),
				repeated:  fd.IsList(),
				optional:  fd.HasOptionalKeyword(),
				isMessage: fd.Kind() == protoref.MessageKind,
			}
			if message.generatedDesc != nil {
				field.genDesc = message.generatedDesc.Fields().ByName(fd.Name())
			}
			if field.isMessage {
				child, ok := runtime.messageByFullName[string(fd.Message().FullName())]
				if !ok {
					return nil, fmt.Errorf("message runtime not found for %s", fd.Message().FullName())
				}
				field.message = child
			}
			message.fieldsByName[field.name] = field
		}
	}

	for i := range compiler.doc.Methods {
		method := &compiler.doc.Methods[i]
		runtime.methodByName[method.Name] = method
	}

	for i := range compiler.doc.Services {
		service := &compiler.doc.Services[i]
		for _, methodRef := range service.MethodsRefs {
			if methodRef < 0 || methodRef >= len(compiler.doc.Methods) {
				return nil, fmt.Errorf("invalid method ref %d for service %s", methodRef, service.Name)
			}
			runtime.serviceNamesByMethod[compiler.doc.Methods[methodRef].Name] = service.FullName
		}
	}

	return runtime, nil
}

func (r *v2SchemaRuntime) messageRuntime(name string) (*v2MessageRuntime, bool) {
	msg, ok := r.messageByName[name]
	return msg, ok
}

func (m *v2MessageRuntime) newMessage() protoref.Message {
	if m.generatedType != nil {
		return m.generatedType.New()
	}
	return m.dynamicType.New()
}

func (m *v2MessageRuntime) newDecodeMessage(shared *hyperpb.Shared) protoref.Message {
	if m.generatedType != nil {
		return m.generatedType.New()
	}
	if m.hyperType != nil && shared != nil {
		return shared.NewMessage(m.hyperType)
	}
	return m.dynamicType.New()
}

func (f *v2FieldRuntime) descriptorFor(message protoref.Message) protoref.FieldDescriptor {
	if f.genDesc != nil && f.owner != nil && f.owner.generatedType != nil && message.Type() == f.owner.generatedType {
		return f.genDesc
	}
	return f.desc
}
