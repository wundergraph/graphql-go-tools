package grpcdatasource

type (
	// InputArgumentMap is a map of GraphQL input arguments to the corresponding gRPC input arguments
	InputArgumentMap map[string]string
	// RPCConfigMap is a map of RPC names to RPC configurations
	RPCConfigMap map[string]RPCConfig
	// FieldMap defines the mapping between a GraphQL field and a gRPC field
	FieldMap map[string]FieldMapData
)

type GRPCMapping struct {
	// Services maps user-friendly service names to the actual gRPC service names
	Services map[string]string
	// InputArguments maps GraphQL input arguments to the corresponding gRPC input arguments
	InputArguments map[string]InputArgumentMap
	// QueryRPCs maps GraphQL query fields to the corresponding gRPC RPC configurations
	QueryRPCs RPCConfigMap
	// MutationRPCs maps GraphQL mutation fields to the corresponding gRPC RPC configurations
	MutationRPCs RPCConfigMap
	// SubscriptionRPCs maps GraphQL subscription fields to the corresponding gRPC RPC configurations
	SubscriptionRPCs RPCConfigMap
	// EntityRPCs defines how GraphQL types are resolved as entities using specific RPCs
	EntityRPCs map[string]EntityRPCConfig
	// Fields defines the field mappings between GraphQL types and gRPC messages
	Fields map[string]FieldMap
}

// RPCConfig defines the configuration for a specific RPC operation
type RPCConfig struct {
	// RPC is the name of the RPC method to call
	RPC string
	// Request is the name of the request message type
	Request string
	// Response is the name of the response message type
	Response string
}

// EntityRPCConfig defines the configuration for entity lookups
type EntityRPCConfig struct {
	// Key is a list of field names that uniquely identify the entity
	Key string
	// RPCConfig is the embedded configuration for the RPC operation
	RPCConfig
}

type FieldMapData struct {
	TargetName       string
	ArgumentMappings FieldArgumentMap
}

// FieldArgumentMap defines the mapping between a GraphQL field argument and a gRPC field
type FieldArgumentMap map[string]string

func (g *GRPCMapping) ResolveInputArgumentMapping(fieldName string, argumentName string) (string, bool) {
	if g == nil || g.InputArguments == nil {
		return "", false
	}

	inputArgMap, ok := g.InputArguments[fieldName]
	if !ok || inputArgMap == nil {
		return "", false
	}

	grpcFieldName, ok := inputArgMap[argumentName]
	return grpcFieldName, ok
}

// ResolveFieldMapping resolves the gRPC field name for a given GraphQL field name and type
func (g *GRPCMapping) ResolveFieldMapping(typeName string, fieldName string) (string, bool) {
	if g == nil || g.Fields == nil {
		return "", false
	}

	fieldMap, ok := g.Fields[typeName]
	if !ok || fieldMap == nil {
		return "", false
	}

	field, ok := fieldMap[fieldName]
	if !ok || field.TargetName == "" {
		return "", false
	}

	return field.TargetName, true
}

func (g *GRPCMapping) ResolveFieldArgumentMapping(typeName, fieldName, argumentName string) (string, bool) {
	if g == nil || g.Fields == nil {
		return "", false
	}

	fieldMap, ok := g.Fields[typeName]
	if !ok || fieldMap == nil {
		return "", false
	}

	args, ok := fieldMap[fieldName]
	if !ok || args.ArgumentMappings == nil {
		return "", false
	}

	grpcFieldName, ok := args.ArgumentMappings[argumentName]
	return grpcFieldName, ok
}
