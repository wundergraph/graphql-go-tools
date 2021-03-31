package federation

import (
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestRemoveFieldDirective(t *testing.T) {
	cases := []struct{
		operationDef string
		directives []string
		expected string
		name string
	} {
		{
			name: "simple directive",
			operationDef: `
				type Dog {
					name: String @notForDelete
					favoriteToy: String @forDelete
					barkVolume: Int
					isHousetrained(atOtherHomes: Boolean): Boolean! @forDelete
					doesKnowCommand(dogCommand: DogCommand!): Boolean!
				}
			`,
			directives: []string{"forDelete"},
			expected: `
				type Dog {
					name: String @notForDelete	
					barkVolume: Int
					doesKnowCommand(dogCommand: DogCommand!): Boolean!
				}
			`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			report := operationreport.Report{}
			walker := astvisitor.NewWalker(48)

			operationDocument := unsafeparser.ParseGraphqlDocumentString(tc.operationDef)
			definition :=  ast.NewDocument()
			if err := asttransform.MergeDefinitionWithBaseSchema(definition); err != nil {
				t.Fatal(err)
			}

			expectedOutputDocument := unsafeparser.ParseGraphqlDocumentString(tc.expected)

			removeDirective(tc.directives...)(&walker)

			walker.Walk(&operationDocument, definition, &report)

			if report.HasErrors() {
				t.Fatalf(report.Error())
			}

			got := mustString(astprinter.PrintString(&operationDocument, nil))
			want := mustString(astprinter.PrintString(&expectedOutputDocument, nil))

			assert.Equal(t, want, got)
		})
	}
}

var mustString = func(str string, err error) string {
	if err != nil {
		panic(err)
	}
	return str
}