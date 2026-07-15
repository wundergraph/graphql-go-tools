package postprocess

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// createMultiFetch merges same-subgraph entity fetches that would execute in
// the same parallel wave into a single MultiEntityFetch with aliased
// _entities fields. It always clears MergeableOperation artifacts, even when
// disabled, so no AST survives postprocessing.
type createMultiFetch struct {
	disable bool
}

func (c *createMultiFetch) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if !c.disable {
		for _, group := range c.collectGroups(root) {
			c.mergeGroup(root, group)
		}
	}
	c.clearMergeableOperations(root)
}

// mergeGroup merges a candidate group into one MultiEntityFetch, printing the
// aliased operation, authoring per-entry template material, and rewiring
// dependents onto the survivor fetch ID. Any precondition failure leaves the
// group untouched.
func (c *createMultiFetch) mergeGroup(root *resolve.FetchTreeNode, group []*resolve.FetchTreeNode) {
	members := make([]*resolve.SingleFetch, len(group))
	for i, node := range group {
		members[i] = node.Item.Fetch.(*resolve.SingleFetch)
	}

	splits := make([]fetchInputSplit, len(members))
	for i, m := range members {
		s, ok := splitEntityFetchInput(m.Input)
		if !ok {
			return
		}
		splits[i] = s
	}

	// Envelope precondition: every member's input minus its query and variables
	// ranges must be byte-equal, and any shared $$K$$ token must reference the
	// same variable across members.
	baseRemainder := envelopeRemainder(members[0].Input, splits[0])
	for i := 1; i < len(members); i++ {
		if envelopeRemainder(members[i].Input, splits[i]) != baseRemainder {
			return
		}
	}
	for _, k := range envelopeTokenIndices(baseRemainder) {
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

	s1 := members[0]
	split1 := splits[0]
	var headerSource, footerSource string
	if split1.queryStart < split1.variablesStart {
		// repo shape: query before variables
		headerSource = s1.Input[:split1.queryStart] + compact + s1.Input[split1.queryEnd:split1.variablesStart] + "{"
		footerSource = "}" + s1.Input[split1.variablesEnd:]
	} else {
		// append shape: variables before query
		headerSource = s1.Input[:split1.variablesStart] + "{"
		footerSource = "}" + s1.Input[split1.variablesEnd:split1.queryStart] + compact + s1.Input[split1.queryEnd:]
	}
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
		repValue := m.MergeableOperation.Variables[repIndex].Value
		var reps resolve.InputTemplate
		resolveInputTemplate(m.Variables, string(repValue[1:len(repValue)-1]), &reps)
		reps.SetTemplateOutputToNullOnVariableNull = true

		repPrefix := `"representations_f` + kStr + `":[`
		if i > 0 {
			repPrefix = "," + repPrefix
		}

		var variables []resolve.MultiEntityFetchVariable
		for j := range m.MergeableOperation.Variables {
			if j == repIndex {
				continue
			}
			frag := m.MergeableOperation.Variables[j]
			var tpl resolve.InputTemplate
			resolveInputTemplate(m.Variables, string(frag.Value), &tpl)
			variables = append(variables, resolve.MultiEntityFetchVariable{
				KeyPrefix: []byte(`,"` + frag.Name + "_f" + kStr + `":`),
				Value:     tpl,
			})
		}

		itemCopy := *group[i].Item
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
	for i := range entries {
		entries[i].Item.Fetch = multi
	}

	// Replace the first member's node with the multi node, drop the rest, then
	// repoint dependents from every merged member ID onto the survivor.
	memberNodes := make(map[*resolve.FetchTreeNode]struct{}, len(group))
	for _, n := range group {
		memberNodes[n] = struct{}{}
	}
	first := group[0]
	multiNode := &resolve.FetchTreeNode{Kind: resolve.FetchTreeNodeKindSingle, Item: &resolve.FetchItem{Fetch: multi}}
	newChildren := make([]*resolve.FetchTreeNode, 0, len(root.ChildNodes))
	for _, n := range root.ChildNodes {
		if n == first {
			newChildren = append(newChildren, multiNode)
			continue
		}
		if _, isMember := memberNodes[n]; isMember {
			continue
		}
		newChildren = append(newChildren, n)
	}
	root.ChildNodes = newChildren

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
	var deps []int
	for _, m := range members {
		for _, dep := range m.DependsOnFetchIDs {
			if _, isMember := memberSet[dep]; isMember {
				continue
			}
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

// collectGroups returns the candidate sets to merge: candidates sharing a
// DataSourceID within the same simulated parallel wave, ordered as they appear
// in that wave, keeping only sets with at least two members.
func (c *createMultiFetch) collectGroups(root *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode {
	var result [][]*resolve.FetchTreeNode
	for _, wave := range c.wavesInOrder(root) {
		byDataSource := map[string][]*resolve.FetchTreeNode{}
		var order []string
		for _, node := range wave {
			if !c.isCandidate(node) {
				continue
			}
			id := node.Item.Fetch.(*resolve.SingleFetch).Info.DataSourceID
			if _, seen := byDataSource[id]; !seen {
				order = append(order, id)
			}
			byDataSource[id] = append(byDataSource[id], node)
		}
		for _, id := range order {
			if len(byDataSource[id]) >= 2 {
				result = append(result, byDataSource[id])
			}
		}
	}
	return result
}

// wavesInOrder simulates the organize stages on a scratch copy per DeferID
// partition and returns the flat child nodes grouped into execution waves. The
// stages mutate only the scratch root's ChildNodes slice, never the nodes.
func (c *createMultiFetch) wavesInOrder(root *resolve.FetchTreeNode) [][]*resolve.FetchTreeNode {
	var waves [][]*resolve.FetchTreeNode
	for _, partition := range c.partitionByDeferID(root.ChildNodes) {
		scratch := &resolve.FetchTreeNode{
			Kind:       resolve.FetchTreeNodeKindSequence,
			ChildNodes: append([]*resolve.FetchTreeNode(nil), partition...),
		}
		(&orderSequenceByDependencies{}).ProcessFetchTree(scratch)
		(&createParallelNodes{}).ProcessFetchTree(scratch)
		for _, child := range scratch.ChildNodes {
			switch child.Kind {
			case resolve.FetchTreeNodeKindParallel:
				waves = append(waves, child.ChildNodes)
			default:
				waves = append(waves, []*resolve.FetchTreeNode{child})
			}
		}
	}
	return waves
}

// partitionByDeferID buckets children by DeferID, emitting buckets in
// first-seen order and preserving each bucket's original child order.
func (c *createMultiFetch) partitionByDeferID(children []*resolve.FetchTreeNode) [][]*resolve.FetchTreeNode {
	buckets := map[int][]*resolve.FetchTreeNode{}
	var order []int
	for _, node := range children {
		deferID := node.Item.Fetch.Dependencies().DeferID
		if _, seen := buckets[deferID]; !seen {
			order = append(order, deferID)
		}
		buckets[deferID] = append(buckets[deferID], node)
	}
	result := make([][]*resolve.FetchTreeNode, 0, len(order))
	for _, deferID := range order {
		result = append(result, buckets[deferID])
	}
	return result
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
	if fetch.MergeableOperation == nil || fetch.Info == nil {
		return false
	}
	return representationsFragmentIndex(fetch) != -1
}

var representationsFragmentPattern = regexp.MustCompile(`^\[\$\$(\d+)\$\$\]$`)

// representationsFragmentIndex returns the index in MergeableOperation.Variables
// of the sole representations fragment (`[$$N$$]`) whose token points at a
// ResolvableObjectVariable, or -1 when the record is malformed: duplicate
// names, or not exactly one such well-formed fragment.
func representationsFragmentIndex(fetch *resolve.SingleFetch) int {
	op := fetch.MergeableOperation
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

// clearMergeableOperations nils MergeableOperation on every SingleFetch child so
// no planner AST survives postprocessing.
func (c *createMultiFetch) clearMergeableOperations(root *resolve.FetchTreeNode) {
	for _, node := range root.ChildNodes {
		if fetch, ok := node.Item.Fetch.(*resolve.SingleFetch); ok {
			fetch.MergeableOperation = nil
		}
	}
}
