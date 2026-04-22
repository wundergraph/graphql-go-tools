package grpcdatasource

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type program struct {
	stages []stage
}

type stage struct {
	fetches []fetch
}

type fetch struct {
	id            int
	kind          CallKind
	dependentCall *RPCCall
	serviceName   string
	methodName    string
	responsePath  ast.Path
	request       *fetchRequest
	response      *fetchResponse
}

type fetchRequest struct {
	message    *runtimeMessage
	rpcMessage RPCMessage
	// The wire message will be created fromt the
	// request structure.
	wire *wireMessage
}

type fetchResponse struct {
	// reponse type is the type of the response message.
	responseType *runtimeMessage
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

	stageMap := make(map[int][]fetch, stageCount)

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

func compileFetch(call *RPCCall, runtime *runtimeSchema, dependentCall *RPCCall) (fetch, error) {
	serviceName, ok := runtime.serviceNamesByMethod[call.MethodName]
	if !ok {
		return fetch{}, fmt.Errorf("service name not found for method %s", call.MethodName)
	}

	f := fetch{
		id:            call.ID,
		kind:          call.Kind,
		dependentCall: dependentCall,
		serviceName:   serviceName,
		methodName:    call.MethodName,
		responsePath:  call.ResponsePath,
	}

	requestMessage := runtime.getMessageByName(call.Request.Name)
	if requestMessage == nil {
		return fetch{}, fmt.Errorf("request message not found for method %s", call.MethodName)
	}

	responseMessage := runtime.getMessageByName(call.Response.Name)
	if responseMessage == nil {
		return fetch{}, fmt.Errorf("response message not found for method %s", call.MethodName)
	}

	f.request = &fetchRequest{
		message:    requestMessage,
		rpcMessage: call.Request,
	}

	f.response = &fetchResponse{
		responseType: responseMessage,
	}

	wireMessage, err := compileWireMessage(runtime, &f.request.rpcMessage, requestMessage)
	if err != nil {
		return fetch{}, err
	}

	f.request.wire = wireMessage

	return f, nil
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
