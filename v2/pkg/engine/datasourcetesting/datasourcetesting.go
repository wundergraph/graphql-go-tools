package datasourcetesting

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"time"

	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gonum.org/v1/gonum/stat/combin"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

type testOptions struct {
	postProcessors            []postprocess.PostProcessor
	postProcessExtractFetches bool
	skipReason                string
}

func WithPostProcessors(postProcessors ...postprocess.PostProcessor) func(*testOptions) {
	return func(o *testOptions) {
		o.postProcessors = postProcessors
	}
}

func WithPostProcessExtractFetches() func(*testOptions) {
	return func(o *testOptions) {
		o.postProcessExtractFetches = true
	}
}

func WithSkipReason(reason string) func(*testOptions) {
	return func(o *testOptions) {
		o.skipReason = reason
	}
}

func WithMultiFetchPostProcessor() func(*testOptions) {
	return WithPostProcessors(&postprocess.CreateMultiFetchTypes{})
}

func RunWithPermutations(t *testing.T, definition, operation, operationName string, expectedPlan plan.Plan, config plan.Configuration, options ...func(*testOptions)) {
	t.Helper()

	dataSourcePermutations := DataSourcePermutations(config.DataSources)

	for i := range dataSourcePermutations {
		permutation := dataSourcePermutations[i]
		t.Run(fmt.Sprintf("permutation %v", permutation.Order), func(t *testing.T) {
			permutationPlanConfiguration := config
			permutationPlanConfiguration.DataSources = permutation.DataSources

			t.Run("run", RunTest(
				definition,
				operation,
				operationName,
				expectedPlan,
				permutationPlanConfiguration,
				options...,
			))
		})
	}
}

func RunWithPermutationsVariants(t *testing.T, definition, operation, operationName string, expectedPlans []plan.Plan, config plan.Configuration, options ...func(*testOptions)) {
	dataSourcePermutations := DataSourcePermutations(config.DataSources)

	if len(dataSourcePermutations) != len(expectedPlans) {
		t.Fatalf("expected %d plan variants, got %d", len(dataSourcePermutations), len(expectedPlans))
	}

	for i := range dataSourcePermutations {
		permutation := dataSourcePermutations[i]
		t.Run(fmt.Sprintf("permutation %v", permutation.Order), func(t *testing.T) {
			permutationPlanConfiguration := config
			permutationPlanConfiguration.DataSources = permutation.DataSources

			t.Run("run", RunTest(
				definition,
				operation,
				operationName,
				expectedPlans[i],
				permutationPlanConfiguration,
				options...,
			))
		})
	}
}

func RunTest(definition, operation, operationName string, expectedPlan plan.Plan, config plan.Configuration, options ...func(*testOptions)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		opts := &testOptions{}
		for _, o := range options {
			o(opts)
		}

		if opts.skipReason != "" {
			t.Skip(opts.skipReason)
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

		p, err := plan.NewPlanner(config)
		require.NoError(t, err)
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
			postprocess.NewProcessor(opts.postProcessors, opts.postProcessExtractFetches).Process(actualPlan)
		}

		actualBytes, _ := json.MarshalIndent(actualPlan, "", "  ")
		expectedBytes, _ := json.MarshalIndent(expectedPlan, "", "  ")

		if !assert.Equal(t, string(expectedBytes), string(actualBytes)) {
			formatterConfig := map[reflect.Type]interface{}{
				reflect.TypeOf([]byte{}): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
			}

			prettyCfg := &pretty.Config{
				Diffable:          true,
				IncludeUnexported: false,
				Formatter:         formatterConfig,
			}

			if diff := prettyCfg.Compare(expectedPlan, actualPlan); diff != "" {
				t.Errorf("Plan does not match(-want +got)\n%s", diff)
			}
		}
	}
}

// ShuffleDS randomizes the order of the data sources
// to ensure that the order doesn't matter
func ShuffleDS(dataSources []plan.DataSource) []plan.DataSource {
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	rnd.Shuffle(len(dataSources), func(i, j int) {
		dataSources[i], dataSources[j] = dataSources[j], dataSources[i]
	})

	return dataSources
}

func OrderDS(dataSources []plan.DataSource, order []int) (out []plan.DataSource) {
	out = make([]plan.DataSource, 0, len(dataSources))

	for _, i := range order {
		out = append(out, dataSources[i])
	}

	return out
}

func DataSourcePermutations(dataSources []plan.DataSource) []*Permutation {
	size := len(dataSources)
	elementsCount := len(dataSources)
	list := combin.Permutations(size, elementsCount)

	permutations := make([]*Permutation, 0, len(list))

	for _, v := range list {
		permutations = append(permutations, &Permutation{
			Order:       v,
			DataSources: OrderDS(dataSources, v),
		})
	}

	return permutations
}

type Permutation struct {
	Order       []int
	DataSources []plan.DataSource
}
