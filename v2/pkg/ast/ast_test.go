package ast_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
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

	doc := ast.NewDocument()
	doc.Input.ResetInputBytes(schema)

	// ...then parse the Input
}

// Create a new Document without pre-initializing slices.
// Use this if you want to manually create a new Document
func ExampleDocument() {
	// create the same doc as in NewDocument() example but manually.

	doc := &ast.Document{}

	// add Query to the raw input
	queryTypeName := doc.Input.AppendInputString("Query")

	// create a RootOperationTypeDefinition
	rootOperationTypeDefinition := ast.RootOperationTypeDefinition{
		OperationType: ast.OperationTypeQuery,
		NamedType: ast.Type{
			Name: queryTypeName,
		},
	}

	// add the RootOperationTypeDefinition to the ast
	doc.RootOperationTypeDefinitions = append(doc.RootOperationTypeDefinitions, rootOperationTypeDefinition)
	// get a reference to the RootOperationTypeDefinition
	queryRootOperationTypeRef := len(doc.RootOperationTypeDefinitions) - 1

	// create a SchemaDefinition
	schemaDefinition := ast.SchemaDefinition{
		RootOperationTypeDefinitions: ast.RootOperationTypeDefinitionList{
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
	doc.RootNodes = append(doc.RootNodes, ast.Node{Kind: ast.NodeKindSchemaDefinition, Ref: schemaDefinitionRef})

	// add another string to the raw input
	stringName := doc.Input.AppendInputString("String")

	// create a named Type
	stringType := ast.Type{
		TypeKind: ast.TypeKindNamed,
		Name:     stringName,
	}

	// add the Type to the ast
	doc.Types = append(doc.Types, stringType)
	// get a reference to the Type
	stringTypeRef := len(doc.Types) - 1

	// create another Type
	nonNullStringType := ast.Type{
		TypeKind: ast.TypeKindNonNull,
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
	helloFieldDefinition := ast.FieldDefinition{
		Name: helloName,
		// add the Type reference
		Type: nonNullStringTypeRef,
	}

	// add the FieldDefinition to the ast
	doc.FieldDefinitions = append(doc.FieldDefinitions, helloFieldDefinition)
	// get a reference to the FieldDefinition
	helloFieldDefinitionRef := len(doc.FieldDefinitions) - 1

	// create an ObjectTypeDefinition
	queryTypeDefinition := ast.ObjectTypeDefinition{
		Name: queryTypeName,
		// declare that this ObjectTypeDefinition has fields
		// this is necessary for the Walker to understand it must walk FieldDefinitions
		HasFieldDefinitions: true,
		FieldsDefinition: ast.FieldDefinitionList{
			// add the FieldDefinition reference
			Refs: []int{helloFieldDefinitionRef},
		},
	}

	// add ObjectTypeDefinition to the ast
	doc.ObjectTypeDefinitions = append(doc.ObjectTypeDefinitions, queryTypeDefinition)
	// get reference to ObjectTypeDefinition
	queryTypeRef := len(doc.ObjectTypeDefinitions) - 1

	// add ObjectTypeDefinition to the RootNodes
	doc.RootNodes = append(doc.RootNodes, ast.Node{Kind: ast.NodeKindObjectTypeDefinition, Ref: queryTypeRef})
}

func TestCopying(t *testing.T) {
	doc, report := astparser.ParseGraphqlDocumentString(`
		query testQuery($someVariable: String!) {
			user {
				fieldToCopy {
					booleanArgField(arg: true)
					enumArgField(arg: SOME_ENUM_VALUE)
					floatArgField(arg: 3.14)
					intArgField(arg: 6)
					listArgField(arg: [1, 2, 3, 4])
					objectArgField(arg: {key: "value"})
					stringArgField(arg: "hello")
					variableArgField(arg: $someVariable)
					twoArgField(argOne: true, argTwo: false)
					scalarField
					objectField {
						fieldOne
						fieldTwo
					}
					aliasedField: nonAliasedField
					...namedFragment
					... on SomeType {
						inlineFragmentField
						... on AnotherType {
							nestedInlineFragmentField
						}
					}
					directiveField @requires(fields: "scalarField") @anotherDirective()
				}
			}
		}

		fragment namedFragment on SomeType {
			fragmentField
		}
	`)

	assert.False(t, report.HasErrors())

	for ref := range doc.Fields {
		if doc.FieldNameString(ref) == "user" {
			selectionSet := doc.Fields[ref].SelectionSet
			selectionToCopy := doc.SelectionSets[selectionSet].SelectionRefs[0]
			doc.AddSelection(selectionSet, doc.Selections[doc.CopySelection(selectionToCopy)])
			break
		}
	}

	out, err := astprinter.PrintStringIndent(&doc, nil, "  ")

	assert.NoError(t, err)

	expected := `query testQuery($someVariable: String!){
    user {
        fieldToCopy {
            booleanArgField(arg: true)
            enumArgField(arg: SOME_ENUM_VALUE)
            floatArgField(arg: 3.14)
            intArgField(arg: 6)
            listArgField(arg: [1,2,3,4])
            objectArgField(arg: {key: "value"})
            stringArgField(arg: "hello")
            variableArgField(arg: $someVariable)
            twoArgField(argOne: true, argTwo: false)
            scalarField
            objectField {
                fieldOne
                fieldTwo
            }
            aliasedField: nonAliasedField
            ...namedFragment
            ... on SomeType {
                inlineFragmentField
                ... on AnotherType {
                    nestedInlineFragmentField
                }
            }
            directiveField @requires(fields: "scalarField") @anotherDirective
        }
        fieldToCopy {
            booleanArgField(arg: true)
            enumArgField(arg: SOME_ENUM_VALUE)
            floatArgField(arg: 3.14)
            intArgField(arg: 6)
            listArgField(arg: [1,2,3,4])
            objectArgField(arg: {key: "value"})
            stringArgField(arg: "hello")
            variableArgField(arg: $someVariable)
            twoArgField(argOne: true, argTwo: false)
            scalarField
            objectField {
                fieldOne
                fieldTwo
            }
            aliasedField: nonAliasedField
            ...namedFragment
            ... on SomeType {
                inlineFragmentField
                ... on AnotherType {
                    nestedInlineFragmentField
                }
            }
            directiveField @requires(fields: "scalarField") @anotherDirective
        }
    }
}

fragment namedFragment on SomeType {
    fragmentField
}`

	assert.Equal(t, expected, out)
}

func TestKinds(t *testing.T) {
	expectedArray := func(start, count int) (out []int) {
		for i := start; i < start+count; i++ {
			out = append(out, i)
		}
		return
	}

	t.Run("operation types has correct values", func(t *testing.T) {
		operationTypes := []ast.OperationType{
			ast.OperationTypeUnknown,
			ast.OperationTypeQuery,
			ast.OperationTypeMutation,
			ast.OperationTypeSubscription,
		}
		actualValues := make([]int, 0, len(operationTypes))
		for _, t := range operationTypes {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(0, 4), actualValues)
	})

	t.Run("value kinds has correct values", func(t *testing.T) {
		valueKinds := []ast.ValueKind{
			ast.ValueKindUnknown,
			ast.ValueKindString,
			ast.ValueKindBoolean,
			ast.ValueKindInteger,
			ast.ValueKindFloat,
			ast.ValueKindVariable,
			ast.ValueKindNull,
			ast.ValueKindList,
			ast.ValueKindObject,
			ast.ValueKindEnum,
		}
		actualValues := make([]int, 0, len(valueKinds))
		for _, t := range valueKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(4, 10), actualValues)
	})

	t.Run("type kinds has correct values", func(t *testing.T) {
		typeKinds := []ast.TypeKind{
			ast.TypeKindUnknown,
			ast.TypeKindNamed,
			ast.TypeKindList,
			ast.TypeKindNonNull,
		}
		actualValues := make([]int, 0, len(typeKinds))
		for _, t := range typeKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(14, 4), actualValues)
	})

	t.Run("selection kinds has correct values", func(t *testing.T) {
		selectionKinds := []ast.SelectionKind{
			ast.SelectionKindUnknown,
			ast.SelectionKindField,
			ast.SelectionKindFragmentSpread,
			ast.SelectionKindInlineFragment,
		}
		actualValues := make([]int, 0, len(selectionKinds))
		for _, t := range selectionKinds {
			actualValues = append(actualValues, int(t))
		}
		assert.Equal(t, expectedArray(18, 4), actualValues)
	})

	t.Run("node kinds has correct values", func(t *testing.T) {
		nodeKinds := []ast.NodeKind{
			ast.NodeKindUnknown,
			ast.NodeKindSchemaDefinition,
			ast.NodeKindSchemaExtension,
			ast.NodeKindObjectTypeDefinition,
			ast.NodeKindObjectTypeExtension,
			ast.NodeKindInterfaceTypeDefinition,
			ast.NodeKindInterfaceTypeExtension,
			ast.NodeKindUnionTypeDefinition,
			ast.NodeKindUnionTypeExtension,
			ast.NodeKindUnionMemberType,
			ast.NodeKindEnumTypeDefinition,
			ast.NodeKindEnumValueDefinition,
			ast.NodeKindEnumTypeExtension,
			ast.NodeKindInputObjectTypeDefinition,
			ast.NodeKindInputValueDefinition,
			ast.NodeKindInputObjectTypeExtension,
			ast.NodeKindScalarTypeDefinition,
			ast.NodeKindScalarTypeExtension,
			ast.NodeKindDirectiveDefinition,
			ast.NodeKindOperationDefinition,
			ast.NodeKindSelectionSet,
			ast.NodeKindField,
			ast.NodeKindFieldDefinition,
			ast.NodeKindFragmentSpread,
			ast.NodeKindInlineFragment,
			ast.NodeKindFragmentDefinition,
			ast.NodeKindArgument,
			ast.NodeKindDirective,
			ast.NodeKindVariableDefinition,
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
			result := ast.FilterIntSliceByWhitelist(inputIntSlice, inputWhitelistIntSlice)
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

func TestDocument_NodeByName(t *testing.T) {
	schema := "schema {query: Query} type Query {queryName: String}"

	prepareDoc := func() *ast.Document {
		doc := unsafeparser.ParseGraphqlDocumentString(schema)
		return &doc
	}

	t.Run("should return a node", func(t *testing.T) {
		doc := prepareDoc()

		t.Run("when node name is Query", func(t *testing.T) {
			t.Run("NodeByName", func(t *testing.T) {
				node, exists := doc.NodeByName(ast.DefaultQueryTypeName)
				assert.Equal(t, ast.NodeKindObjectTypeDefinition, node.Kind)
				assert.True(t, exists)
			})

			t.Run("NodeByNameStr", func(t *testing.T) {
				node, exists := doc.NodeByNameStr("Query")
				assert.Equal(t, ast.NodeKindObjectTypeDefinition, node.Kind)
				assert.True(t, exists)
			})
		})

		t.Run("when node name is schema", func(t *testing.T) {
			t.Run("NodeByName", func(t *testing.T) {
				node, exists := doc.NodeByName([]byte("schema"))
				assert.Equal(t, ast.NodeKindSchemaDefinition, node.Kind)
				assert.True(t, exists)
			})

			t.Run("NodeByNameStr", func(t *testing.T) {
				node, exists := doc.NodeByNameStr("schema")
				assert.Equal(t, ast.NodeKindSchemaDefinition, node.Kind)
				assert.True(t, exists)
			})
		})
	})

	t.Run("should return false for not existing node", func(t *testing.T) {
		doc := prepareDoc()

		t.Run("NodeByName", func(t *testing.T) {
			node, exists := doc.NodeByName([]byte("NotExisting"))
			assert.Equal(t, ast.InvalidNode, node)
			assert.False(t, exists)
		})

		t.Run("NodeByNameStr", func(t *testing.T) {
			node, exists := doc.NodeByNameStr("NotExisting")
			assert.Equal(t, ast.InvalidNode, node)
			assert.False(t, exists)
		})
	})
}

func TestDirectiveList_RemoveDirectiveByName(t *testing.T) {
	const schema = "type User @directive1 @directive2 @directive3 @directive4 @directive5 { field: String! }"
	doc, _ := astparser.ParseGraphqlDocumentString(schema)
	replacer := strings.NewReplacer(" ", "", "\t", "", "\r", "", "\n", "")
	// delete the last directive
	doc.ObjectTypeDefinitions[0].Directives.RemoveDirectiveByName(&doc, "directive5")
	// delete the middle directive
	doc.ObjectTypeDefinitions[0].Directives.RemoveDirectiveByName(&doc, "directive3")
	// delete the first directive
	doc.ObjectTypeDefinitions[0].Directives.RemoveDirectiveByName(&doc, "directive1")
	out, _ := astprinter.PrintString(&doc, nil)
	assert.Equal(t, replacer.Replace("type User @directive2 @directive4 { field: String! }"), replacer.Replace(out))
}

func TestDirectiveList_HasDirectiveByName(t *testing.T) {
	const schema = "type User @directive1 @directive2 @directive3 @directive4 @directive5 { field: String! }"
	doc, _ := astparser.ParseGraphqlDocumentString(schema)
	l := doc.ObjectTypeDefinitions[0].Directives
	// search the last directive
	assert.Equal(t, true, l.HasDirectiveByName(&doc, "directive5"))
	// search the middle directive
	assert.Equal(t, true, l.HasDirectiveByName(&doc, "directive3"))
	// search the first directive
	assert.Equal(t, true, l.HasDirectiveByName(&doc, "directive1"))
	// search not found
	assert.Equal(t, false, l.HasDirectiveByName(&doc, "directive0"))
}
