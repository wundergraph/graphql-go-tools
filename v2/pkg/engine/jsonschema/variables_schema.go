package jsonschema

import (
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// VariablesSchemaBuilder creates a unified JSON schema for the variables of a GraphQL operation
type VariablesSchemaBuilder struct {
	operationDocument  *ast.Document
	definitionDocument *ast.Document
	schema             *JsonSchema
	report             *operationreport.Report
	// Track recursion depth for each type to handle recursive types
	recursionTracker  map[string]int
	maxRecursionDepth int
}

// Ensure VariablesSchemaBuilder implements the necessary astvisitor interfaces
var (
	_ astvisitor.EnterDocumentVisitor           = (*VariablesSchemaBuilder)(nil)
	_ astvisitor.EnterVariableDefinitionVisitor = (*VariablesSchemaBuilder)(nil)
)

// NewVariablesSchemaBuilder creates a new VariablesSchemaBuilder with default settings
func NewVariablesSchemaBuilder(operationDocument, definitionDocument *ast.Document) *VariablesSchemaBuilder {
	return NewVariablesSchemaBuilderWithOptions(operationDocument, definitionDocument, 3)
}

// NewVariablesSchemaBuilderWithOptions creates a new VariablesSchemaBuilder with custom options
func NewVariablesSchemaBuilderWithOptions(operationDocument, definitionDocument *ast.Document, maxRecursionDepth int) *VariablesSchemaBuilder {
	return &VariablesSchemaBuilder{
		operationDocument:  operationDocument,
		definitionDocument: definitionDocument,
		schema:             NewObjectSchema(),
		report:             &operationreport.Report{},
		recursionTracker:   make(map[string]int),
		maxRecursionDepth:  maxRecursionDepth,
	}
}

// EnterDocument implements the astvisitor.EnterDocumentVisitor interface
func (v *VariablesSchemaBuilder) EnterDocument(operation, definition *ast.Document) {
	v.schema = NewObjectSchema()
	v.recursionTracker = make(map[string]int) // Reset recursion tracker for each build

	// Extract descriptions from root fields
	var descriptions []string
	if len(operation.OperationDefinitions) == 0 {
	    return
	}
		operationDefinition := operation.OperationDefinitions[0]

		// Process SelectionSet to extract field descriptions
		if operationDefinition.HasSelections {
			selectionSetRef := operationDefinition.SelectionSet
			for _, selectionRef := range operation.SelectionSets[selectionSetRef].SelectionRefs {
				selection := operation.Selections[selectionRef]
				if selection.Kind == ast.SelectionKindField {
					fieldName := operation.FieldNameString(selection.Ref)

					// Look up field in schema definition to get description
					operationType := operationDefinition.OperationType
					var rootTypeName string

					// Determine root type based on operation type
					switch operationType {
					case ast.OperationTypeQuery:
						rootTypeName = "Query"
					case ast.OperationTypeMutation:
						rootTypeName = "Mutation"
					case ast.OperationTypeSubscription:
						rootTypeName = "Subscription"
					default:
						v.report.AddInternalError(fmt.Errorf("unsupported operation type %q", operationType))
						return
					}

					rootType, exists := definition.Index.FirstNodeByNameStr(rootTypeName)
					if exists && rootType.Kind == ast.NodeKindObjectTypeDefinition {
						// Find the field in the root type
						for _, fieldDefRef := range definition.ObjectTypeDefinitions[rootType.Ref].FieldsDefinition.Refs {
							fieldDefName := definition.FieldDefinitionNameString(fieldDefRef)

							// Match field name
							if fieldDefName == fieldName && definition.FieldDefinitions[fieldDefRef].Description.IsDefined {
								description := definition.FieldDefinitionDescriptionString(fieldDefRef)
								if description != "" {
									descriptions = append(descriptions, description)
								}
								break
							}
						}
					}
				}
			}
		}
	}

	// Set concatenated descriptions on root schema if any were found
	if len(descriptions) > 0 {
		v.schema.Description = ""
		for i, desc := range descriptions {
			if i > 0 {
				v.schema.Description += " "
			}
			v.schema.Description += desc
		}
	}
}

// EnterVariableDefinition implements the astvisitor.EnterVariableDefinitionVisitor interface
func (v *VariablesSchemaBuilder) EnterVariableDefinition(ref int) {
	varName := v.operationDocument.VariableDefinitionNameString(ref)
	typeRef := v.operationDocument.VariableDefinitions[ref].Type

	// Convert type to schema starting from the operation document
	varSchema := v.processOperationTypeRef(typeRef)

	// Skip this variable if we reached maximum recursion depth
	if varSchema == nil {
		return
	}

	// Add variable to required list if it's non-nullable
	if v.operationDocument.TypeIsNonNull(typeRef) {
		v.schema.Required = append(v.schema.Required, varName)
	}

	// Set default value if exists
	if v.operationDocument.VariableDefinitionHasDefaultValue(ref) {
		defaultValue := v.operationDocument.VariableDefinitionDefaultValue(ref)
		varSchema.Default = v.convertOperationValueToNative(defaultValue)
	}

	// Force top-level object fields to be not nullable (Nullable=false) so they can't be null
	// This ensures they appear as empty objects at minimum
	if varSchema.Type == TypeObject {
		// Setting Nullable to false means the field can't be null
		// Since the nullable field is only included when true, this effectively removes it
		// from the output JSON, which is what we want
		varSchema.Nullable = false
	}

	// Add variable to schema
	v.schema.Properties[varName] = varSchema
}

// GetSchema returns the built schema
func (v *VariablesSchemaBuilder) GetSchema() *JsonSchema {
	// If we have required fields, the root schema cannot be nullable
	if len(v.schema.Required) > 0 {
		v.schema.Nullable = false
	}
	return v.schema
}

// GetReport returns the report containing any errors
func (v *VariablesSchemaBuilder) GetReport() *operationreport.Report {
	return v.report
}

// Build traverses the operation and builds a unified JSON schema for its variables
func (v *VariablesSchemaBuilder) Build() (*JsonSchema, error) {
	// Create a new walker for AST traversal
	walker := astvisitor.NewDefaultWalker()

	// Register this builder as a visitor
	walker.RegisterEnterDocumentVisitor(v)
	walker.RegisterEnterVariableDefinitionVisitor(v)

	// Walk the AST
	walker.Walk(v.operationDocument, v.definitionDocument, v.report)

	if v.report.HasErrors() {
		return nil, fmt.Errorf("%s", v.report.Error())
	}

	return v.GetSchema(), nil
}

// processOperationTypeRef processes a type reference from the operation document
func (v *VariablesSchemaBuilder) processOperationTypeRef(typeRef int) *JsonSchema {
	switch v.operationDocument.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		ofType := v.operationDocument.Types[typeRef].OfType
		schema := v.processOperationTypeRef(ofType)
		if schema == nil {
			return nil
		}
		// Non-null types are not nullable
		schema.Nullable = false
		return schema

	case ast.TypeKindList:
		ofType := v.operationDocument.Types[typeRef].OfType
		itemSchema := v.processOperationTypeRef(ofType)
		if itemSchema == nil {
			return nil
		}
		// If we're not in a non-null context, list is nullable
		schema := NewArraySchema(itemSchema)
		schema.Nullable = true
		return schema

	case ast.TypeKindNamed:
		typeName := v.operationDocument.TypeNameString(typeRef)
		schema := v.processTypeByName(typeName)
		if schema != nil {
			// If we're not in a non-null context, named type is nullable
			schema.Nullable = true
		}
		return schema
	}

	return nil
}

// processTypeByName processes a type by its name, looking it up in the definition document
func (v *VariablesSchemaBuilder) processTypeByName(typeName string) *JsonSchema {
	// Handle built-in scalars
	switch typeName {
	case "String", "ID":
		return NewStringSchema()
	case "Int":
		return NewIntegerSchema()
	case "Float":
		return NewNumberSchema()
	case "Boolean":
		return NewBooleanSchema()
	}

	// For custom types, look up in the definition document
	node, exists := v.definitionDocument.Index.FirstNodeByNameStr(typeName)
	if !exists {
		v.report.AddInternalError(fmt.Errorf("type %s is not defined", typeName))
		return NewObjectSchema()
	}

	var shouldCleanupTracker bool

	// Check recursion depth for complex types that could be recursive
	if node.Kind == ast.NodeKindEnumTypeDefinition || node.Kind == ast.NodeKindInputObjectTypeDefinition {
		currentDepth, exists := v.recursionTracker[typeName]
		if exists {
			// We've seen this type before
			currentDepth++
			v.recursionTracker[typeName] = currentDepth
			shouldCleanupTracker = true

			// If we've hit our recursion limit, return nil to signal field removal
			if currentDepth > v.maxRecursionDepth {
				return nil
			}
		} else {
			// First time seeing this type
			v.recursionTracker[typeName] = 1
			shouldCleanupTracker = true
		}
	}

	// Process the type based on its kind
	var schema *JsonSchema
	switch node.Kind {
	case ast.NodeKindEnumTypeDefinition:
		schema = v.processEnumType(node)

	case ast.NodeKindInputObjectTypeDefinition:
		schema = v.processInputObjectType(node)

	case ast.NodeKindScalarTypeDefinition:
		schema = NewAnySchema()

		// Add description if available
		if v.definitionDocument.ScalarTypeDefinitions[node.Ref].Description.IsDefined {
			schema.Description = v.definitionDocument.ScalarTypeDefinitionDescriptionString(node.Ref)
		}

	default:
		// If we can't determine the type, default to any
		schema = NewAnySchema()
	}

	// Clean up the recursion tracker before returning
	if shouldCleanupTracker {
		currentDepth := v.recursionTracker[typeName]
		if currentDepth > 1 {
			// Decrement the depth as we're exiting the recursion
			v.recursionTracker[typeName]--
		} else {
			// Remove the type from the tracker if depth is 1
			delete(v.recursionTracker, typeName)
		}
	}

	return schema
}

// processEnumType processes an enum type definition
func (v *VariablesSchemaBuilder) processEnumType(node ast.Node) *JsonSchema {
	values := make([]string, 0)
	enumDef := v.definitionDocument.EnumTypeDefinitions[node.Ref]

	for _, valueRef := range enumDef.EnumValuesDefinition.Refs {
		valueName := v.definitionDocument.EnumValueDefinitionNameString(valueRef)
		values = append(values, valueName)
	}

	schema := NewEnumSchema(values)

	// Add description if available
	if enumDef.Description.IsDefined {
		schema.Description = v.definitionDocument.EnumTypeDefinitionDescriptionString(node.Ref)
	}

	return schema
}

// processInputObjectType processes an input object type definition
func (v *VariablesSchemaBuilder) processInputObjectType(node ast.Node) *JsonSchema {
	schema := NewObjectSchema()
	inputDef := v.definitionDocument.InputObjectTypeDefinitions[node.Ref]

	// Set description if available
	if inputDef.Description.IsDefined {
		schema.Description = v.definitionDocument.InputObjectTypeDefinitionDescriptionString(node.Ref)
	}

	if !inputDef.HasInputFieldsDefinition {
		return schema
	}

	// Process each input field
	for _, fieldRef := range inputDef.InputFieldsDefinition.Refs {
		v.processInputField(fieldRef, schema)
	}

	return schema
}

// processInputField processes a single input field
func (v *VariablesSchemaBuilder) processInputField(fieldRef int, schema *JsonSchema) {
	fieldName := v.definitionDocument.InputValueDefinitionNameString(fieldRef)
	fieldTypeRef := v.definitionDocument.InputValueDefinitionType(fieldRef)

	// Process the field type starting from the definition document
	fieldSchema := v.processDefinitionTypeRef(fieldTypeRef)

	// Skip this field if we reached maximum recursion depth
	if fieldSchema == nil {
		return
	}

	// Add to required list if non-nullable
	if v.definitionDocument.TypeIsNonNull(fieldTypeRef) {
		schema.Required = append(schema.Required, fieldName)
	}

	// Set field description if exists
	if v.definitionDocument.InputValueDefinitions[fieldRef].Description.IsDefined {
		description := v.definitionDocument.InputValueDefinitionDescriptionString(fieldRef)
		fieldSchema.Description = description
	}

	// Set default value if exists
	if v.definitionDocument.InputValueDefinitionHasDefaultValue(fieldRef) {
		defaultValue := v.definitionDocument.InputValueDefinitionDefaultValue(fieldRef)
		fieldSchema.Default = v.convertDefinitionValueToNative(defaultValue)
	}

	// Add field to schema
	schema.Properties[fieldName] = fieldSchema
}

// processDefinitionTypeRef processes a type reference from the definition document
func (v *VariablesSchemaBuilder) processDefinitionTypeRef(typeRef int) *JsonSchema {
	switch v.definitionDocument.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		ofType := v.definitionDocument.Types[typeRef].OfType
		schema := v.processDefinitionTypeRef(ofType)
		if schema == nil {
			return nil
		}
		// Non-null types are not nullable
		schema.Nullable = false
		return schema

	case ast.TypeKindList:
		ofType := v.definitionDocument.Types[typeRef].OfType
		itemSchema := v.processDefinitionTypeRef(ofType)
		if itemSchema == nil {
			return nil
		}
		// If we're not in a non-null context, list is nullable
		schema := NewArraySchema(itemSchema)
		schema.Nullable = true
		return schema

	case ast.TypeKindNamed:
		typeName := v.definitionDocument.TypeNameString(typeRef)
		schema := v.processTypeByName(typeName)
		if schema != nil {
			// If we're not in a non-null context, named type is nullable
			schema.Nullable = true
		}
		return schema
	}

	return nil
}

// convertOperationValueToNative converts a GraphQL AST value from the operation document to a native Go value
func (v *VariablesSchemaBuilder) convertOperationValueToNative(value ast.Value) interface{} {
	switch value.Kind {
	case ast.ValueKindString:
		return v.operationDocument.StringValueContentString(value.Ref)
	case ast.ValueKindInteger:
		return v.operationDocument.IntValueAsInt(value.Ref)
	case ast.ValueKindFloat:
		return v.operationDocument.FloatValueAsFloat32(value.Ref)
	case ast.ValueKindBoolean:
		return v.operationDocument.BooleanValue(value.Ref)
	case ast.ValueKindNull:
		return nil
	case ast.ValueKindEnum:
		return v.operationDocument.EnumValueNameString(value.Ref)
	case ast.ValueKindList:
		list := make([]interface{}, 0)
		for _, itemRef := range v.operationDocument.ListValues[value.Ref].Refs {
			item := v.operationDocument.Value(itemRef)
			list = append(list, v.convertOperationValueToNative(item))
		}
		return list
	case ast.ValueKindObject:
		obj := make(map[string]interface{})
		for _, fieldRef := range v.operationDocument.ObjectValues[value.Ref].Refs {
			fieldName := v.operationDocument.ObjectFieldNameString(fieldRef)
			fieldValue := v.operationDocument.ObjectFieldValue(fieldRef)
			obj[fieldName] = v.convertOperationValueToNative(fieldValue)
		}
		return obj
	}

	return nil
}

// convertDefinitionValueToNative converts a GraphQL AST value from the definition document to a native Go value
func (v *VariablesSchemaBuilder) convertDefinitionValueToNative(value ast.Value) interface{} {
	switch value.Kind {
	case ast.ValueKindString:
		return v.definitionDocument.StringValueContentString(value.Ref)
	case ast.ValueKindInteger:
		return v.definitionDocument.IntValueAsInt(value.Ref)
	case ast.ValueKindFloat:
		return v.definitionDocument.FloatValueAsFloat32(value.Ref)
	case ast.ValueKindBoolean:
		return v.definitionDocument.BooleanValue(value.Ref)
	case ast.ValueKindNull:
		return nil
	case ast.ValueKindEnum:
		return v.definitionDocument.EnumValueNameString(value.Ref)
	case ast.ValueKindList:
		list := make([]interface{}, 0)
		for _, itemRef := range v.definitionDocument.ListValues[value.Ref].Refs {
			item := v.definitionDocument.Value(itemRef)
			list = append(list, v.convertDefinitionValueToNative(item))
		}
		return list
	case ast.ValueKindObject:
		obj := make(map[string]interface{})
		for _, fieldRef := range v.definitionDocument.ObjectValues[value.Ref].Refs {
			fieldName := v.definitionDocument.ObjectFieldNameString(fieldRef)
			fieldValue := v.definitionDocument.ObjectFieldValue(fieldRef)
			obj[fieldName] = v.convertDefinitionValueToNative(fieldValue)
		}
		return obj
	}

	return nil
}

// BuildJsonSchema builds a JSON schema for the variables of the given operation
// using the default recursion depth of 1
func BuildJsonSchema(operationDocument, definitionDocument *ast.Document) (*JsonSchema, error) {
	return BuildJsonSchemaWithOptions(operationDocument, definitionDocument, 1)
}

// BuildJsonSchemaWithOptions builds a JSON schema for the variables of the given operation
// with a custom recursion depth limit
func BuildJsonSchemaWithOptions(operationDocument, definitionDocument *ast.Document, maxRecursionDepth int) (*JsonSchema, error) {
	if len(operationDocument.OperationDefinitions) == 0 {
		return nil, fmt.Errorf("no operations found in document")
	}

	builder := NewVariablesSchemaBuilderWithOptions(operationDocument, definitionDocument, maxRecursionDepth)

	return builder.Build()
}
