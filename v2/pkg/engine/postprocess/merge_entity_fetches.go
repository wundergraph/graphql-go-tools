package postprocess

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

// mergeEntityFetches fuses same-subgraph, dependency-compatible entity fetches
// (EntityFetch / BatchEntityFetch) that occur at several points of a single
// operation into ONE upstream request using aliased _entities selections
// (a resolve.MultiEntityFetch). This is the plan-side half of ROUTER-62; the
// loader renders and demultiplexes the produced node.
//
// It runs on the flat, dependency-ordered children of the root sequence, before
// createParallelNodes groups siblings — so it sees EntityFetch/BatchEntityFetch
// nodes as direct children and the merged node still gets parallel-grouped with
// other siblings afterwards.
type mergeEntityFetches struct {
	disable bool
}

func (m *mergeEntityFetches) ProcessFetchTree(root *resolve.FetchTreeNode) {
	if m.disable || root == nil {
		return
	}
	m.mergeSiblings(root)
	for i := range root.ChildNodes {
		// Nested sequences (rare in the flat tree, but be safe) are processed too.
		if root.ChildNodes[i].Kind == resolve.FetchTreeNodeKindSequence ||
			root.ChildNodes[i].Kind == resolve.FetchTreeNodeKindParallel {
			m.ProcessFetchTree(root.ChildNodes[i])
		}
	}
}

func (m *mergeEntityFetches) mergeSiblings(root *resolve.FetchTreeNode) {
	children := root.ChildNodes
	consumed := make([]bool, len(children))
	newChildren := make([]*resolve.FetchTreeNode, 0, len(children))

	for i := range children {
		if consumed[i] {
			continue
		}
		base := children[i]
		if !isMergeableEntityNode(base) {
			newChildren = append(newChildren, base)
			continue
		}

		// Track the group's own fetch IDs so a candidate that depends on (or is
		// depended on by) a member is not pulled into the same request.
		group := []*resolve.FetchTreeNode{base}
		groupIDs := map[int]bool{base.Item.Fetch.Dependencies().FetchID: true}

		for j := i + 1; j < len(children); j++ {
			if consumed[j] {
				continue
			}
			cand := children[j]
			if !isMergeableEntityNode(cand) || !sameEntityGroup(base, cand) {
				continue
			}
			if !dependencyCompatible(cand, group, groupIDs) {
				continue
			}
			group = append(group, cand)
			groupIDs[cand.Item.Fetch.Dependencies().FetchID] = true
			consumed[j] = true
		}

		if len(group) < 2 {
			// Lone provider fetch (or nothing dependency-compatible to merge with):
			// leave it untouched.
			newChildren = append(newChildren, base)
			continue
		}

		merged := m.buildMultiEntityNode(base, group)
		mergedID := merged.Item.Fetch.Dependencies().FetchID
		for _, member := range group[1:] {
			replaceDependsOnFetchID(root, member.Item.Fetch.Dependencies().FetchID, mergedID)
		}
		newChildren = append(newChildren, merged)
	}

	root.ChildNodes = newChildren
}

// isMergeableEntityNode reports whether n is a Single node holding a concrete
// entity fetch that can participate in a merge.
func isMergeableEntityNode(n *resolve.FetchTreeNode) bool {
	if n == nil || n.Kind != resolve.FetchTreeNodeKindSingle || n.Item == nil || n.Item.Fetch == nil {
		return false
	}
	switch n.Item.Fetch.(type) {
	case *resolve.EntityFetch, *resolve.BatchEntityFetch:
		return true
	default:
		return false
	}
}

// sameEntityGroup reports whether two entity fetches may share one upstream
// request: same data source (⇒ same transport/headers) and same defer scope.
func sameEntityGroup(a, b *resolve.FetchTreeNode) bool {
	ia, ib := a.Item.Fetch.FetchInfo(), b.Item.Fetch.FetchInfo()
	if ia == nil || ib == nil {
		return false
	}
	if ia.DataSourceID == "" || ia.DataSourceID != ib.DataSourceID {
		return false
	}
	return a.Item.Fetch.Dependencies().DeferID == b.Item.Fetch.Dependencies().DeferID
}

// dependencyCompatible reports whether cand can run in the same request as the
// current group: neither depends on the other (they must be safe to issue at a
// single point in the execution order).
func dependencyCompatible(cand *resolve.FetchTreeNode, group []*resolve.FetchTreeNode, groupIDs map[int]bool) bool {
	for _, dep := range cand.Item.Fetch.Dependencies().DependsOnFetchIDs {
		if groupIDs[dep] {
			return false
		}
	}
	candID := cand.Item.Fetch.Dependencies().FetchID
	for _, member := range group {
		if slices.Contains(member.Item.Fetch.Dependencies().DependsOnFetchIDs, candID) {
			return false
		}
	}
	return true
}

// entityMember captures the per-member data recovered from a concrete entity
// fetch that is needed to assemble the merged node.
type entityMember struct {
	node      *resolve.FetchTreeNode
	batch     bool
	url       string
	method    string
	query     string
	items     []resolve.InputTemplate
	dataPath  []string
	skipNull  bool
	skipEmpty bool
	skipErr   bool
}

// buildMultiEntityNode replaces base with a Single node holding the merged
// MultiEntityFetch built from group (group[0] == base).
func (m *mergeEntityFetches) buildMultiEntityNode(base *resolve.FetchTreeNode, group []*resolve.FetchTreeNode) *resolve.FetchTreeNode {
	members := make([]entityMember, 0, len(group))
	for _, n := range group {
		members = append(members, recoverEntityMember(n))
	}

	baseFetch := base.Item.Fetch
	baseInfo := baseFetch.FetchInfo()

	// Merged operation: one aliased _entities block per member with isolated
	// per-alias representation variables. Assembled by reprinting each member's
	// planned _entities selection set (robust; no whole-query string surgery).
	varDefs := make([]string, 0, len(members))
	fields := make([]string, 0, len(members))
	subs := make([]*resolve.MultiEntitySubFetch, 0, len(members))

	dependsOn := map[int]bool{}
	var rootFields []resolve.GraphCoordinate
	var coordinateDeps []resolve.FetchDependency

	for i, member := range members {
		alias := "f" + strconv.Itoa(i+1)
		varName := "representations_" + alias

		varDefs = append(varDefs, "$"+varName+": [_Any!]!")
		selectionSet, _ := entitiesSelectionSet(member.query)
		fields = append(fields, alias+": _entities(representations: $"+varName+")"+selectionSet)

		subHeader := `"` + varName + `":[`
		if i > 0 {
			subHeader = "," + subHeader
		}

		sub := &resolve.MultiEntitySubFetch{
			Alias:        alias,
			FetchPath:    member.node.Item.FetchPath,
			ResponsePath: member.node.Item.ResponsePath,
			Batch:        member.batch,
			Input: resolve.BatchInput{
				Header:               staticInputTemplate(subHeader),
				Items:                member.items,
				Separator:            staticInputTemplate(","),
				Footer:               staticInputTemplate("]"),
				SkipNullItems:        member.skipNull,
				SkipEmptyObjectItems: member.skipEmpty,
				SkipErrItems:         member.skipErr,
			},
			PostProcessing: resolve.PostProcessingConfiguration{
				SelectResponseDataPath: aliasDataPath(member.dataPath, alias),
			},
		}
		subs = append(subs, sub)

		for _, id := range member.node.Item.Fetch.Dependencies().DependsOnFetchIDs {
			dependsOn[id] = true
		}
		if info := member.node.Item.Fetch.FetchInfo(); info != nil {
			rootFields = append(rootFields, info.RootFields...)
			coordinateDeps = append(coordinateDeps, info.CoordinateDependencies...)
		}
	}

	mergedQuery := "query(" + strings.Join(varDefs, ", ") + "){" + strings.Join(fields, " ") + "}"

	header := `{"method":"` + members[0].method + `","url":"` + members[0].url +
		`","body":{"query":"` + mergedQuery + `","variables":{`

	mergedFetchID := baseFetch.Dependencies().FetchID
	dependsOnFetchIDs := make([]int, 0, len(dependsOn))
	for id := range dependsOn {
		dependsOnFetchIDs = append(dependsOnFetchIDs, id)
	}
	slices.Sort(dependsOnFetchIDs)

	var info *resolve.FetchInfo
	if baseInfo != nil {
		info = &resolve.FetchInfo{
			DataSourceID:           baseInfo.DataSourceID,
			DataSourceName:         baseInfo.DataSourceName,
			OperationType:          baseInfo.OperationType,
			RootFields:             rootFields,
			CoordinateDependencies: coordinateDeps,
		}
	}

	merged := &resolve.MultiEntityFetch{
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           mergedFetchID,
			DependsOnFetchIDs: dependsOnFetchIDs,
			DeferID:           baseFetch.Dependencies().DeferID,
		},
		Fetches:              subs,
		Header:               staticInputTemplate(header),
		Footer:               staticInputTemplate("}}}"),
		DataSource:           entityDataSource(baseFetch),
		DataSourceIdentifier: entityDataSourceIdentifier(baseFetch),
		Info:                 info,
		PostProcessing: resolve.PostProcessingConfiguration{
			SelectResponseErrorsPath: []string{"errors"},
		},
	}

	base.Item.Fetch = merged
	return base
}

// recoverEntityMember pulls the transport envelope, planned query, representation
// renderer(s), and post-processing out of a concrete entity fetch.
func recoverEntityMember(n *resolve.FetchTreeNode) entityMember {
	member := entityMember{node: n}
	switch f := n.Item.Fetch.(type) {
	case *resolve.EntityFetch:
		member.batch = false
		member.items = []resolve.InputTemplate{f.Input.Item}
		member.dataPath = f.PostProcessing.SelectResponseDataPath
		// A single representation may still render null (null parent); drop it
		// rather than sending a null into the merged batch.
		member.skipNull = true
		member.skipEmpty = true
		member.skipErr = f.Input.SkipErrItem
		member.url, member.method, member.query = recoverRequestEnvelope(f.Input.Header, f.Input.Footer)
	case *resolve.BatchEntityFetch:
		member.batch = true
		member.items = f.Input.Items
		member.dataPath = f.PostProcessing.SelectResponseDataPath
		member.skipNull = f.Input.SkipNullItems
		member.skipEmpty = f.Input.SkipEmptyObjectItems
		member.skipErr = f.Input.SkipErrItems
		member.url, member.method, member.query = recoverRequestEnvelope(f.Input.Header, f.Input.Footer)
	}
	return member
}

func entityDataSource(f resolve.Fetch) resolve.DataSource {
	switch v := f.(type) {
	case *resolve.EntityFetch:
		return v.DataSource
	case *resolve.BatchEntityFetch:
		return v.DataSource
	}
	return nil
}

func entityDataSourceIdentifier(f resolve.Fetch) []byte {
	switch v := f.(type) {
	case *resolve.EntityFetch:
		return v.DataSourceIdentifier
	case *resolve.BatchEntityFetch:
		return v.DataSourceIdentifier
	}
	return nil
}

// recoverRequestEnvelope concatenates the static Header and Footer segments of an
// entity fetch (whose only dynamic part is the representations variable in
// between) into the full request JSON with an empty representations list, then
// unmarshals it to recover the url, method, and planned query.
func recoverRequestEnvelope(header, footer resolve.InputTemplate) (url, method, query string) {
	var sb strings.Builder
	writeStaticSegments(&sb, header)
	writeStaticSegments(&sb, footer)

	var envelope struct {
		Method string `json:"method"`
		URL    string `json:"url"`
		Body   struct {
			Query string `json:"query"`
		} `json:"body"`
	}
	if err := json.Unmarshal([]byte(sb.String()), &envelope); err != nil {
		return "", "", ""
	}
	return envelope.URL, envelope.Method, envelope.Body.Query
}

func writeStaticSegments(sb *strings.Builder, tpl resolve.InputTemplate) {
	for _, seg := range tpl.Segments {
		if seg.SegmentType == resolve.StaticSegmentType {
			sb.Write(seg.Data)
		}
	}
}

// aliasDataPath rewrites a SelectResponseDataPath that selects _entities to select
// the aliased block instead, e.g. ["data","_entities"] -> ["data","f1"] and
// ["data","_entities","0"] -> ["data","f1","0"].
func aliasDataPath(path []string, alias string) []string {
	out := make([]string, len(path))
	copy(out, path)
	for i := range out {
		if out[i] == "_entities" {
			out[i] = alias
		}
	}
	return out
}

// entitiesSelectionSet extracts the _entities root-field selection set (including
// its enclosing braces) from a planned entities query such as
// `query($representations: [_Any!]!){_entities(representations: $representations){... on Employee {__typename products}}}`.
// The scan is quote-aware so string literals inside the selection cannot unbalance
// the braces.
func entitiesSelectionSet(query string) (string, bool) {
	idx := strings.Index(query, "_entities")
	if idx < 0 {
		return "", false
	}
	i := idx + len("_entities")
	// Skip the arguments list (which may itself contain braces in object values).
	for i < len(query) && query[i] != '{' {
		if query[i] == '(' {
			i = skipBalanced(query, i, '(', ')')
			continue
		}
		i++
	}
	if i >= len(query) || query[i] != '{' {
		return "", false
	}
	end := skipBalanced(query, i, '{', '}')
	if end <= i {
		return "", false
	}
	return query[i:end], true
}

// skipBalanced returns the index just past the balanced group that opens at
// query[open] (which must equal openCh). It is quote-aware. On imbalance it
// returns len(query).
func skipBalanced(query string, open int, openCh, closeCh byte) int {
	depth := 0
	inStr := false
	for i := open; i < len(query); i++ {
		c := query[i]
		if inStr {
			if c == '\\' {
				i++
				continue
			}
			if c == '"' {
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case openCh:
			depth++
		case closeCh:
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(query)
}

func staticInputTemplate(data string) resolve.InputTemplate {
	return resolve.InputTemplate{
		Segments: []resolve.TemplateSegment{
			{
				Data:        []byte(data),
				SegmentType: resolve.StaticSegmentType,
			},
		},
	}
}
