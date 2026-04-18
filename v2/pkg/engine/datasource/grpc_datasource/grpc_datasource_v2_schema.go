package grpcdatasource

import (
	"fmt"

	"buf.build/go/hyperpb"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/dynamicpb"
)

// v2SchemaRuntime caches every message + method + field descriptor used by the
// compiled plan. Built once at NewDataSourceV2 and shared across all requests.
//
// Each message carries descriptors for BOTH backend families: generated Go
// types (when the user's binary is linked with the .pb.go files) and the
// dynamic descriptor from the proto compiler. Field access dispatches on the
// actual message's type at runtime via `descriptorFor(msg)`. This mirrors the
// dual-backend pattern codex's hollow-playroom implementation used to deliver
// its biggest measured win — dynamicpb → generated proto cuts ~30% of heavy-
// benchmark allocations in one step.
type v2SchemaRuntime struct {
	messageByName        map[string]*v2MessageRuntime
	messageByFullName    map[string]*v2MessageRuntime
	methodByName         map[string]*Method
	serviceNamesByMethod map[string]string
}

// v2MessageRuntime is the multi-backend descriptor entry for a single message.
//
// Three backend families on the decode side:
//   - generated proto (fastest when linked)
//   - hyperpb (fastest for pure-decode workloads when generated not available)
//   - dynamicpb (always-available fallback)
//
// On the encode/write side only generated + dynamicpb are candidates — hyperpb
// is read-only. newMessage() picks the backend for an outgoing message;
// newReadMessage() picks for an incoming response.
type v2MessageRuntime struct {
	name          string
	desc          protoref.MessageDescriptor // compiler-provided dynamic descriptor
	generatedDesc protoref.MessageDescriptor // generated protobuf descriptor (nil if not linked)
	dynamicType   protoref.MessageType       // always present — dynamicpb
	generatedType protoref.MessageType       // non-nil when generated types are linked in-process
	hyperpbType   *hyperpb.MessageType       // pre-compiled once; only used for read paths
	fieldsByName  map[string]*v2FieldRuntime
}

// v2FieldRuntime carries both descriptor families so field access picks the
// right one based on which backend the caller's message actually uses.
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

	// Phase 1: allocate message runtimes, auto-discovering generated types.
	for i := range compiler.doc.Messages {
		message := compiler.doc.Messages[i]
		generatedType, _ := protoregistry.GlobalTypes.FindMessageByName(message.Desc.FullName())
		v2Message := &v2MessageRuntime{
			name:          message.Name,
			desc:          message.Desc,
			dynamicType:   dynamicpb.NewMessageType(message.Desc),
			generatedType: generatedType,
			hyperpbType:   hyperpb.CompileMessageDescriptor(message.Desc),
			fieldsByName:  make(map[string]*v2FieldRuntime, message.Desc.Fields().Len()),
		}
		if generatedType != nil {
			v2Message.generatedDesc = generatedType.Descriptor()
		}
		runtime.messageByName[message.Name] = v2Message
		runtime.messageByFullName[string(message.Desc.FullName())] = v2Message
	}

	// Phase 2: populate fields. Needs all messages built so child refs resolve.
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

// newMessage picks the cheapest available backend:
//   - generated protobuf (if linked) — fastest, zero reflection in marshal/unmarshal
//   - dynamicpb — always available fallback
//
// hyperpb is handled elsewhere (response-only via codec) and not plumbed here
// because it's read-only; the v2 request path still needs a writable message.
func (m *v2MessageRuntime) newMessage() protoref.Message {
	if m.generatedType != nil {
		return m.generatedType.New()
	}
	return m.dynamicType.New()
}

// descriptorFor returns the field descriptor matching the concrete message
// implementation. Generated protobuf types and dynamicpb use different
// descriptor identities for the same field.
func (f *v2FieldRuntime) descriptorFor(message protoref.Message) protoref.FieldDescriptor {
	if f.genDesc != nil && f.owner != nil && f.owner.generatedType != nil && message.Type() == f.owner.generatedType {
		return f.genDesc
	}
	return f.desc
}
