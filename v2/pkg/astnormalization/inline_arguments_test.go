package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const inlineArgumentsTestSchema = `
	schema { query: Query }
	type Query {
		userById(userId: ID!): User
		user: User
		field(order: Sort, flag: Boolean, by: [Int], obj: Filter): String
	}
	type User {
		loginName: String
		posts(first: Int): String
	}
	enum Sort { ASC DESC }
	input Filter { a: Int }
`

func runInlineArgumentsRule(t *testing.T, operation string, opts InlineArgumentsValidationOptions) (*NormalizationResult, *operationreport.Report) {
	t.Helper()
	return runInlineArgumentsRuleWithRunOpts(t, operation, opts, RunOptions{})
}

func runInlineArgumentsRuleWithRunOpts(t *testing.T, operation string, opts InlineArgumentsValidationOptions, runOpts RunOptions) (*NormalizationResult, *operationreport.Report) {
	t.Helper()

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(inlineArgumentsTestSchema)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := &operationreport.Report{}

	normalizer := NewWithOpts(WithInlineArgumentsValidation(opts))
	result := normalizer.NormalizeNamedOperationWithResult(&operationDocument, &definitionDocument, nil, report, runOpts)

	return result, report
}

func TestInlineArgumentsRule_Detection(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		expected  []InlineArgument
	}{
		{
			name:      "inline string field argument",
			operation: `query GetUserById { userById(userId: "12345") { loginName } }`,
			expected: []InlineArgument{
				{ArgumentName: "userId", AncestorName: "userById", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindString},
			},
		},
		{
			name:      "variable field argument is compliant",
			operation: `query GetUserById($userId: ID!) { userById(userId: $userId) { loginName } }`,
			expected:  nil,
		},
		{
			name:      "inline enum argument",
			operation: `query { field(order: ASC) }`,
			expected: []InlineArgument{
				{ArgumentName: "order", AncestorName: "field", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindEnum},
			},
		},
		{
			name:      "inline null argument",
			operation: `query { field(flag: null) }`,
			expected: []InlineArgument{
				{ArgumentName: "flag", AncestorName: "field", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindNull},
			},
		},
		{
			name:      "inline list argument recorded once",
			operation: `query { field(by: [1, 2, 3]) }`,
			expected: []InlineArgument{
				{ArgumentName: "by", AncestorName: "field", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindList},
			},
		},
		{
			name:      "inline object argument recorded once",
			operation: `query { field(obj: { a: 1 }) }`,
			expected: []InlineArgument{
				{ArgumentName: "obj", AncestorName: "field", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindObject},
			},
		},
		{
			name:      "mixed variable and literal flags only the literal",
			operation: `query q($flag: Boolean) { field(flag: $flag, order: DESC) }`,
			expected: []InlineArgument{
				{ArgumentName: "order", AncestorName: "field", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindEnum},
			},
		},
		{
			name:      "inline directive argument (@include)",
			operation: `query q($userId: ID!) { userById(userId: $userId) @include(if: true) { loginName } }`,
			expected: []InlineArgument{
				{ArgumentName: "if", AncestorName: "include", AncestorKind: ast.NodeKindDirective, ValueKind: ast.ValueKindBoolean},
			},
		},
		{
			name:      "variable directive argument is compliant",
			operation: `query q($userId: ID!, $show: Boolean!) { userById(userId: $userId) @include(if: $show) { loginName } }`,
			expected:  nil,
		},
		{
			name:      "introspection field argument",
			operation: `query { __type(name: "User") { name } }`,
			expected: []InlineArgument{
				{ArgumentName: "name", AncestorName: "__type", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindString},
			},
		},
		{
			// Proves detection runs before @skip/@include prunes the node: both the
			// directive's own `if` and the child field's `first` are reported even
			// though normalization would delete the `user` selection.
			name:      "argument under a @skip(if:true)-removed node still flagged",
			operation: `query q($userId: ID!) { user @skip(if: true) { posts(first: 10) } userById(userId: $userId) { loginName } }`,
			expected: []InlineArgument{
				{ArgumentName: "if", AncestorName: "skip", AncestorKind: ast.NodeKindDirective, ValueKind: ast.ValueKindBoolean},
				{ArgumentName: "first", AncestorName: "posts", AncestorKind: ast.NodeKindField, ValueKind: ast.ValueKindInteger},
			},
		},
		{
			name:      "no inline arguments",
			operation: `query q($userId: ID!) { userById(userId: $userId) { loginName } }`,
			expected:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, report := runInlineArgumentsRule(t, tt.operation, InlineArgumentsValidationOptions{Enforce: false})
			require.False(t, report.HasErrors(), "log-only mode must never error: %s", report.Error())
			require.NotNil(t, result)

			if len(tt.expected) == 0 {
				assert.Empty(t, result.InlineArguments)
				return
			}

			require.Len(t, result.InlineArguments, len(tt.expected))
			got := make([]InlineArgument, len(result.InlineArguments))
			for i, f := range result.InlineArguments {
				f.Position = tt.expected[i].Position // ignore position in this comparison
				got[i] = f
			}
			assert.Equal(t, tt.expected, got)
		})
	}
}

func TestInlineArgumentsRule_Position(t *testing.T) {
	// The reported position points at the argument in the operation as parsed.
	// `userId` starts at column 30 on line 1 of the operation below.
	operation := `query GetUserById { userById(userId: "12345") { loginName } }`
	result, report := runInlineArgumentsRule(t, operation, InlineArgumentsValidationOptions{
		Enforce: false,
	})
	require.False(t, report.HasErrors())
	require.NotNil(t, result)
	require.Len(t, result.InlineArguments, 1)

	pos := result.InlineArguments[0].Position
	assert.Equal(t, uint32(1), pos.LineStart)
	assert.Equal(t, uint32(30), pos.CharStart)
}

func TestInlineArgumentsRule_Enforce(t *testing.T) {
	t.Run("stops at the first inline argument and reports a typed error", func(t *testing.T) {
		result, report := runInlineArgumentsRule(t,
			`query { userById(userId: "12345") { loginName } field(order: ASC) }`,
			InlineArgumentsValidationOptions{
				Enforce:      true,
				ErrorMessage: "Inline argument values are not allowed. Use variables instead.",
				ErrorCode:    "INLINE_ARGUMENT_VALUES_NOT_ALLOWED",
				StatusCode:   400,
			},
		)

		require.True(t, report.HasErrors())
		require.Len(t, report.ExternalErrors, 1)
		extErr := report.ExternalErrors[0]
		assert.Equal(t, "Inline argument values are not allowed. Use variables instead.", extErr.Message)
		assert.Equal(t, "INLINE_ARGUMENT_VALUES_NOT_ALLOWED", extErr.ExtensionCode)
		assert.Equal(t, 400, extErr.StatusCode)

		// Enforce rejects on the first inline argument and stops the walk, so
		// normalization fails and no result is returned.
		assert.Nil(t, result)

		// The rejection is a generic error: no per-argument location is attached.
		assert.Empty(t, extErr.Locations)
	})

	t.Run("compliant operation passes enforce mode", func(t *testing.T) {
		result, report := runInlineArgumentsRule(t,
			`query q($userId: ID!) { userById(userId: $userId) { loginName } }`,
			InlineArgumentsValidationOptions{Enforce: true, ErrorMessage: "nope", ErrorCode: "CODE", StatusCode: 400},
		)
		require.False(t, report.HasErrors(), "compliant operation must not error: %s", report.Error())
		require.NotNil(t, result)
		assert.Empty(t, result.InlineArguments)
	})

	t.Run("SkipInlineArguments records nothing and does not enforce", func(t *testing.T) {
		result, report := runInlineArgumentsRuleWithRunOpts(t,
			`query { userById(userId: "12345") { loginName } }`,
			InlineArgumentsValidationOptions{Enforce: true, ErrorMessage: "x", ErrorCode: "C", StatusCode: 400},
			RunOptions{SkipInlineArguments: true},
		)

		require.False(t, report.HasErrors())
		require.NotNil(t, result)
		assert.Empty(t, result.InlineArguments)
	})
}

func TestInlineArgumentsRule_OptionOffReturnsNil(t *testing.T) {
	definitionDocument := unsafeparser.ParseGraphqlDocumentString(inlineArgumentsTestSchema)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))
	operationDocument := unsafeparser.ParseGraphqlDocumentString(`query { userById(userId: "12345") { loginName } }`)
	report := &operationreport.Report{}

	// No WithInlineArgumentsValidation option: there is no result to produce.
	normalizer := NewWithOpts()
	result := normalizer.NormalizeNamedOperationWithResult(&operationDocument, &definitionDocument, nil, report, RunOptions{})

	require.False(t, report.HasErrors())
	assert.Nil(t, result)
}
