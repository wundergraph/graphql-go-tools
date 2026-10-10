package astnormalization_test

import (
	"testing"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// AB is a strict subset of ABC, so GetPossibleTypes(ABC) and GetPossibleTypes(AB)
// intersect on {A, B}. Per spec 5.5.2.3 a fragment on AB may be spread into a
// field of type ABC.
const testUnionDefinition = `
type A { a: String }
type B { b: String }
type C { c: String }

union AB  = A | B
union ABC = A | B | C

type Query {
  wide:   ABC!
  narrow: AB!
}
`

// Case 1: the reduced form. Spread sits directly in the operation.
const opSubsetIntoSuperset = `
query Q {
  wide {
    ...narrowFragment
  }
}

fragment narrowFragment on AB {
  ... on A { a }
  ... on B { b }
}
`

// Case 2: partial overlap in the other direction. ABC and AB share {A, B};
// the "... on C" branch is unreachable but the spread is still legal.
const opSupersetIntoSubset = `
query Q {
  narrow {
    ...wideFragment
  }
}

fragment wideFragment on ABC {
  ... on A { a }
  ... on C { c }
}
`

// Case 3: the shape as it appears in a real client. The outer fragment's type
// condition matches its field exactly, so it inlines; the inner union-into-union
// spread is the one that survives.
const opNestedThroughOuterFragment = `
query Q {
  wide {
    ...outer
  }
}

fragment outer on ABC {
  ...narrowFragment
}

fragment narrowFragment on AB {
  ... on A { a }
  ... on B { b }
}
`

// Control: identical union. This one passes today.
const opIdenticalUnion = `
query Q {
  wide {
    ...sameFragment
  }
}

fragment sameFragment on ABC {
  ... on A { a }
}
`

func TestUnionFragmentSpreadIntoDifferentUnion(t *testing.T) {
	for _, tc := range []struct {
		name      string
		operation string
	}{
		{"subset into superset", opSubsetIntoSuperset},
		{"superset into subset", opSupersetIntoSubset},
		{"nested through an outer fragment", opNestedThroughOuterFragment},
		{"control: identical union", opIdenticalUnion},
	} {
		t.Run(tc.name, func(t *testing.T) {
			def, rep := astparser.ParseGraphqlDocumentString(testUnionDefinition)
			if rep.HasErrors() {
				t.Fatalf("parsing definition: %s", rep.Error())
			}
			if err := asttransform.MergeDefinitionWithBaseSchema(&def); err != nil {
				t.Fatalf("merging base schema: %s", err)
			}

			op, rep := astparser.ParseGraphqlDocumentString(tc.operation)
			if rep.HasErrors() {
				t.Fatalf("parsing operation: %s", rep.Error())
			}

			var report operationreport.Report
			astnormalization.NormalizeNamedOperation(&op, &def, []byte("Q"), &report)
			if report.HasErrors() {
				t.Fatalf("normalization reported: %s", report.Error())
			}

			astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
			if report.HasErrors() {
				t.Fatalf("validation reported: %s", report.Error())
			}
		})
	}
}
