package service_datasource

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// ServiceSDL is the GraphQL SDL for service capability types.
// This is provided for documentation purposes. The actual types are added
// programmatically via ExtendSchemaWithServiceTypes for robustness.
const ServiceSDL = `"""
Service capabilities exposed via __service query.
"""
type _Service {
	"""
	List of capabilities supported by this service.
	"""
	capabilities: [_Capability!]!
}

"""
A single service capability.
"""
type _Capability {
	"""
	Unique identifier for this capability (e.g., "graphql.onError").
	"""
	identifier: String!
	"""
	Optional value associated with the capability.
	"""
	value: String
	"""
	Human-readable description of the capability.
	"""
	description: String
}
`

// ExtendSchemaWithServiceTypes adds _Service, _Capability types and
// __service field to the Query type in the given schema document.
// This follows the same pattern as MergeDefinitionWithBaseSchema for introspection.
//
// The function:
// 1. Adds the _Capability type with identifier, value, and description fields
// 2. Adds the _Service type with capabilities field
// 3. Adds the __service field to the Query type
//
// This is the recommended integration method for Cosmo router and similar frameworks
// that need to extend an existing schema with service capabilities.
//
// IMPORTANT: Call this AFTER MergeDefinitionWithBaseSchema if you need both
// introspection types and service capability types.
func ExtendSchemaWithServiceTypes(schema *ast.Document) error {
	// 1. Find Query type first to fail fast
	queryNode, found := findQueryType(schema)
	if !found {
		return fmt.Errorf("Query type not found in schema")
	}

	// 2. Add _Capability type (must be added before _Service since _Service references it)
	addCapabilityType(schema)

	// 3. Add _Service type
	addServiceType(schema)

	// 4. Add __service field to Query type
	addServiceField(schema, queryNode.Ref)

	return nil
}

// findQueryType locates the Query type in the schema document.
func findQueryType(schema *ast.Document) (ast.Node, bool) {
	// First try to find via index (handles custom query type names)
	if len(schema.Index.QueryTypeName) > 0 {
		queryNode, ok := schema.Index.FirstNodeByNameBytes(schema.Index.QueryTypeName)
		if ok {
			return queryNode, true
		}
	}

	// Fall back to looking for "Query" by name
	queryNode, ok := schema.Index.FirstNodeByNameStr("Query")
	if ok {
		return queryNode, true
	}

	// Manual search through root nodes
	for i := range schema.RootNodes {
		if schema.RootNodes[i].Kind == ast.NodeKindObjectTypeDefinition {
			name := schema.ObjectTypeDefinitionNameString(schema.RootNodes[i].Ref)
			if name == "Query" {
				return schema.RootNodes[i], true
			}
		}
	}

	return ast.Node{}, false
}

// addCapabilityType adds the _Capability type to the schema:
//
//	type _Capability {
//	    identifier: String!
//	    value: String
//	    description: String
//	}
func addCapabilityType(schema *ast.Document) {
	// Check if type already exists
	if _, found := schema.Index.FirstNodeByNameStr("_Capability"); found {
		return
	}

	// identifier: String!
	identifierTypeRef := schema.AddNonNullNamedType([]byte("String"))
	identifierFieldRef := schema.ImportFieldDefinition(
		"identifier",
		"Unique identifier for this capability (e.g., \"graphql.onError\").",
		identifierTypeRef,
		nil,
		nil,
	)

	// value: String
	valueTypeRef := schema.AddNamedType([]byte("String"))
	valueFieldRef := schema.ImportFieldDefinition(
		"value",
		"Optional value associated with the capability.",
		valueTypeRef,
		nil,
		nil,
	)

	// description: String
	descTypeRef := schema.AddNamedType([]byte("String"))
	descFieldRef := schema.ImportFieldDefinition(
		"description",
		"Human-readable description of the capability.",
		descTypeRef,
		nil,
		nil,
	)

	// Create _Capability type
	schema.ImportObjectTypeDefinition(
		"_Capability",
		"A single service capability.",
		[]int{identifierFieldRef, valueFieldRef, descFieldRef},
		nil,
	)
}

// addServiceType adds the _Service type to the schema:
//
//	type _Service {
//	    capabilities: [_Capability!]!
//	}
func addServiceType(schema *ast.Document) {
	// Check if type already exists
	if _, found := schema.Index.FirstNodeByNameStr("_Service"); found {
		return
	}

	// capabilities: [_Capability!]!
	// Build the type: [_Capability!]!
	capabilityTypeRef := schema.AddNonNullNamedType([]byte("_Capability")) // _Capability!
	listTypeRef := schema.AddListType(capabilityTypeRef)                   // [_Capability!]
	nonNullListTypeRef := schema.AddNonNullType(listTypeRef)               // [_Capability!]!

	capabilitiesFieldRef := schema.ImportFieldDefinition(
		"capabilities",
		"List of capabilities supported by this service.",
		nonNullListTypeRef,
		nil,
		nil,
	)

	// Create _Service type
	schema.ImportObjectTypeDefinition(
		"_Service",
		"Service capabilities exposed via __service query.",
		[]int{capabilitiesFieldRef},
		nil,
	)
}

// addServiceField adds the __service: _Service! field to the Query type.
func addServiceField(schema *ast.Document, queryRef int) {
	// Check if __service field already exists
	if schema.ObjectTypeDefinitionHasField(queryRef, []byte("__service")) {
		return
	}

	// Create __service: _Service! field
	fieldNameRef := schema.Input.AppendInputBytes([]byte("__service"))
	fieldTypeRef := schema.AddNonNullNamedType([]byte("_Service"))

	fieldRef := schema.AddFieldDefinition(ast.FieldDefinition{
		Name: fieldNameRef,
		Type: fieldTypeRef,
	})

	// Add field to Query type
	schema.ObjectTypeDefinitions[queryRef].FieldsDefinition.Refs = append(
		schema.ObjectTypeDefinitions[queryRef].FieldsDefinition.Refs,
		fieldRef,
	)
	schema.ObjectTypeDefinitions[queryRef].HasFieldDefinitions = true
}
