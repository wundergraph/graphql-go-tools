package grpcdatasource

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// compileV2Program walks the RPC plan and emits an IR program the runtime can
// execute. Shapes outside the MVP (oneofs, list wrappers, optional scalars,
// required-fields context resolution) are flagged via fallbackReasons and
// cause nativeOperation to be false — those queries route through the V1
// fallback datasource at Load time.
//
// MVP coverage:
//   - CallKindStandard with no dependencies
//   - CallKindResolve with exactly one dependency
//   - Request fields: scalar + nested message + repeated scalar/message
//   - Response fields: scalar + nested message + repeated scalar/message
//
// Anything outside this is explicitly declined with a readable reason so the
// expansion roadmap is data-driven. This is codex's discipline: no native
// impl silently covers something it doesn't handle correctly.
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
			program.fallbackReasons = append(program.fallbackReasons,
				fmt.Sprintf("call %d (%s): %s", call.ID, call.MethodName, fetch.fallbackReason))
		}
	}

	for i := 0; i < stageCount; i++ {
		program.stages = append(program.stages, v2Stage{fetches: stageMap[i]})
	}

	program.nativeOperation = !program.requiresFallback
	return program, nil
}

// compileV2StageIndexes assigns each call to a stage (dependency level).
// Standard fetches with no deps land at stage 0; resolve-kind fetches land at
// stage N+1 of their deepest dependency.
func compileV2StageIndexes(plan *RPCExecutionPlan) ([]int, error) {
	levels := make([]int, len(plan.Calls))
	for i := range levels {
		levels[i] = -1
	}
	var assign func(id int) (int, error)
	assign = func(id int) (int, error) {
		if id < 0 || id >= len(plan.Calls) {
			return 0, fmt.Errorf("invalid call id %d", id)
		}
		if levels[id] != -1 {
			return levels[id], nil
		}
		call := &plan.Calls[id]
		maxDep := -1
		for _, dep := range call.DependentCalls {
			lv, err := assign(dep)
			if err != nil {
				return 0, err
			}
			if lv > maxDep {
				maxDep = lv
			}
		}
		levels[id] = maxDep + 1
		return levels[id], nil
	}
	for i := range plan.Calls {
		if _, err := assign(plan.Calls[i].ID); err != nil {
			return nil, err
		}
	}
	return levels, nil
}

func compileV2Fetch(plan *RPCExecutionPlan, call *RPCCall, schema *v2SchemaRuntime, compiler *RPCCompiler) (v2Fetch, error) {
	serviceName, ok := compiler.resolveServiceName(call)
	if !ok {
		return v2Fetch{}, fmt.Errorf("failed to resolve service name for method %s", call.MethodName)
	}

	fetch := v2Fetch{
		id:             call.ID,
		kind:           call.Kind,
		dependencies:   append([]int(nil), call.DependentCalls...),
		serviceName:    serviceName,
		methodName:     call.MethodName,
		methodFullName: "/" + serviceName + "/" + call.MethodName,
		responsePath:   call.ResponsePath,
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
	case CallKindStandard:
		if len(call.DependentCalls) > 0 {
			fetch.fallbackReason = "dependent standard fetches not yet handled"
			return fetch, nil
		}
		requestProgram, err = compileV2RequestProgram(requestRuntime, &call.Request)
	case CallKindEntity:
		// CallKindEntity needs two things V2 doesn't have yet:
		//   1. Per-type filtering of `representations` by __typename when building
		//      the request — half implemented (matchesMemberType) but untested end-
		//      to-end.
		//   2. Merging multiple entity calls' `_entities` arrays into a single list
		//      ordered by original representation position. Today's attach overwrites.
		// Until both are done, route Entity calls through V1 for correctness.
		fetch.fallbackReason = "entity fetches need _entities array merge"
		return fetch, nil
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
		fetch.fallbackReason = "fetch kind not yet native"
		return fetch, nil
	}
	if err != nil {
		fetch.fallbackReason = err.Error()
		return fetch, nil
	}

	responseProgram, err := compileV2ResponseProgram(responseRuntime, &call.Response)
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
		return nil, fmt.Errorf("oneof request messages not yet supported")
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
			return nil, fmt.Errorf("list wrapper request fields not yet supported")
		}
		if rpcField.IsOptionalScalar() {
			return nil, fmt.Errorf("optional scalar wrapper request fields not yet supported")
		}
		if fieldRuntime.dataType == DataTypeEnum {
			return nil, fmt.Errorf("enum request fields not yet supported")
		}

		fieldProgram := v2RequestFieldProgram{
			runtime:     fieldRuntime,
			jsonPath:    rpcField.JSONPath,
			staticValue: rpcField.StaticValue,
			enumName:    rpcField.EnumName,
			optional:    rpcField.Optional,
			repeated:    rpcField.Repeated,
		}
		if fieldRuntime.isMessage {
			if rpcField.Message == nil {
				return nil, fmt.Errorf("message field %s has no child rpc message", rpcField.Name)
			}
			child, err := compileV2RequestProgram(fieldRuntime.message, rpcField.Message)
			if err != nil {
				return nil, err
			}
			fieldProgram.child = child
			fieldProgram.memberTypes = rpcField.Message.MemberTypes
		}
		program.fields = append(program.fields, fieldProgram)
	}
	return program, nil
}

func compileV2ResolveRequestProgram(runtime *v2MessageRuntime, message *RPCMessage, dependencyRuntime *v2MessageRuntime) (*v2RequestProgram, error) {
	if message == nil {
		return nil, fmt.Errorf("request rpc message is nil")
	}
	if message.IsOneOf() {
		return nil, fmt.Errorf("oneof request messages not yet supported")
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
			ctxProgram, err := compileV2ContextProgram(fieldRuntime, rpcField, dependencyRuntime)
			if err != nil {
				return nil, err
			}
			program.context = ctxProgram
		default:
			fieldProgram, err := compileV2RequestFieldProgram(fieldRuntime, rpcField)
			if err != nil {
				return nil, err
			}
			program.fields = append(program.fields, fieldProgram)
		}
	}
	if program.context == nil {
		return nil, fmt.Errorf("resolve request %s missing a context field", message.Name)
	}
	return program, nil
}

func compileV2RequestFieldProgram(fieldRuntime *v2FieldRuntime, rpcField *RPCField) (v2RequestFieldProgram, error) {
	if rpcField.IsListType {
		return v2RequestFieldProgram{}, fmt.Errorf("list wrapper request fields not yet supported")
	}
	if rpcField.IsOptionalScalar() {
		return v2RequestFieldProgram{}, fmt.Errorf("optional scalar wrapper request fields not yet supported")
	}
	if fieldRuntime.dataType == DataTypeEnum {
		return v2RequestFieldProgram{}, fmt.Errorf("enum request fields not yet supported")
	}

	fieldProgram := v2RequestFieldProgram{
		runtime:     fieldRuntime,
		jsonPath:    rpcField.JSONPath,
		staticValue: rpcField.StaticValue,
		enumName:    rpcField.EnumName,
		optional:    rpcField.Optional,
		repeated:    rpcField.Repeated,
	}
	if fieldRuntime.isMessage {
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
		return nil, fmt.Errorf("resolve context %s must be a repeated message", rpcField.Name)
	}
	if rpcField.Message == nil {
		return nil, fmt.Errorf("resolve context %s has no child rpc message", rpcField.Name)
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
			return nil, fmt.Errorf("resolve context runtime missing for %s", contextField.Name)
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
	program := v2ResolvePathProgram{steps: make([]v2ResolvePathStep, 0, path.Len())}
	current := runtime
	for i := range path {
		fieldName := path[i].FieldName.String()
		fieldRuntime, ok := current.fieldsByName[fieldName]
		if !ok {
			return v2ResolvePathProgram{}, fmt.Errorf("resolve path step %s not found on message %s", fieldName, current.name)
		}
		program.steps = append(program.steps, v2ResolvePathStep{runtime: fieldRuntime})
		if fieldRuntime.isMessage && fieldRuntime.message != nil {
			current = fieldRuntime.message
		}
	}
	return program, nil
}

func compileV2ResponseProgram(runtime *v2MessageRuntime, message *RPCMessage) (*v2ResponseProgram, error) {
	if message == nil {
		return nil, fmt.Errorf("response rpc message is nil")
	}
	if message.IsOneOf() {
		return nil, fmt.Errorf("oneof response messages not yet supported")
	}

	program := &v2ResponseProgram{
		message: runtime,
		fields:  make([]v2ResponseFieldProgram, 0, len(message.Fields)),
	}
	for i := range message.Fields {
		rpcField := &message.Fields[i]
		fieldProgram, err := compileV2ResponseFieldProgram(runtime, rpcField)
		if err != nil {
			return nil, err
		}
		program.fields = append(program.fields, fieldProgram)
	}
	return program, nil
}

func compileV2ResponseFieldProgram(runtime *v2MessageRuntime, rpcField *RPCField) (v2ResponseFieldProgram, error) {
	if rpcField.IsListType {
		return v2ResponseFieldProgram{}, fmt.Errorf("list wrapper response fields not yet supported")
	}
	if rpcField.IsOptionalScalar() {
		return v2ResponseFieldProgram{}, fmt.Errorf("optional scalar wrapper response fields not yet supported")
	}

	fieldProgram := v2ResponseFieldProgram{
		name:        rpcField.AliasOrPath(),
		staticValue: rpcField.StaticValue,
		enumName:    rpcField.EnumName,
		repeated:    rpcField.Repeated,
	}

	if rpcField.StaticValue != "" {
		return fieldProgram, nil
	}

	fieldRuntime, ok := runtime.fieldsByName[rpcField.Name]
	if !ok {
		return v2ResponseFieldProgram{}, fmt.Errorf("response field %s not present on message %s", rpcField.Name, runtime.name)
	}
	fieldProgram.runtime = fieldRuntime
	fieldProgram.scalarType = fieldRuntime.dataType

	if fieldRuntime.isMessage {
		if rpcField.Message == nil {
			return v2ResponseFieldProgram{}, fmt.Errorf("message response field %s has no child", rpcField.Name)
		}
		child, err := compileV2ResponseProgram(fieldRuntime.message, rpcField.Message)
		if err != nil {
			return v2ResponseFieldProgram{}, err
		}
		fieldProgram.child = child
	}
	return fieldProgram, nil
}
