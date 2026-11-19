package resolve

import (
	"context"
	"encoding/binary"
	"io"
	"slices"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
	"golang.org/x/sync/errgroup"
)

// ContextSource describes how to read a hidden argument from the ancestor at Depth.
type ContextSource struct {
	// An operation root has a statically known __typename, even when the subgraph never selects it.
	RootTypeName string
	Types        []string
	Depth        int
	Selection    []ContextSelection
}

type ContextSelection struct {
	Field    string
	Types    []string
	Children []ContextSelection
}

func (s ContextSource) Equal(other ContextSource) bool {
	return s.Depth == other.Depth && s.RootTypeName == other.RootTypeName &&
		slices.Equal(s.Types, other.Types) && slices.EqualFunc(s.Selection, other.Selection, ContextSelection.equal)
}

func (s ContextSelection) equal(other ContextSelection) bool {
	return s.Field == other.Field && slices.Equal(s.Types, other.Types) &&
		slices.EqualFunc(s.Children, other.Children, ContextSelection.equal)
}

type ContextBinding struct {
	Variable string
	Sources  []ContextSource
}

func (b *ContextBinding) GetVariableKind() VariableKind { return AncestorVariableKind }
func (b *ContextBinding) Equals(other Variable) bool {
	next, ok := other.(*ContextBinding)
	if !ok || b == nil || next == nil {
		return ok && b == next
	}
	return b.Variable == next.Variable && slices.EqualFunc(b.Sources, next.Sources, ContextSource.Equal)
}
func (b *ContextBinding) TemplateSegment() TemplateSegment {
	return TemplateSegment{SegmentType: VariableSegmentType, VariableKind: AncestorVariableKind, VariableSourcePath: []string{b.Variable}, Renderer: b}
}
func (b *ContextBinding) GetKind() string { return VariableRendererKindJson }
func (b *ContextBinding) RenderVariable(_ context.Context, value *astjson.Value, out io.Writer) error {
	_, err := out.Write(value.MarshalTo(nil))
	return err
}

func fetchContextBindings(fetch Fetch) (out []ContextBinding) {
	var header, footer *InputTemplate
	switch f := fetch.(type) {
	case *BatchEntityFetch:
		header, footer = &f.Input.Header, &f.Input.Footer
	case *EntityFetch:
		header, footer = &f.Input.Header, &f.Input.Footer
	default:
		return nil
	}
	for _, t := range []*InputTemplate{header, footer} {
		for _, s := range t.Segments {
			if b, ok := s.Renderer.(*ContextBinding); ok {
				out = append(out, *b)
			}
		}
	}
	return
}

func (db *DataBuffer) addContextPath(path []FetchItemPathElement) {
	var fields []string
	for _, element := range path {
		fields = append(fields, element.Path...)
	}
	db.contextFetchPaths = append(db.contextFetchPaths, fields)
}

// Walk in execution order. An ordinary fetch can depend on contextual data
// through @requires and write a different value on an ancestor or sibling.
func (db *DataBuffer) collectContextPaths(node *FetchTreeNode, dependent map[int]bool) {
	if node == nil {
		return
	}
	if item := node.Item; item != nil && item.Fetch != nil {
		db.collectContextWrite(item, dependent)
	}
	for _, child := range node.ChildNodes {
		db.collectContextPaths(child, dependent)
	}
}

func (db *DataBuffer) collectContextWrite(item *FetchItem, dependent map[int]bool) {
	deps := item.Fetch.Dependencies()
	if len(fetchContextBindings(item.Fetch)) == 0 {
		if deps == nil || !slices.ContainsFunc(deps.DependsOnFetchIDs, func(id int) bool { return dependent[id] }) {
			return
		}
	}
	if deps != nil {
		dependent[deps.FetchID] = true
	}
	if multi, ok := item.Fetch.(*MultiEntityFetch); ok {
		for _, id := range multi.MergedFetchIDs {
			dependent[id] = true
		}
		for _, entry := range multi.Input.Entries {
			db.addContextPath(entry.Item.FetchPath)
		}
		return
	}
	db.addContextPath(item.FetchPath)
}

func (db *DataBuffer) collectDeferredContextPaths(node *DeferTreeNode, dependent map[int]bool) {
	if node == nil {
		return
	}
	if node.Item != nil {
		db.collectContextPaths(node.Item.Fetches, dependent)
	}
	for _, child := range node.ChildNodes {
		db.collectDeferredContextPaths(child, dependent)
	}
}

// Called under the merge lock. Only overlapping subtrees can receive different
// writes for repeated representations. Ignore list wrappers and type conditions
// when comparing paths, so all runtime variants of a field remain isolated.
func (db *DataBuffer) batchTargetsNeedIsolation(fetchPath []FetchItemPathElement, mergePath []string) bool {
	if db.fetchTreeToCheck != nil || db.deferTreeToCheck != nil {
		db.contextFetchPaths = nil
		dependent := make(map[int]bool)
		db.collectContextPaths(db.fetchTreeToCheck, dependent)
		db.collectDeferredContextPaths(db.deferTreeToCheck, dependent)
		db.fetchTreeToCheck = nil
		db.deferTreeToCheck = nil
		db.contextPathsKnown = true
	}
	if !db.contextPathsKnown {
		return true
	}
	for _, path := range db.contextFetchPaths {
		if mergeOverlapsContext(fetchPath, mergePath, path) {
			return true
		}
	}
	return false
}

// Either the contextual fetch or the merge can be the ancestor. Compare the
// full merge destination field by field without allocating a combined path.
func mergeOverlapsContext(fetchPath []FetchItemPathElement, mergePath, contextPath []string) bool {
	matches := func(fields []string) bool {
		common := min(len(fields), len(contextPath))
		if !slices.Equal(fields[:common], contextPath[:common]) {
			return false
		}
		contextPath = contextPath[common:]
		return true
	}
	for _, element := range fetchPath {
		if !matches(element.Path) {
			return false
		}
	}
	return matches(mergePath)
}

// The entity and all its targets already belong to the response arena. Scalar
// values can be shared because subsequent merges replace them in their parent.
func copyEntityContainers(a arena.Arena, value *astjson.Value) *astjson.Value {
	switch value.Type() {
	case astjson.TypeObject:
		out := astjson.ObjectValue(a)
		value.GetObject().Visit(func(key []byte, field *astjson.Value) {
			out.Set(a, string(key), copyEntityContainers(a, field))
		})
		return out
	case astjson.TypeArray:
		out := astjson.ArrayValue(a)
		for i, element := range value.GetArray() {
			out.SetArrayItem(a, i, copyEntityContainers(a, element))
		}
		return out
	default:
		return value
	}
}

type contextGroup struct {
	variables *astjson.Value
	items     []*astjson.Value
}

func (l *Loader) resolveContextGroups(ctx context.Context, item *FetchItem, bindings []ContextBinding) error {
	l.dataBuffer.Lock()
	if l.shouldSkipErroredDependencyLocked(item) {
		l.dataBuffer.Unlock()
		return nil
	}
	groups := l.contextGroups(item.FetchPath, bindings)
	l.dataBuffer.Unlock()
	// Each distinct ancestor binding can require a separate subgraph request.
	// Bound per-fetch fan-out even when every item in a large list has a different binding.
	const maxConcurrentContextGroups = 8
	var loads errgroup.Group
	if len(groups) > 1 {
		loads.SetLimit(maxConcurrentContextGroups)
	}
	for _, group := range groups {
		l.dataBuffer.Lock()
		prepared, err := l.prepareItems(item, group.items, group.variables)
		l.dataBuffer.Unlock()
		if len(groups) == 1 {
			return l.executePrepared(ctx, prepared, err)
		}
		loads.Go(func() error { return l.executePrepared(ctx, prepared, err) })
	}
	return loads.Wait()
}

func (b *ContextBinding) sourceFor(ancestors []*astjson.Value) (*ContextSource, *astjson.Value) {
	for i := range b.Sources {
		source := &b.Sources[i]
		if source.Depth < 0 || source.Depth >= len(ancestors) {
			continue
		}
		ancestor := ancestors[source.Depth]
		if len(source.Types) == 0 || slices.Contains(source.Types, string(ancestor.GetStringBytes("__typename"))) {
			return source, ancestor
		}
	}
	return nil, nil
}

// A depth-first walk visits the receivers of an ancestor consecutively. Keep
// the last projection per argument so a changing inner binding doesn't cause
// a shared outer list to be projected and serialized again.
type contextProjection struct {
	source   *ContextSource
	ancestor *astjson.Value
	value    *astjson.Value
	id       uint64

	valueIDs map[string]uint64
	encoded  []byte
}

func (p *contextProjection) read(l *Loader, binding *ContextBinding, ancestors []*astjson.Value) {
	source, ancestor := binding.sourceFor(ancestors)
	if p.value != nil && p.source == source && p.ancestor == ancestor {
		return
	}
	p.source, p.ancestor = source, ancestor
	p.value, p.id = astjson.NullValue, 0
	if source == nil {
		return
	}
	if source.RootTypeName != "" {
		p.value = astjson.StringValue(l.jsonArena, source.RootTypeName)
	} else if len(source.Selection) > 0 {
		p.value = l.contextValue(ancestor, source.Selection)
	}
	if p.value.Type() == astjson.TypeNull {
		return
	}

	// Equal values on different ancestors belong to the same group. Intern the
	// encoded value once and use the small ID in each receiver's argument tuple.
	p.encoded = p.value.MarshalTo(p.encoded[:0])
	if p.id = p.valueIDs[string(p.encoded)]; p.id == 0 {
		if p.valueIDs == nil {
			p.valueIDs = make(map[string]uint64)
		}
		p.id = uint64(len(p.valueIDs) + 1)
		p.valueIDs[string(p.encoded)] = p.id
	}
}

func (l *Loader) contextGroups(path []FetchItemPathElement, bindings []ContextBinding) []contextGroup {
	projections := make([]contextProjection, len(bindings))
	byValues := make(map[string]int)
	var groups []contextGroup
	var tuple []byte
	var visit func(int, []*astjson.Value)
	visit = func(i int, ancestors []*astjson.Value) {
		obj := ancestors[len(ancestors)-1]
		if obj == nil || obj.Type() == astjson.TypeNull {
			return
		}
		if i == len(path) {
			if len(l.taintedObjs) != 0 && l.taintedObjs.isTainted(obj, 0) {
				return
			}
			tuple = tuple[:0]
			for i := range bindings {
				projection := &projections[i]
				projection.read(l, &bindings[i], ancestors)
				tuple = binary.AppendUvarint(tuple, projection.id)
			}
			idx, ok := byValues[string(tuple)]
			if !ok {
				idx = len(groups)
				byValues[string(tuple)] = idx
				vars := astjson.ObjectValue(l.jsonArena)
				for i := range bindings {
					vars.Set(l.jsonArena, bindings[i].Variable, projections[i].value)
				}
				groups = append(groups, contextGroup{variables: vars})
			}
			groups[idx].items = append(groups[idx].items, obj)
			return
		}
		el := path[i]
		if !isItemAllowedByTypename(obj, el.TypeNames) {
			return
		}
		var descend func(int, []*astjson.Value)
		descend = func(j int, chain []*astjson.Value) {
			if j == len(el.Path) {
				visit(i+1, chain)
				return
			}
			next := chain[len(chain)-1].Get(el.Path[j])
			if next == nil {
				return
			}
			if next.Type() == astjson.TypeArray {
				for _, child := range next.GetArray() {
					descend(j+1, append(chain, child))
				}
				return
			}
			descend(j+1, append(chain, next))
		}
		descend(0, ancestors)
	}
	visit(0, []*astjson.Value{l.dataBuffer.Get()})
	return groups
}

func (l *Loader) contextValue(obj *astjson.Value, selection []ContextSelection) *astjson.Value {
	if obj == nil || obj.Type() == astjson.TypeNull {
		return astjson.NullValue
	}
	if len(selection) == 0 {
		return obj
	}
	if obj.Type() == astjson.TypeArray {
		result := astjson.ArrayValue(l.jsonArena)
		for i, v := range obj.GetArray() {
			result.SetArrayItem(l.jsonArena, i, l.contextValue(v, selection))
		}
		return result
	}
	for _, step := range selection {
		if step.Field != "" {
			return l.contextValue(obj.Get(step.Field), step.Children)
		}
		if slices.Contains(step.Types, string(obj.GetStringBytes("__typename"))) {
			return l.contextValue(obj, step.Children)
		}
	}
	return astjson.NullValue
}
