package grpcdatasource

import (
	"fmt"
	"strings"
)

const (
	FederationKeyDirectiveName = "key"
)

// RPCExecutionPlan represents a plan for executing one or more RPC calls
// to gRPC services. It defines the sequence of calls and their dependencies.
type RPCExecutionPlan struct {
	// Groups is a list of groups of gRPC calls that are executed in the same group
	Groups []RPCCallGroup
}

// RPCCallGroup represents a group of gRPC calls that are executed in the same group
// to make sure related calls are executed in the same group
type RPCCallGroup struct {
	// Calls is a list of gRPC calls to execute as part of this group
	Calls []RPCCall
}

// RPCCall represents a single call to a gRPC service method.
// It contains all the information needed to make the call and process the response.
type RPCCall struct {
	// CallID is the unique identifier for the call
	CallID int
	// DependentCalls is a list of calls that must be executed before this call
	DependentCalls []int
	// ServiceName is the name of the gRPC service to call
	ServiceName string
	// MethodName is the name of the method on the service to call
	MethodName string
	// Request contains the message structure for the gRPC request
	Request RPCMessage
	// Response contains the message structure for the gRPC response
	Response RPCMessage
}

// RPCMessage represents a gRPC message structure for requests and responses.
// It defines the structure of the message including all its fields.
type RPCMessage struct {
	// Name is the name of the message type in the protobuf definition
	Name string
	// Fields is a list of fields in the message
	Fields RPCFields
}

// RPCField represents a single field in a gRPC message.
// It contains all information required to extract data from GraphQL variables
// and construct the appropriate protobuf field.
type RPCField struct {
	// Repeated indicates if the field is a repeated field (array/list)
	Repeated bool
	// Name is the name of the field as defined in the protobuf message
	Name string
	// TypeName is the name of the type of the field in the protobuf definition
	TypeName string
	// JSONPath defines the path within the variables to provide the value for the field
	// This is used to extract data from the GraphQL variables
	JSONPath string
	// Index is the index of the field in the message
	Index int
	// StaticValue is the static value of the field
	StaticValue string
	// Message is the message type if the field is a nested message type
	// This allows for recursive construction of complex message types
	Message *RPCMessage
}

// RPCFields is a list of RPCFields that provides helper methods
// for working with collections of fields.
type RPCFields []RPCField

// ByName returns a field by its name from the collection of fields.
// Returns nil if no field with the given name exists.
func (r RPCFields) ByName(name string) *RPCField {
	for _, field := range r {
		if field.Name == name {
			return &field
		}
	}

	return nil
}

func (r *RPCExecutionPlan) String() string {
	var result strings.Builder

	result.WriteString("RPCExecutionPlan:\n")

	for i, group := range r.Groups {
		result.WriteString(fmt.Sprintf("  Group %d:\n", i))

		for j, call := range group.Calls {
			result.WriteString(fmt.Sprintf("    Call %d:\n", j))
			result.WriteString(fmt.Sprintf("      CallID: %d\n", call.CallID))

			if len(call.DependentCalls) > 0 {
				result.WriteString("      DependentCalls: [")
				for k, depID := range call.DependentCalls {
					if k > 0 {
						result.WriteString(", ")
					}
					result.WriteString(fmt.Sprintf("%d", depID))
				}
				result.WriteString("]\n")
			} else {
				result.WriteString("      DependentCalls: []\n")
			}

			result.WriteString(fmt.Sprintf("      Service: %s\n", call.ServiceName))
			result.WriteString(fmt.Sprintf("      Method: %s\n", call.MethodName))

			result.WriteString("      Request:\n")
			formatRPCMessage(&result, call.Request, 8)

			result.WriteString("      Response:\n")
			formatRPCMessage(&result, call.Response, 8)
		}
	}

	return result.String()
}

// formatRPCMessage formats an RPCMessage and adds it to the string builder with the specified indentation
func formatRPCMessage(sb *strings.Builder, message RPCMessage, indent int) {
	indentStr := strings.Repeat(" ", indent)

	sb.WriteString(fmt.Sprintf("%sName: %s\n", indentStr, message.Name))
	sb.WriteString(fmt.Sprintf("%sFields:\n", indentStr))

	for _, field := range message.Fields {
		sb.WriteString(fmt.Sprintf("%s  - Name: %s\n", indentStr, field.Name))
		sb.WriteString(fmt.Sprintf("%s    TypeName: %s\n", indentStr, field.TypeName))
		sb.WriteString(fmt.Sprintf("%s    Repeated: %v\n", indentStr, field.Repeated))
		sb.WriteString(fmt.Sprintf("%s    JSONPath: %s\n", indentStr, field.JSONPath))
		sb.WriteString(fmt.Sprintf("%s    Index: %d\n", indentStr, field.Index))

		if field.Message != nil {
			sb.WriteString(fmt.Sprintf("%s    Message:\n", indentStr))
			formatRPCMessage(sb, *field.Message, indent+6)
		}
	}
}
