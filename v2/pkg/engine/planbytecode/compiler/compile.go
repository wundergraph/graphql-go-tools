package compiler

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	engineplan "github.com/wundergraph/graphql-go-tools/v2/pkg/engine/plan"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/planbytecode"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type Compiler struct {
	program    planbytecode.Program
	stringRefs map[string]uint32
	pathRefs   map[string]uint32
}

func Compile(plan plan.Plan) (*planbytecode.Program, error) {
	compiler := Compiler{
		stringRefs: make(map[string]uint32),
		pathRefs:   make(map[string]uint32),
	}

	return compiler.Compile(plan)
}

func (compiler *Compiler) Compile(
	plan plan.Plan,
) (*planbytecode.Program, error) {
	if plan == nil {
		return nil, errors.New(
			"compile bytecode plan: nil plan",
		)
	}

	switch typed := plan.(type) {
	case *engineplan.SynchronousResponsePlan:
		if typed.Response == nil {
			return nil, errors.New(
				"compile bytecode plan: nil response",
			)
		}

		compiler.compileGraphQLResponse(typed.Response)
	case *engineplan.SubscriptionResponsePlan:
		compiler.unsupported(
			"subscription",
			"subscriptions remain on the interpreted resolver path",
		)
	default:
		compiler.unsupported(
			"plan_kind",
			fmt.Sprintf("unsupported plan type %T",
				plan,
			),
		)
	}

	return &compiler.program, nil
}

func (compiler *Compiler) compileGraphQLResponse(
	response *resolve.GraphQLResponse,
) {
	compiler.compileFetchTree(response.Fetches)
	compiler.compileDirectResponse(response)
	compiler.compileNode(response.Data)
	compiler.emit(planbytecode.OpEmitResponse, 0, 0, 0)
}

func (compiler *Compiler) compileDirectResponse(
	response *resolve.GraphQLResponse,
) {
	if response == nil || response.Data == nil {
		return
	}

	if !directFetchTreeEligible(response.Fetches) {
		return
	}

	if len(response.Data.Path) != 0 ||
		len(response.Data.PossibleTypes) != 0 ||
		response.Data.Nullable {
		return
	}

	fields := make([]planbytecode.DirectField, 0, len(response.Data.Fields))

	for _, field := range response.Data.Fields {
		direct, ok := compiler.compileDirectField(field)
		if !ok {
			return
		}
		fields = append(fields, direct)
	}

	if len(fields) == 0 {
		return
	}

	compiler.program.DirectResponse = &planbytecode.DirectResponse{
		Fields: fields,
	}
}

func (compiler *Compiler) compileDirectField(
	field *resolve.Field,
) (planbytecode.DirectField, bool) {
	if field == nil ||
		field.Value == nil ||
		field.Defer != nil ||
		field.Stream != nil ||
		len(field.OnTypeNames) != 0 ||
		len(field.ParentOnTypeNames) != 0 ||
		(field.Info != nil && field.Info.HasAuthorizationRule) {
		return planbytecode.DirectField{}, false
	}

	name := string(field.Name)
	switch typed := field.Value.(type) {
	case *resolve.StaticString:
		return planbytecode.DirectField{
			NameRef:    compiler.stringRef(name),
			LiteralRef: compiler.stringRef(typed.Value),
			Flags: planbytecode.EncodeDirectFieldFlags(
				uint32(resolve.NodeKindStaticString), false, true,
			),
		}, true
	case *resolve.Null:
		return planbytecode.DirectField{
			NameRef: compiler.stringRef(name),
			Flags: planbytecode.EncodeDirectFieldFlags(
				uint32(resolve.NodeKindNull), true, true,
			),
		}, true
	case *resolve.Object:
		if !directFieldPathMatches(name, typed.Path) ||
			len(typed.PossibleTypes) != 0 {
			return planbytecode.DirectField{}, false
		}
		children, ok := compiler.compileDirectFields(typed.Fields)
		if !ok {
			return planbytecode.DirectField{}, false
		}
		return planbytecode.DirectField{
			NameRef: compiler.stringRef(name),
			PathRef: compiler.pathRef(typed.Path),
			Flags: planbytecode.EncodeDirectFieldFlags(
				uint32(resolve.NodeKindObject), typed.Nullable, false,
			),
			Children: children,
		}, true
	case *resolve.Array:
		if typed.SkipItem != nil || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
		itemFlags, children, ok := compiler.compileDirectArrayItem(typed.Item)
		if !ok {
			return planbytecode.DirectField{}, false
		}
		return planbytecode.DirectField{
			NameRef: compiler.stringRef(name),
			PathRef: compiler.pathRef(typed.Path),
			Flags: planbytecode.EncodeDirectFieldFlags(
				uint32(resolve.NodeKindArray), typed.Nullable, false,
			),
			ItemFlags: itemFlags,
			Children:  children,
		}, true
	case *resolve.String:
		if typed.Export != nil || typed.UnescapeResponseJson || typed.IsTypeName || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
	case *resolve.Boolean:
		if typed.Export != nil || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
	case *resolve.Integer:
		if typed.Export != nil || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
	case *resolve.Float:
		if typed.Export != nil || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
	case *resolve.BigInt:
		if typed.Export != nil || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
	case *resolve.Scalar:
		if typed.Export != nil || !directFieldPathMatches(name, typed.Path) {
			return planbytecode.DirectField{}, false
		}
	default:
		return planbytecode.DirectField{}, false
	}

	return planbytecode.DirectField{
		NameRef: compiler.stringRef(name),
		PathRef: compiler.pathRef(field.Value.NodePath()),
		Flags: planbytecode.EncodeDirectFieldFlags(
			uint32(field.Value.NodeKind()), field.Value.NodeNullable(), false,
		),
	}, true
}

func (compiler *Compiler) compileDirectFields(
	fields []*resolve.Field,
) ([]planbytecode.DirectField, bool) {
	out := make([]planbytecode.DirectField, 0, len(fields))
	for _, field := range fields {
		direct, ok := compiler.compileDirectField(field)
		if !ok {
			return nil, false
		}
		out = append(out, direct)
	}
	return out, true
}

func (compiler *Compiler) compileDirectArrayItem(
	item resolve.Node,
) (uint32, []planbytecode.DirectField, bool) {
	switch typed := item.(type) {
	case *resolve.Object:
		if len(typed.Path) != 0 ||
			len(typed.PossibleTypes) != 0 {
			return 0, nil, false
		}

		children, ok := compiler.compileDirectFields(typed.Fields)

		if !ok {
			return 0, nil, false
		}

		return planbytecode.EncodeDirectFieldFlags(
			uint32(resolve.NodeKindObject), typed.Nullable, false,
		), children, true
	case *resolve.String:
		if typed.Export != nil || typed.UnescapeResponseJson || typed.IsTypeName || len(typed.Path) != 0 {
			return 0, nil, false
		}
	case *resolve.Boolean:
		if typed.Export != nil || len(typed.Path) != 0 {
			return 0, nil, false
		}
	case *resolve.Integer:
		if typed.Export != nil || len(typed.Path) != 0 {
			return 0, nil, false
		}
	case *resolve.Float:
		if typed.Export != nil || len(typed.Path) != 0 {
			return 0, nil, false
		}
	case *resolve.BigInt:
		if typed.Export != nil || len(typed.Path) != 0 {
			return 0, nil, false
		}
	case *resolve.Scalar:
		if typed.Export != nil || len(typed.Path) != 0 {
			return 0, nil, false
		}
	default:
		return 0, nil, false
	}

	return planbytecode.EncodeDirectFieldFlags(
		uint32(item.NodeKind()), item.NodeNullable(), false,
	), nil, true
}

func directFieldPathMatches(name string, path []string) bool {
	return len(path) == 1 && path[0] == name
}

func directFetchTreeEligible(node *resolve.FetchTreeNode) bool {
	var dataAvailable bool
	return directFetchTreeEligibleWithState(node, &dataAvailable)
}

func directFetchTreeEligibleWithState(node *resolve.FetchTreeNode, dataAvailable *bool) bool {
	if node == nil {
		return false
	}
	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if node.Item == nil || node.Item.Fetch == nil {
			return false
		}
		if directFetchItemRequiresParentData(node.Item) && !*dataAvailable {
			return false
		}
		if !directFetchItemEligible(node.Item) {
			return false
		}
		*dataAvailable = true
		return true
	case resolve.FetchTreeNodeKindSequence, resolve.FetchTreeNodeKindParallel:
		if len(node.ChildNodes) == 0 {
			return false
		}
		if node.Kind == resolve.FetchTreeNodeKindParallel {
			availableBeforeParallel := *dataAvailable
			anyChildProducesData := false
			for _, child := range node.ChildNodes {
				childDataAvailable := availableBeforeParallel
				if !directFetchTreeEligibleWithState(child, &childDataAvailable) {
					return false
				}
				anyChildProducesData = anyChildProducesData || childDataAvailable
			}
			*dataAvailable = *dataAvailable || anyChildProducesData
			return true
		}
		for _, child := range node.ChildNodes {
			if !directFetchTreeEligibleWithState(child, dataAvailable) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func directFetchItemRequiresParentData(item *resolve.FetchItem) bool {
	return item != nil && len(item.FetchPath) != 0
}

func directFetchItemEligible(item *resolve.FetchItem) bool {
	switch item.Fetch.(type) {
	case *resolve.SingleFetch:
		return len(item.FetchPath) == 0
	case *resolve.EntityFetch:
		return directObjectFetchPathEligible(item.FetchPath)
	case *resolve.BatchEntityFetch:
		return directArrayFetchPathEligible(item.FetchPath)
	default:
		return false
	}
}

func directObjectFetchPathEligible(path []resolve.FetchItemPathElement) bool {
	if !directFetchPathEligible(path) ||
		path[len(path)-1].Kind != resolve.FetchItemPathElementKindObject {
		return false
	}
	return true
}

func directArrayFetchPathEligible(path []resolve.FetchItemPathElement) bool {
	if !directFetchPathEligible(path) {
		return false
	}
	for i := range path {
		if path[i].Kind == resolve.FetchItemPathElementKindArray {
			return true
		}
	}
	return false
}

func directFetchPathEligible(path []resolve.FetchItemPathElement) bool {
	if len(path) == 0 {
		return false
	}
	for i := range path {
		if len(path[i].Path) != 1 || len(path[i].TypeNames) != 0 {
			return false
		}
		switch path[i].Kind {
		case resolve.FetchItemPathElementKindObject, resolve.FetchItemPathElementKindArray:
		default:
			return false
		}
	}
	return true
}

func (compiler *Compiler) compileFetchTree(node *resolve.FetchTreeNode) {
	if node == nil {
		return
	}

	switch node.Kind {
	case resolve.FetchTreeNodeKindSingle:
		if node.Item == nil || node.Item.Fetch == nil {
			return
		}

		if compiler.fetchSelectionSetIsEmpty(node.Item.Fetch) {
			compiler.program.Stats.DCEFetches++
			return
		}

		fetchRef := compiler.addFetch(node.Item)
		compiler.emit(planbytecode.OpFetchSubgraph, fetchRef, uint32(node.Item.Fetch.FetchKind()), 0)
		compiler.emit(planbytecode.OpPasteAtPointer, fetchRef, compiler.pathRef(postProcessing(node.Item.Fetch).SelectResponseDataPath), compiler.pathRef(postProcessing(node.Item.Fetch).MergePath))
	case resolve.FetchTreeNodeKindSequence:
		opRef := compiler.emit(planbytecode.OpEnterSequence, 0, 0, 0)
		var emittedChildren uint32

		for _, child := range node.ChildNodes {
			before := len(compiler.program.Ops)
			compiler.compileFetchTree(child)

			if len(compiler.program.Ops) != before {
				emittedChildren++
			}
		}

		compiler.program.Ops[opRef].A = emittedChildren
		leaveRef := compiler.emit(planbytecode.OpLeaveSequence, 0, 0, 0)
		compiler.program.Ops[opRef].B = uint32(leaveRef)
	case resolve.FetchTreeNodeKindParallel:
		opRef := compiler.emit(planbytecode.OpEnterParallel, 0, 0, 0)
		var emittedChildren uint32

		for _, child := range node.ChildNodes {
			if child == nil || child.Kind != resolve.FetchTreeNodeKindSingle {
				compiler.unsupported("nested_parallel", "bytecode interpreter only supports direct fetch children in parallel groups")
			}

			before := len(compiler.program.Ops)
			compiler.compileFetchTree(child)

			if len(compiler.program.Ops) != before {
				emittedChildren++
			}
		}

		compiler.program.Ops[opRef].A = emittedChildren
		leaveRef := compiler.emit(planbytecode.OpLeaveParallel, 0, 0, 0)
		compiler.program.Ops[opRef].B = uint32(leaveRef)
	case resolve.FetchTreeNodeKindTrigger:
		compiler.unsupported("trigger_fetch", "trigger fetches are subscription-only")
	default:
		compiler.unsupported("fetch_tree", fmt.Sprintf("unsupported fetch tree node kind %q", node.Kind))
	}
}

func (compiler *Compiler) compileNode(node resolve.Node) {
	if node == nil {
		return
	}

	compiler.program.Stats.ResponseNodes++

	switch typed := node.(type) {
	case *resolve.Object:
		compiler.program.Stats.Objects++
		compiler.emit(planbytecode.OpEnterObject, compiler.pathRef(typed.Path), uint32(len(typed.Fields)), boolOperand(typed.Nullable))

		for _, field := range typed.Fields {
			compiler.compileField(field)
		}

		compiler.emit(planbytecode.OpLeaveObject, 0, 0, 0)
	case *resolve.EmptyObject:
		compiler.program.Stats.Objects++
		compiler.emit(planbytecode.OpEnterObject, compiler.pathRef(nil), 0, 0)
		compiler.emit(planbytecode.OpLeaveObject, 0, 0, 0)
	case *resolve.Array:
		compiler.program.Stats.Arrays++
		compiler.emit(planbytecode.OpEnterArray, compiler.pathRef(typed.Path), 0, boolOperand(typed.Nullable))

		if typed.SkipItem != nil {
			compiler.unsupported("array_skip_item", "array item predicate requires interpreted resolver")
		}

		compiler.compileNode(typed.Item)
		compiler.emit(planbytecode.OpLeaveArray, 0, 0, 0)
	case *resolve.EmptyArray:
		compiler.program.Stats.Arrays++
		compiler.emit(planbytecode.OpEnterArray, compiler.pathRef(nil), 0, 0)
		compiler.emit(planbytecode.OpLeaveArray, 0, 0, 0)
	case *resolve.StaticString:
		compiler.program.Stats.Literals++
		compiler.emit(planbytecode.OpEmitLiteral, compiler.stringRef(typed.Value), compiler.pathRef(typed.Path), uint32(node.NodeKind()))
	case *resolve.CustomNode:
		compiler.unsupported("custom_node", "custom field resolver requires interpreted resolver")
		compiler.emit(planbytecode.OpProjectField, 0, compiler.pathRef(typed.Path), encodeNodeFlags(node.NodeKind(), typed.Nullable))
	default:
		compiler.emit(planbytecode.OpProjectField, 0, compiler.pathRef(node.NodePath()), encodeNodeFlags(node.NodeKind(), node.NodeNullable()))
	}
}

func (compiler *Compiler) compileField(field *resolve.Field) {
	if field == nil {
		return
	}
	if field.Value == nil {
		compiler.unsupported("nil_field_value", "field has no response value node")
		return
	}
	compiler.program.Stats.Fields++
	if field.Defer != nil {
		compiler.unsupported("defer", "@defer fields remain on the interpreted resolver path")
	}
	if field.Stream != nil {
		compiler.unsupported("stream", "@stream fields remain on the interpreted resolver path")
	}
	if len(field.OnTypeNames) != 0 || len(field.ParentOnTypeNames) != 0 {
		compiler.unsupported("abstract_type_guard", "runtime typename guards require interpreted resolver")
	}
	if field.Info != nil && field.Info.HasAuthorizationRule {
		compiler.unsupported("authorization", "field authorization can skip fields at runtime")
	}

	compiler.emit(planbytecode.OpProjectField, compiler.stringRef(string(field.Name)), compiler.pathRef(field.Value.NodePath()), encodeNodeFlags(field.Value.NodeKind(), field.Value.NodeNullable()))
	compiler.compileNestedNode(field.Value)
}

func (compiler *Compiler) compileNestedNode(node resolve.Node) {
	switch node.(type) {
	case *resolve.Object, *resolve.EmptyObject, *resolve.Array, *resolve.EmptyArray, *resolve.StaticString, *resolve.CustomNode:
		compiler.compileNode(node)
	}
}

func (compiler *Compiler) addFetch(item *resolve.FetchItem) uint32 {
	post := postProcessing(item.Fetch)
	info := item.Fetch.FetchInfo()
	fetch := planbytecode.Fetch{
		Kind:                        uint32(item.Fetch.FetchKind()),
		ResponsePathRef:             compiler.stringRef(item.ResponsePath),
		SelectResponseDataPathRef:   compiler.pathRef(post.SelectResponseDataPath),
		SelectResponseErrorsPathRef: compiler.pathRef(post.SelectResponseErrorsPath),
		MergePathRef:                compiler.pathRef(post.MergePath),
		Item:                        item,
	}

	if deps := item.Fetch.Dependencies(); deps != nil {
		fetch.DependsOnFetchIDs = deps.DependsOnFetchIDs
	}

	if info != nil {
		fetch.DataSourceIDRef = compiler.stringRef(info.DataSourceID)
		fetch.DataSourceNameRef = compiler.stringRef(info.DataSourceName)
	}

	compiler.program.Fetches = append(compiler.program.Fetches, fetch)
	compiler.program.Stats.Fetches++

	return uint32(len(compiler.program.Fetches) - 1)
}

func (compiler *Compiler) fetchSelectionSetIsEmpty(fetch resolve.Fetch) bool {
	query := fetchQuery(fetch)
	return query != "" && graphQLSelectionSetIsEmpty(query)
}

func (compiler *Compiler) emit(code planbytecode.Opcode, a, b, d uint32) int {
	compiler.program.Ops = append(compiler.program.Ops, planbytecode.Op{Code: code, A: a, B: b, C: d})
	return len(compiler.program.Ops) - 1
}

func (compiler *Compiler) unsupported(feature, reason string) {
	compiler.program.Unsupported = append(
		compiler.program.Unsupported, planbytecode.UnsupportedFeature{
			Feature: feature,
			Reason:  reason,
		},
	)

	compiler.program.Stats.UnsupportedOps++
}

func (compiler *Compiler) stringRef(value string) uint32 {
	if ref, ok := compiler.stringRefs[value]; ok {
		return ref
	}

	ref := uint32(len(compiler.program.Strings))
	compiler.stringRefs[value] = ref
	compiler.program.Strings = append(compiler.program.Strings, value)

	compiler.program.QuotedStrings = append(
		compiler.program.QuotedStrings, strconv.Quote(value),
	)

	return ref
}

func (compiler *Compiler) pathRef(path []string) uint32 {
	keyBytes, _ := json.Marshal(path)
	key := string(keyBytes)

	if ref, ok := compiler.pathRefs[key]; ok {
		return ref
	}

	ref := uint32(len(compiler.program.Paths))
	compiler.pathRefs[key] = ref
	copied := append([]string(nil), path...)
	compiler.program.Paths = append(compiler.program.Paths, copied)

	return ref
}

func boolOperand(v bool) uint32 {
	if v {
		return 1
	}

	return 0
}

func encodeNodeFlags(kind resolve.NodeKind, nullable bool) uint32 {
	out := uint32(kind)

	if nullable {
		out |= 1 << 16
	}

	return out
}

func postProcessing(fetch resolve.Fetch) resolve.PostProcessingConfiguration {
	switch typed := fetch.(type) {
	case *resolve.SingleFetch:
		return typed.PostProcessing
	case *resolve.EntityFetch:
		return typed.PostProcessing
	case *resolve.BatchEntityFetch:
		return typed.PostProcessing
	default:
		return resolve.PostProcessingConfiguration{}
	}
}

func fetchQuery(fetch resolve.Fetch) string {
	if info := fetch.FetchInfo(); info != nil && info.QueryPlan != nil && info.QueryPlan.Query != "" {
		return info.QueryPlan.Query
	}

	switch typed := fetch.(type) {
	case *resolve.SingleFetch:
		if typed.QueryPlan != nil && typed.QueryPlan.Query != "" {
			return typed.QueryPlan.Query
		}

		return queryFromJSONInput(typed.Input)
	case *resolve.EntityFetch:
		return queryFromTemplates(typed.Input.Header, typed.Input.Item, typed.Input.Footer)
	case *resolve.BatchEntityFetch:
		templates := []resolve.InputTemplate{typed.Input.Header}
		templates = append(templates, typed.Input.Items...)
		templates = append(templates, typed.Input.Footer)

		return queryFromTemplates(templates...)
	default:
		return ""
	}
}

func queryFromTemplates(templates ...resolve.InputTemplate) string {
	for _, tmpl := range templates {
		for _, segment := range tmpl.Segments {
			if segment.SegmentType == resolve.StaticSegmentType {
				if query := queryFromJSONInput(string(segment.Data)); query != "" {
					return query
				}
			}
		}
	}

	return ""
}

func queryFromJSONInput(input string) string {
	if input == "" {
		return ""
	}

	var value any

	if err := json.Unmarshal([]byte(input), &value); err != nil {
		return ""
	}

	return findQueryString(value)
}

func findQueryString(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		if query, ok := typed["query"].(string); ok {
			return query
		}

		for _, child := range typed {
			if query := findQueryString(child); query != "" {
				return query
			}
		}
	case []any:
		for _, child := range typed {
			if query := findQueryString(child); query != "" {
				return query
			}
		}
	}
	return ""
}

func graphQLSelectionSetIsEmpty(query string) bool {
	doc, report := astparser.ParseGraphqlDocumentString(query)
	if report.HasErrors() {
		return false
	}

	for i := range doc.OperationDefinitions {
		op := doc.OperationDefinitions[i]

		if op.HasSelections && !selectionSetIsEmpty(&doc, op.SelectionSet) {
			return false
		}
	}

	return true
}

func selectionSetIsEmpty(doc *ast.Document, selectionSetRef int) bool {
	for _, selectionRef := range doc.SelectionSets[selectionSetRef].SelectionRefs {
		selection := doc.Selections[selectionRef]

		switch selection.Kind {
		case ast.SelectionKindField:
			fieldName := doc.FieldNameString(selection.Ref)
			alias := doc.FieldAliasOrNameString(selection.Ref)

			if fieldName == "__typename" && alias == "__internal__typename_placeholder" {
				continue
			}

			return false
		case ast.SelectionKindInlineFragment:
			fragment := doc.InlineFragments[selection.Ref]

			if fragment.HasSelections && !selectionSetIsEmpty(doc, fragment.SelectionSet) {
				return false
			}
		default:
			return false
		}
	}

	return true
}
