package datasourcetesting

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/astprinter"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

func RunTest(definition, operation, operationName string, expectedPlan plan.Plan, config plan.Configuration) func(t *testing.T) {
	return func(t *testing.T) {
		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			t.Fatal(err)
		}
		norm := astnormalization.NewNormalizer(true)
		var report operationreport.Report
		norm.NormalizeOperation(&op, &def, &report)
		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, &report)
		p := plan.NewPlanner(&def, config)
		actualPlan := p.Plan(&op, []byte(operationName), &report)
		if report.HasErrors() {
			printedDoc,err := astprinter.PrintStringIndent(&def,nil,"  ")
			if err != nil {
				t.Fatal(err)
			}
			_ = printedDoc
			printedOp,err := astprinter.PrintStringIndent(&op,&def,"  ")
			if err != nil {
				t.Fatal(err)
			}
			_ = printedOp
			t.Fatal(report.Error())
		}
		assert.Equal(t,expectedPlan,actualPlan)
	}
}
