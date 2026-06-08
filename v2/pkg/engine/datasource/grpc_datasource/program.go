package grpcdatasource

import (
	"fmt"

	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type program struct {
	stages []stage
}

type stage struct {
	fetches []fetchProgram
}

type fetchProgram struct {
	id                  int
	kind                CallKind
	dependentCall       *RPCCall
	serviceName         string
	methodName          string
	methodFullName      string
	responsePath        ast.Path
	request             *request
	response            *response
	requestedEntityType string
}

type request struct {
	message *programMessage
	fields  []programField
	context *fetchRequestContext
	// The wire message will be created fromt the
	// request structure.
	wire *wireMessage
}

type programMessage struct {
	name        string
	runtime     *runtimeMessage
	oneOfType   OneOfType
	oneOfFields map[string][]programField
	memberTypes []string
	fields      []programField
}

type programField struct {
	runtime      *runtimeField
	dataType     DataType
	jsonPath     string
	enumName     string
	staticValue  string
	optional     bool
	repeated     bool
	listMetadata *ListMetadata
	child        *programMessage
}

type fetchRequestContext struct {
	message *runtimeMessage
	context *runtimeMessage
	fields  []fetchRequestContextField
}

type fetchRequestContextField struct {
	runtime     *runtimeField
	jsonName    string
	resolvePath ast.Path
}

type response struct {
	// response type is the type of the response message.
	responseType *runtimeMessage
	rpcMessage   RPCMessage
}

func (f *request) createProtoWire(requestVariables *astjson.Value) ([]byte, error) {
	return f.wire.createProtoWire(requestVariables)
}

func (f *request) createProtoWireWithContext(a arena.Arena, requestVariables *astjson.Value, contextMessage protoref.Message) ([]byte, error) {
	return f.wire.createProtoWireWithContext(a, requestVariables, f.context, contextMessage)
}

func (f *request) createProtoMessage(requestVariables *astjson.Value) (protoref.Message, error) {
	return f.wire.createProtoMessage(requestVariables)
}

func (f *request) createProtoMessageWithContext(a arena.Arena, requestVariables *astjson.Value, contextMessage protoref.Message) (protoref.Message, error) {
	return f.wire.createProtoMessageWithContext(a, requestVariables, f.context, contextMessage)
}

func compileProgram(plan *RPCExecutionPlan, runtime *runtimeSchema) (*program, error) {
	stageIndexes, err := compileStageIndexes(plan)
	if err != nil {
		return nil, err
	}

	// We are calculating the number of stages by finding the maximum stage index and adding 1.
	stageCount := 0
	for _, stageIndex := range stageIndexes {
		if stageIndex+1 > stageCount {
			stageCount = stageIndex + 1
		}
	}

	program := &program{
		stages: make([]stage, stageCount),
	}

	stageMap := make(map[int][]fetchProgram, stageCount)

	for i := range plan.Calls {
		call := &plan.Calls[i]

		// Currently we only support one dependent call.
		var dependentCall *RPCCall
		if len(call.DependentCalls) > 0 {
			dependentCall = &plan.Calls[call.DependentCalls[0]]
		}

		fetch, err := compileFetch(call, runtime, dependentCall)
		if err != nil {
			return nil, err
		}

		stageMap[stageIndexes[call.ID]] = append(stageMap[stageIndexes[call.ID]], fetch)
	}

	for i := 0; i < stageCount; i++ {
		program.stages[i] = stage{
			fetches: stageMap[i],
		}
	}

	return program, nil
}

func compileFetch(call *RPCCall, runtime *runtimeSchema, dependentCall *RPCCall) (fetchProgram, error) {
	serviceName, ok := runtime.serviceNamesByMethod[call.MethodName]
	if !ok {
		return fetchProgram{}, fmt.Errorf("service name not found for method %s", call.MethodName)
	}

	f := fetchProgram{
		id:                  call.ID,
		kind:                call.Kind,
		dependentCall:       dependentCall,
		serviceName:         serviceName,
		methodName:          call.MethodName,
		methodFullName:      "/" + serviceName + "/" + call.MethodName,
		responsePath:        call.ResponsePath,
		requestedEntityType: call.RequestedEntityType,
	}

	requestMessage := runtime.getMessageByName(call.Request.Name)
	if requestMessage == nil {
		return fetchProgram{}, fmt.Errorf("request message not found for method %s", call.MethodName)
	}

	responseMessage := runtime.getMessageByName(call.Response.Name)
	if responseMessage == nil {
		return fetchProgram{}, fmt.Errorf("response message not found for method %s", call.MethodName)
	}

	f.response = &response{
		responseType: responseMessage,
		rpcMessage:   call.Response,
	}

	switch f.kind {
	case CallKindStandard, CallKindEntity, CallKindRequired:
		fetchRequest, err := compileFetchRequest(runtime, &call.Request, requestMessage)
		if err != nil {
			return fetchProgram{}, err
		}
		f.request = fetchRequest

	case CallKindResolve:
		dependentMessage := runtime.getMessageByName(dependentCall.Response.Name)
		if dependentMessage == nil {
			return fetchProgram{}, fmt.Errorf("dependent message not found for method %s", dependentCall.MethodName)
		}

		fetchRequest, err := compileFetchRequestWithContext(runtime, requestMessage, dependentMessage, &call.Request)
		if err != nil {
			return fetchProgram{}, err
		}
		f.request = fetchRequest
	}

	wireMessage, err := compileWireMessageFromRequest(runtime, f.request)
	if err != nil {
		return fetchProgram{}, err
	}

	f.request.wire = wireMessage

	return f, nil
}

func compileFetchRequest(runtime *runtimeSchema, rpcMessage *RPCMessage, message *runtimeMessage) (*request, error) {
	requestMessage, err := compileMessage(runtime, rpcMessage, message, make(map[string]*programMessage))
	if err != nil {
		return nil, err
	}

	return &request{
		message: requestMessage,
		fields:  requestMessage.fields,
	}, nil
}

func getOneOfDescriptor(rtMessage *runtimeMessage, oneOfType OneOfType) protoref.OneofDescriptor {
	switch oneOfType {
	case OneOfTypeInterface:
		return rtMessage.desc.Oneofs().ByName(protoref.Name("instance"))
	case OneOfTypeUnion:
		return rtMessage.desc.Oneofs().ByName(protoref.Name("value"))
	}

	return nil
}

func compileMessage(runtime *runtimeSchema, rpcMessage *RPCMessage, rtMessage *runtimeMessage, cycleMap map[string]*programMessage) (*programMessage, error) {
	if seen, ok := cycleMap[rpcMessage.Name]; ok {
		return seen, nil
	}

	msg := &programMessage{
		name:    rpcMessage.Name,
		runtime: rtMessage,
		fields:  make([]programField, 0, len(rpcMessage.Fields)),
	}

	cycleMap[rpcMessage.Name] = msg

	if rpcMessage.IsOneOf() {
		msg.oneOfType = rpcMessage.OneOfType
		msg.memberTypes = rpcMessage.MemberTypes
		msg.oneOfFields = make(map[string][]programField)

		for _, memberType := range rpcMessage.MemberTypes {
			fragmentFields, ok := rpcMessage.FragmentFields[memberType]
			if !ok {
				continue
			}

			oneOfDescriptor := getOneOfDescriptor(rtMessage, rpcMessage.OneOfType)
			if oneOfDescriptor == nil {
				return nil, fmt.Errorf("oneof descriptor not found for message %s", rpcMessage.Name)
			}

			fullName := ""
			for i := range oneOfDescriptor.Fields().Len() {
				field := oneOfDescriptor.Fields().Get(i)
				if field.Kind() != protoref.MessageKind {
					continue
				}

				if field.Message().Name() == protoref.Name(memberType) {
					fullName = string(field.Message().FullName())
					break
				}
			}

			memberTypeMessage := runtime.getMessageByFullName(fullName)
			if memberTypeMessage == nil {
				return nil, fmt.Errorf("message not found for name %s", fullName)
			}

			oneOfFields := make([]programField, 0, len(fragmentFields))
			for _, fragmentField := range fragmentFields {
				if fragmentField.Name == typenameFieldName {
					continue
				}

				runtimeField := memberTypeMessage.fieldsByName[fragmentField.Name]
				if runtimeField == nil {
					return nil, fmt.Errorf("field not found for name %s", fragmentField.Name)
				}

				requestField, err := compileField(runtime, fragmentField, runtimeField, cycleMap)
				if err != nil {
					return nil, err
				}
				oneOfFields = append(oneOfFields, requestField)
				msg.oneOfFields[memberType] = oneOfFields
			}
		}

		return msg, nil
	}

	for _, f := range rpcMessage.Fields {
		if f.Name == typenameFieldName {
			continue
		}

		rtFieldMessage := runtime.getMessageByFullName(rpcMessage.Name)
		if rtFieldMessage == nil {
			rtFieldMessage = rtMessage
		}

		runtimeField := rtFieldMessage.fieldsByName[f.Name]

		if runtimeField == nil {
			return nil, fmt.Errorf("field not found for name %s", f.Name)
		}

		requestField, err := compileField(runtime, f, runtimeField, cycleMap)
		if err != nil {
			return nil, err
		}
		msg.fields = append(msg.fields, requestField)
	}

	return msg, nil
}

func compileField(runtime *runtimeSchema, rpcField RPCField, rtField *runtimeField, cycleMap map[string]*programMessage) (programField, error) {
	f := programField{
		runtime:      rtField,
		dataType:     rpcField.ProtoTypeName,
		jsonPath:     rpcField.JSONPath,
		enumName:     rpcField.EnumName,
		staticValue:  rpcField.StaticValue,
		optional:     rpcField.Optional,
		repeated:     rpcField.Repeated,
		listMetadata: rpcField.ListMetadata,
		child:        nil,
	}

	if rpcField.Message != nil {
		if rtField.message == nil {
			return programField{}, fmt.Errorf("child message not found for name %s", rpcField.Message.Name)
		}

		childMessage, err := compileMessage(runtime, rpcField.Message, rtField.message, cycleMap)
		if err != nil {
			return programField{}, err
		}

		f.child = childMessage
	}

	return f, nil
}

func compileFetchRequestWithContext(runtime *runtimeSchema, message *runtimeMessage, dependentMessage *runtimeMessage, rpcMessage *RPCMessage) (*request, error) {
	request := &request{}

	requestMessage, err := compileMessage(runtime, rpcMessage, message, make(map[string]*programMessage))
	if err != nil {
		return nil, err
	}

	request.message = requestMessage
	request.fields = requestMessage.fields

	contextField := rpcMessage.Fields.ByName(contextFieldName)
	if contextField == nil {
		return nil, fmt.Errorf("context field not found for method %s", rpcMessage.Name)
	}

	contextRuntimeField, found := message.fieldsByName[contextFieldName]
	if !found {
		return nil, fmt.Errorf("context field not found for method %s", rpcMessage.Name)
	}

	fetchRequestContext, err := compileFetchRequestContext(contextRuntimeField.message, dependentMessage, contextField.Message)
	if err != nil {
		return nil, err
	}

	request.context = fetchRequestContext

	return request, nil
}

func compileFetchRequestContext(message, contextMessage *runtimeMessage, rpcMessage *RPCMessage) (*fetchRequestContext, error) {
	if message == nil || contextMessage == nil {
		return nil, fmt.Errorf("unable to compile fetch request context: message or dependent message is nil")
	}

	if rpcMessage == nil {
		return nil, fmt.Errorf("unable to compile fetch request context: rpc message is nil")
	}

	fetchRequestContext := &fetchRequestContext{
		message: message,
		context: contextMessage,
		fields:  make([]fetchRequestContextField, 0, len(rpcMessage.Fields)),
	}

	for _, field := range rpcMessage.Fields {
		rtField, found := message.fieldsByName[field.Name]
		if !found {
			return nil, fmt.Errorf("field not found for name %s", field.Name)
		}

		fetchRequestContextField := &fetchRequestContextField{
			runtime:     rtField,
			resolvePath: field.ResolvePath,
			jsonName:    field.JSONPath,
		}
		fetchRequestContext.fields = append(fetchRequestContext.fields, *fetchRequestContextField)
	}

	return fetchRequestContext, nil
}

func compileStageIndexes(plan *RPCExecutionPlan) ([]int, error) {
	// We are using a slice to store the batch index for each noded ordered.
	stageIndexes := initializeSlice(len(plan.Calls), -1)
	cycleChecks := make([]bool, len(plan.Calls))

	var visit func(index int) error
	visit = func(index int) error {
		if cycleChecks[index] {
			return fmt.Errorf("cycle detected")
		}

		// We are marking the call as visited to avoid cycles.
		cycleChecks[index] = true

		call := &plan.Calls[index]
		if len(call.DependentCalls) == 0 {
			// If the call has no dependencies, we are setting the level to 0 and return early.
			stageIndexes[index] = 0
			return nil
		}

		currentLevel := 0
		// We are iterating over the dependent calls of the current call.
		for _, depCallIndex := range call.DependentCalls {
			if depCallIndex < 0 || depCallIndex >= len(plan.Calls) {
				return fmt.Errorf("unable to find dependent call %d in execution plan", depCallIndex)
			}

			// If the dependent call has already been visited, we are checking if the level of the dependent call is greater than the current level.
			// If it is, we are updating the current level to the level of the dependent call.
			if depLevel := stageIndexes[depCallIndex]; depLevel >= 0 {
				if depLevel > currentLevel {
					currentLevel = depLevel
				}
				continue
			}

			// If the dependent call has not been visited, we are visiting it.
			if err := visit(depCallIndex); err != nil {
				return err
			}

			// If the level of the dependent call is greater than the current level, we are updating the current level to the level of the dependent call.
			if l := stageIndexes[depCallIndex]; l > currentLevel {
				currentLevel = l
			}
		}

		// After receiving the maximum level of the dependent calls, we increment the level by 1.
		stageIndexes[index] = currentLevel + 1
		return nil
	}

	for callIndex := range plan.Calls {
		if err := visit(callIndex); err != nil {
			return nil, err
		}

		clear(cycleChecks)
	}

	return stageIndexes, nil
}
