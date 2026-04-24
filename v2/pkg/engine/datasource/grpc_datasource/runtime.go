package grpcdatasource

import (
	"fmt"

	protoref "google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

type runtimeSchema struct {
	messageByName        map[string]*runtimeMessage
	messageByFullname    map[string]*runtimeMessage
	enumByName           map[string]*runtimeEnum
	serviceNamesByMethod map[string]string
}

type runtimeMessage struct {
	name         string
	desc         protoref.MessageDescriptor
	dynamicType  protoref.MessageType
	fieldsByName map[string]*runtimeField
}

type runtimeEnum struct {
	name         string
	valuesByName map[string]*runtimeEnumValue
}

type runtimeEnumValue struct {
	name  string
	value int32
}

type runtimeField struct {
	name     string
	owner    *runtimeMessage
	desc     protoref.FieldDescriptor
	genDesc  protoref.FieldDescriptor
	dataType DataType
	message  *runtimeMessage
	repeated bool
	optional bool
}

func newSchemaRuntime(doc *Document) (*runtimeSchema, error) {
	runtime := &runtimeSchema{
		messageByName:        make(map[string]*runtimeMessage, len(doc.Messages)),
		messageByFullname:    make(map[string]*runtimeMessage, len(doc.Messages)),
		serviceNamesByMethod: make(map[string]string, len(doc.Methods)),
		enumByName:           make(map[string]*runtimeEnum, len(doc.Enums)),
	}

	for i := range doc.Messages {
		message := &doc.Messages[i]

		rtMessage := &runtimeMessage{
			name:         message.Name,
			desc:         message.Desc,
			dynamicType:  dynamicpb.NewMessageType(message.Desc),
			fieldsByName: make(map[string]*runtimeField, message.Desc.Fields().Len()),
		}

		runtime.messageByName[message.Name] = rtMessage
		runtime.messageByFullname[string(message.Desc.FullName())] = rtMessage
	}

	for _, message := range runtime.messageByName {
		if err := appendMessageFields(runtime, message); err != nil {
			return nil, err
		}
	}

	for _, service := range doc.Services {
		for i := range service.MethodsRefs {
			runtime.serviceNamesByMethod[doc.Methods[i].Name] = service.FullName
		}
	}

	for _, enum := range doc.Enums {
		rtEnum := &runtimeEnum{
			name:         enum.Name,
			valuesByName: make(map[string]*runtimeEnumValue, len(enum.Values)),
		}

		for _, value := range enum.Values {
			rtEnum.valuesByName[value.GraphqlValue] = &runtimeEnumValue{
				name:  value.Name,
				value: value.Number,
			}
		}

		runtime.enumByName[enum.Name] = rtEnum
	}

	return runtime, nil
}

func appendMessageFields(runtime *runtimeSchema, message *runtimeMessage) error {
	for i := 0; i < message.desc.Fields().Len(); i++ {
		fieldDesc := message.desc.Fields().Get(i)

		field := &runtimeField{
			owner:    message,
			name:     string(fieldDesc.Name()),
			desc:     fieldDesc,
			dataType: parseDataType(fieldDesc.Kind()),
			repeated: fieldDesc.IsList(),
			optional: fieldDesc.Cardinality() == protoref.Optional,
		}

		if field.dataType == DataTypeMessage {
			child, found := runtime.messageByFullname[string(fieldDesc.Message().FullName())]
			if !found {
				return fmt.Errorf("message %s not found in document", string(fieldDesc.Message().FullName()))
			}

			field.message = child
		}

		message.fieldsByName[string(fieldDesc.Name())] = field
	}

	return nil
}

func (r *runtimeSchema) getMessageByName(name string) *runtimeMessage {
	message, found := r.messageByName[name]
	if !found {
		return nil
	}

	return message
}

func (r *runtimeSchema) getMessageByFullName(fullname string) *runtimeMessage {
	message, found := r.messageByFullname[fullname]
	if !found {
		return nil
	}

	return message
}

func (m *runtimeMessage) newEmptyMessage() protoref.Message {
	return m.dynamicType.New()
}
