package grpcdatasource

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protowire"

	"github.com/wundergraph/astjson"
)

var errShouldSkip = errors.New("skip")

type PreWiredInputMessage struct {
	size   int
	buffer []byte
}

func NewPreWiredInputMessage(buffer []byte) *PreWiredInputMessage {
	return &PreWiredInputMessage{
		size:   len(buffer),
		buffer: buffer,
	}
}

func (c *PreWiredInputMessage) wire() ([]byte, error) {
	if c.buffer == nil {
		return nil, fmt.Errorf("connect message not initialized")
	}

	return c.buffer, nil
}

type wireMessage struct {
	fields      []wireField
	runtime     *runtimeMessage
	oneOfType   OneOfType
	oneOfFields map[string][]wireField
}

type wireField struct {
	tag          []byte
	runtime      *runtimeField
	number       protowire.Number
	dataType     DataType // GraphQL data type. Use runtime.dataType to encode scalar values.
	wireType     protowire.Type
	runtimeEnum  *runtimeEnum
	staticValue  string
	jsonPath     string
	optional     bool
	repeated     bool
	listMetadata *ListMetadata
	fieldMessage *runtimeMessage
	child        *wireMessage
}

const (
	minBufferSize = 1 << 8 // 256 bytes
)

func compileWireMessageFromRequest(schema *runtimeSchema, request *request) (*wireMessage, error) {
	if request == nil {
		return nil, fmt.Errorf("unable to compile wire message from request: request is nil")
	}

	return compileWireMessage(schema, request.message, make(map[string]*wireMessage))
}

func compileWireMessage(schema *runtimeSchema, msg *programMessage, cycleMap map[string]*wireMessage) (*wireMessage, error) {
	if msg == nil {
		return nil, fmt.Errorf("message not found for fetch request")
	}

	if seen, ok := cycleMap[msg.name]; ok {
		return seen, nil
	}

	messageFields := msg.fields

	wm := &wireMessage{
		runtime:   msg.runtime,
		fields:    make([]wireField, len(messageFields)),
		oneOfType: msg.oneOfType,
	}

	cycleMap[msg.name] = wm

	if wm.oneOfType != OneOfTypeNone {
		wm.oneOfFields = make(map[string][]wireField, len(msg.memberTypes))

		for _, memberType := range msg.memberTypes {

			fields, err := compileMessageFields(schema, msg.oneOfFields[memberType], cycleMap)
			if err != nil {
				return nil, err
			}

			wm.oneOfFields[memberType] = fields
		}
	}

	fields, err := compileMessageFields(schema, messageFields, cycleMap)
	if err != nil {
		return nil, err
	}

	wm.fields = fields
	return wm, nil
}

func compileMessageFields(schema *runtimeSchema, messageFields []programField, cycleMap map[string]*wireMessage) ([]wireField, error) {
	if len(messageFields) == 0 {
		return nil, nil
	}

	fields := make([]wireField, len(messageFields))

	for i := range messageFields {
		messageField := messageFields[i]

		wf := wireField{
			runtime:      messageField.runtime,
			number:       messageField.runtime.desc.Number(),
			fieldMessage: messageField.runtime.message,
			dataType:     messageField.dataType,
			wireType:     getWireType(messageField.runtime.dataType),
			jsonPath:     messageField.jsonPath,
			staticValue:  messageField.staticValue,
			optional:     messageField.optional,
			repeated:     messageField.repeated,
			listMetadata: messageField.listMetadata,
		}

		if messageField.enumName != "" {
			rtEnum, ok := schema.enumByName[messageField.enumName]
			if !ok {
				return nil, fmt.Errorf("enum not found for name %s", messageField.enumName)
			}

			wf.runtimeEnum = rtEnum
		}

		if messageField.child != nil {
			fieldMessageRuntime := messageField.child.runtime

			// we we are using wrapper messages, they are compiled from the protobuf schema but doesn't match with the RPC planner schema.
			// We need to resolve the correct message from the runtime schema.
			if fieldMessageRuntime.name != messageField.child.name {
				fieldMessageRuntime = schema.getMessageByName(messageField.child.runtime.name)
				if fieldMessageRuntime == nil {
					return nil, fmt.Errorf("message not found for name %s", messageField.child.runtime.name)
				}
			}

			child, err := compileWireMessage(schema, messageField.child, cycleMap)
			if err != nil {
				return nil, err
			}

			wf.child = child
		}

		wf.tag = protowire.AppendTag(nil, wf.number, wf.wireType)
		fields[i] = wf
	}

	return fields, nil
}

// jsonStringBytes returns the bytes for a proto string or bytes field.
// A JSON string yields its content. Any other value yields its JSON text: a
// number for a numeric GraphQL ID, or an object, array or boolean for a JSON
// scalar, which protographic maps to a proto string.
func jsonStringBytes(data *astjson.Value) []byte {
	switch data.Type() {
	case astjson.TypeString:
		return data.GetStringBytes()
	case astjson.TypeNull:
		return nil
	default:
		return data.MarshalTo(nil)
	}
}
