package datasourcetesting

import (
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/kylelemons/godebug/diff"
	"github.com/kylelemons/godebug/pretty"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/permutations"

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
	postProcessors []*postprocess.Processor
	skipReason     string
	withFieldInfo  bool
	withPrintPlan  bool
}

func WithPostProcessors(postProcessors ...*postprocess.Processor) func(*testOptions) {
	return func(o *testOptions) {
		o.postProcessors = postProcessors
	}
}

func WithSkipReason(reason string) func(*testOptions) {
	return func(o *testOptions) {
		o.skipReason = reason
	}
}

func WithDefaultPostProcessor() func(*testOptions) {
	return WithPostProcessors(postprocess.NewProcessor(postprocess.DisableResolveInputTemplates(), postprocess.DisableCreateConcreteSingleFetchTypes(), postprocess.DisableCreateParallelNodes(), postprocess.DisableMergeFields()))
}

func WithDefaultCustomPostProcessor(options ...postprocess.ProcessorOption) func(*testOptions) {
	return WithPostProcessors(postprocess.NewProcessor(options...))
}

func WithFieldInfo() func(*testOptions) {
	return func(o *testOptions) {
		o.withFieldInfo = true
	}
}

func WithPrintPlan() func(*testOptions) {
	return func(o *testOptions) {
		o.withPrintPlan = true
		o.withFieldInfo = true
	}
}

func RunWithPermutations(t *testing.T, definition, operation, operationName string, expectedPlan plan.Plan, config plan.Configuration, options ...func(*testOptions)) {
	t.Helper()

	dataSourcePermutations := permutations.Generate(config.DataSources)

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
	dataSourcePermutations := permutations.Generate(config.DataSources)

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
	return RunTestWithVariables(definition, operation, operationName, "", expectedPlan, config, options...)
}

func RunTestWithVariables(definition, operation, operationName, variables string, expectedPlan plan.Plan, config plan.Configuration, options ...func(*testOptions)) func(t *testing.T) {
	return func(t *testing.T) {
		t.Helper()

		// by default, we don't want to have field info in the tests because it's too verbose
		config.DisableIncludeInfo = true

		opts := &testOptions{}
		for _, o := range options {
			o(opts)
		}

		if opts.withFieldInfo {
			config.DisableIncludeInfo = false
		}

		if opts.skipReason != "" {
			t.Skip(opts.skipReason)
		}

		def := unsafeparser.ParseGraphqlDocumentString(definition)
		op := unsafeparser.ParseGraphqlDocumentString(operation)
		if variables != "" {
			op.Input.Variables = []byte(variables)
		}
		err := asttransform.MergeDefinitionWithBaseSchema(&def)
		if err != nil {
			t.Fatal(err)
		}
		norm := astnormalization.NewWithOpts(astnormalization.WithExtractVariables(), astnormalization.WithInlineFragmentSpreads(), astnormalization.WithRemoveFragmentDefinitions(), astnormalization.WithRemoveUnusedVariables())
		var report operationreport.Report
		norm.NormalizeOperation(&op, &def, &report)

		normalized := unsafeprinter.PrettyPrint(&op)
		_ = normalized

		valid := astvalidation.DefaultOperationValidator()
		valid.Validate(&op, &def, &report)

		p, err := plan.NewPlanner(config)
		require.NoError(t, err)

		var planOpts []plan.Opts
		if opts.withPrintPlan {
			planOpts = append(planOpts, plan.IncludeQueryPlanInResponse())
		}

		actualPlan := p.Plan(&op, &def, operationName, &report, planOpts...)
		if report.HasErrors() {
			_, err := astprinter.PrintStringIndent(&def, "  ")
			if err != nil {
				t.Fatal(err)
			}
			_, err = astprinter.PrintStringIndent(&op, "  ")
			if err != nil {
				t.Fatal(err)
			}
			t.Fatal(report.Error())
		}

		if opts.postProcessors != nil {
			for _, processor := range opts.postProcessors {
				processor.Process(actualPlan)
			}
		}

		if opts.withPrintPlan {
			t.Log("\n", actualPlan.(*plan.SynchronousResponsePlan).Response.Fetches.QueryPlan().PrettyPrint())
		}

		formatterConfig := map[reflect.Type]interface{}{
			// normalize byte slices to strings
			reflect.TypeOf([]byte{}): func(b []byte) string { return fmt.Sprintf(`"%s"`, string(b)) },
			// normalize map[string]struct{} to json array of keys
			reflect.TypeOf(map[string]struct{}{}): func(m map[string]struct{}) string {
				var keys []string
				for k := range m {
					keys = append(keys, k)
				}
				slices.Sort(keys)

				keysPrinted, _ := json.Marshal(keys)
				return string(keysPrinted)
			},
			reflect.TypeOf(resolve.SkipArrayItem(func(ctx *resolve.Context, arrayItem *astjson.Value) bool { return false })): func(resolve.SkipArrayItem) string { return "skip_function" },
		}

		prettyCfg := &pretty.Config{
			Diffable:          true,
			IncludeUnexported: false,
			Formatter:         formatterConfig,
		}

		exp := prettyCfg.Sprint(expectedPlan)
		act := prettyCfg.Sprint(actualPlan)

		if !assert.Equal(t, exp, act) {
			if diffResult := diff.Diff(exp, act); diffResult != "" {
				t.Errorf("Plan does not match(-want +got)\n%s", diffResult)
			}
		}
	}
}
