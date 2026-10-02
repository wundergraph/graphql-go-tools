package astvalidation

import (
	"cmp"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// newDeferPrevalidationNormalizer builds a normalizer with the options and the prevalidation rules of the execution engine.
// The rules must reject an invalid directive before the defer inline normalization removes a disabled directive.
// A non-empty operationName adds the removal of the operations that the request did not select.
func newDeferPrevalidationNormalizer(operationName string) *astnormalization.OperationNormalizer {
	options := []astnormalization.Option{
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithEnableDefer(),
		astnormalization.WithPrevalidationRules(
			DeferStreamOnValidOperations(),
			DeferStreamHaveUniqueLabels(),
			DirectivesAreInValidLocations(),
			StreamAppliedToListFieldsOnly(),
		),
	}
	if operationName != "" {
		options = append(options, astnormalization.WithRemoveNotMatchingOperationDefinitions())
	}
	return astnormalization.NewWithOpts(options...)
}

// normalizeWithDeferPrevalidation returns the report of one normalization run.
// An empty operationName normalizes the full document.
func normalizeWithDeferPrevalidation(t *testing.T, definitionInput, operationInput, variables, operationName string) operationreport.Report {
	t.Helper()

	definition := unsafeparser.ParseGraphqlDocumentString(definitionInput)
	operation := unsafeparser.ParseGraphqlDocumentString(operationInput)
	operation.Input.Variables = []byte(variables)
	normalizer := newDeferPrevalidationNormalizer(operationName)

	report := operationreport.Report{}
	if operationName == "" {
		normalizer.NormalizeOperation(&operation, &definition, &report)
		return report
	}

	normalizer.NormalizeNamedOperation(&operation, &definition, []byte(operationName), &report)
	return report
}

func TestDeferStreamOnValidOperations(t *testing.T) {
	testCases := []struct {
		name       string
		definition string
		operation  string
		variables  string
		expected   operationreport.Report
	}{
		{
			name: "[M1] mutation root inline defer with if false",
			operation: `mutation {
  ... @defer(if: false) { mutateDog { name } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("mutation")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[M2] mutation root stream with if false",
			operation: `mutation {
  mutateDogs @stream(if: false) { name }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@stream" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.FieldName, FieldName: []byte("mutateDogs")},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 14}},
			}}},
		},
		{
			name: "[M3] mutation root inline defer with if variable false",
			operation: `mutation M($d: Boolean!) {
  ... @defer(if: $d) { mutateDog { name } }
}`,
			variables: `{"d":false}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("mutation")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[M4] mutation root inline defer with absent if variable",
			operation: `mutation M($d: Boolean) {
  ... @defer(if: $d) { mutateDog { name } }
}`,
			variables: `{}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("mutation")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[M5] mutation root fragment spread defer with if false",
			operation: `mutation {
  ...rootFragment @defer(if: false)
}
fragment rootFragment on Mutation { mutateDog { name } }`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("mutation")}},
				Locations: []operationreport.Location{{Line: 2, Column: 19}},
			}}},
		},
		{
			name: "[M6] defer with if false at the top of a fragment on the mutation root type",
			operation: `mutation {
  ...rootFragment
}
fragment rootFragment on Mutation { ... @defer(if: false) { mutateDog { name } } }`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("Mutation")}},
				Locations: []operationreport.Location{{Line: 4, Column: 41}},
			}}},
		},
		{
			name: "[M7] mutation root inline fragment on the mutation root type with defer if false",
			operation: `mutation {
  ... on Mutation @defer(if: false) { mutateDog { name } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.InlineFragmentName, FieldName: []byte("Mutation")},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 19}},
			}}},
		},
		{
			name: "[M8] fragment before mutation with enabled defer on the mutation root type",
			operation: `fragment F on Mutation {
  ... @defer { mutateDog { name } }
}
mutation { ...F }`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("Mutation")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[M9] fragment before mutation with disabled defer on the mutation root type",
			operation: `fragment F on Mutation {
  ... @defer(if: false) { mutateDog { name } }
}
mutation { ...F }`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on root fields of mutation operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("Mutation")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[S1] subscription root inline defer with if false",
			operation: `subscription {
  ... @defer(if: false) { subscribeDog { name } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on subscription operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("subscription")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[S2] subscription root stream with if false",
			operation: `subscription {
  subscribeDogs @stream(if: false) { name }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@stream" is not allowed on subscription operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("subscription")},
					{Kind: ast.FieldName, FieldName: []byte("subscribeDogs")},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 17}},
			}}},
		},
		{
			name: "[S3] subscription root inline defer with if variable false",
			operation: `subscription S($d: Boolean!) {
  ... @defer(if: $d) { subscribeDog { name } }
}`,
			variables: `{"d":false}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on subscription operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("subscription")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name: "[S4] defer with if false at the top of a fragment on the subscription root type",
			operation: `subscription {
  ...rootFragment
}
fragment rootFragment on Subscription { ... @defer(if: false) { subscribeDog { name } } }`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on subscription operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("Subscription")}},
				Locations: []operationreport.Location{{Line: 4, Column: 45}},
			}}},
		},
		{
			name: "[S5] fragment before subscription with disabled defer on the subscription root type",
			operation: `fragment F on Subscription {
  ... @defer(if: false) { subscribeDog { name } }
}
subscription { ...F }`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message:   `directive "@defer" is not allowed on subscription operations`,
				Path:      ast.Path{{Kind: ast.FieldName, FieldName: []byte("Subscription")}},
				Locations: []operationreport.Location{{Line: 2, Column: 7}},
			}}},
		},
		{
			name:       "[P1] defer in a nested inline fragment on the mutation root type",
			definition: parentTypeDefinition,
			operation: `mutation {
  ... on Mutation { ... @defer { mutateDog { name } } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.InlineFragmentName, FieldName: []byte("Mutation"), FragmentRef: 1},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 25}},
			}}},
		},
		{
			name:       "[P2] defer with if false in a nested inline fragment on the mutation root type",
			definition: parentTypeDefinition,
			operation: `mutation {
  ... on Mutation { ... @defer(if: false) { mutateDog { name } } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.InlineFragmentName, FieldName: []byte("Mutation"), FragmentRef: 1},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 25}},
			}}},
		},
		{
			name:       "[P3] defer below a field that returns the mutation root type",
			definition: parentTypeDefinition,
			operation: `mutation {
  nestedMutation { ... @defer { mutateDog { name } } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.FieldName, FieldName: []byte("nestedMutation")},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 24}},
			}}},
		},
		{
			name:       "[P4] stream below a field that returns the mutation root type",
			definition: parentTypeDefinition,
			operation: `mutation {
  nestedMutation { mutateDogs @stream { name } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@stream" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.FieldName, FieldName: []byte("nestedMutation")},
					{Kind: ast.FieldName, FieldName: []byte("mutateDogs")},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 31}},
			}}},
		},
		{
			name:       "[P5] defer with if false in a nested inline fragment on the subscription root type",
			definition: parentTypeDefinition,
			operation: `subscription {
  ... on Subscription { ... @defer(if: false) { subscribeDog { name } } }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" is not allowed on subscription operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("subscription")},
					{Kind: ast.InlineFragmentName, FieldName: []byte("Subscription"), FragmentRef: 1},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 29}},
			}}},
		},
		{
			name:       "[P6] defer on an interface inline fragment at the mutation root is rejected",
			definition: parentTypeDefinition,
			operation: `mutation {
  ... on Node @defer { id }
}`,
			expected: operationreport.Report{ExternalErrors: []operationreport.ExternalError{{
				Message: `directive "@defer" is not allowed on root fields of mutation operations`,
				Path: ast.Path{
					{Kind: ast.FieldName, FieldName: []byte("mutation")},
					{Kind: ast.InlineFragmentName, FieldName: []byte("Node")},
				},
				Locations: []operationreport.Location{{Line: 2, Column: 15}},
			}}},
		},
		{
			name:       "[P7] defer inside an interface inline fragment at the mutation root is valid",
			definition: parentTypeDefinition,
			operation: `mutation {
  ... on Node { ... @defer { id } }
}`,
			expected: operationreport.Report{},
		},
		{
			name:       "[P8] defer below a nested field of a non-root type is valid",
			definition: parentTypeDefinition,
			operation: `mutation {
  nestedMutation { mutateDog { ... @defer { name } } }
}`,
			expected: operationreport.Report{},
		},
		{
			name:       "[U1] nested defer below an unknown field in a schema without a mutation type is valid",
			definition: unknownFieldDefinition,
			operation:  `query { missing { ... @defer { ok } } }`,
			expected:   operationreport.Report{},
		},
		{
			name:       "[U2] stream below an unknown field in a schema without a mutation type is valid",
			definition: unknownFieldDefinition,
			operation:  `query { missing { other @stream } }`,
			expected:   operationreport.Report{},
		},
		{
			name: "[G1] nested mutation defer with if false is valid",
			operation: `mutation {
  mutateDog { ... @defer(if: false) { name } }
}`,
			expected: operationreport.Report{},
		},
		{
			name: "[G2] nested subscription defer with if false is valid",
			operation: `subscription {
  newMessage { ... @defer(if: false) { body } }
}`,
			expected: operationreport.Report{},
		},
		{
			name: "[G3] query root defer with if false is valid",
			operation: `query {
  ... @defer(if: false) { dog { name } }
}`,
			expected: operationreport.Report{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizeWithDeferPrevalidation(t, cmp.Or(tc.definition, testDefinition), tc.operation, tc.variables, ""))
		})
	}
}

// parentTypeDefinition adds a mutation field that returns the mutation root type,
// and an interface that the mutation root type implements.
const parentTypeDefinition = `
directive @defer(label: String, if: Boolean = true) on FRAGMENT_SPREAD | INLINE_FRAGMENT
directive @stream(label: String, initialCount: Int, if: Boolean = true) on FIELD

schema {
	query: Query
	mutation: Mutation
	subscription: Subscription
}

scalar ID
scalar String
scalar Int
scalar Boolean

interface Node { id: ID }
type Dog { name: String }
type Query { dog: Dog }
type Mutation implements Node {
	id: ID
	mutateDog: Dog
	mutateDogs: [Dog]
	nestedMutation: Mutation
}
type Subscription { subscribeDog: Dog }
`

// unknownFieldDefinition has no mutation type and no subscription type.
const unknownFieldDefinition = `
directive @defer(label: String, if: Boolean = true) on FRAGMENT_SPREAD | INLINE_FRAGMENT
directive @stream(label: String, initialCount: Int, if: Boolean = true) on FIELD

schema {
	query: Query
}

scalar String
scalar Boolean
scalar Int

type Query { ok: String }
`
