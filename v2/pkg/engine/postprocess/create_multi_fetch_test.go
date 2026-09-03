package postprocess

import (
	"slices"
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
			SubgraphOperation: &resolve.SubgraphOperation{
				Variables: []resolve.SubgraphVariable{
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
			ids[j] = n.FetchID()
		}
		out[i] = ids
	}
	return out
}

func collectGroups(c *createMultiFetch, root *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode {
	var result [][]*resolve.FetchTreeNode
	var walk func(node *resolve.FetchTreeNode)
	walk = func(node *resolve.FetchTreeNode) {
		if node == nil {
			return
		}
		if node.Kind == resolve.FetchTreeNodeKindParallel {
			result = append(result, c.groupCandidatesByDataSource(node)...)
		}
		for _, child := range node.ChildNodes {
			walk(child)
		}
	}
	walk(root)
	return result
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

// organizeForTest runs the organize stages (order + parallel) so collectGroups
// sees the same real parallel groups it walks in the production pipeline, where
// createMultiFetch runs AFTER organizeFetchTree.
func organizeForTest(tree *resolve.FetchTreeNode) *resolve.FetchTreeNode {
	(&orderSequenceByDependencies{}).ProcessFetchTree(tree)
	(&createParallelNodes{}).ProcessFetchTree(tree)
	return tree
}

func TestCreateMultiFetch_CollectGroups(t *testing.T) {
	c := &createMultiFetch{}

	t.Run("root and two same-datasource candidates", func(t *testing.T) {
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds1"),
		))
		require.Equal(t, [][]int{{1, 2}}, groupFetchIDs(collectGroups(c, tree)))
	})

	t.Run("two candidates with empty dependencies", func(t *testing.T) {
		tree := organizeForTest(resolve.Sequence(
			multiFetchCandidate(1, nil, 0, "ds1"),
			multiFetchCandidate(2, nil, 0, "ds1"),
		))
		require.Equal(t, [][]int{{1, 2}}, groupFetchIDs(collectGroups(c, tree)))
	})

	t.Run("different datasource does not group", func(t *testing.T) {
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds2"),
		))
		require.Empty(t, collectGroups(c, tree))
	})

	t.Run("dependent candidates land in different waves", func(t *testing.T) {
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{1}, 0, "ds1"),
		))
		require.Empty(t, collectGroups(c, tree))
	})

	// Defer groups are extracted into their own trees before organizeFetchTree
	// (see Processor.Process for DeferResponsePlan), so each defer group is
	// organized and merged independently. This replaces the old DeferID
	// partitioning that createMultiFetch did on a single flat tree.
	t.Run("each defer group is organized and merged independently", func(t *testing.T) {
		initial := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			multiFetchCandidate(2, []int{0}, 0, "ds1"),
		))
		require.Equal(t, [][]int{{1, 2}}, groupFetchIDs(collectGroups(c, initial)))

		deferGroup := organizeForTest(resolve.Sequence(
			multiFetchCandidate(3, nil, 7, "ds1"),
			multiFetchCandidate(4, nil, 7, "ds1"),
		))
		require.Equal(t, [][]int{{3, 4}}, groupFetchIDs(collectGroups(c, deferGroup)))
	})

	t.Run("defer candidates depending out of group stay serial", func(t *testing.T) {
		// In the defer group's own tree, fetch 0 lives in the initial response
		// tree, not here; createParallelNodes cannot satisfy that dependency
		// within the group, so the two candidates never share a parallel wave and
		// are not merged.
		deferGroup := organizeForTest(resolve.Sequence(
			multiFetchCandidate(3, []int{0}, 7, "ds1"),
			multiFetchCandidate(4, []int{0}, 7, "ds1"),
		))
		require.Empty(t, collectGroups(c, deferGroup))
	})

	t.Run("nil Info is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).Info = nil
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		))
		require.Empty(t, collectGroups(c, tree))
	})

	t.Run("nil SubgraphOperation is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).SubgraphOperation = nil
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		))
		require.Empty(t, collectGroups(c, tree))
	})

	t.Run("non-entity fetch is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).RequiresEntityBatchFetch = false
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		))
		require.Empty(t, collectGroups(c, tree))
	})

	t.Run("duplicate names is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		bad.Item.Fetch.(*resolve.SingleFetch).SubgraphOperation.Variables = []resolve.SubgraphVariable{
			{Name: "a", Value: []byte("1")},
			{Name: "a", Value: []byte("2")},
			{Name: "representations", Value: []byte("[$$0$$]")},
		}
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		))
		require.Empty(t, collectGroups(c, tree))
	})

	t.Run("representations token pointing at non-resolvable-object is not a candidate", func(t *testing.T) {
		bad := multiFetchCandidate(2, []int{0}, 0, "ds1")
		badFetch := bad.Item.Fetch.(*resolve.SingleFetch)
		badFetch.Variables = resolve.NewVariables(&resolve.ContextVariable{Path: []string{"x"}})
		tree := organizeForTest(resolve.Sequence(
			multiFetchNonCandidate(0, nil, 0),
			multiFetchCandidate(1, []int{0}, 0, "ds1"),
			bad,
		))
		require.Empty(t, collectGroups(c, tree))
	})
}

func TestCreateMultiFetch_RepresentationsFragmentIndex(t *testing.T) {
	resolvableVars := resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}))

	t.Run("well-formed", func(t *testing.T) {
		fetch := &resolve.SingleFetch{
			FetchConfiguration: resolve.FetchConfiguration{
				Variables: resolvableVars,
				SubgraphOperation: &resolve.SubgraphOperation{
					Variables: []resolve.SubgraphVariable{
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
				SubgraphOperation: &resolve.SubgraphOperation{
					Variables: []resolve.SubgraphVariable{
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
				SubgraphOperation: &resolve.SubgraphOperation{
					Variables: []resolve.SubgraphVariable{
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
				SubgraphOperation: &resolve.SubgraphOperation{
					Variables: []resolve.SubgraphVariable{
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
				SubgraphOperation: &resolve.SubgraphOperation{
					Variables: []resolve.SubgraphVariable{
						{Name: "representations", Value: []byte("[$$0$$]")},
					},
				},
			},
		}
		require.Equal(t, -1, representationsFragmentIndex(fetch))
	})
}

// TestCreateMultiFetch_PipelineClearingUnconditional asserts the end-to-end
// guarantee that no SubgraphOperation artifact survives postprocessing.
func TestCreateMultiFetch_PipelineClearingUnconditional(t *testing.T) {
	p := &plan.SynchronousResponsePlan{
		Response: &resolve.GraphQLResponse{
			RawFetches: []*resolve.FetchItem{
				{
					Fetch: &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{FetchID: 0},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:             `{"q":"0"}`,
							SubgraphOperation: &resolve.SubgraphOperation{},
						},
					},
				},
				{
					Fetch: &resolve.SingleFetch{
						FetchDependencies: resolve.FetchDependencies{FetchID: 1},
						FetchConfiguration: resolve.FetchConfiguration{
							Input:             `{"q":"1"}`,
							SubgraphOperation: &resolve.SubgraphOperation{},
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
		require.Nil(t, f.SubgraphOperation)
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
				SubgraphOperation: &resolve.SubgraphOperation{
					Variables: []resolve.SubgraphVariable{
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
		require.Nil(t, f.SubgraphOperation)
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
					SubgraphOperation: &resolve.SubgraphOperation{
						Document: parseUpstreamDocument(t, m1Source),
						Variables: []resolve.SubgraphVariable{
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
					SubgraphOperation: &resolve.SubgraphOperation{
						Document: parseUpstreamDocument(t, m2Source),
						Variables: []resolve.SubgraphVariable{
							{Name: "representations", Value: []byte("[$$0$$]")},
							{Name: "first", Value: []byte("$$1$$")},
						},
					},
				},
			},
		}
	}

	const mergedBody = `query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products(first: $first_f1){upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes(first: $first_f2)}}}`

	t.Run("anonymous with renamed variables and stale key", func(t *testing.T) {
		compact, pretty, err := buildMergedOperation(newMembers(""))
		require.NoError(t, err)
		require.Equal(t, mergedBody, compact)
		require.Equal(t, `query($representations_f1: [_Any!]!, $first_f1: Int, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){
  f1: _entities(representations: $representations_f1)@include(if: $includeF1) {
    ... on Employee {
      __typename
      products(first: $first_f1){
        upc
      }
    }
  }
  f2: _entities(representations: $representations_f2)@include(if: $includeF2) {
    ... on Employee {
      __typename
      notes(first: $first_f2)
    }
  }
}`, pretty)
	})

	t.Run("shared operation name yields multi name", func(t *testing.T) {
		compact, _, err := buildMergedOperation(newMembers("Q"))
		require.NoError(t, err)
		require.Equal(t, "query Q__multi_3_5"+mergedBody[len("query"):], compact)
	})

	t.Run("root selection not a single _entities field is an error", func(t *testing.T) {
		members := []*resolve.SingleFetch{
			{
				FetchDependencies: resolve.FetchDependencies{FetchID: 1},
				FetchConfiguration: resolve.FetchConfiguration{
					SubgraphOperation: &resolve.SubgraphOperation{
						Document: parseUpstreamDocument(t, `query($representations: [_Any!]!){notEntities(representations: $representations){__typename}}`),
						Variables: []resolve.SubgraphVariable{
							{Name: "representations", Value: []byte("[$$0$$]")},
						},
					},
				},
			},
		}
		_, _, err := buildMergedOperation(members)
		require.Error(t, err)
	})

	t.Run("document without operation definition is an error", func(t *testing.T) {
		members := []*resolve.SingleFetch{
			{
				FetchDependencies: resolve.FetchDependencies{FetchID: 1},
				FetchConfiguration: resolve.FetchConfiguration{
					SubgraphOperation: &resolve.SubgraphOperation{
						Document: ast.NewSmallDocument(),
					},
				},
			},
		}
		_, _, err := buildMergedOperation(members)
		require.EqualError(t, err, "createMultiFetch: member 1 document has no operation definition")
	})
}

const (
	mergeM1Source = `query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products {upc}}}}`
	mergeM2Source = `query($representations: [_Any!]!, $first: Int){_entities(representations: $representations){... on Employee {__typename notes(first: $first)}}}`
	// mergeGroupMergedOperation is the merged operation buildMergedOperation
	// produces for the two-member fixtures above.
	mergeGroupMergedOperation = `query($representations_f1: [_Any!]!, $includeF1: Boolean!, $representations_f2: [_Any!]!, $first_f2: Int, $includeF2: Boolean!){f1: _entities(representations: $representations_f1)@include(if: $includeF1) {... on Employee {__typename products {upc}}} f2: _entities(representations: $representations_f2)@include(if: $includeF2) {... on Employee {__typename notes(first: $first_f2)}}}`
)

type mergeMemberSpec struct {
	fetchID      int
	deps         []int
	input        string
	source       string
	batch        bool
	mergePath    string
	fragments    []resolve.SubgraphVariable
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
			SubgraphOperation: &resolve.SubgraphOperation{
				Document:  parseUpstreamDocument(t, spec.source),
				Variables: spec.fragments,
				// The structured mergeGroup builds Header/Footer from the envelope
				// (not from the printed Input, which is now ignored). All fixtures
				// target the same POST http://x subgraph endpoint.
				Envelope: resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"},
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
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "stale", Value: []byte("1")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 2, deps: []int{0}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
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
		require.Nil(t, e1.Item.Fetch, "entry items must not point back at the multi fetch (cyclic plans break structural comparison)")
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

		require.Equal(t, `{"method":"POST","url":"http://x","body":{"query":"`+mergeGroupMergedOperation+`","variables":{`, staticData(t, multi.Input.Header))
		require.Equal(t, `}}}`, staticData(t, multi.Input.Footer))

		var fetches []*resolve.SingleFetch
		collectSingleFetches(tree, &fetches)
		for _, f := range fetches {
			require.Nil(t, f.SubgraphOperation)
		}
	})

	t.Run("three members", func(t *testing.T) {
		m3 := `{"method":"POST","url":"http://x","body":{"query":"` + mergeM1Source + `","variables":{"representations":[$$0$$]}}}`
		p := &plan.SynchronousResponsePlan{
			Response: &resolve.GraphQLResponse{
				RawFetches: []*resolve.FetchItem{
					buildMergeNonCandidate(0, nil, `{"q":"0"}`),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 1, deps: []int{0}, input: mergeM1Repo(), source: mergeM1Source, batch: true, mergePath: "a",
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "stale", Value: []byte("1")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 2, deps: []int{0}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()}),
						responsePath: "employee",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 4, deps: []int{0}, input: m3, source: mergeM1Source, batch: true, mergePath: "c",
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}},
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
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}},
						variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
						responsePath: "employees.@",
					}),
					buildMergeMember(t, mergeMemberSpec{
						fetchID: 4, deps: []int{0, 1}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
						fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
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
		// Members are sorted by FetchID before aliasing, so the merged
		// id list is ascending regardless of the members' position in the wave.
		require.Equal(t, []int{4, 7}, multi.MergedFetchIDs)

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

// mergeAbortMember builds a well-formed entity-fetch member with a valid,
// mergeable _entities document, parameterised by its structured envelope and
// fetch variables. Because the document always merges cleanly, any abort in
// mergeGroup is attributable solely to the structured guard under test (envelope
// mismatch or a $$K$$ envelope-token variable mismatch), never to a downstream
// buildMergedOperation failure.
func mergeAbortMember(t *testing.T, env resolve.SubgraphRequestEnvelope, vars resolve.Variables) *resolve.SingleFetch {
	t.Helper()
	return &resolve.SingleFetch{
		Info: &resolve.FetchInfo{DataSourceID: "ds1"},
		FetchConfiguration: resolve.FetchConfiguration{
			Variables:                vars,
			RequiresEntityBatchFetch: true,
			SubgraphOperation: &resolve.SubgraphOperation{
				Document:  parseUpstreamDocument(t, mergeM1Source),
				Variables: []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}},
				Envelope:  env,
			},
		},
	}
}

func TestCreateMultiFetch_CollapsesGroupOfOne(t *testing.T) {
	// An organized Parallel group whose only children are the merge members must
	// collapse back to a bare Single node once they merge into one
	// MultiEntityFetch, preserving createParallelNodes' invariant that a Parallel
	// node exists only for >1 children.
	m1 := buildMergeMember(t, mergeMemberSpec{
		fetchID: 1, deps: []int{0}, input: mergeM1Repo(), source: mergeM1Source, batch: true, mergePath: "a",
		fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}},
		variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{})),
		responsePath: "employees.@",
	})
	m2 := buildMergeMember(t, mergeMemberSpec{
		fetchID: 2, deps: []int{0}, input: mergeM2Repo(), source: mergeM2Source, batch: false, mergePath: "b",
		fragments:    []resolve.SubgraphVariable{{Name: "representations", Value: []byte("[$$0$$]")}, {Name: "first", Value: []byte("$$1$$")}},
		variables:    resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"first"}, Renderer: resolve.NewJSONVariableRenderer()}),
		responsePath: "employee",
	})
	node1 := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: m1}
	node2 := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: m2}
	root := resolve.Sequence(resolve.Parallel(node1, node2))

	(&createMultiFetch{}).ProcessFetchTree(root)

	require.Len(t, root.ChildNodes, 1)
	require.Equal(t, resolve.FetchTreeNodeKindSingle, root.ChildNodes[0].Kind, "the parallel group of one must collapse to a bare Single node")
	multi, ok := root.ChildNodes[0].Item.Fetch.(*resolve.MultiEntityFetch)
	require.True(t, ok)
	require.Equal(t, []int{1, 2}, multi.MergedFetchIDs)
}

func TestCreateMultiFetch_MergeGroupAborts(t *testing.T) {
	c := &createMultiFetch{}

	baseVars := func() resolve.Variables {
		return resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}))
	}

	// Envelope mismatch (guard 1): two members whose structured
	// SubgraphOperation.Envelope differ must not merge. Both carry valid,
	// mergeable documents, so the abort is attributable solely to the envelope
	// comparison replacing the old envelope-remainder byte scan.
	t.Run("different envelope url", func(t *testing.T) {
		m1 := mergeAbortMember(t, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, baseVars())
		m2 := mergeAbortMember(t, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://y"}, baseVars())
		node1, node2 := resolve.Single(m1), resolve.Single(m2)
		root := resolve.Sequence(node1, node2)
		c.mergeGroup(root, root, []*resolve.FetchTreeNode{node1, node2})
		require.Len(t, root.ChildNodes, 2)
		require.False(t, treeHasMultiEntityFetch(root))
	})

	t.Run("different envelope header presence", func(t *testing.T) {
		m1 := mergeAbortMember(t, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x"}, baseVars())
		m2 := mergeAbortMember(t, resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x", Header: []byte(`{"Auth":["secret"]}`)}, baseVars())
		node1, node2 := resolve.Single(m1), resolve.Single(m2)
		root := resolve.Sequence(node1, node2)
		c.mergeGroup(root, root, []*resolve.FetchTreeNode{node1, node2})
		require.Len(t, root.ChildNodes, 2)
		require.False(t, treeHasMultiEntityFetch(root))
	})

	// Envelope $$K$$-token variable mismatch (guard 2): when the (identical)
	// envelope bytes embed a $$K$$ token, the fetch variable it references must be
	// .Equals() across members. Here the envelopes are byte-equal (so guard 1
	// passes) but the HeaderVariable at index 2 differs, so the token guard aborts
	// the merge. The envelope Header bytes are hand-built to carry the token,
	// exercising the structured equivalent of the old envelope-token scan.
	t.Run("envelope token references different variable", func(t *testing.T) {
		env := resolve.SubgraphRequestEnvelope{Method: "POST", URL: "http://x", Header: []byte(`{"Auth":["$$2$$"]}`)}
		m1 := mergeAbortMember(t, env, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"x"}}, &resolve.HeaderVariable{Path: []string{"Auth"}}))
		m2 := mergeAbortMember(t, env, resolve.NewVariables(resolve.NewResolvableObjectVariable(&resolve.Object{}), &resolve.ContextVariable{Path: []string{"x"}}, &resolve.HeaderVariable{Path: []string{"Other"}}))
		node1, node2 := resolve.Single(m1), resolve.Single(m2)
		root := resolve.Sequence(node1, node2)
		c.mergeGroup(root, root, []*resolve.FetchTreeNode{node1, node2})
		require.Len(t, root.ChildNodes, 2)
		require.False(t, treeHasMultiEntityFetch(root))
	})
}
