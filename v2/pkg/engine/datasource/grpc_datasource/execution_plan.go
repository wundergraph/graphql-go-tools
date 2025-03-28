package grpcdatasource

// RPCExecutionPlan represents a plan for executing one or more RPC calls
// to gRPC services.
type RPCExecutionPlan struct {
	Calls []RPCCall
}

// RPCCall represents a single call to a gRPC service method.
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
type RPCMessage struct {
	// Name is the name of the message type
	Name string
	// Fields is a list of fields in the message
	Fields RPCFields
}

// RPCField represents a single field in a gRPC message.
type RPCField struct {
	// Repeated indicates if the field is a repeated field
	Repeated bool
	// Name is the name of the field
	Name string
	// TypeName is the name of the type of the field
	TypeName string
	// JSONPath defines the path within the variables to provide the value for the field
	JSONPath string
	// Index is the index of the field in the message
	Index int
	// Message is the message type if the field is a nested message type
	Message *RPCMessage
}

// RPCFields is a list of RPCFields
type RPCFields []RPCField

func (r RPCFields) ByName(name string) *RPCField {
	for _, field := range r {
		if field.Name == name {
			return &field
		}
	}

	return nil
}
