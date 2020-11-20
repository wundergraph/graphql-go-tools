package ast

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// Create a new document with initialized slices.
// In case you're on a hot path you always want to use a pre-initialized Document.
func ExampleNewDocument() {

	schema := []byte(`
		schema {
			query: Query
		}
		
		type Query {
			hello: String!
		}
	`)

	doc := NewDocument()
	doc.Input.ResetInputBytes(schema)

	// ...then parse the Input
}

// Create a new Document without pre-initializing slices.
// Use this if you want to manually create a new Document
func ExampleDocument() {

	// create the same doc as in NewDocument() example but manually.

	doc := &Document{}

	// add Query to the raw input
	queryTypeName := doc.Input.AppendInputString("Query")

	// create a RootOperationTypeDefinition
	rootOperationTypeDefinition := RootOperationTypeDefinition{
		OperationType: OperationTypeQuery,
		NamedType: Type{
			Name: queryTypeName,
		},
	}

	// add the RootOperationTypeDefinition to the ast
	doc.RootOperationTypeDefinitions = append(doc.RootOperationTypeDefinitions, rootOperationTypeDefinition)
	// get a reference to the RootOperationTypeDefinition
	queryRootOperationTypeRef := len(doc.RootOperationTypeDefinitions) - 1

	// create a SchemaDefinition
	schemaDefinition := SchemaDefinition{
		RootOperationTypeDefinitions: RootOperationTypeDefinitionList{
			// add the RootOperationTypeDefinition reference
			Refs: []int{queryRootOperationTypeRef},
		},
	}

	// add the SchemaDefinition to the ast
	doc.SchemaDefinitions = append(doc.SchemaDefinitions, schemaDefinition)
	// get a reference to the SchemaDefinition
	schemaDefinitionRef := len(doc.SchemaDefinitions) - 1

	// add the SchemaDefinition to the RootNodes
	// all root level nodes have to be added to the RootNodes slice in order to make them available to the Walker for traversal
	doc.RootNodes = append(doc.RootNodes, Node{Kind: NodeKindSchemaDefinition, Ref: schemaDefinitionRef})

	// add another string to the raw input
	stringName := doc.Input.AppendInputString("String")

	// create a named Type
	stringType := Type{
		TypeKind: TypeKindNamed,
		Name:     stringName,
	}

	// add the Type to the ast
	doc.Types = append(doc.Types, stringType)
	// get a reference to the Type
	stringTypeRef := len(doc.Types) - 1

	// create another Type
	nonNullStringType := Type{
		TypeKind: TypeKindNonNull,
		// add a reference to the named type
		OfType: stringTypeRef,
	}
	// Result: NonNull String / String!

	// add the Type to the ast
	doc.Types = append(doc.Types, nonNullStringType)
	// get a reference to the Type
	nonNullStringTypeRef := len(doc.Types) - 1

	// add another string to the raw input
	helloName := doc.Input.AppendInputString("hello")

	// create a FieldDefinition
	helloFieldDefinition := FieldDefinition{
		Name: helloName,
		// add the Type reference
		Type: nonNullStringTypeRef,
	}

	// add the FieldDefinition to the ast
	doc.FieldDefinitions = append(doc.FieldDefinitions, helloFieldDefinition)
	// get a reference to the FieldDefinition
	helloFieldDefinitionRef := len(doc.FieldDefinitions) - 1

	// create an ObjectTypeDefinition
	queryTypeDefinition := ObjectTypeDefinition{
		Name: queryTypeName,
		// declare that this ObjectTypeDefinition has fields
		// this is necessary for the Walker to understand it must walk FieldDefinitions
		HasFieldDefinitions: true,
		FieldsDefinition: FieldDefinitionList{
			// add the FieldDefinition reference
			Refs: []int{helloFieldDefinitionRef},
		},
	}

	// add ObjectTypeDefinition to the ast
	doc.ObjectTypeDefinitions = append(doc.ObjectTypeDefinitions, queryTypeDefinition)
	// get reference to ObjectTypeDefinition
	queryTypeRef := len(doc.ObjectTypeDefinitions) - 1

	// add ObjectTypeDefinition to the RootNodes
	doc.RootNodes = append(doc.RootNodes, Node{Kind: NodeKindObjectTypeDefinition, Ref: queryTypeRef})
}

func TestKinds(t *testing.T) {
	expectedArray := func(start, count int) (out []int) {
		for i := start; i < start+count; i++ {
			out = append(out, i)
		}
		return
	}

	t.Run("operation types has correct values", func(t *testing.T) {
		operationTypes := []OperationType{
			OperationTypeUnknown,
			OperationTypeQuery,
			OperationTypeMutation,
			OperationTypeSubscription,
		}
		actualValues := make([]int, 0, len(operationTypes))
		for _, t := range operationTypes {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(0, 4), actualValues)
	})

	t.Run("value kinds has correct values", func(t *testing.T) {
		valueKinds := []ValueKind{
			ValueKindUnknown,
			ValueKindString,
			ValueKindBoolean,
			ValueKindInteger,
			ValueKindFloat,
			ValueKindVariable,
			ValueKindNull,
			ValueKindList,
			ValueKindObject,
			ValueKindEnum,
		}
		actualValues := make([]int, 0, len(valueKinds))
		for _, t := range valueKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(4, 10), actualValues)
	})

	t.Run("type kinds has correct values", func(t *testing.T) {
		typeKinds := []TypeKind{
			TypeKindUnknown,
			TypeKindNamed,
			TypeKindList,
			TypeKindNonNull,
		}
		actualValues := make([]int, 0, len(typeKinds))
		for _, t := range typeKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(14, 4), actualValues)
	})

	t.Run("selection kinds has correct values", func(t *testing.T) {
		selectionKinds := []SelectionKind{
			SelectionKindUnknown,
			SelectionKindField,
			SelectionKindFragmentSpread,
			SelectionKindInlineFragment,
		}
		actualValues := make([]int, 0, len(selectionKinds))
		for _, t := range selectionKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(18, 4), actualValues)
	})

	t.Run("node kinds has correct values", func(t *testing.T) {
		nodeKinds := []NodeKind{
			NodeKindUnknown,
			NodeKindSchemaDefinition,
			NodeKindSchemaExtension,
			NodeKindObjectTypeDefinition,
			NodeKindObjectTypeExtension,
			NodeKindInterfaceTypeDefinition,
			NodeKindInterfaceTypeExtension,
			NodeKindUnionTypeDefinition,
			NodeKindUnionTypeExtension,
			NodeKindUnionMemberType,
			NodeKindEnumTypeDefinition,
			NodeKindEnumValueDefinition,
			NodeKindEnumTypeExtension,
			NodeKindInputObjectTypeDefinition,
			NodeKindInputValueDefinition,
			NodeKindInputObjectTypeExtension,
			NodeKindScalarTypeDefinition,
			NodeKindScalarTypeExtension,
			NodeKindDirectiveDefinition,
			NodeKindOperationDefinition,
			NodeKindSelectionSet,
			NodeKindField,
			NodeKindFieldDefinition,
			NodeKindFragmentSpread,
			NodeKindInlineFragment,
			NodeKindFragmentDefinition,
			NodeKindArgument,
			NodeKindDirective,
			NodeKindVariableDefinition,
		}
		actualValues := make([]int, 0, len(nodeKinds))
		for _, t := range nodeKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(22, 29), actualValues)
	})
}

func TestFilterIntSliceByWhitelist(t *testing.T) {
	run := func(inputIntSlice []int, inputWhitelistIntSlice []int, expectedFilteredIntSlice []int) func(t *testing.T) {
		return func(t *testing.T) {
			result := FilterIntSliceByWhitelist(inputIntSlice, inputWhitelistIntSlice)
			assert.Equal(t, expectedFilteredIntSlice, result)
		}
	}

	t.Run("should return empty slice when all input slices are nil",
		run(nil, nil, []int{}),
	)

	t.Run("should return empty slice when all input slices are empty",
		run([]int{}, []int{}, []int{}),
	)

	t.Run("should return empty slice when whitelisted is empty",
		run([]int{1, 2, 3}, []int{}, []int{}),
	)

	t.Run("should return a slice with filtered int values",
		run([]int{1, 2, 3, 4}, []int{2, 3, 10}, []int{2, 3}),
	)

	t.Run("should return all values when all are whitelisted",
		run([]int{1, 2, 3}, []int{1, 2, 3, 4, 5}, []int{1, 2, 3}),
	)
}
