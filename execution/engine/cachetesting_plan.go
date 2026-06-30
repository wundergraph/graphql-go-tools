package engine

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/sebdah/goldie/v2"
	"github.com/stretchr/testify/require"
	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
	"google.golang.org/protobuf/encoding/protojson"
)

type PlanResult struct {
	Response *resolve.GraphQLResponse
	Fakes    *cachetesting.FakeRegistry
}

func Plan(tb testing.TB, stage cachetesting.CacheStage, query string, responses map[string]string) PlanResult {
	tb.Helper()

	data, err := os.ReadFile("testdata/cache_commerce/config.json")
	require.NoError(tb, err)

	var rc nodev1.RouterConfig
	require.NoError(tb, protojson.Unmarshal(data, &rc))

	f := NewFederationEngineConfigFactory(tb.Context())
	conf, err := f.BuildEngineConfiguration(&rc)
	require.NoError(tb, err)
	cfg := conf.PlannerConfig()

	def, parseReport := astparser.ParseGraphqlDocumentString(rc.EngineConfig.GraphqlSchema)
	require.False(tb, parseReport.HasErrors(), parseReport.Error())
	op, parseReport := astparser.ParseGraphqlDocumentString(query)
	require.False(tb, parseReport.HasErrors(), parseReport.Error())
	require.NoError(tb, asttransform.MergeDefinitionWithBaseSchema(&def))

	norm := astnormalization.NewWithOpts(
		astnormalization.WithExtractVariables(),
		astnormalization.WithInlineFragmentSpreads(),
		astnormalization.WithRemoveFragmentDefinitions(),
		astnormalization.WithRemoveUnusedVariables(),
		astnormalization.WithEnableDefer(),
	)
	var report operationreport.Report
	norm.NormalizeOperation(&op, &def, &report)
	astvalidation.DefaultOperationValidator().Validate(&op, &def, &report)
	require.False(tb, report.HasErrors(), report.Error())

	cfg.CacheConfigProviders = cacheProvidersForStage(cfg, stage)
	planner, err := plan.NewPlanner(cfg)
	require.NoError(tb, err)
	raw := planner.Plan(&op, &def, "", &report)
	require.False(tb, report.HasErrors(), report.Error())

	proc := postprocess.NewProcessor(
		postprocess.EnableCaching(cacheProvidersForStage(cfg, stage), federationByDS(cfg), &def),
	)
	proc.Process(raw)

	resp := planResponse(tb, raw)
	t, ok := tb.(*testing.T)
	require.True(tb, ok, "Plan goldens require *testing.T")
	goldie.New(t, goldie.WithNameSuffix(".golden")).Assert(t, tb.Name(), []byte(renderPlanWithCache(resp)))

	fakes := cachetesting.NewFakeRegistry(responses)
	cachetesting.SwapDataSources(resp.Fetches, fakes)
	return PlanResult{Response: resp, Fakes: fakes}
}

func cacheProvidersForStage(cfg plan.Configuration, stage cachetesting.CacheStage) map[string]cacheconfig.CacheConfigProvider {
	// TODO(A1+): real per-stage providers.
	return map[string]cacheconfig.CacheConfigProvider{}
}

func federationByDS(cfg plan.Configuration) map[string]plan.FederationMetaData {
	out := make(map[string]plan.FederationMetaData, len(cfg.DataSources))
	for _, ds := range cfg.DataSources {
		out[ds.Id()] = ds.FederationConfiguration()
	}
	return out
}

func planResponse(tb testing.TB, raw plan.Plan) *resolve.GraphQLResponse {
	tb.Helper()

	switch p := raw.(type) {
	case *plan.SynchronousResponsePlan:
		return p.Response
	case *plan.DeferResponsePlan:
		return p.Response.Response
	case *plan.SubscriptionResponsePlan:
		return p.Response.Response
	default:
		tb.Fatalf("unsupported plan type %T", raw)
		return nil
	}
}

func renderPlanWithCache(resp *resolve.GraphQLResponse) string {
	var b strings.Builder
	if resp == nil || resp.Fetches == nil {
		return "<nil>\n"
	}
	b.WriteString(resp.Fetches.QueryPlan().PrettyPrint())
	b.WriteString("\n\nFetch cache configs:\n")
	renderFetchCache(&b, resp.Fetches)
	return b.String()
}

func renderFetchCache(b *strings.Builder, node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	if node.Item != nil {
		cfg := fetchCacheConfig(node.Item.Fetch)
		fmt.Fprintf(b, "- path:%q cache:%s keySpec:%s\n", node.Item.ResponsePath, cfg.String(), renderKeySpec(cfg))
	}
	for _, child := range node.ChildNodes {
		renderFetchCache(b, child)
	}
}

func fetchCacheConfig(fetch resolve.Fetch) *resolve.FetchCacheConfig {
	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		return f.Cache
	case *resolve.EntityFetch:
		return f.Cache
	case *resolve.BatchEntityFetch:
		return f.Cache
	default:
		return nil
	}
}

func renderKeySpec(cfg *resolve.FetchCacheConfig) string {
	if cfg == nil {
		return "<nil>"
	}
	parts := make([]string, 0, len(cfg.KeySpec.EntityKeyMappings))
	for _, mapping := range cfg.KeySpec.EntityKeyMappings {
		parts = append(parts, fmt.Sprintf("%s:%d", mapping.EntityTypeName, len(mapping.FieldMappings)))
	}
	return fmt.Sprintf(
		"{scope:%s type:%q field:%q candidates:%d mappings:[%s]}",
		cfg.KeySpec.Scope,
		cfg.KeySpec.TypeName,
		cfg.KeySpec.FieldName,
		len(cfg.KeySpec.Candidates),
		strings.Join(parts, ","),
	)
}
