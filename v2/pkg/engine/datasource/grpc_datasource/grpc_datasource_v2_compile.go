package grpcdatasource

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func compileV2Program(plan *RPCExecutionPlan, schema *v2SchemaRuntime, compiler *RPCCompiler) (*v2Program, error) {
	stageIndexes, err := compileV2StageIndexes(plan)
	if err != nil {
		return nil, err
	}

	stageCount := 0
	for _, idx := range stageIndexes {
		if idx+1 > stageCount {
			stageCount = idx + 1
		}
	}

	stageMap := make(map[int][]v2Fetch, stageCount)
	program := &v2Program{
		stages: make([]v2Stage, 0, stageCount),
	}

	for i := range plan.Calls {
		call := &plan.Calls[i]
		fetch, err := compileV2Fetch(plan, call, schema, compiler)
		if err != nil {
			return nil, err
		}
		stageMap[stageIndexes[call.ID]] = append(stageMap[stageIndexes[call.ID]], fetch)
		if !fetch.native {
			program.requiresFallback = true
			program.fallbackReasons = append(program.fallbackReasons, fmt.Sprintf("call %d (%s): %s", call.ID, call.MethodName, fetch.fallbackReason))
		}
	}

	for i := 0; i < stageCount; i++ {
		program.stages = append(program.stages, v2Stage{fetches: stageMap[i]})
	}

	program.nativeOperation = !program.requiresFallback
	return program, nil
}

func compileV2Fetch(plan *RPCExecutionPlan, call *RPCCall, schema *v2SchemaRuntime, compiler *RPCCompiler) (v2Fetch, error) {
	serviceName, ok := compiler.resolveServiceName(call)
	if !ok {
		return v2Fetch{}, fmt.Errorf("failed to resolve service name for method %s", call.MethodName)
	}

	fetch := v2Fetch{
		id:           call.ID,
		kind:         call.Kind,
		dependencies: append([]int(nil), call.DependentCalls...),
		serviceName:  serviceName,
		methodName:   call.MethodName,
		responsePath: call.ResponsePath,
	}

	requestRuntime, ok := schema.messageRuntime(call.Request.Name)
	if !ok {
		fetch.fallbackReason = "request runtime missing"
		return fetch, nil
	}

	responseRuntime, ok := schema.messageRuntime(call.Response.Name)
	if !ok {
		fetch.fallbackReason = "response runtime missing"
		return fetch, nil
	}

	var (
		requestProgram *v2RequestProgram
		err            error
	)
	switch call.Kind {
	case CallKindStandard, CallKindEntity, CallKindRequired:
		if len(call.DependentCalls) > 0 {
			fetch.fallbackReason = "dependent standard/entity/required fetches are routed through v1"
			return fetch, nil
		}
		requestProgram, err = compileV2RequestProgram(requestRuntime, &call.Request)
	case CallKindResolve:
		if len(call.DependentCalls) != 1 {
			fetch.fallbackReason = "resolve fetches require exactly one dependency"
			return fetch, nil
		}
		dependencyCall := &plan.Calls[call.DependentCalls[0]]
		dependencyRuntime, ok := schema.messageRuntime(dependencyCall.Response.Name)
		if !ok {
			fetch.fallbackReason = "resolve dependency runtime missing"
			return fetch, nil
		}
		requestProgram, err = compileV2ResolveRequestProgram(requestRuntime, &call.Request, dependencyRuntime)
	default:
		fetch.fallbackReason = "fetch kind is routed through v1"
		return fetch, nil
	}
	if err != nil {
		fetch.fallbackReason = err.Error()
		return fetch, nil
	}

	responseProgram, err := compileV2ResponseProgram(schema, responseRuntime, &call.Response)
	if err != nil {
		fetch.fallbackReason = err.Error()
		return fetch, nil
	}

	fetch.request = requestProgram
	fetch.response = responseProgram
	fetch.native = true
	return fetch, nil
}

func compileV2RequestProgram(runtime *v2MessageRuntime, message *RPCMessage) (*v2RequestProgram, error) {
	if message == nil {
		return nil, fmt.Errorf("request rpc message is nil")
	}
	if message.IsOneOf() {
		return nil, fmt.Errorf("oneof request messages are not yet supported natively")
	}

	program := &v2RequestProgram{
		message: runtime,
		fields:  make([]v2RequestFieldProgram, 0, len(message.Fields)),
	}

	for i := range message.Fields {
		rpcField := &message.Fields[i]
		fieldRuntime, ok := runtime.fieldsByName[rpcField.Name]
		if !ok {
			continue
		}

		if rpcField.IsListType {
			return nil, fmt.Errorf("list wrapper request fields are not yet supported natively")
		}

		fieldProgram := v2RequestFieldProgram{
			runtime:     fieldRuntime,
			jsonPath:    rpcField.JSONPath,
			staticValue: rpcField.StaticValue,
			enumName:    rpcField.EnumName,
			optional:    rpcField.Optional,
			repeated:    rpcField.Repeated,
		}

		if rpcField.IsOptionalScalar() {
			if !fieldRuntime.isMessage || fieldRuntime.message == nil {
				return nil, fmt.Errorf("optional scalar wrapper field %s is missing message runtime", rpcField.Name)
			}
			wrapper := rpcField.ToOptionalTypeMessage(fieldRuntime.message.name)
			wrapper.Fields[0].JSONPath = ""
			child, err := compileV2RequestProgram(fieldRuntime.message, wrapper)
			if err != nil {
				return nil, err
			}
			fieldProgram.child = child
		} else if fieldRuntime.isMessage {
			if rpcField.Message == nil {
				return nil, fmt.Errorf("message field %s has no child rpc message", rpcField.Name)
			}
			child, err := compileV2RequestProgram(fieldRuntime.message, rpcField.Message)
			if err != nil {
				return nil, err
			}
			fieldProgram.child = child
		}

		program.fields = append(program.fields, fieldProgram)
	}

	if wire, ok := compileV2WirePlan(program); ok {
		program.wire = wire
	}

	return program, nil
}

func compileV2ResolveRequestProgram(runtime *v2MessageRuntime, message *RPCMessage, dependencyRuntime *v2MessageRuntime) (*v2RequestProgram, error) {
	if message == nil {
		return nil, fmt.Errorf("request rpc message is nil")
	}
	if message.IsOneOf() {
		return nil, fmt.Errorf("oneof request messages are not yet supported natively")
	}

	program := &v2RequestProgram{
		message: runtime,
		fields:  make([]v2RequestFieldProgram, 0, len(message.Fields)),
	}

	for i := range message.Fields {
		rpcField := &message.Fields[i]
		fieldRuntime, ok := runtime.fieldsByName[rpcField.Name]
		if !ok {
			continue
		}

		switch rpcField.Name {
		case "context":
			contextProgram, err := compileV2ContextProgram(fieldRuntime, rpcField, dependencyRuntime)
			if err != nil {
				return nil, err
			}
			program.context = contextProgram
		default:
			fieldProgram, err := compileV2RequestFieldProgram(fieldRuntime, rpcField)
			if err != nil {
				return nil, err
			}
			program.fields = append(program.fields, fieldProgram)
		}
	}

	if program.context == nil {
		return nil, fmt.Errorf("resolve request message %s is missing a context program", message.Name)
	}

	return program, nil
}

func compileV2RequestFieldProgram(fieldRuntime *v2FieldRuntime, rpcField *RPCField) (v2RequestFieldProgram, error) {
	if rpcField.IsListType {
		return v2RequestFieldProgram{}, fmt.Errorf("list wrapper request fields are not yet supported natively")
	}
	fieldProgram := v2RequestFieldProgram{
		runtime:     fieldRuntime,
		jsonPath:    rpcField.JSONPath,
		staticValue: rpcField.StaticValue,
		enumName:    rpcField.EnumName,
		optional:    rpcField.Optional,
		repeated:    rpcField.Repeated,
	}

	if rpcField.IsOptionalScalar() {
		if !fieldRuntime.isMessage || fieldRuntime.message == nil {
			return v2RequestFieldProgram{}, fmt.Errorf("optional scalar wrapper field %s is missing message runtime", rpcField.Name)
		}
		wrapper := rpcField.ToOptionalTypeMessage(fieldRuntime.message.name)
		wrapper.Fields[0].JSONPath = ""
		child, err := compileV2RequestProgram(fieldRuntime.message, wrapper)
		if err != nil {
			return v2RequestFieldProgram{}, err
		}
		fieldProgram.child = child
	} else if fieldRuntime.isMessage {
		if rpcField.Message == nil {
			return v2RequestFieldProgram{}, fmt.Errorf("message field %s has no child rpc message", rpcField.Name)
		}
		child, err := compileV2RequestProgram(fieldRuntime.message, rpcField.Message)
		if err != nil {
			return v2RequestFieldProgram{}, err
		}
		fieldProgram.child = child
	}

	return fieldProgram, nil
}

func compileV2ContextProgram(fieldRuntime *v2FieldRuntime, rpcField *RPCField, dependencyRuntime *v2MessageRuntime) (*v2ContextProgram, error) {
	if !fieldRuntime.repeated || !fieldRuntime.isMessage {
		return nil, fmt.Errorf("resolve context field %s must be a repeated message", rpcField.Name)
	}
	if rpcField.Message == nil {
		return nil, fmt.Errorf("resolve context field %s has no message definition", rpcField.Name)
	}

	program := &v2ContextProgram{
		runtime: fieldRuntime,
		message: fieldRuntime.message,
		fields:  make([]v2ContextFieldProgram, 0, len(rpcField.Message.Fields)),
	}

	for i := range rpcField.Message.Fields {
		contextField := &rpcField.Message.Fields[i]
		contextRuntime, ok := fieldRuntime.message.fieldsByName[contextField.Name]
		if !ok {
			return nil, fmt.Errorf("resolve context field runtime missing for %s", contextField.Name)
		}

		pathProgram, err := compileV2ResolvePathProgram(dependencyRuntime, contextField.ResolvePath)
		if err != nil {
			return nil, err
		}

		program.fields = append(program.fields, v2ContextFieldProgram{
			runtime: contextRuntime,
			path:    pathProgram,
		})
	}

	return program, nil
}

func compileV2ResolvePathProgram(runtime *v2MessageRuntime, path ast.Path) (v2ResolvePathProgram, error) {
	if path.Len() == 0 {
		return v2ResolvePathProgram{}, fmt.Errorf("resolve path is empty")
	}

	program := v2ResolvePathProgram{
		steps: make([]v2ResolvePathStep, 0, path.Len()),
	}

	current := runtime
	for i := range path {
		fieldName := path[i].FieldName.String()
		if len(fieldName) > 0 && fieldName[0] == '@' {
			return v2ResolvePathProgram{}, fmt.Errorf("resolve path %s uses nested list markers which are not yet supported natively", path.String())
		}

		fieldRuntime, ok := current.fieldsByName[fieldName]
		if !ok {
			return v2ResolvePathProgram{}, fmt.Errorf("resolve path field %s not found in %s", fieldName, current.name)
		}

		program.steps = append(program.steps, v2ResolvePathStep{runtime: fieldRuntime})
		if i < len(path)-1 {
			if !fieldRuntime.isMessage {
				return v2ResolvePathProgram{}, fmt.Errorf("resolve path %s terminates early on scalar field %s", path.String(), fieldName)
			}
			current = fieldRuntime.message
		}
	}

	return program, nil
}

func compileV2ResponseProgram(schema *v2SchemaRuntime, runtime *v2MessageRuntime, message *RPCMessage) (*v2ResponseProgram, error) {
	if message == nil {
		return nil, fmt.Errorf("response rpc message is nil")
	}

	program := &v2ResponseProgram{
		message:   runtime,
		fields:    make([]v2ResponseFieldProgram, 0, len(message.Fields)),
		oneOfType: message.OneOfType,
	}

	if len(message.FragmentFields) > 0 {
		program.fragments = make(map[string]*v2ResponseProgram, len(message.FragmentFields))
		for typeName, fragmentFields := range message.FragmentFields {
			fragmentRuntime, ok := schema.messageRuntime(typeName)
			if !ok {
				return nil, fmt.Errorf("response fragment runtime missing for %s", typeName)
			}
			fragmentProgram, err := compileV2ResponseProgram(schema, fragmentRuntime, &RPCMessage{
				Name:   typeName,
				Fields: fragmentFields,
			})
			if err != nil {
				return nil, err
			}
			program.fragments[typeName] = fragmentProgram
		}
	}

	for i := range message.Fields {
		rpcField := &message.Fields[i]

		fieldProgram := v2ResponseFieldProgram{
			name:        rpcField.AliasOrPath(),
			staticValue: rpcField.StaticValue,
			enumName:    rpcField.EnumName,
			repeated:    rpcField.Repeated,
			scalarType:  rpcField.ProtoTypeName,
		}

		if rpcField.StaticValue != "" {
			program.fields = append(program.fields, fieldProgram)
			continue
		}

		fieldRuntime, ok := runtime.fieldsByName[rpcField.Name]
		if !ok {
			continue
		}
		fieldProgram.runtime = fieldRuntime

		if rpcField.IsListType {
			return nil, fmt.Errorf("list wrapper response fields are not yet supported natively")
		}
		if rpcField.IsOptionalScalar() {
			return nil, fmt.Errorf("optional scalar wrapper response fields are not yet supported natively")
		}
		if rpcField.JSONPath == "" {
			return nil, fmt.Errorf("flattened response fields are not yet supported natively")
		}
		if fieldRuntime.dataType == DataTypeEnum {
			return nil, fmt.Errorf("enum response fields are not yet supported natively")
		}

		if fieldRuntime.isMessage {
			if rpcField.Message == nil {
				return nil, fmt.Errorf("message field %s has no child rpc message", rpcField.Name)
			}
			child, err := compileV2ResponseProgram(schema, fieldRuntime.message, rpcField.Message)
			if err != nil {
				return nil, err
			}
			fieldProgram.child = child
		}

		program.fields = append(program.fields, fieldProgram)
	}

	return program, nil
}

func compileV2StageIndexes(plan *RPCExecutionPlan) ([]int, error) {
	stageIndexes := initializeSlice(len(plan.Calls), -1)
	cycleChecks := make([]bool, len(plan.Calls))

	var visit func(index int) error
	visit = func(index int) error {
		if cycleChecks[index] {
			return fmt.Errorf("cycle detected")
		}
		cycleChecks[index] = true

		call := &plan.Calls[index]
		if len(call.DependentCalls) == 0 {
			stageIndexes[index] = 0
			return nil
		}

		currentLevel := 0
		for _, dep := range call.DependentCalls {
			if dep < 0 || dep >= len(plan.Calls) {
				return fmt.Errorf("unable to find dependent call %d in execution plan", dep)
			}
			if level := stageIndexes[dep]; level >= 0 {
				if level > currentLevel {
					currentLevel = level
				}
				continue
			}
			if err := visit(dep); err != nil {
				return err
			}
			if level := stageIndexes[dep]; level > currentLevel {
				currentLevel = level
			}
		}

		stageIndexes[index] = currentLevel + 1
		return nil
	}

	for i := range plan.Calls {
		if err := visit(i); err != nil {
			return nil, err
		}
		clear(cycleChecks)
	}

	return stageIndexes, nil
}
