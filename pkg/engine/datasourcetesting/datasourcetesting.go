package datasourcetesting

import (
	"reflect"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/jensneuse/diffview"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafeparser"
	"github.com/jensneuse/graphql-go-tools/pkg/astnormalization"
	"github.com/jensneuse/graphql-go-tools/pkg/asttransform"
	"github.com/jensneuse/graphql-go-tools/pkg/astvalidation"
	"github.com/jensneuse/graphql-go-tools/pkg/engine/plan"
	"github.com/jensneuse/graphql-go-tools/pkg/operationreport"
)

type Option func(config *plan.Configuration, planner *plan.Planner)

func WithDataSource(dataSource plan.DataSourceConfiguration) Option {
	return func(config *plan.Configuration, planner *plan.Planner) {
		config.DataSources = append(config.DataSources, dataSource)
	}
}

func WithPlanner(dataSourcePlanner plan.DataSourcePlanner) Option {
	return func(config *plan.Configuration, planner *plan.Planner) {
		planner.RegisterDataSourcePlanner(dataSourcePlanner)
	}
}

func RunTest(definition, operation, operationName string, expectedPlan plan.Plan, option ...Option) func(t *testing.T) {
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
		config := plan.Configuration{
			DataSources: []plan.DataSourceConfiguration{},
		}
		p := plan.NewPlanner(&def, config)
		for i := range option {
			option[i](&config, p)
		}
		actualPlan := p.Plan(&op, []byte(operationName), &report)
		if report.HasErrors() {
			t.Fatal(report.Error())
		}
		if !reflect.DeepEqual(expectedPlan, actualPlan) {
			diffview.NewGoland().DiffViewAny("diff", expectedPlan, actualPlan)
			t.Errorf("want:\n%s\ngot:\n%s\n", spew.Sdump(expectedPlan), spew.Sdump(actualPlan))
		}
	}
}
