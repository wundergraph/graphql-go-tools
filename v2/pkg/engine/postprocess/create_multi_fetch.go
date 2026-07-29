package postprocess

import (
	"bytes"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// createMultiFetch merges same-subgraph entity fetches that would execute in
// the same parallel wave into a single MultiEntityFetch with aliased
// _entities fields. The SubgraphOperation artifacts are cleared by the
// renderSubgraphInputs stage, which runs immediately after this one.
type createMultiFetch struct {
	disable bool
}

func (c *createMultiFetch) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if c.disable {
		return
	}
	c.walk(root, root)
}

// walk descends the ORGANIZED tree (produced by orderSequenceByDependencies +
// createParallelNodes, run just before this stage) and merges same-datasource
// entity fetches inside every real FetchTreeNodeKindParallel group. There is no
// scratch tree and no DeferID partitioning: defer groups are extracted into
// their own trees and organized separately before this stage runs, so the
// parallel groups we see already encode the correct execution waves.
func (c *createMultiFetch) walk(root, node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}
	for i := 0; i < len(node.ChildNodes); i++ {
		child := node.ChildNodes[i]
		if child.Kind == resolve.FetchTreeNodeKindParallel {
			for _, group := range c.groupCandidatesByDataSource(child) {
				c.mergeGroup(root, child, group)
			}
			// createParallelNodes materializes a Parallel node only for >1
			// children; once merging collapses a group to a single survivor the
			// Parallel wrapper must collapse back to the bare Single node to
			// preserve that invariant.
			if len(child.ChildNodes) == 1 {
				node.ChildNodes[i] = child.ChildNodes[0]
			}
		}
		c.walk(root, node.ChildNodes[i])
	}
}

// mergeGroup merges a candidate group into one MultiEntityFetch, printing the
// aliased operation, authoring per-entry template material, and rewiring
// dependents onto the survivor fetch ID. The merged node replaces the first
// group member inside its containing parallel node; the other members are
// removed from that node. Any precondition failure leaves the group untouched.
//
// parent is the FetchTreeNodeKindParallel node that holds the group members;
// root is the whole organized tree, walked to rewire dependents.
func (c *createMultiFetch) mergeGroup(root, parent *resolve.FetchTreeNode, group []*resolve.FetchTreeNode) {
	// Order-stability insurance (design D2): sort members by FetchID before
	// aliasing so alias/entry order is independent of any ordering behaviour in
	// orderSequenceByDependencies. Real waves already list members in ascending
	// FetchID order, so this reproduces the historical flat-order aliasing.
	slices.SortFunc(group, func(a, b *resolve.FetchTreeNode) int {
		return a.Item.Fetch.(*resolve.SingleFetch).FetchID - b.Item.Fetch.(*resolve.SingleFetch).FetchID
	})

	members := make([]*resolve.SingleFetch, len(group))
	for i, node := range group {
		members[i] = node.Item.Fetch.(*resolve.SingleFetch)
	}

	// Envelope precondition: every member's structured envelope (method, url,
	// header bytes) must be byte-equal. This replaces the old envelope-remainder
	// byte comparison of the printed inputs; because every member's input is
	// assembled from its envelope by the same httpclient helper, equal envelopes
	// imply equal envelope remainders.
	base := members[0].SubgraphOperation
	for i := 1; i < len(members); i++ {
		op := members[i].SubgraphOperation
		if op.Envelope.Method != base.Envelope.Method ||
			op.Envelope.URL != base.Envelope.URL ||
			!bytes.Equal(op.Envelope.Header, base.Envelope.Header) {
			return
		}
	}
	// Any $$K$$ token embedded in the envelope (method/url/header) must reference
	// the same fetch variable across members — the structured equivalent of the
	// old envelope-token check. In practice the planner never emits $$K$$ tokens
	// into the envelope (url/method are static strings and the header is static
	// config marshalled to JSON), so for real plans this check finds no tokens
	// and reduces to nothing; it is retained to preserve the exact merge-abort
	// semantics for envelopes that do carry template tokens.
	envelopeTokenSource := base.Envelope.Method + base.Envelope.URL + string(base.Envelope.Header)
	for _, k := range envelopeTokenIndices(envelopeTokenSource) {
		for i := 1; i < len(members); i++ {
			if k >= len(members[i].Variables) || k >= len(members[0].Variables) ||
				!members[i].Variables[k].Equals(members[0].Variables[k]) {
				return
			}
		}
	}

	compact, pretty, err := buildMergedOperation(members)
	if err != nil {
		return
	}

	// Build Header/Footer from the structured envelope + merged compact operation
	// via the SAME httpclient assembly the planner uses for an unmerged fetch,
	// guaranteeing byte-identity of the envelope. We assemble with an empty-object
	// variables placeholder and split at the "variables":{} boundary we injected;
	// the per-entry templates render the variables object body between Header and
	// Footer. No scanning of an unknown input is involved.
	s1 := members[0]
	env := s1.SubgraphOperation.Envelope
	assembled, err := httpclient.AssembleGraphQLRequestInput([]byte("{}"), []byte(compact), env.Header, env.URL, env.Method)
	if err != nil {
		return
	}
	const varsOpen = `"variables":{`
	marker := []byte(`"variables":{}`)
	idx := bytes.Index(assembled, marker)
	if idx == -1 {
		return
	}
	boundary := idx + len(varsOpen)
	headerSource := string(assembled[:boundary])
	footerSource := string(assembled[boundary:])
	var header, footer resolve.InputTemplate
	resolveInputTemplate(s1.Variables, headerSource, &header)
	resolveInputTemplate(s1.Variables, footerSource, &footer)

	ids := make([]int, len(members))
	entries := make([]resolve.MultiEntityFetchEntry, len(members))
	for i, m := range members {
		ids[i] = m.FetchID
		kStr := strconv.Itoa(i + 1)
		alias := "f" + kStr

		originKind := resolve.EntityFetchOriginBatch
		if m.RequiresEntityFetch {
			originKind = resolve.EntityFetchOriginSingle
		}

		repIndex := representationsFragmentIndex(m)
		repValue := m.SubgraphOperation.Variables[repIndex].Value
		var reps resolve.InputTemplate
		resolveInputTemplate(m.Variables, string(repValue[1:len(repValue)-1]), &reps)
		// The fragment is exactly one $$N$$ token; drop the empty static segments
		// the blind splitter produces around it so the template matches the shape
		// of a batch entity fetch item.
		reps.Segments = slices.DeleteFunc(reps.Segments, func(s resolve.TemplateSegment) bool {
			return s.SegmentType == resolve.StaticSegmentType && len(s.Data) == 0
		})
		reps.SetTemplateOutputToNullOnVariableNull = true

		// Derive the key from the recorded fragment name (name-agnostic, like the
		// document builder): a collision-renamed synthetic ("_representations")
		// must keep the input key and the merged query's variable in agreement.
		repPrefix := `"` + m.SubgraphOperation.Variables[repIndex].Name + "_f" + kStr + `":[`
		if i > 0 {
			repPrefix = "," + repPrefix
		}

		var variables []resolve.MultiEntityFetchVariable
		for j := range m.SubgraphOperation.Variables {
			if j == repIndex {
				continue
			}
			frag := m.SubgraphOperation.Variables[j]
			var tpl resolve.InputTemplate
			resolveInputTemplate(m.Variables, string(frag.Value), &tpl)
			variables = append(variables, resolve.MultiEntityFetchVariable{
				KeyPrefix: []byte(`,"` + frag.Name + "_f" + kStr + `":`),
				Value:     tpl,
			})
		}

		itemCopy := *group[i].Item
		// The copied item must not retain the replaced member fetch: it would keep
		// the merged-away SingleFetch (and its SubgraphOperation document) alive
		// inside the cached plan, and a multi backpointer would make the plan cyclic.
		itemCopy.Fetch = nil
		entries[i] = resolve.MultiEntityFetchEntry{
			Alias: alias,
			Item:  &itemCopy,
			Info:  m.Info,
			PostProcessing: resolve.PostProcessingConfiguration{
				SelectResponseDataPath:   []string{"data", alias},
				SelectResponseErrorsPath: []string{"errors"},
				MergePath:                m.PostProcessing.MergePath,
			},
			OriginKind:            originKind,
			RepresentationsPrefix: []byte(repPrefix),
			Representations:       reps,
			IncludePrefix:         []byte(`],"includeF` + kStr + `":`),
			Variables:             variables,
			SkipNullItems:         true,
			SkipEmptyObjectItems:  true,
			SkipErrItems:          true,
		}
	}

	multi := &resolve.MultiEntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           minID(ids),
			DependsOnFetchIDs: unionDependencies(members, ids),
			DeferID:           members[0].DeferID,
		},
		Input:                resolve.MultiEntityInput{Header: header, Entries: entries, Footer: footer},
		DataSource:           members[0].DataSource,
		DataSourceIdentifier: members[0].DataSourceIdentifier,
		MergedFetchIDs:       ids,
		Info:                 mergedFetchInfo(members, pretty),
	}
	// Entry items deliberately keep a nil Fetch: a backpointer to the multi
	// would make the plan cyclic (breaking structural plan comparison), and
	// the per-entry merge path never dereferences it.

	// Replace the first member encountered inside the parallel node with the
	// multi node, drop the rest, then repoint dependents (anywhere in the
	// organized tree) from every merged member ID onto the survivor.
	memberNodes := make(map[*resolve.FetchTreeNode]struct{}, len(group))
	for _, n := range group {
		memberNodes[n] = struct{}{}
	}
	multiNode := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: multi}}
	newChildren := make([]*resolve.FetchTreeNode, 0, len(parent.ChildNodes))
	inserted := false
	for _, n := range parent.ChildNodes {
		if _, isMember := memberNodes[n]; isMember {
			if !inserted {
				newChildren = append(newChildren, multiNode)
				inserted = true
			}
			continue
		}
		newChildren = append(newChildren, n)
	}
	parent.ChildNodes = newChildren

	for _, id := range ids {
		if id != multi.FetchID {
			replaceDependsOnFetchID(root, id, multi.FetchID)
		}
	}
}

// envelopeRemainder returns the input with its query and variables value ranges
// removed, i.e. the transport envelope shared by same-subgraph members.
func envelopeRemainder(input string, s fetchInputSplit) string {
	aStart, aEnd := s.queryStart, s.queryEnd
	bStart, bEnd := s.variablesStart, s.variablesEnd
	if bStart < aStart {
		aStart, aEnd, bStart, bEnd = bStart, bEnd, aStart, aEnd
	}
	return input[:aStart] + input[aEnd:bStart] + input[bEnd:]
}

// envelopeTokenIndices returns the $$K$$ token indices in the envelope using the
// same blind alternation as resolveInputTemplate.
func envelopeTokenIndices(remainder string) []int {
	if !strings.Contains(remainder, "$$") {
		return nil
	}
	segments := strings.Split(remainder, "$$")
	var indices []int
	isToken := false
	for _, seg := range segments {
		if isToken {
			if n, err := strconv.Atoi(seg); err == nil {
				indices = append(indices, n)
			}
			isToken = false
			continue
		}
		isToken = true
	}
	return indices
}

// minID returns the lowest member fetch ID, used as the merged fetch's survivor ID.
func minID(ids []int) int {
	m := ids[0]
	for _, id := range ids[1:] {
		if id < m {
			m = id
		}
	}
	return m
}

// unionDependencies returns the members' DependsOnFetchIDs minus member IDs;
// duplicates are tolerated, mirroring deduplicateSingleFetches.
func unionDependencies(members []*resolve.SingleFetch, ids []int) []int {
	memberSet := make(map[int]struct{}, len(ids))
	for _, id := range ids {
		memberSet[id] = struct{}{}
	}
	seen := make(map[int]struct{})
	var deps []int
	for _, m := range members {
		for _, dep := range m.DependsOnFetchIDs {
			if _, isMember := memberSet[dep]; isMember {
				continue
			}
			if _, dup := seen[dep]; dup {
				continue
			}
			seen[dep] = struct{}{}
			deps = append(deps, dep)
		}
	}
	return deps
}

// mergedFetchInfo builds the transport-level FetchInfo for the merged fetch:
// shared datasource identity, a deduplicated RootFields union, concatenated
// dependency/reason metadata, and a merged QueryPlan only when every member has one.
func mergedFetchInfo(members []*resolve.SingleFetch, pretty string) *resolve.FetchInfo {
	info := &resolve.FetchInfo{
		DataSourceID:   members[0].Info.DataSourceID,
		DataSourceName: members[0].Info.DataSourceName,
		OperationType:  ast.OperationTypeQuery,
	}
	seen := make(map[resolve.GraphCoordinate]struct{})
	allQueryPlans := true
	var dependsOnFields []resolve.Representation
	for _, m := range members {
		for _, rf := range m.Info.RootFields {
			if _, ok := seen[rf]; ok {
				continue
			}
			seen[rf] = struct{}{}
			info.RootFields = append(info.RootFields, rf)
		}
		info.CoordinateDependencies = append(info.CoordinateDependencies, m.Info.CoordinateDependencies...)
		info.FetchReasons = append(info.FetchReasons, m.Info.FetchReasons...)
		info.PropagatedFetchReasons = append(info.PropagatedFetchReasons, m.Info.PropagatedFetchReasons...)
		if m.Info.QueryPlan == nil {
			allQueryPlans = false
			continue
		}
		dependsOnFields = append(dependsOnFields, m.Info.QueryPlan.DependsOnFields...)
	}
	if allQueryPlans {
		info.QueryPlan = &resolve.QueryPlan{Query: pretty, DependsOnFields: dependsOnFields}
	}
	return info
}

// collectGroups returns every candidate set to merge across the ORGANIZED tree:
// it descends into each FetchTreeNodeKindParallel group and, within it, buckets
// candidates by DataSourceID (see groupCandidatesByDataSource). It is a
// read-only query used by ProcessFetchTree's walk and by unit tests; it does not
// mutate the tree.
func (c *createMultiFetch) collectGroups(root *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode {
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

// groupCandidatesByDataSource buckets the candidate children of a single
// parallel node by DataSourceID, in first-seen order, keeping only buckets with
// at least two members. Because every member of a parallel group is schedulable
// in the same wave, any bucket is safe to merge into one MultiEntityFetch.
func (c *createMultiFetch) groupCandidatesByDataSource(parallel *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode {
	byDataSource := map[string][]*resolve.FetchTreeNode{}
	var order []string
	for _, node := range parallel.ChildNodes {
		if !c.isCandidate(node) {
			continue
		}
		id := node.Item.Fetch.(*resolve.SingleFetch).Info.DataSourceID
		if _, seen := byDataSource[id]; !seen {
			order = append(order, id)
		}
		byDataSource[id] = append(byDataSource[id], node)
	}
	var groups [][]*resolve.FetchTreeNode
	for _, id := range order {
		if len(byDataSource[id]) >= 2 {
			groups = append(groups, byDataSource[id])
		}
	}
	return groups
}

// isCandidate reports whether the node holds a well-formed entity SingleFetch
// eligible for merging.
func (c *createMultiFetch) isCandidate(node *resolve.FetchTreeNode) bool {
	if node.Kind != resolve.FetchTreeNodeKindSingle {
		return false
	}
	fetch, ok := node.Item.Fetch.(*resolve.SingleFetch)
	if !ok {
		return false
	}
	if !fetch.RequiresEntityFetch && !fetch.RequiresEntityBatchFetch {
		return false
	}
	if fetch.SubgraphOperation == nil || fetch.Info == nil {
		return false
	}
	return representationsFragmentIndex(fetch) != -1
}

var representationsFragmentPattern = regexp.MustCompile(`^\[\$\$(\d+)\$\$\]$`)

// representationsFragmentIndex returns the index in SubgraphOperation.Variables
// of the sole representations fragment (`[$$N$$]`) whose token points at a
// ResolvableObjectVariable, or -1 when the record is malformed: duplicate
// names, or not exactly one such well-formed fragment.
func representationsFragmentIndex(fetch *resolve.SingleFetch) int {
	op := fetch.SubgraphOperation
	if op == nil {
		return -1
	}
	seen := make(map[string]struct{}, len(op.Variables))
	index := -1
	for i := range op.Variables {
		name := op.Variables[i].Name
		if _, dup := seen[name]; dup {
			return -1
		}
		seen[name] = struct{}{}
		match := representationsFragmentPattern.FindSubmatch(op.Variables[i].Value)
		if match == nil {
			continue
		}
		if index != -1 {
			return -1
		}
		n, err := strconv.Atoi(string(match[1]))
		if err != nil || n >= len(fetch.Variables) {
			return -1
		}
		if _, ok := fetch.Variables[n].(*resolve.ResolvableObjectVariable); !ok {
			return -1
		}
		index = i
	}
	return index
}
