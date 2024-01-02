package datasourcetesting

import (
	"context"
	"encoding/json"
	"math/rand"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type testOptions struct {
	postProcessors []postprocess.PostProcessor
}

func WithPostProcessors(postProcessors ...postprocess.PostProcessor) func(*testOptions) {
	return func(o *testOptions) {
		o.postProcessors = postProcessors
	}
}

func WithMultiFetchPostProcessor() func(*testOptions) {
	return WithPostProcessors(&postprocess.CreateMultiFetchTypes{})
}

func RunTest(definition, operation, operationName string, expectedPlan plan.Plan, config plan.Configuration, options ...func(*testOptions)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		opts := &testOptions{}
		for _, o := range options {
			o(opts)
		}

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			t.Fatal(err)
		}
		norm := astnormalization.NewNormalizer(true, true)
		var report operationreport.Report
		norm.NormalizeOperation(&op, &def, &report)
		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, &report)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		p := plan.NewPlanner(ctx, config)
		actualPlan := p.Plan(&op, &def, operationName, &report)
		if report.HasErrors() {
			_, err := astprinter.PrintStringIndent(&def, nil, "  ")
			if err != nil {
				t.Fatal(err)
			}
			_, err = astprinter.PrintStringIndent(&op, &def, "  ")
			if err != nil {
				t.Fatal(err)
			}
			t.Fatal(report.Error())
		}

		if opts.postProcessors != nil {
			for _, pp := range opts.postProcessors {
				actualPlan = pp.Process(actualPlan)
			}
		}

		actualBytes, _ := json.MarshalIndent(actualPlan, "", "  ")
		expectedBytes, _ := json.MarshalIndent(expectedPlan, "", "  ")

		if string(expectedBytes) != string(actualBytes) {
			assert.Equal(t, expectedPlan, actualPlan)
			t.Error(cmp.Diff(string(expectedBytes), string(actualBytes)))
		}
	}
}

// ShuffleDS randomizes the order of the data sources
// to ensure that the order doesn't matter
func ShuffleDS(dataSources []plan.DataSourceConfiguration) []plan.DataSourceConfiguration {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(dataSources), func(i, j int) {
		dataSources[i], dataSources[j] = dataSources[j], dataSources[i]
	})

	return dataSources
}
