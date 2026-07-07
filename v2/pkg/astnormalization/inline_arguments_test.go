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

func runInlineArgumentsRule(t *testing.T, operation string, opts InlineArgumentsValidationOptions) (*InlineArgumentsValidator, *operationreport.Report) {
	t.Helper()

	definitionDocument := unsafeparser.ParseGraphqlDocumentString(inlineArgumentsTestSchema)
	require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))

	operationDocument := unsafeparser.ParseGraphqlDocumentString(operation)
	report := &operationreport.Report{}

	validator := &InlineArgumentsValidator{Options: opts}
	normalizer := NewWithOpts(WithPrevalidationRules(InlineArgumentsRule(validator)))
	normalizer.NormalizeOperation(&operationDocument, &definitionDocument, report)

	return validator, report
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
				{ArgumentName: "userId", EnclosingName: "userById", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindString},
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
				{ArgumentName: "order", EnclosingName: "field", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindEnum},
			},
		},
		{
			name:      "inline null argument",
			operation: `query { field(flag: null) }`,
			expected: []InlineArgument{
				{ArgumentName: "flag", EnclosingName: "field", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindNull},
			},
		},
		{
			name:      "inline list argument recorded once",
			operation: `query { field(by: [1, 2, 3]) }`,
			expected: []InlineArgument{
				{ArgumentName: "by", EnclosingName: "field", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindList},
			},
		},
		{
			name:      "inline object argument recorded once",
			operation: `query { field(obj: { a: 1 }) }`,
			expected: []InlineArgument{
				{ArgumentName: "obj", EnclosingName: "field", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindObject},
			},
		},
		{
			name:      "mixed variable and literal flags only the literal",
			operation: `query q($flag: Boolean) { field(flag: $flag, order: DESC) }`,
			expected: []InlineArgument{
				{ArgumentName: "order", EnclosingName: "field", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindEnum},
			},
		},
		{
			name:      "inline directive argument (@include)",
			operation: `query q($userId: ID!) { userById(userId: $userId) @include(if: true) { loginName } }`,
			expected: []InlineArgument{
				{ArgumentName: "if", EnclosingName: "include", EnclosingKind: ast.NodeKindDirective, ValueKind: ast.ValueKindBoolean},
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
				{ArgumentName: "name", EnclosingName: "__type", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindString},
			},
		},
		{
			// Proves detection runs before @skip/@include prunes the node: both the
			// directive's own `if` and the child field's `first` are reported even
			// though normalization would delete the `user` selection.
			name:      "argument under a @skip(if:true)-removed node still flagged",
			operation: `query q($userId: ID!) { user @skip(if: true) { posts(first: 10) } userById(userId: $userId) { loginName } }`,
			expected: []InlineArgument{
				{ArgumentName: "if", EnclosingName: "skip", EnclosingKind: ast.NodeKindDirective, ValueKind: ast.ValueKindBoolean},
				{ArgumentName: "first", EnclosingName: "posts", EnclosingKind: ast.NodeKindField, ValueKind: ast.ValueKindInteger},
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
			validator, report := runInlineArgumentsRule(t, tt.operation, InlineArgumentsValidationOptions{Enforce: false})
			require.False(t, report.HasErrors(), "log-only mode must never error: %s", report.Error())

			if len(tt.expected) == 0 {
				assert.Empty(t, validator.Findings)
				return
			}

			require.Len(t, validator.Findings, len(tt.expected))
			got := make([]InlineArgument, len(validator.Findings))
			for i, f := range validator.Findings {
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
	validator, report := runInlineArgumentsRule(t, operation, InlineArgumentsValidationOptions{
		Enforce: false,
	})
	require.False(t, report.HasErrors())
	require.Len(t, validator.Findings, 1)

	pos := validator.Findings[0].Position
	assert.Equal(t, uint32(1), pos.LineStart)
	assert.Equal(t, uint32(30), pos.CharStart)
}

func TestInlineArgumentsRule_Enforce(t *testing.T) {
	t.Run("stops at the first inline argument and reports a typed error", func(t *testing.T) {
		validator, report := runInlineArgumentsRule(t,
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

		// Enforce rejects on the first inline argument and stops the walk, so no
		// findings are collected.
		assert.Empty(t, validator.Findings)
	})

	t.Run("names the offending argument in the message when ReturnInResponseExtensions is set", func(t *testing.T) {
		_, report := runInlineArgumentsRule(t,
			`query { userById(userId: "12345") { loginName } field(order: ASC) }`,
			InlineArgumentsValidationOptions{
				Enforce:                    true,
				ErrorMessage:               "Inline argument values are not allowed. Use variables instead.",
				ErrorCode:                  "INLINE_ARGUMENT_VALUES_NOT_ALLOWED",
				StatusCode:                 400,
				ReturnInResponseExtensions: true,
			},
		)

		require.True(t, report.HasErrors())
		require.Len(t, report.ExternalErrors, 1)
		// The first offending argument is named in the message; the walk still stops
		// there, so only that one is reported.
		assert.Equal(t,
			`Inline argument values are not allowed. Use variables instead. Inline value provided for argument "userById.userId".`,
			report.ExternalErrors[0].Message,
		)
	})

	t.Run("compliant operation passes enforce mode", func(t *testing.T) {
		validator, report := runInlineArgumentsRule(t,
			`query q($userId: ID!) { userById(userId: $userId) { loginName } }`,
			InlineArgumentsValidationOptions{Enforce: true, ErrorMessage: "nope", ErrorCode: "CODE", StatusCode: 400},
		)
		require.False(t, report.HasErrors(), "compliant operation must not error: %s", report.Error())
		assert.False(t, validator.HadInlineArguments())
	})

	t.Run("disabled validator records nothing", func(t *testing.T) {
		definitionDocument := unsafeparser.ParseGraphqlDocumentString(inlineArgumentsTestSchema)
		require.NoError(t, asttransform.MergeDefinitionWithBaseSchema(&definitionDocument))
		operationDocument := unsafeparser.ParseGraphqlDocumentString(`query { userById(userId: "12345") { loginName } }`)
		report := &operationreport.Report{}

		validator := &InlineArgumentsValidator{Options: InlineArgumentsValidationOptions{Enforce: true, ErrorMessage: "x", ErrorCode: "C", StatusCode: 400}}
		validator.Disabled = true

		normalizer := NewWithOpts(WithPrevalidationRules(InlineArgumentsRule(validator)))
		normalizer.NormalizeOperation(&operationDocument, &definitionDocument, report)

		require.False(t, report.HasErrors())
		assert.False(t, validator.HadInlineArguments())
	})
}
