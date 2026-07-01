// Package cachingtesting is the plan-driven caching test harness: it loads the
// committed wgc-composed config.json of the dedicated caching subgraphs, runs
// the REAL v2 planner and postprocess over a query (with caching wired exactly
// as the engine Configuration wires it), and swaps every fetch's transport for
// an in-process fake. Tests then drive the public resolve entry points and
// assert complete responses — no hand-written plans, no golden files.
package cachingtesting

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"

	nodev1 "github.com/wundergraph/cosmo/router/gen/proto/wg/cosmo/node/v1"

	"github.com/wundergraph/graphql-go-tools/execution/engine"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astnormalization"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/asttransform"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvalidation"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/cache/cachetesting"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan/cacheconfig"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/postprocess"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

// PlanResult is the harness output: the postprocessed response (and defer
// response for defer plans) with all datasources swapped for fakes.
type PlanResult struct {
	Response      *resolve.GraphQLResponse
	DeferResponse *resolve.GraphQLDeferResponse
	Fakes         *cachetesting.FakeRegistry
}

// Plan runs the real planner + postprocess over query against the committed
// caching fixture config. caching is keyed by SUBGRAPH NAME (translated to
// datasource IDs internally); nil/empty leaves caching fully unconfigured (the
// no-op baseline). responses feed the fake datasources and may also be keyed
// by subgraph name (or name:responsePath, responsePath, "*").
func Plan(tb testing.TB, query string, caching map[string]cacheconfig.CachingConfiguration, responses map[string]string) PlanResult {
	tb.Helper()

	rc := routerConfig(tb)
	factory := engine.NewFederationEngineConfigFactory(tb.Context())
	conf, err := factory.BuildEngineConfiguration(rc)
	require.NoError(tb, err)
	cfg := conf.PlannerConfig()

	def, parseReport := astparser.ParseGraphqlDocumentString(rc.EngineConfig.GraphqlSchema)
	require.False(tb, parseReport.HasErrors(), "parse schema: %v", parseReport)
	require.NoError(tb, asttransform.MergeDefinitionWithBaseSchema(&def))
	op, parseReport := astparser.ParseGraphqlDocumentString(query)
	require.False(tb, parseReport.HasErrors(), "parse query: %v", parseReport)

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
	require.False(tb, report.HasErrors(), "normalize/validate: %s", report.Error())

	nameToID := subgraphNameToDatasourceID(rc)

	// Wire caching exactly as NewExecutionEngine does for SetCaching: providers
	// + federation keyed by datasource ID, FetchInfo force-enabled.
	var processorOptions []postprocess.ProcessorOption
	if len(caching) > 0 {
		providers := make(map[string]cacheconfig.CacheConfigProvider, len(caching))
		for name, cachingCfg := range caching {
			id, ok := nameToID[name]
			require.True(tb, ok, "caching configured for unknown subgraph %q", name)
			providers[id] = &cachingCfg
		}
		federation := make(map[string]plan.FederationMetaData, len(providers))
		for _, ds := range cfg.DataSources {
			if _, ok := providers[ds.Id()]; ok {
				federation[ds.Id()] = ds.FederationConfiguration()
			}
		}
		cfg.CacheConfigProviders = providers
		cfg.DisableIncludeInfo = false
		processorOptions = append(processorOptions, postprocess.EnableCaching(providers, federation, &def))
	}

	planner, err := plan.NewPlanner(cfg)
	require.NoError(tb, err)
	// Query plans are always included so tests can make full-value plan-shape
	// assertions on the rendered fetch trees (instead of golden files).
	raw := planner.Plan(&op, &def, "", &report, plan.IncludeQueryPlanInResponse())
	require.False(tb, report.HasErrors(), "plan: %s", report.Error())

	postprocess.NewProcessor(processorOptions...).Process(raw)

	result := PlanResult{
		Fakes: cachetesting.NewFakeRegistry(translateResponseKeys(responses, nameToID)),
	}
	switch p := raw.(type) {
	case *plan.SynchronousResponsePlan:
		result.Response = p.Response
	case *plan.DeferResponsePlan:
		result.Response = p.Response.Response
		result.DeferResponse = p.Response
	default:
		tb.Fatalf("unsupported plan type %T", raw)
	}

	cachetesting.SwapDataSources(result.Response.Fetches, result.Fakes)
	for _, group := range DeferGroups(result.DeferResponse) {
		cachetesting.SwapDataSources(group.Fetches, result.Fakes)
	}
	return result
}

// ResolveResponse drives the PUBLIC sync entry point over a harness plan with
// the given cache controller (nil = caching off) and returns the complete
// response body.
func ResolveResponse(tb testing.TB, resp *resolve.GraphQLResponse, controller resolve.CacheController) string {
	tb.Helper()

	ctx := resolve.NewContext(tb.Context())
	if controller != nil {
		ctx.SetCacheController(controller)
	}
	var buf bytes.Buffer
	r := resolve.New(tb.Context(), resolve.ResolverOptions{MaxConcurrency: 16})
	_, err := r.ResolveGraphQLResponse(ctx, resp, nil, &buf)
	require.NoError(tb, err)
	return buf.String()
}

// DeferGroups flattens a defer response's groups (the extracted Defers list
// plus the built DeferTree) so tests can reach every deferred fetch tree.
func DeferGroups(resp *resolve.GraphQLDeferResponse) []*resolve.DeferFetchGroup {
	if resp == nil {
		return nil
	}
	groups := append([]*resolve.DeferFetchGroup(nil), resp.Defers...)
	return appendDeferTreeGroups(groups, resp.DeferTree)
}

func appendDeferTreeGroups(groups []*resolve.DeferFetchGroup, node *resolve.DeferTreeNode) []*resolve.DeferFetchGroup {
	if node == nil {
		return groups
	}
	if node.Item != nil {
		groups = append(groups, node.Item)
	}
	for _, child := range node.ChildNodes {
		groups = appendDeferTreeGroups(groups, child)
	}
	return groups
}

// routerConfig loads the committed wgc-composed config.json next to this file,
// so the harness works from any test package in the execution module.
func routerConfig(tb testing.TB) *nodev1.RouterConfig {
	tb.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	require.True(tb, ok, "cannot locate cachingtesting package dir")
	data, err := os.ReadFile(filepath.Join(filepath.Dir(thisFile), "config.json"))
	require.NoError(tb, err)

	var rc nodev1.RouterConfig
	require.NoError(tb, protojson.Unmarshal(data, &rc))
	return &rc
}

// subgraphNameToDatasourceID maps subgraph names to the datasource IDs the
// factory-built planner configuration uses (the router config keeps them in
// its subgraph list).
func subgraphNameToDatasourceID(rc *nodev1.RouterConfig) map[string]string {
	out := make(map[string]string, len(rc.Subgraphs))
	for _, sg := range rc.Subgraphs {
		out[sg.Name] = sg.Id
	}
	return out
}

// translateResponseKeys rewrites subgraph-name response keys ("products",
// "products:path") to the datasource-ID keys FakeRegistry matches on at
// runtime; unknown prefixes (e.g. "*") pass through unchanged.
func translateResponseKeys(responses map[string]string, nameToID map[string]string) map[string]string {
	out := make(map[string]string, len(responses))
	for key, value := range responses {
		name, path, hasPath := strings.Cut(key, ":")
		if id, ok := nameToID[name]; ok {
			if hasPath {
				out[id+":"+path] = value
			} else {
				out[id] = value
			}
			continue
		}
		out[key] = value
	}
	return out
}
