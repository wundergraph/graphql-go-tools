package responsejsonschema

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

const cycleTestEnvironment = "GGT_RESPONSE_JSON_SCHEMA_CYCLE_CASE"

func TestBuildResponseSchema_RejectsSelectionAndSchemaCyclesWithoutOverflow(t *testing.T) {
	cycleCases := []struct {
		name   string
		mutate func(operation, definition *ast.Document)
	}{
		{
			name: "inline fragment self selection set",
			mutate: func(operation, _ *ast.Document) {
				nodeSelectionSet := operation.Fields[operationFieldRef(t, operation, "node")].SelectionSet
				inlineSelectionRef := operationSelectionRef(t, operation, nodeSelectionSet, ast.SelectionKindInlineFragment, "")
				inlineFragment := operation.Selections[inlineSelectionRef].Ref
				operation.SelectionSets[operation.InlineFragments[inlineFragment].SelectionSet].SelectionRefs = []int{inlineSelectionRef}
			},
		},
		{
			name: "inline fragment ancestor selection set",
			mutate: func(operation, _ *ast.Document) {
				nodeSelectionSet := operation.Fields[operationFieldRef(t, operation, "node")].SelectionSet
				operation.InlineFragments[0].SelectionSet = nodeSelectionSet
			},
		},
		{
			name: "field child selection set schema recursion",
			mutate: func(operation, definition *ast.Document) {
				nodeSelectionSet := operation.Fields[operationFieldRef(t, operation, "node")].SelectionSet
				operation.Fields[operationFieldRef(t, operation, "child")].SelectionSet = nodeSelectionSet
				definition.FieldDefinitions[definitionFieldRef(t, definition, "child")].Type =
					definition.FieldDefinitions[definitionFieldRef(t, definition, "node")].Type
			},
		},
	}

	selectedCase := os.Getenv(cycleTestEnvironment)
	if selectedCase != "" {
		for _, test := range cycleCases {
			if test.name != selectedCase {
				continue
			}
			operation := parseDocument(t, corruptASTOperation)
			definition := parseDocument(t, corruptASTDefinition)
			test.mutate(&operation, &definition)
			_, err := Build(&operation, &definition, []string{"entry"})
			require.ErrorContains(t, err, "selection set cycle")
			return
		}
		require.FailNow(t, "unknown cycle case", selectedCase)
	}

	for _, test := range cycleCases {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(
				os.Args[0],
				"-test.run=^TestBuildResponseSchema_RejectsSelectionAndSchemaCyclesWithoutOverflow$",
				"-test.timeout=5s",
			)
			command.Env = append(os.Environ(), cycleTestEnvironment+"="+test.name)
			output, err := command.CombinedOutput()
			require.NoError(t, err, "cycle subprocess failed or overflowed:\n%s", output)
		})
	}
}

func TestBuildResponseSchema_RejectsExcessiveAcyclicDepth(t *testing.T) {
	const deepChainLength = 300

	t.Run("selection and object type chain", func(t *testing.T) {
		definition := parseDocument(t, `type Query { root: Node! } type Node { next: Node!, value: String! }`)
		operationText := "query { root { " + strings.Repeat("next { ", deepChainLength) + "value" + strings.Repeat(" }", deepChainLength) + " } }"
		operation := parseDocument(t, operationText)

		var err error
		require.NotPanics(t, func() {
			_, err = Build(&operation, &definition, []string{"root"})
		})
		require.ErrorContains(t, err, "recursion depth limit")
	})

	t.Run("wrapped GraphQL type chain", func(t *testing.T) {
		wrappedType := strings.Repeat("[", deepChainLength) + "String" + strings.Repeat("]", deepChainLength)
		definition := parseDocument(t, fmt.Sprintf("type Query { value: %s }", wrappedType))
		operation := parseDocument(t, `query { value }`)

		var err error
		require.NotPanics(t, func() {
			_, err = Build(&operation, &definition, []string{"value"})
		})
		require.ErrorContains(t, err, "recursion depth limit")
	})
}

func TestBuildResponseSchema_AllowsSiblingSelectionSetReuse(t *testing.T) {
	definition := parseDocument(t, `
		type Query { root: Parent! }
		type Parent { child: Child! }
		type Child { name: String! }
	`)
	operation := parseDocument(t, `query { root { left: child { name } right: child { name } } }`)
	leftFieldRef := operationFieldRef(t, &operation, "child")
	rightFieldRef := ast.InvalidRef
	for fieldRef := leftFieldRef + 1; fieldRef < len(operation.Fields); fieldRef++ {
		if operation.FieldNameString(fieldRef) == "child" {
			rightFieldRef = fieldRef
			break
		}
	}
	require.NotEqual(t, ast.InvalidRef, rightFieldRef)
	operation.Fields[rightFieldRef].SelectionSet = operation.Fields[leftFieldRef].SelectionSet

	_, err := Build(&operation, &definition, []string{"root"})
	require.NoError(t, err)
}
