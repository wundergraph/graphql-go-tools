package responsejsonschema

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

const corruptASTDefinition = `
	directive @tag on OBJECT | ENUM_VALUE

	type Query {
		node: Node!
		status: Status!
		search: Search!
	}

	interface Node {
		id: ID!
		child: Child!
	}

	type User implements Node @tag {
		id: ID!
		child: Child!
		username: String!
		role: String!
	}

	type Bot implements Node {
		id: ID!
		child: Child!
	}

	type Child {
		name: String!
	}

	union Search = User | Bot

	enum Status {
		OK @tag
	}
`

const corruptASTOperation = `
	query CorruptAST($show: Boolean!) {
		entry: node {
			id @include(if: true)
			child { name }
			... on User @include(if: true) { username }
			...UserFields @skip(if: $show)
		}
		status
		search { __typename }
	}

	fragment UserFields on User {
		role
	}
`

func TestBuildResponseSchema_RejectsCorruptBorrowedASTWithoutPanicking(t *testing.T) {
	type corruptCase struct {
		name      string
		fieldPath []string
		mutate    func(operation, definition *ast.Document)
		wantError string
	}

	invalidByteRange := func(document *ast.Document) ast.ByteSliceReference {
		return ast.ByteSliceReference{Start: uint32(len(document.Input.RawBytes)), End: uint32(len(document.Input.RawBytes) + 1)}
	}

	tests := []corruptCase{
		{
			name: "selection set selection reference",
			mutate: func(operation, _ *ast.Document) {
				selectionSetRef := operation.Fields[operationFieldRef(t, operation, "node")].SelectionSet
				operation.SelectionSets[selectionSetRef].SelectionRefs[0] = len(operation.Selections)
			},
			wantError: "selection reference",
		},
		{
			name: "field selection reference",
			mutate: func(operation, _ *ast.Document) {
				selectionRef := operationSelectionRef(t, operation, operation.OperationDefinitions[0].SelectionSet, ast.SelectionKindField, "node")
				operation.Selections[selectionRef].Ref = len(operation.Fields)
			},
			wantError: "field reference",
		},
		{
			name: "inline fragment selection reference",
			mutate: func(operation, _ *ast.Document) {
				selectionSetRef := operation.Fields[operationFieldRef(t, operation, "node")].SelectionSet
				selectionRef := operationSelectionRef(t, operation, selectionSetRef, ast.SelectionKindInlineFragment, "")
				operation.Selections[selectionRef].Ref = len(operation.InlineFragments)
			},
			wantError: "inline fragment reference",
		},
		{
			name: "fragment spread selection reference",
			mutate: func(operation, _ *ast.Document) {
				selectionSetRef := operation.Fields[operationFieldRef(t, operation, "node")].SelectionSet
				selectionRef := operationSelectionRef(t, operation, selectionSetRef, ast.SelectionKindFragmentSpread, "")
				operation.Selections[selectionRef].Ref = len(operation.FragmentSpreads)
			},
			wantError: "fragment spread reference",
		},
		{
			name: "field child selection set reference",
			mutate: func(operation, _ *ast.Document) {
				operation.Fields[operationFieldRef(t, operation, "child")].SelectionSet = len(operation.SelectionSets)
			},
			wantError: "selection set reference",
		},
		{
			name: "inline fragment child selection set reference",
			mutate: func(operation, _ *ast.Document) {
				operation.InlineFragments[0].SelectionSet = len(operation.SelectionSets)
			},
			wantError: "selection set reference",
		},
		{
			name: "fragment definition child selection set reference",
			mutate: func(operation, _ *ast.Document) {
				operation.FragmentDefinitions[0].SelectionSet = len(operation.SelectionSets)
			},
			wantError: "selection set reference",
		},
		{
			name: "field directive reference",
			mutate: func(operation, _ *ast.Document) {
				fieldRef := operationFieldRef(t, operation, "id")
				operation.Fields[fieldRef].Directives.Refs[0] = len(operation.Directives)
			},
			wantError: "directive reference",
		},
		{
			name: "inline fragment directive reference",
			mutate: func(operation, _ *ast.Document) {
				operation.InlineFragments[0].Directives.Refs[0] = len(operation.Directives)
			},
			wantError: "directive reference",
		},
		{
			name: "fragment spread directive reference",
			mutate: func(operation, _ *ast.Document) {
				operation.FragmentSpreads[0].Directives.Refs[0] = len(operation.Directives)
			},
			wantError: "directive reference",
		},
		{
			name: "directive name byte range",
			mutate: func(operation, _ *ast.Document) {
				operation.Directives[0].Name = invalidByteRange(operation)
			},
			wantError: "directive name byte range",
		},
		{
			name: "directive argument reference",
			mutate: func(operation, _ *ast.Document) {
				operation.Directives[0].Arguments.Refs[0] = len(operation.Arguments)
			},
			wantError: "argument reference",
		},
		{
			name: "directive argument name byte range",
			mutate: func(operation, _ *ast.Document) {
				argumentRef := operation.Directives[0].Arguments.Refs[0]
				operation.Arguments[argumentRef].Name = invalidByteRange(operation)
			},
			wantError: "argument name byte range",
		},
		{
			name: "directive boolean value reference",
			mutate: func(operation, _ *ast.Document) {
				argumentRef := operation.Directives[0].Arguments.Refs[0]
				operation.Arguments[argumentRef].Value.Ref = len(operation.BooleanValues)
			},
			wantError: "boolean value reference",
		},
		{
			name: "directive variable value reference",
			mutate: func(operation, _ *ast.Document) {
				directiveRef := operation.FragmentSpreads[0].Directives.Refs[0]
				argumentRef := operation.Directives[directiveRef].Arguments.Refs[0]
				operation.Arguments[argumentRef].Value.Ref = len(operation.VariableValues)
			},
			wantError: "variable value reference",
		},
		{
			name: "directive variable name byte range",
			mutate: func(operation, _ *ast.Document) {
				directiveRef := operation.FragmentSpreads[0].Directives.Refs[0]
				argumentRef := operation.Directives[directiveRef].Arguments.Refs[0]
				variableRef := operation.Arguments[argumentRef].Value.Ref
				operation.VariableValues[variableRef].Name = invalidByteRange(operation)
			},
			wantError: "variable name byte range",
		},
		{
			name: "field name byte range",
			mutate: func(operation, _ *ast.Document) {
				operation.Fields[operationFieldRef(t, operation, "node")].Name = invalidByteRange(operation)
			},
			wantError: "field name byte range",
		},
		{
			name: "field alias byte range",
			mutate: func(operation, _ *ast.Document) {
				operation.Fields[operationFieldRef(t, operation, "node")].Alias.Name = invalidByteRange(operation)
			},
			wantError: "field alias byte range",
		},
		{
			name: "inline fragment type reference",
			mutate: func(operation, _ *ast.Document) {
				operation.InlineFragments[0].TypeCondition.Type = len(operation.Types)
			},
			wantError: "inline fragment type reference",
		},
		{
			name: "inline fragment type name byte range",
			mutate: func(operation, _ *ast.Document) {
				typeRef := operation.InlineFragments[0].TypeCondition.Type
				operation.Types[typeRef].Name = invalidByteRange(operation)
			},
			wantError: "inline fragment type name byte range",
		},
		{
			name: "fragment spread name byte range",
			mutate: func(operation, _ *ast.Document) {
				operation.FragmentSpreads[0].FragmentName = invalidByteRange(operation)
			},
			wantError: "fragment spread name byte range",
		},
		{
			name: "fragment definition name byte range",
			mutate: func(operation, _ *ast.Document) {
				operation.FragmentDefinitions[0].Name = invalidByteRange(operation)
			},
			wantError: "fragment definition name byte range",
		},
		{
			name: "fragment definition type reference",
			mutate: func(operation, _ *ast.Document) {
				operation.FragmentDefinitions[0].TypeCondition.Type = len(operation.Types)
			},
			wantError: "fragment definition type reference",
		},
		{
			name: "fragment definition type name byte range",
			mutate: func(operation, _ *ast.Document) {
				typeRef := operation.FragmentDefinitions[0].TypeCondition.Type
				operation.Types[typeRef].Name = invalidByteRange(operation)
			},
			wantError: "fragment definition type name byte range",
		},
		{
			name: "definition field reference",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				queryNode, ok := definition.Index.FirstNodeByNameStr("Query")
				require.True(t, ok)
				definition.ObjectTypeDefinitions[queryNode.Ref].FieldsDefinition.Refs[0] = len(definition.FieldDefinitions)
			},
			wantError: "field definition reference",
		},
		{
			name: "root type index name mismatch",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				queryNode, ok := definition.Index.FirstNodeByNameStr("Query")
				require.True(t, ok)
				userNode, ok := definition.Index.FirstNodeByNameStr("User")
				require.True(t, ok)
				definition.Index.ReplaceNode([]byte("Query"), queryNode, userNode)
			},
			wantError: "index lookup for type \"Query\" returned node named \"User\"",
		},
		{
			name: "nested type index name mismatch",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("Node")
				require.True(t, ok)
				childNode, ok := definition.Index.FirstNodeByNameStr("Child")
				require.True(t, ok)
				definition.Index.ReplaceNode([]byte("Node"), node, childNode)
			},
			wantError: "index lookup for type \"Node\" returned node named \"Child\"",
		},
		{
			name: "fragment condition index name mismatch",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				userNode, ok := definition.Index.FirstNodeByNameStr("User")
				require.True(t, ok)
				botNode, ok := definition.Index.FirstNodeByNameStr("Bot")
				require.True(t, ok)
				definition.Index.ReplaceNode([]byte("User"), userNode, botNode)
			},
			wantError: "index lookup for type \"User\" returned node named \"Bot\"",
		},
		{
			name:      "enum type index name mismatch",
			fieldPath: []string{"status"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				statusNode, ok := definition.Index.FirstNodeByNameStr("Status")
				require.True(t, ok)
				searchNode, ok := definition.Index.FirstNodeByNameStr("Search")
				require.True(t, ok)
				definition.Index.ReplaceNode([]byte("Status"), statusNode, searchNode)
			},
			wantError: "index lookup for type \"Status\" returned node named \"Search\"",
		},
		{
			name: "definition field name byte range",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				definition.FieldDefinitions[definitionFieldRef(t, definition, "node")].Name = invalidByteRange(definition)
			},
			wantError: "field definition name byte range",
		},
		{
			name: "definition field type reference",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				definition.FieldDefinitions[definitionFieldRef(t, definition, "node")].Type = len(definition.Types)
			},
			wantError: "type reference",
		},
		{
			name: "definition named type byte range",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				typeRef := definition.FieldDefinitions[definitionFieldRef(t, definition, "node")].Type
				for definition.Types[typeRef].TypeKind != ast.TypeKindNamed {
					typeRef = definition.Types[typeRef].OfType
				}
				definition.Types[typeRef].Name = invalidByteRange(definition)
			},
			wantError: "type name byte range",
		},
		{
			name: "definition wrapped type reference",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				typeRef := definition.FieldDefinitions[definitionFieldRef(t, definition, "node")].Type
				definition.Types[typeRef].OfType = len(definition.Types)
			},
			wantError: "inner type reference",
		},
		{
			name: "interface implementor reference",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("Node")
				require.True(t, ok)
				definition.InterfaceTypeDefinitions[node.Ref].ImplementedByObjectDefinitions[0] = len(definition.ObjectTypeDefinitions)
			},
			wantError: "implementing object reference",
		},
		{
			name: "interface name byte range",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("Node")
				require.True(t, ok)
				definition.InterfaceTypeDefinitions[node.Ref].Name = invalidByteRange(definition)
			},
			wantError: "interface type name byte range",
		},
		{
			name: "implementing object name byte range",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("User")
				require.True(t, ok)
				definition.ObjectTypeDefinitions[node.Ref].Name = invalidByteRange(definition)
			},
			wantError: "object type name byte range",
		},
		{
			name: "object directive reference",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("User")
				require.True(t, ok)
				definition.ObjectTypeDefinitions[node.Ref].Directives.Refs[0] = len(definition.Directives)
			},
			wantError: "object directive reference",
		},
		{
			name: "object directive name byte range",
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("User")
				require.True(t, ok)
				directiveRef := definition.ObjectTypeDefinitions[node.Ref].Directives.Refs[0]
				definition.Directives[directiveRef].Name = invalidByteRange(definition)
			},
			wantError: "object directive name byte range",
		},
		{
			name:      "union member type reference",
			fieldPath: []string{"search"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("Search")
				require.True(t, ok)
				definition.UnionTypeDefinitions[node.Ref].UnionMemberTypes.Refs[0] = len(definition.Types)
			},
			wantError: "union member type reference",
		},
		{
			name:      "union member type name byte range",
			fieldPath: []string{"search"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("Search")
				require.True(t, ok)
				typeRef := definition.UnionTypeDefinitions[node.Ref].UnionMemberTypes.Refs[0]
				definition.Types[typeRef].Name = invalidByteRange(definition)
			},
			wantError: "union member type name byte range",
		},
		{
			name:      "enum value definition reference",
			fieldPath: []string{"status"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				node, ok := definition.Index.FirstNodeByNameStr("Status")
				require.True(t, ok)
				definition.EnumTypeDefinitions[node.Ref].EnumValuesDefinition.Refs[0] = len(definition.EnumValueDefinitions)
			},
			wantError: "enum value definition reference",
		},
		{
			name:      "enum value name byte range",
			fieldPath: []string{"status"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				definition.EnumValueDefinitions[0].EnumValue = invalidByteRange(definition)
			},
			wantError: "enum value name byte range",
		},
		{
			name:      "enum value directive reference",
			fieldPath: []string{"status"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				definition.EnumValueDefinitions[0].Directives.Refs[0] = len(definition.Directives)
			},
			wantError: "enum value directive reference",
		},
		{
			name:      "enum value directive name byte range",
			fieldPath: []string{"status"},
			mutate: func(_ *ast.Document, definition *ast.Document) {
				directiveRef := definition.EnumValueDefinitions[0].Directives.Refs[0]
				definition.Directives[directiveRef].Name = invalidByteRange(definition)
			},
			wantError: "enum value directive name byte range",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			operation := parseDocument(t, corruptASTOperation)
			definition := parseDocument(t, corruptASTDefinition)
			test.mutate(&operation, &definition)
			fieldPath := test.fieldPath
			if len(fieldPath) == 0 {
				fieldPath = []string{"entry"}
			}

			var err error
			require.NotPanics(t, func() {
				_, err = Build(&operation, &definition, fieldPath)
			})
			require.ErrorContains(t, err, test.wantError)
		})
	}
}

func operationFieldRef(t *testing.T, operation *ast.Document, fieldName string) int {
	t.Helper()
	for fieldRef := range operation.Fields {
		if operation.FieldNameString(fieldRef) == fieldName {
			return fieldRef
		}
	}
	require.FailNow(t, "operation field not found", fieldName)
	return ast.InvalidRef
}

func operationSelectionRef(t *testing.T, operation *ast.Document, selectionSetRef int, kind ast.SelectionKind, fieldName string) int {
	t.Helper()
	for _, selectionRef := range operation.SelectionSets[selectionSetRef].SelectionRefs {
		selection := operation.Selections[selectionRef]
		if selection.Kind != kind {
			continue
		}
		if kind != ast.SelectionKindField || operation.FieldNameString(selection.Ref) == fieldName {
			return selectionRef
		}
	}
	require.FailNow(t, "operation selection not found")
	return ast.InvalidRef
}

func definitionFieldRef(t *testing.T, definition *ast.Document, fieldName string) int {
	t.Helper()
	for fieldRef := range definition.FieldDefinitions {
		if definition.FieldDefinitionNameString(fieldRef) == fieldName {
			return fieldRef
		}
	}
	require.FailNow(t, "definition field not found", fieldName)
	return ast.InvalidRef
}
