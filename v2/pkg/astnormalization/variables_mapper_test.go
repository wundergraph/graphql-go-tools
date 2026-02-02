package astnormalization

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

func TestVariablesMapper(t *testing.T) {
	definition := unsafeparser.ParseGraphqlDocumentStringWithBaseSchema(`
	  type Object {
		id: ID!
		name: String!
		echo(value: String!): String!
		echoTwo(value: String!): String!
		copy(input: InputObject!): Object!
	  }
	
	  input InputObject {
		id: ID!
		name: String!
	  }
	
	  type Query {
		object(id: ID!): Object
	  }
	
	  type Mutation {
		updateObject(name: String!, files: [Upload!]): Object!
		uploadFile(file: Upload!): Object!
	  }
	
	  type Subscription {
		subscribe(id: ID!): Object!
	  }`)

	variablesMapper := NewVariablesMapper()

	normalizer := NewWithOpts(
		WithRemoveNotMatchingOperationDefinitions(),
		WithInlineFragmentSpreads(),
		WithRemoveFragmentDefinitions(),
		WithRemoveUnusedVariables(),
	)
	variablesNormalizer := NewVariablesNormalizer(false)

	testCases := []struct {
		name             string
		input            string
		output           string
		variablesMapping map[string]string
	}{
		{
			name: "1.1 Simple external variable (query)",
			input: `
				query MyQuery($varOne: ID!) {
					object(id: $varOne) {
					  name
					}
				}`,
			output: `
				query MyQuery($a: ID!) {
					object(id: $a) { 
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "varOne",
			},
		},
		{
			name: "1.2 Simple external variable (mutation)",
			input: `
				mutation MyMutation($varOne: String!) {
					updateObject(name: $varOne) {
						name
					}
				}`,
			output: `
				mutation MyMutation($a: String!) {
					updateObject(name: $a) {
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "varOne",
			},
		},
		{
			name: "1.3 Simple external variable (subscription)",
			input: `
				subscription MySubscription($varOne: ID!) {
					subscribe(id: $varOne) {
						name
					}
				}`,
			output: `
				subscription MySubscription($a: ID!) {
					subscribe(id: $a) {
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "varOne",
			},
		},
		{
			name: "2 Simple inline variable",
			input: `
				query MyQuery {
					object(id: "abc123") {
						name
					}
				}`,
			output: `
				query MyQuery($a: ID!) {
					object(id: $a) { 
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
			},
		},
		{
			name: "3.1 Colliding external variable",
			input: `
				query MyQuery($a: ID!) {
					object(id: $a) {
						name
					}
				}`,
			output: `
				query MyQuery($a: ID!) {
					object(id: $a) { 
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
			},
		},
		{
			name: "3.2 Colliding external variable used in 2 places",
			input: `
				query MyQuery($a: String!) {
					echo(id: $a)
					echoTwo(id: $a)
				}`,
			output: `
				query MyQuery($a: String!) {
					echo(id: $a)
					echoTwo(id: $a)
				}`,
			variablesMapping: map[string]string{
				"a": "a",
			},
		},
		{
			name: "3.3 Colliding external variable along with inline values",
			input: `
				query MyQuery($a: String!) {
					object(id: 1) {
						echo(value: "Hello World")
						echoTwo(value: $a)
					}
				}`,
			output: `
				query MyQuery($a: ID!, $b: String!, $c: String!) {
					object(id: $a) {
						echo(value: $b)
						echoTwo(value: $c)
					}
				}`,

			variablesMapping: map[string]string{
				"a": "b",
				"b": "c",
				"c": "a",
			},
		},
		{
			name: "3.4 Colliding external variables",
			input: `
				query MyQuery($b: String!, $e: String! $c: ID!) {
					object(id: $c) {
						echo(value: $e)
						echoTwo(value: $b)
					}
				}`,
			output: `
				query MyQuery($a: ID!, $b: String!, $c: String!) {
					object(id: $a) {
						echo(value: $b)
						echoTwo(value: $c)
					}
				}`,

			variablesMapping: map[string]string{
				"a": "c",
				"b": "e",
				"c": "b",
			},
		},
		{
			name: "3.5 all inline values",
			input: `
				query MyQuery {
					object(id: 1) {
						echo(value: "Hello")
						echoTwo(value: "World")
					}
				}`,
			output: `
				query MyQuery($a: ID!, $b: String!, $c: String!){
					object(id: $a) {
						echo(value: $b)
						echoTwo(value: $c)
					}
				}`,

			variablesMapping: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
			},
		},
		{
			name: "4 Inline variable and external variable",
			input: `
				query MyQuery($varOne: ID!) {
					object(id: $varOne) {
						name
						echo(value: "Hello World!")
					}
				}`,
			output: `
				query MyQuery($a: ID! $b: String!) {
					object(id: $a) {
						name
						echo(value: $b)
					}
				}`,

			variablesMapping: map[string]string{
				"a": "varOne",
				"b": "a",
			},
		},
		{
			name: "5.1 Multiple external variables",
			input: `
				query MyQuery($varOne: ID! $varTwo: String!) {
					object(id: $varOne) {
						name
						echo(value: $varTwo)
					}
				}`,
			output: `
				query MyQuery($a: ID! $b: String!) {
					object(id: $a) {
						name
						echo(value: $b)
					}
				}`,
			variablesMapping: map[string]string{
				"a": "varOne",
				"b": "varTwo",
			},
		},
		{
			name: "6 Multiple colliding external variables",
			input: `
				query MyQuery($a: String! $b: ID!) {
					object(id: $b) {
						echo(value: $a)
						name
					}
				}`,
			output: `
				query MyQuery($a: ID! $b: String!) {
					object(id: $a) {
						echo(value: $b)
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "b",
				"b": "a",
			},
		},
		{
			name: "7 multiple inline variables",
			input: `
				query MyQuery {
					object(id: "abc123") {
						echo(value: "Hello World!")
						name
					}
				}`,
			output: `
				query MyQuery($a: ID! $b: String!) {
					object(id: $a) {
						echo(value: $b)
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "8 Inline variable with multiple colliding external variables",
			input: `
				query MyQuery($a: ID! $b: String! ) {
					object(id: $a) {
						name
						copy(input: { id: "abc123", name: "MyObject"}),
						echo(value: $b)
					}
				}`,
			output: `
				query MyQuery($a: ID! $b: InputObject! $c: String!) {
					object(id: $a) {
						name
						copy(input: $b),
						echo(value: $c)
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
				"b": "c",
				"c": "b",
			},
		},
		{
			name: "9 Inline variable with multiple colliding external variables",
			input: `
				query MyQuery($a: ID! $b: String! $c: String! ) {
					object(id: $a) {
						name
						copy(input: { id: "abc123", name: $b})
						echo(value: $c)
					}
				}`,
			output: `
				query MyQuery($a: ID! $b: InputObject! $c: String! ) {
					object(id: $a) {
						name
						copy(input: $b)
						echo(value: $c)
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
				"b": "d",
				"c": "c",
			},
		},
		{
			name: "10 Colliding external variable with multiple inline variables",
			input: `
			  query MyQuery($a: ID!) {
			    object(id: $a) {
			      name
				  copy(input: { id: "abc123", name: "MyObject" })
				  echo(value: "Hello World")
			    }
			  }`,
			output: `
				query MyQuery($a: ID! $b: InputObject! $c: String! ) {
					object(id: $a) {
						name
						copy(input: $b)
						echo(value: $c)
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
				"b": "b",
				"c": "c",
			},
		},
		{
			name: "11 Reused external variable",
			input: `
				query MyQuery($varOne: String!) {
					object(id: 1) {
						echo(value: $varOne)
						echoTwo(value: $varOne)
					}
				}`,
			output: `
				query MyQuery($a: ID!, $b: String!) {
					object(id: $a) {
						echo(value: $b)
						echoTwo(value: $b)
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
				"b": "varOne",
			},
		},
		{
			name: "12 Reused inline value",
			input: `
				query MyQuery {
					object(id: 1) {
						echo(value: 12)
						echoTwo(value: 12)
					}
				}`,
			output: `
				query MyQuery ($a: ID!, $b: String!) {
					object(id: $a) {
						echo(value: $b)
						echoTwo(value: $b)
					}
				}`,
			variablesMapping: map[string]string{
				"a": "a",
				"b": "b",
			},
		},
		{
			name: "13 Mutation with file uploads",
			input: `
				mutation MyMutation($varOne: String! $files: [Upload!]) {
					updateObject(name: $varOne, files: $files) {
						name
					}
				}`,
			output: `
				mutation MyMutation($a: String!, $files: [Upload!]) {
					updateObject(name: $a, files: $files) {
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "varOne",
			},
		},
		{
			name: "13 Mutation with file uploads - reverse order",
			input: `
				mutation MyMutation($files: [Upload!] $varOne: String!) {
					updateObject(name: $varOne, files: $files) {
						name
					}
				}`,
			output: `
				mutation MyMutation($a: String!, $files: [Upload!]) {
					updateObject(name: $a, files: $files) {
						name
					}
				}`,
			variablesMapping: map[string]string{
				"a": "varOne",
			},
		},
		{
			name: "13 Mutation with single file upload",
			input: `
				mutation MyMutation($file: Upload!) {
					uploadFile(files: $file) {
						name
					}
				}`,
			output: `
				mutation MyMutation($file: Upload!) {
					uploadFile(files: $file) {
						name
					}
				}`,
			variablesMapping: map[string]string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			operation := unsafeparser.ParseGraphqlDocumentString(tc.input)
			report := &operationreport.Report{}

			normalizer.NormalizeNamedOperation(&operation, &definition, operation.OperationDefinitionNameBytes(0), report)
			require.False(t, report.HasErrors())

			variablesNormalizer.NormalizeOperation(&operation, &definition, report)
			require.False(t, report.HasErrors())

			mapping := variablesMapper.NormalizeOperation(&operation, &definition, report)
			require.False(t, report.HasErrors())

			expectedOut := unsafeprinter.Prettify(tc.output)
			printedOperation := unsafeprinter.PrettyPrint(&operation)
			assert.Equal(t, expectedOut, printedOperation)
			assert.Equal(t, tc.variablesMapping, mapping)
		})
	}
}
