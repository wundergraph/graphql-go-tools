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

	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/internal/unsafeprinter"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/testing/permutations"
)

type testOptions struct {
	postProcessors        []*postprocess.Processor
	skipReason            string
	withFieldInfo         bool
	withPrintPlan         bool
	withFieldDependencies bool
	withFetchReasons      bool
	withEntityCaching     bool
	withFetchProvidesData bool
	withCacheKeyTemplates bool
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

func WithFieldDependencies() func(*testOptions) {
	return func(o *testOptions) {
		o.withFieldInfo = true
		o.withFieldDependencies = true
	}
}

func WithFetchReasons() func(*testOptions) {
	return func(o *testOptions) {
		o.withFieldInfo = true
		o.withFieldDependencies = true
		o.withFetchReasons = true
	}
}

func WithEntityCaching() func(*testOptions) {
	return func(o *testOptions) {
		o.withFieldInfo = true
		o.withFieldDependencies = true
		o.withEntityCaching = true
	}
}

func WithFetchProvidesData() func(*testOptions) {
	return func(o *testOptions) {
		o.withFieldInfo = true
		o.withFieldDependencies = true
		o.withFetchProvidesData = true
	}
}

func WithCacheKeyTemplates() func(*testOptions) {
	return func(o *testOptions) {
		o.withCacheKeyTemplates = true
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
		config.DisableIncludeFieldDependencies = true
		config.DisableEntityCaching = true
		config.DisableFetchProvidesData = true

		opts := &testOptions{}
		for _, o := range options {
			o(opts)
		}

		if opts.withFieldInfo {
			config.DisableIncludeInfo = false
		}

		if opts.withFieldDependencies {
			config.DisableIncludeFieldDependencies = false
		}

		if opts.withFetchReasons {
			config.BuildFetchReasons = true
		}

		if opts.withEntityCaching {
			config.DisableEntityCaching = false
		}

		if opts.withFetchProvidesData {
			config.DisableFetchProvidesData = false
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

		// Clear CacheKeyTemplate from actual plan by default since most tests don't need
		// to verify the internal cache key template structure. Tests that need to verify
		// caching behavior should use WithCacheKeyTemplates() to opt in.
		if !opts.withCacheKeyTemplates {
			clearCacheKeyTemplates(actualPlan)
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

// clearCacheKeyTemplates recursively clears CacheKeyTemplate from all fetches in the plan.
// This is called by default so tests don't need to specify the internal cache key template structure.
// Use WithCacheKeyTemplates() to opt in to including cache key templates in tests.
func clearCacheKeyTemplates(p plan.Plan) {
	switch pl := p.(type) {
	case *plan.SynchronousResponsePlan:
		if pl.Response != nil {
			if pl.Response.Fetches != nil {
				clearCacheKeyTemplatesFromFetchTree(pl.Response.Fetches)
			}
			// Also clear from RawFetches (pre-postprocessed fetch items)
			for _, item := range pl.Response.RawFetches {
				if item != nil && item.Fetch != nil {
					clearCacheKeyTemplateFromFetch(item.Fetch)
				}
			}
		}
	case *plan.SubscriptionResponsePlan:
		if pl.Response != nil && pl.Response.Response != nil {
			if pl.Response.Response.Fetches != nil {
				clearCacheKeyTemplatesFromFetchTree(pl.Response.Response.Fetches)
			}
			// Also clear from RawFetches
			for _, item := range pl.Response.Response.RawFetches {
				if item != nil && item.Fetch != nil {
					clearCacheKeyTemplateFromFetch(item.Fetch)
				}
			}
		}
	}
}

func clearCacheKeyTemplatesFromFetchTree(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}

	// Clear from this node's fetch
	if node.Item != nil && node.Item.Fetch != nil {
		clearCacheKeyTemplateFromFetch(node.Item.Fetch)
	}

	// Clear from trigger
	if node.Trigger != nil {
		clearCacheKeyTemplatesFromFetchTree(node.Trigger)
	}

	// Clear from children
	for _, child := range node.ChildNodes {
		clearCacheKeyTemplatesFromFetchTree(child)
	}
}

func clearCacheKeyTemplateFromFetch(f resolve.Fetch) {
	switch fetch := f.(type) {
	case *resolve.SingleFetch:
		fetch.FetchConfiguration.Caching.CacheKeyTemplate = nil
		fetch.FetchConfiguration.Caching.RootFieldL1EntityCacheKeyTemplates = nil
		// Clear UseL1Cache to avoid test failures when comparing expected vs actual
		// since the planner now defaults to true but most tests expect false (zero value)
		fetch.FetchConfiguration.Caching.UseL1Cache = false
	}
}
