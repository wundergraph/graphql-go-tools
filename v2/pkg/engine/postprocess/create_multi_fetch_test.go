package postprocess

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// multiFetchCandidate builds a well-formed entity-fetch candidate node.
func multiFetchCandidate(fetchID int, dependsOn []int, deferID int, dataSourceID string) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: dependsOn,
			DeferID:           deferID,
		},
		Info: &resolve.FetchInfo{DataSourceID: dataSourceID},
		FetchConfiguration: resolve.FetchConfiguration{
			RequiresEntityBatchFetch: true,
			MergeableOperation: &resolve.MergeableOperation{
				Variables: []resolve.NamedVariableFragment{
					{Name: "representations", Value: []byte("[$$0$$]")},
				},
			},
			Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
		},
	})
}

// multiFetchNonCandidate builds a plain fetch that is never a merge candidate.
func multiFetchNonCandidate(fetchID int, dependsOn []int, deferID int) *resolve.FetchTreeNode {
	return resolve.Single(&resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           fetchID,
			DependsOnFetchIDs: dependsOn,
			DeferID:           deferID,
		},
	})
}

func groupFetchIDs(groups [][]*resolve.FetchTreeNode) [][]int {
	out := make([][]int, len(groups))
	for i, g := range groups {
		ids := make([]int, len(g))
		for j, n := range g {
			ids[j] = n.Item.Fetch.Dependencies().FetchID
		}
		out[i] = ids
	}
	return out
}

func collectSingleFetches(node *resolve.FetchTreeNode, out *[]*resolve.SingleFetch) {
	if node == nil {
		return
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if f, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			*out = append(*out, f)
		}
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		for _, c := range node.ChildNodes {
			collectSingleFetches(c, out)
		}
	}
}

func treeHasMultiEntityFetch(node *resolve.FetchTreeNode) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		_, ok := node.Item.Fetch.(*resolve.MultiEntityFetch)
		return ok
	case resolve.FetchTreeNodeKindParallel, resolve.FetchTreeNodeKindSequence:
		return slices.ContainsFunc(node.ChildNodes, treeHasMultiEntityFetch)
	}
	return false
}

func TestCreateMultiFetch_CollectGroups(t *testing.T) {
	c := &createMultiFetch{}

	t.Run("root and two same-datasource candidates", func(t *testing.T) {
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds1"),
		)
		require.Equal(t, [][]int{{1, 2}}, groupFetchIDs(c.collectGroups(tree)))
	})

	t.Run("two candidates with empty dependencies", func(t *testing.T) {
		tree := resolve.Sequence(
			multiFetchCandidate(1, nil, 0, "ds1"),
			multiFetchCandidate(2, nil, 0, "ds1"),
		)
		require.Equal(t, [][]int{{1, 2}}, groupFetchIDs(c.collectGroups(tree)))
	})

	t.Run("different datasource does not group", func(t *testing.T) {
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds2"),
		)
		require.Empty(t, c.collectGroups(tree))
	})

	t.Run("dependent candidates land in different waves", func(t *testing.T) {
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{1}, 0, "ds1"),
		)
		require.Empty(t, c.collectGroups(tree))
	})

	t.Run("waves computed per DeferID partition", func(t *testing.T) {
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds1"),
			multiFetchCandidate(3, nil, 7, "ds1"),
			multiFetchCandidate(4, nil, 7, "ds1"),
		)
		require.Equal(t, [][]int{{1, 2}, {3, 4}}, groupFetchIDs(c.collectGroups(tree)))
	})

	t.Run("defer candidates depending out of partition stay serial", func(t *testing.T) {
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds1"),
			multiFetchCandidate(3, []int{0}, 7, "ds1"),
			multiFetchCandidate(4, []int{0}, 7, "ds1"),
		)
		require.Equal(t, [][]int{{1, 2}}, groupFetchIDs(c.collectGroups(tree)))
	})

	t.Run("nil Info is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).Info = nil
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		)
		require.Empty(t, c.collectGroups(tree))
	})

	t.Run("nil MergeableOperation is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).MergeableOperation = nil
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		)
		require.Empty(t, c.collectGroups(tree))
	})

	t.Run("non-entity fetch is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).RequiresEntityBatchFetch = false
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		)
		require.Empty(t, c.collectGroups(tree))
	})

	t.Run("duplicate names is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).MergeableOperation.Variables = []resolve.NamedVariableFragment{
			{Name: "a", Value: []byte("1")},
			{Name: "a", Value: []byte("2")},
			{Name: "representations", Value: []byte("[$$0$$]")},
		}
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		)
		require.Empty(t, c.collectGroups(tree))
	})

	t.Run("representations token pointing at non-resolvable-object is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		badFetch := bad.Item.Fetch.(*resolve.SingleFetch)
		badFetch.Variables = resolve.NewVariables(&resolve.ContextVariable{Path: []string{"x"}})
		tree := resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		)
		require.Empty(t, c.collectGroups(tree))
	})
}

func TestCreateMultiFetch_RepresentationsFragmentIndex(t *testing.T) {
	resolvableVars := resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}))

	t.Run("well-formed", func(t *testing.T) {
		fetch := &resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Variables: resolvableVars,
				MergeableOperation: &resolve.MergeableOperation{
					Variables: []resolve.NamedVariableFragment{
						{Name: "first", Value: []byte("$$1$$")},
						{Name: "representations", Value: []byte("[$$0$$]")},
					},
				},
			},
		}
		require.Equal(t, 1, representationsFragmentIndex(fetch))
	})

	t.Run("no representations fragment", func(t *testing.T) {
		fetch := &resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Variables: resolvableVars,
				MergeableOperation: &resolve.MergeableOperation{
					Variables: []resolve.NamedVariableFragment{
						{Name: "first", Value: []byte("$$1$$")},
					},
				},
			},
		}
		require.Equal(t, -1, representationsFragmentIndex(fetch))
	})

	t.Run("two representations fragments", func(t *testing.T) {
		fetch := &resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Variables: resolvableVars,
				MergeableOperation: &resolve.MergeableOperation{
					Variables: []resolve.NamedVariableFragment{
						{Name: "representations", Value: []byte("[$$0$$]")},
						{Name: "other", Value: []byte("[$$0$$]")},
					},
				},
			},
		}
		require.Equal(t, -1, representationsFragmentIndex(fetch))
	})

	t.Run("token out of range", func(t *testing.T) {
		fetch := &resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Variables: resolvableVars,
				MergeableOperation: &resolve.MergeableOperation{
					Variables: []resolve.NamedVariableFragment{
						{Name: "representations", Value: []byte("[$$5$$]")},
					},
				},
			},
		}
		require.Equal(t, -1, representationsFragmentIndex(fetch))
	})

	t.Run("token points at non-resolvable-object", func(t *testing.T) {
		fetch := &resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Variables: resolve.NewVariables(&resolve.ContextVariable{Path: []string{"x"}}),
				MergeableOperation: &resolve.MergeableOperation{
					Variables: []resolve.NamedVariableFragment{
						{Name: "representations", Value: []byte("[$$0$$]")},
					},
				},
			},
		}
		require.Equal(t, -1, representationsFragmentIndex(fetch))
	})
}

func TestCreateMultiFetch_ClearMergeableOperations(t *testing.T) {
	tree := resolve.Sequence(
		multiFetchCandidate(1, nil, 0, "ds1"),
		multiFetchCandidate(2, nil, 0, "ds1"),
	)
	(&createMultiFetch{disable: true}).ProcessFetchTree(tree)
	for _, node := range tree.ChildNodes {
		require.Nil(t, node.Item.Fetch.(*resolve.SingleFetch).MergeableOperation)
	}
}

func TestCreateMultiFetch_PipelineClearingUnconditional(t *testing.T) {
	p := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			RawFetches: []*resolve.FetchItem{
				{
					Fetch: &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{FetchID: 0},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:              `{"q":"0"}`,
							MergeableOperation: &resolve.MergeableOperation{},
						},
					},
				},
				{
					Fetch: &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{FetchID: 1},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:              `{"q":"1"}`,
							MergeableOperation: &resolve.MergeableOperation{},
						},
					},
				},
			},
			Data: &resolve.Object{},
		},
	}

	NewProcessor().Process(p)

	var fetches []*resolve.SingleFetch
	collectSingleFetches(p.Response.Fetches, &fetches)
	require.Len(t, fetches, 2)
	for _, f := range fetches {
		require.Nil(t, f.MergeableOperation)
	}
}

func TestCreateMultiFetch_PipelineDisableResolveInputTemplates(t *testing.T) {
	newCandidateFetch := func(fetchID int, input string) *resolve.SingleFetch {
		return &resolve.SingleFetch{
			FetchDependencies: resolve.FetchDependencies{FetchID: fetchID},
			Info:              &resolve.FetchInfo{DataSourceID: "ds1"},
			FetchConfiguration: resolve.FetchConfiguration{
				Input:                    input,
				RequiresEntityBatchFetch: true,
				MergeableOperation: &resolve.MergeableOperation{
					Variables: []resolve.NamedVariableFragment{
						{Name: "representations", Value: []byte("[$$0$$]")},
					},
				},
				Variables: resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
			},
		}
	}

	p := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			RawFetches: []*resolve.FetchItem{
				{Fetch: newCandidateFetch(0, `{"q":"0"}`)},
				{Fetch: newCandidateFetch(1, `{"q":"1"}`)},
			},
			Data: &resolve.Object{},
		},
	}

	NewProcessor(EnableMultiFetch(), DisableResolveInputTemplates()).Process(p)

	require.False(t, treeHasMultiEntityFetch(p.Response.Fetches))

	var fetches []*resolve.SingleFetch
	collectSingleFetches(p.Response.Fetches, &fetches)
	require.Len(t, fetches, 2)
	inputs := map[string]bool{}
	for _, f := range fetches {
		require.Nil(t, f.MergeableOperation)
		inputs[f.Input] = true
	}
	require.True(t, inputs[`{"q":"0"}`])
	require.True(t, inputs[`{"q":"1"}`])
}

func parseUpstreamDocument(t *testing.T, input string) *ast.Document {
	t.Helper()
	doc, report := astparser.ParseGraphqlDocumentString(input)
	require.False(t, report.HasErrors())
	return &doc
}

func TestBuildMergedOperation(t *testing.T) {
	const m1Source = `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename products(first: $first) {upc}}}}`
	const m2Source = `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`

	newMembers := func(operationName string) []*resolve.SingleFetch {
		return []*resolve.SingleFetch{
			{
				FetchDependencies: resolve.FetchDependencies{FetchID: 3},
				FetchConfiguration: resolve.FetchConfiguration{
					OperationName: operationName,
					MergeableOperation: &resolve.MergeableOperation{
						Document: parseUpstreamDocument(t, m1Source),
						Variables: []resolve.NamedVariableFragment{
							{Name: "representations", Value: []byte("[$$0$$]")},
							{Name: "first", Value: []byte("$$1$$")},
							{Name: "stale", Value: []byte("1")},
						},
					},
				},
			},
			{
				FetchDependencies: resolve.FetchDependencies{FetchID: 5},
				FetchConfiguration: resolve.FetchConfiguration{
					OperationName: operationName,
					MergeableOperation: &resolve.MergeableOperation{
						Document: parseUpstreamDocument(t, m2Source),
						Variables: []resolve.NamedVariableFragment{
							{Name: "representations", Value: []byte("[$$0$$]")},
							{Name: "first", Value: []byte("$$1$$")},
						},
					},
				},
			},
		}
	}

	t.Run("anonymous with renamed variables and stale key", func(t *testing.T) {
		compact, pretty, err := buildMergedOperation(newMembers(""))
		require.NoError(t, err)
		require.NotEmpty(t, pretty)

		require.Contains(t, compact, "$representations_f1: [_Any!]!")
		require.Contains(t, compact, "$first_f1: Int")
		require.Contains(t, compact, "$includeF1: Boolean!")
		require.Contains(t, compact, "$representations_f2")
		require.Contains(t, compact, "$first_f2")
		require.Contains(t, compact, "$includeF2")
		require.NotContains(t, compact, "$stale_f1")

		require.Contains(t, compact, "f1: _entities(representations: $representations_f1)@include(if: $includeF1)")
		require.Contains(t, compact, "f2: _entities(representations: $representations_f2)@include(if: $includeF2)")
		require.Contains(t, compact, "products(first: $first_f1)")
		require.Contains(t, compact, "notes(first: $first_f2)")

		require.Equal(t, `query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1){upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes(first: $first_f2)}}}`, compact)
	})

	t.Run("shared operation name yields multi name", func(t *testing.T) {
		compact, _, err := buildMergedOperation(newMembers("Q"))
		require.NoError(t, err)
		require.True(t, strings.HasPrefix(compact, "query Q__multi_3_5("), "got: %s", compact)
	})

	t.Run("root selection not a single _entities field is an error", func(t *testing.T) {
		members := []*resolve.SingleFetch{
			{
				FetchDependencies: resolve.FetchDependencies{FetchID: 1},
				FetchConfiguration: resolve.FetchConfiguration{
					MergeableOperation: &resolve.MergeableOperation{
						Document: parseUpstreamDocument(t, `query($representations: [_Any!]!){notEntities(representations: $representations){__typename}}`),
						Variables: []resolve.NamedVariableFragment{
							{Name: "representations", Value: []byte("[$$0$$]")},
						},
					},
				},
			},
		}
		_, _, err := buildMergedOperation(members)
		require.Error(t, err)
	})
}

const (
	mergeM1Source = `query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products {upc}}}}`
	mergeM2Source = `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`
)

func TestSplitEntityFetchInput(t *testing.T) {
	t.Run("repo shape canonical", func(t *testing.T) {
		input := `{"method":"POST","url":"http://x","header":{"Auth":["$$2$$"]},"body":{"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename}}","variables":{"representations":[$$0$$],"a":$$1$$}}}`
		s, ok := splitEntityFetchInput(input)
		require.True(t, ok)
		require.Equal(t, `query($representations: [_Any!]!){_entities(representations: $representations){__typename}}`, input[s.queryStart:s.queryEnd])
		require.Equal(t, `{"representations":[$$0$$],"a":$$1$$}`, input[s.variablesStart:s.variablesEnd])
	})

	t.Run("repo shape with escapes in literal fragment", func(t *testing.T) {
		input := `{"method":"POST","url":"http://x","body":{"query":"query{__typename}","variables":{"a":"x\"}","b":"y\\","representations":[$$0$$]}}}`
		s, ok := splitEntityFetchInput(input)
		require.True(t, ok)
		require.Equal(t, `query{__typename}`, input[s.queryStart:s.queryEnd])
		require.Equal(t, `{"a":"x\"}","b":"y\\","representations":[$$0$$]}`, input[s.variablesStart:s.variablesEnd])
	})

	t.Run("append shape", func(t *testing.T) {
		input := `{"body":{"variables":{"representations":[$$0$$],"a":$$1$$},"query":"query($representations: [_Any!]!){_entities(representations: $representations){__typename}}"},"header":{"Auth":["$$2$$"]},"url":"http://x","method":"POST"}`
		s, ok := splitEntityFetchInput(input)
		require.True(t, ok)
		require.Equal(t, `query($representations: [_Any!]!){_entities(representations: $representations){__typename}}`, input[s.queryStart:s.queryEnd])
		require.Equal(t, `{"representations":[$$0$$],"a":$$1$$}`, input[s.variablesStart:s.variablesEnd])
	})

	t.Run("append shape with escaped header containing quote-brace-comma", func(t *testing.T) {
		input := `{"body":{"variables":{"representations":[$$0$$]},"query":"query{__typename}"},"header":"{\"a\":\"b\"},x","url":"http://x","method":"POST"}`
		s, ok := splitEntityFetchInput(input)
		require.True(t, ok)
		require.Equal(t, `query{__typename}`, input[s.queryStart:s.queryEnd])
		require.Equal(t, `{"representations":[$$0$$]}`, input[s.variablesStart:s.variablesEnd])
	})

	t.Run("append shape with second false query anchor fails", func(t *testing.T) {
		// A later top-level value whose string member ends in a raw '}' produces a
		// second `"},` match; ambiguity must fail safe.
		input := `{"body":{"variables":{"representations":[$$0$$]},"query":"query{__typename}"},"extensions":{"note":"x}"},"url":"http://x"}`
		_, ok := splitEntityFetchInput(input)
		require.False(t, ok)
	})

	t.Run("no body.query anchor fails", func(t *testing.T) {
		_, ok := splitEntityFetchInput(`{"method":"POST","body":{"data":{}}}`)
		require.False(t, ok)
	})

	t.Run("repo shape not ending in triple brace fails", func(t *testing.T) {
		_, ok := splitEntityFetchInput(`{"method":"POST","body":{"query":"query{__typename}","variables":{"representations":[$$0$$]}}`)
		require.False(t, ok)
	})

	t.Run("truncated input fails", func(t *testing.T) {
		_, ok := splitEntityFetchInput(`{"body":{"query":"`)
		require.False(t, ok)
	})

	t.Run("body-but-not-variables prefix fails", func(t *testing.T) {
		_, ok := splitEntityFetchInput(`{"body":{"operationName":"x","variables":{}}}`)
		require.False(t, ok)
	})
}

type mergeMemberSpec struct {
	fetchID      int
	deps         []int
	input        string
	source       string
	batch        bool
	mergePath    string
	fragments    []resolve.NamedVariableFragment
	variables    resolve.Variables
	responsePath string
}

func buildMergeMember(t *testing.T, spec mergeMemberSpec) *resolve.FetchItem {
	t.Helper()
	f := &resolve.SingleFetch{
		FetchDependencies: resolve.FetchDependencies{FetchID: spec.fetchID, DependsOnFetchIDs: spec.deps},
		Info:              &resolve.FetchInfo{DataSourceID: "products-id", DataSourceName: "products", OperationType: ast.OperationTypeQuery},
		FetchConfiguration: resolve.FetchConfiguration{
			Input:          spec.input,
			Variables:      spec.variables,
			PostProcessing: resolve.PostProcessingConfiguration{MergePath: []string{spec.mergePath}},
			MergeableOperation: &resolve.MergeableOperation{
				Document:  parseUpstreamDocument(t, spec.source),
				Variables: spec.fragments,
			},
		},
	}
	if spec.batch {
		f.RequiresEntityBatchFetch = true
	} else {
		f.RequiresEntityFetch = true
	}
	return resolve.FetchItemWithPath(f, spec.responsePath)
}

func buildMergeNonCandidate(fetchID int, deps []int, input string) *resolve.FetchItem {
	return &resolve.FetchItem{
		Fetch: &resolve.SingleFetch{
			FetchDependencies:  resolve.FetchDependencies{FetchID: fetchID, DependsOnFetchIDs: deps},
			FetchConfiguration: resolve.FetchConfiguration{Input: input},
		},
	}
}

func findMultiEntityFetch(node *resolve.FetchTreeNode) *resolve.MultiEntityFetch {
	var found *resolve.MultiEntityFetch
	var walk func(n *resolve.FetchTreeNode)
	walk = func(n *resolve.FetchTreeNode) {
		if n == nil || found != nil {
			return
		}
		if n.Kind == resolve.FetchTreeNodeKindSingle {
			if m, ok := n.Item.Fetch.(*resolve.MultiEntityFetch); ok {
				found = m
			}
			return
		}
		for _, c := range n.ChildNodes {
			walk(c)
		}
	}
	walk(node)
	return found
}

func findSingleFetchByID(node *resolve.FetchTreeNode, id int) *resolve.SingleFetch {
	var fetches []*resolve.SingleFetch
	collectSingleFetches(node, &fetches)
	for _, f := range fetches {
		if f.FetchID == id {
			return f
		}
	}
	return nil
}

func staticData(t *testing.T, tpl resolve.InputTemplate) string {
	t.Helper()
	require.Len(t, tpl.Segments, 1)
	require.Equal(t, resolve.StaticSegmentType, tpl.Segments[0].SegmentType)
	return string(tpl.Segments[0].Data)
}

func mergeM1Repo() string {
	return `{"method":"POST","url":"http://x","body":{"query":"` + mergeM1Source + `","variables":{"representations":[$$0$$],"stale":1}}}`
}

func mergeM2Repo() string {
	return `{"method":"POST","url":"http://x","body":{"query":"` + mergeM2Source + `","variables":{"representations":[$$0$$],"first":$$1$$}}}`
}

func TestCreateMultiFetch_MergeGroup(t *testing.T) {
	t.Run("repo shape two members", func(t *testing.T) {
		p := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				RawFetches: []*resolve.FetchItem{
					buildMergeNonCandidate(0, nil, `{"q":"0"}`),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 1, deps: []int{0}, input: mergeM1Repo(), source: mergeM1Source, batch: true, mergePath: "a",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "stale", Value: []byte("1")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 2, deps: []int{0}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()}),
						responsePath: "employee",
					}),
					buildMergeNonCandidate(3, []int{2}, `{"q":"3"}`),
				},
				Data: &resolve.Object{},
			},
		}

		NewProcessor(EnableMultiFetch()).Process(p)

		tree := p.Response.Fetches
		require.Equal(t, resolve.FetchTreeNodeKindSequence, tree.Kind)
		require.Len(t, tree.ChildNodes, 3)

		s0, ok := tree.ChildNodes[0].Item.Fetch.(*resolve.SingleFetch)
		require.True(t, ok)
		require.Equal(t, 0, s0.FetchID)

		multi, ok := tree.ChildNodes[1].Item.Fetch.(*resolve.MultiEntityFetch)
		require.True(t, ok)
		require.Equal(t, 1, multi.FetchID)
		require.Equal(t, []int{1, 2}, multi.MergedFetchIDs)
		require.Equal(t, ast.OperationTypeQuery, multi.Info.OperationType)

		s3, ok := tree.ChildNodes[2].Item.Fetch.(*resolve.SingleFetch)
		require.True(t, ok)
		require.Equal(t, 3, s3.FetchID)
		require.Equal(t, []int{1}, s3.DependsOnFetchIDs)

		require.Len(t, multi.Input.Entries, 2)

		e1 := multi.Input.Entries[0]
		require.Equal(t, "f1", e1.Alias)
		require.Equal(t, `"representations_f1":[`, string(e1.RepresentationsPrefix))
		require.Equal(t, `],"includeF1":`, string(e1.IncludePrefix))
		require.Equal(t, []string{"data", "f1"}, e1.PostProcessing.SelectResponseDataPath)
		require.Equal(t, []string{"errors"}, e1.PostProcessing.SelectResponseErrorsPath)
		require.Equal(t, []string{"a"}, e1.PostProcessing.MergePath)
		require.Equal(t, resolve.EntityFetchOriginBatch, e1.OriginKind)
		require.True(t, e1.Representations.SetTemplateOutputToNullOnVariableNull)
		require.Same(t, multi, e1.Item.Fetch)
		require.Len(t, e1.Variables, 1)
		require.Equal(t, `,"stale_f1":`, string(e1.Variables[0].KeyPrefix))
		require.Equal(t, "1", staticData(t, e1.Variables[0].Value))

		e2 := multi.Input.Entries[1]
		require.Equal(t, "f2", e2.Alias)
		require.Equal(t, `,"representations_f2":[`, string(e2.RepresentationsPrefix))
		require.Equal(t, `],"includeF2":`, string(e2.IncludePrefix))
		require.Equal(t, []string{"data", "f2"}, e2.PostProcessing.SelectResponseDataPath)
		require.Equal(t, resolve.EntityFetchOriginSingle, e2.OriginKind)
		require.Len(t, e2.Variables, 1)
		require.Equal(t, `,"first_f2":`, string(e2.Variables[0].KeyPrefix))
		require.Contains(t, segmentKinds(e2.Variables[0].Value), resolve.VariableSegmentType)

		header := staticData(t, multi.Input.Header)
		require.True(t, strings.HasPrefix(header, `{"method":"POST"`))
		require.Contains(t, header, `"query":"query(`)
		require.True(t, strings.HasSuffix(header, `"variables":{`))
		require.Equal(t, `}}}`, staticData(t, multi.Input.Footer))

		var fetches []*resolve.SingleFetch
		collectSingleFetches(tree, &fetches)
		for _, f := range fetches {
			require.Nil(t, f.MergeableOperation)
		}
	})

	t.Run("append shape two members", func(t *testing.T) {
		m1 := `{"body":{"variables":{"representations":[$$0$$],"stale":1},"query":"` + mergeM1Source + `"},"url":"http://x","method":"POST"}`
		m2 := `{"body":{"variables":{"representations":[$$0$$],"first":$$1$$},"query":"` + mergeM2Source + `"},"url":"http://x","method":"POST"}`
		p := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				RawFetches: []*resolve.FetchItem{
					buildMergeNonCandidate(0, nil, `{"q":"0"}`),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 1, deps: []int{0}, input: m1, source: mergeM1Source, batch: true, mergePath: "a",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "stale", Value: []byte("1")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 2, deps: []int{0}, input: m2, source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()}),
						responsePath: "employee",
					}),
				},
				Data: &resolve.Object{},
			},
		}

		NewProcessor(EnableMultiFetch()).Process(p)

		multi := findMultiEntityFetch(p.Response.Fetches)
		require.NotNil(t, multi)
		require.Equal(t, `{"body":{"variables":{`, staticData(t, multi.Input.Header))
		footer := staticData(t, multi.Input.Footer)
		require.True(t, strings.HasPrefix(footer, `},"query":"query(`), "got: %s", footer)
		require.True(t, strings.HasSuffix(footer, `"},"url":"http://x","method":"POST"}`), "got: %s", footer)
	})

	t.Run("three members", func(t *testing.T) {
		m3 := `{"method":"POST","url":"http://x","body":{"query":"` + mergeM1Source + `","variables":{"representations":[$$0$$]}}}`
		p := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				RawFetches: []*resolve.FetchItem{
					buildMergeNonCandidate(0, nil, `{"q":"0"}`),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 1, deps: []int{0}, input: mergeM1Repo(), source: mergeM1Source, batch: true, mergePath: "a",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "stale", Value: []byte("1")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 2, deps: []int{0}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()}),
						responsePath: "employee",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 4, deps: []int{0}, input: m3, source: mergeM1Source, batch: true, mergePath: "c",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "contractors.@",
					}),
				},
				Data: &resolve.Object{},
			},
		}

		NewProcessor(EnableMultiFetch()).Process(p)

		multi := findMultiEntityFetch(p.Response.Fetches)
		require.NotNil(t, multi)
		require.Equal(t, 1, multi.FetchID)
		require.Equal(t, []int{1, 2, 4}, multi.MergedFetchIDs)
		require.Len(t, multi.Input.Entries, 3)
		require.Equal(t, "f3", multi.Input.Entries[2].Alias)
		require.Equal(t, `,"representations_f3":[`, string(multi.Input.Entries[2].RepresentationsPrefix))
		require.Equal(t, `],"includeF3":`, string(multi.Input.Entries[2].IncludePrefix))
	})

	t.Run("survivor id rewrite", func(t *testing.T) {
		m7 := `{"method":"POST","url":"http://x","body":{"query":"` + mergeM1Source + `","variables":{"representations":[$$0$$]}}}`
		p := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				RawFetches: []*resolve.FetchItem{
					buildMergeNonCandidate(0, nil, `{"q":"0"}`),
					buildMergeNonCandidate(1, nil, `{"q":"1"}`),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 7, deps: []int{0}, input: m7, source: mergeM1Source, batch: true, mergePath: "a",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 4, deps: []int{0, 1}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()}),
						responsePath: "employee",
					}),
					buildMergeNonCandidate(9, []int{7}, `{"q":"9"}`),
				},
				Data: &resolve.Object{},
			},
		}

		NewProcessor(EnableMultiFetch()).Process(p)

		multi := findMultiEntityFetch(p.Response.Fetches)
		require.NotNil(t, multi)
		require.Equal(t, 4, multi.FetchID)
		require.Equal(t, []int{7, 4}, multi.MergedFetchIDs)

		s9 := findSingleFetchByID(p.Response.Fetches, 9)
		require.NotNil(t, s9)
		require.Equal(t, []int{4}, s9.DependsOnFetchIDs)
	})
}

func segmentKinds(tpl resolve.InputTemplate) []resolve.SegmentType {
	kinds := make([]resolve.SegmentType, len(tpl.Segments))
	for i, s := range tpl.Segments {
		kinds[i] = s.SegmentType
	}
	return kinds
}

func mergeAbortMember(input string, vars resolve.Variables) *resolve.SingleFetch {
	return &resolve.SingleFetch{
		Info: &resolve.FetchInfo{DataSourceID: "ds1"},
		FetchConfiguration: resolve.FetchConfiguration{
			Input:                    input,
			Variables:                vars,
			RequiresEntityBatchFetch: true,
			MergeableOperation: &resolve.MergeableOperation{
				Variables: []resolve.NamedVariableFragment{{Name: "representations", Value: []byte("[$$0$$]")}},
			},
		},
	}
}

func TestCreateMultiFetch_MergeGroupAborts(t *testing.T) {
	c := &createMultiFetch{}

	t.Run("different envelope url", func(t *testing.T) {
		m1 := mergeAbortMember(`{"method":"POST","url":"http://x","body":{"query":"query{__typename}","variables":{"representations":[$$0$$]}}}`, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})))
		m2 := mergeAbortMember(`{"method":"POST","url":"http://y","body":{"query":"query{__typename}","variables":{"representations":[$$0$$]}}}`, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})))
		node1, node2 := resolve.Single(m1), resolve.Single(m2)
		root := resolve.Sequence(node1, node2)
		c.mergeGroup(root, []*resolve.FetchTreeNode{node1, node2})
		require.Len(t, root.ChildNodes, 2)
		require.False(t, treeHasMultiEntityFetch(root))
	})

	t.Run("envelope token references different variable", func(t *testing.T) {
		input := `{"method":"POST","url":"http://x","header":{"Auth":["$$2$$"]},"body":{"query":"query{__typename}","variables":{"representations":[$$0$$]}}}`
		m1 := mergeAbortMember(input, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"x"}}, &resolve.HeaderVariable{Path: []string{"Auth"}}))
		m2 := mergeAbortMember(input, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"x"}}, &resolve.HeaderVariable{Path: []string{"Other"}}))
		node1, node2 := resolve.Single(m1), resolve.Single(m2)
		root := resolve.Sequence(node1, node2)
		c.mergeGroup(root, []*resolve.FetchTreeNode{node1, node2})
		require.Len(t, root.ChildNodes, 2)
		require.False(t, treeHasMultiEntityFetch(root))
	})

	t.Run("malformed input", func(t *testing.T) {
		m1 := mergeAbortMember(`{"garbage":true}`, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})))
		m2 := mergeAbortMember(`{"method":"POST","url":"http://x","body":{"query":"query{__typename}","variables":{"representations":[$$0$$]}}}`, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})))
		node1, node2 := resolve.Single(m1), resolve.Single(m2)
		root := resolve.Sequence(node1, node2)
		c.mergeGroup(root, []*resolve.FetchTreeNode{node1, node2})
		require.Len(t, root.ChildNodes, 2)
		require.False(t, treeHasMultiEntityFetch(root))
	})
}
