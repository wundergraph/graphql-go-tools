package plan

import (
	"bytes"
	"fmt"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astparser"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/operationreport"
)

const (
	keyFieldsFragmentTemplate = `fragment Key on %s {%s}`
	requiresFragmentTemplate  = `fragment Requires_for_%s on %s { %s }`
)

func RequiredFieldsFragmentString(typeName, fieldName, requiredFields string, includeTypename bool) (fragment string) {
	if includeTypename {
		requiredFields = "__typename " + requiredFields
	}

	if fieldName == "" {
		fragment = fmt.Sprintf(keyFieldsFragmentTemplate, typeName, requiredFields)
	} else {
		fragment = fmt.Sprintf(requiresFragmentTemplate, fieldName, typeName, requiredFields)
	}

	return fragment
}

func RequiredFieldsFragment(typeName, requiredFields string, includeTypename bool) (*ast.Document, *operationreport.Report) {
	return ParseRequiredFieldsFragment(typeName, "", requiredFields, includeTypename)
}

func ParseRequiredFieldsFragment(typeName, fieldName, requiredFields string, includeTypename bool) (*ast.Document, *operationreport.Report) {
	fragment := RequiredFieldsFragmentString(typeName, fieldName, requiredFields, includeTypename)

	key, report := astparser.ParseGraphqlDocumentString(fragment)
	if report.HasErrors() {
		return nil, &report
	}

	return &key, &report
}

func QueryPlanRequiredFieldsFragment(typeName, fieldName, requiredFields string) (*ast.Document, *operationreport.Report) {
	// fieldName == "" - is not fully accurate, for interface objects we do not request a __typename
	return ParseRequiredFieldsFragment(typeName, fieldName, requiredFields, fieldName == "")
}

type addRequiredFieldsConfiguration struct {
	operation, definition        *ast.Document
	operationSelectionSetRef     int
	isTypeNameForEntityInterface bool
	isKey                        bool
	allowTypename                bool
	typeName                     string
	fieldSet                     string
	deferInfo                    *DeferInfo
	parentFieldDeferID           string

	// addTypenameInNestedSelections controls forced addition of __typename to nested selection sets
	// used by "requires" keys, not only when fragments are present.
	addTypenameInNestedSelections bool
}

// requiredFieldInfo holds pre-computed field properties shared across
// the deferred and non-deferred handling paths.
type requiredFieldInfo struct {
	ref             int
	fieldName       ast.ByteSlice
	isTypeName      bool
	isLeaf          bool
	selectionSetRef int
}

type AddRequiredFieldsResult struct {
	skipFieldRefs     []int
	requiredFieldRefs []int
	modifiedFieldRefs []int
	remappedPaths     map[string]string // path in a requirements to field name
}

func addRequiredFields(config *addRequiredFieldsConfiguration) (out AddRequiredFieldsResult, report *operationreport.Report) {
	parsedSelectionSet, report := RequiredFieldsFragment(config.typeName, config.fieldSet, config.allowTypename)
	if report.HasErrors() {
		return out, report
	}

	walker := astvisitor.NewWalkerWithID(4, "RequiredFieldsVisitor")

	visitor := &requiredFieldsVisitor{
		Walker:            &walker,
		config:            config,
		key:               parsedSelectionSet,
		importer:          &astimport.Importer{},
		skipFieldRefs:     make([]int, 0, 2),
		requiredFieldRefs: make([]int, 0, 2),
		mapping:           make(map[string]string),
	}
	walker.RegisterEnterDocumentVisitor(visitor)
	walker.RegisterFieldVisitor(visitor)
	walker.RegisterSelectionSetVisitor(visitor)
	walker.RegisterInlineFragmentVisitor(visitor)

	walker.Walk(parsedSelectionSet, config.definition, report)

	return AddRequiredFieldsResult{
		skipFieldRefs:     visitor.skipFieldRefs,
		requiredFieldRefs: visitor.requiredFieldRefs,
		modifiedFieldRefs: visitor.modifiedFieldRefs,
		remappedPaths:     visitor.mapping,
	}, report
}

type requiredFieldsVisitor struct {
	*astvisitor.Walker

	OperationNodes []ast.Node
	config         *addRequiredFieldsConfiguration
	importer       *astimport.Importer
	key            *ast.Document

	skipFieldRefs     []int
	requiredFieldRefs []int
	modifiedFieldRefs []int
	mapping           map[string]string // path in a requirements to field name
}

func (v *requiredFieldsVisitor) EnterDocument(_, _ *ast.Document) {
	v.OperationNodes = make([]ast.Node, 0, 3)
	v.OperationNodes = append(v.OperationNodes,
		ast.Node{Kind: ast.NodeKindSelectionSet, Ref: v.config.operationSelectionSetRef})
}

func (v *requiredFieldsVisitor) EnterInlineFragment(ref int) {
	typeName := v.key.InlineFragmentTypeConditionName(ref)

	inlineFragmentRef := v.config.operation.AddInlineFragment(ast.InlineFragment{
		TypeCondition: ast.TypeCondition{
			Type: v.config.operation.AddNamedType(typeName),
		},
	})

	operationNode := v.OperationNodes[len(v.OperationNodes)-1]
	if operationNode.Kind != ast.NodeKindSelectionSet {
		v.Walker.StopWithInternalErr(fmt.Errorf("expected operation node to be of kind selection set, got %s", operationNode.Kind))
		return
	}

	v.config.operation.AddSelection(operationNode.Ref, ast.Selection{
		Kind: ast.SelectionKindInlineFragment,
		Ref:  inlineFragmentRef,
	})

	v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindInlineFragment, Ref: inlineFragmentRef})
}

func (v *requiredFieldsVisitor) LeaveInlineFragment(ref int) {
	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *requiredFieldsVisitor) EnterSelectionSet(ref int) {
	if v.Walker.Depth == 2 {
		return
	}
	operationNode := v.OperationNodes[len(v.OperationNodes)-1]

	keySelectionSetHasFragments := len(v.key.SelectionSetInlineFragmentSelections(ref)) > 0

	if operationNode.Kind == ast.NodeKindField {
		enforcedTypename := v.config.addTypenameInNestedSelections && !v.config.isKey
		if fieldSelectionSetRef, ok := v.config.operation.FieldSelectionSet(operationNode.Ref); ok {
			selectionSetNode := ast.Node{Kind: ast.NodeKindSelectionSet, Ref: fieldSelectionSetRef}
			if (keySelectionSetHasFragments || enforcedTypename) &&
				!v.selectionSetHasTypeNameSelection(fieldSelectionSetRef) {
				v.addTypenameSelection(fieldSelectionSetRef)
			}
			v.OperationNodes = append(v.OperationNodes, selectionSetNode)
			return
		}

		selectionSetNode := v.config.operation.AddSelectionSet()
		if keySelectionSetHasFragments || enforcedTypename {
			v.addTypenameSelection(selectionSetNode.Ref)
		}

		v.config.operation.Fields[operationNode.Ref].HasSelections = true
		v.config.operation.Fields[operationNode.Ref].SelectionSet = selectionSetNode.Ref
		v.OperationNodes = append(v.OperationNodes, selectionSetNode)
		return
	}

	// operation node kind InlineFragment
	selectionSetNode := v.config.operation.AddSelectionSet()
	v.config.operation.InlineFragments[operationNode.Ref].HasSelections = true
	v.config.operation.InlineFragments[operationNode.Ref].SelectionSet = selectionSetNode.Ref
	v.OperationNodes = append(v.OperationNodes, selectionSetNode)
}

func (v *requiredFieldsVisitor) fieldHasDeferInternal(fieldRef int) bool {
	_, exists := v.config.operation.Fields[fieldRef].Directives.HasDirectiveByNameBytes(v.config.operation, literal.DEFER_INTERNAL)
	return exists
}

// fieldDeferID returns the "id" argument value of the @__defer_internal directive
// on fieldRef, or "" if the directive is not present.
func (v *requiredFieldsVisitor) fieldDeferID(fieldRef int) string {
	for _, dirRef := range v.config.operation.Fields[fieldRef].Directives.Refs {
		if !bytes.Equal(v.config.operation.DirectiveNameBytes(dirRef), literal.DEFER_INTERNAL) {
			continue // not the right directive
		}
		// found @__defer_internal — extract the "id" argument
		val, ok := v.config.operation.DirectiveArgumentValueByName(dirRef, []byte("id"))
		if !ok || val.Kind != ast.ValueKindString {
			continue
		}
		return v.config.operation.StringValueContentString(val.Ref)
	}
	return ""
}

type deferAliasResult struct {
	addAlias       bool
	includeDeferID bool
	reuseFieldRef  int // ast.InvalidRef when not reusing
}

// resolveDeferredAlias decides how to alias a deferred required field.
// Precondition: v.config.deferInfo != nil && v.isRootLevel().
//
// Decision table:
//   - __internal_{fieldName} absent              → addAlias=true, includeDeferID=false
//   - __internal_{fieldName} present, same scope → reuseFieldRef set
//   - __internal_{fieldName} present, diff scope, __internal_{deferID}_{fieldName} absent  → addAlias=true, includeDeferID=true
//   - __internal_{fieldName} present, diff scope, __internal_{deferID}_{fieldName} present → reuseFieldRef set
func (v *requiredFieldsVisitor) resolveDeferredAlias(fieldName ast.ByteSlice, selectionSetRef int) deferAliasResult {
	// --- Level 1: look for __internal_{fieldName} ---
	simpleAlias := append([]byte("__internal_"), fieldName...)
	exists, existingRef := v.config.operation.SelectionSetHasFieldSelectionWithNameOrAliasBytes(selectionSetRef, simpleAlias)
	if !exists {
		// no alias yet — create the simple one
		return deferAliasResult{addAlias: true, reuseFieldRef: ast.InvalidRef}
	}
	if v.fieldDeferID(existingRef) == v.config.deferInfo.ID {
		// simple alias already belongs to this defer scope — reuse it
		return deferAliasResult{reuseFieldRef: existingRef}
	}

	// --- Level 2: simple alias belongs to a different scope ---
	// look for an existing conflict alias __internal_{deferID}_{fieldName}
	conflictAlias := fmt.Appendf(nil, "__internal_%s_%s", v.config.deferInfo.ID, fieldName)
	conflictExists, conflictRef := v.config.operation.SelectionSetHasFieldSelectionWithNameOrAliasBytes(selectionSetRef, conflictAlias)
	if conflictExists {
		// conflict alias already exists for this scope — reuse it
		return deferAliasResult{reuseFieldRef: conflictRef}
	}

	// no existing conflict alias — create one with the defer ID included
	return deferAliasResult{addAlias: true, includeDeferID: true, reuseFieldRef: ast.InvalidRef}
}

func (v *requiredFieldsVisitor) selectionSetHasTypeNameSelection(operationSelectionSetRef int) bool {
	exists, _ := v.config.operation.SelectionSetHasFieldSelectionWithExactName(operationSelectionSetRef, typeNameFieldBytes)
	return exists
}

// addTypenameSelection adds __typename selection to the operation when the key/requires selection set has inline fragments
func (v *requiredFieldsVisitor) addTypenameSelection(operationSelectionSetRef int) {
	field := v.config.operation.AddField(ast.Field{
		Name: v.config.operation.Input.AppendInputString("__typename"),
	})
	v.skipFieldRefs = append(v.skipFieldRefs, field.Ref)

	v.config.operation.AddSelection(operationSelectionSetRef, ast.Selection{
		Ref:  field.Ref,
		Kind: ast.SelectionKindField,
	})
}

func (v *requiredFieldsVisitor) LeaveSelectionSet(ref int) {
	if v.Walker.Depth == 0 {
		return
	}

	v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
}

func (v *requiredFieldsVisitor) EnterField(ref int) {
	if v.config.isKey {
		v.handleKeyField(ref)
		return
	}

	v.handleRequiredField(ref)
}

func (v *requiredFieldsVisitor) isRootLevel() bool {
	return len(v.OperationNodes) == 1
}

// handleRequiredField is the EnterField entry point for @requires fields.
// It builds requiredFieldInfo and dispatches to the deferred or non-deferred path.
func (v *requiredFieldsVisitor) handleRequiredField(ref int) {
	fieldName := v.key.FieldNameBytes(ref)

	fi := requiredFieldInfo{
		ref:             ref,
		fieldName:       fieldName,
		isTypeName:      bytes.Equal(fieldName, typeNameFieldBytes),
		isLeaf:          !v.key.FieldHasSelections(ref),
		selectionSetRef: v.OperationNodes[len(v.OperationNodes)-1].Ref,
	}

	// Unlike handleKeyField, __typename IS included in the deferred path here.
	// For interface objects (entity interfaces) the planner adds __typename as a
	// @requires field (not a key field) so the owning subgraph can return the real
	// concrete type. That __typename must travel through the same deferred path as
	// the rest of the requires fields, so it must not be excluded from aliasing.
	if v.config.deferInfo != nil && v.isRootLevel() {
		v.handleRequiredFieldDeferred(fi)
		return
	}
	v.handleRequiredFieldNonDeferred(fi)
}

// handleRequiredFieldDeferred handles @requires fields in a deferred context.
// Uses resolveDeferredAlias to reuse or create __internal_{fieldName} aliases.
func (v *requiredFieldsVisitor) handleRequiredFieldDeferred(fi requiredFieldInfo) {
	aliasResult := v.resolveDeferredAlias(fi.fieldName, fi.selectionSetRef)

	if aliasResult.reuseFieldRef != ast.InvalidRef {
		// reuse the existing aliased field from the same defer scope
		v.recordRemappedPathIfAliased(aliasResult.reuseFieldRef, fi.fieldName)
		if !fi.isTypeName || v.config.isTypeNameForEntityInterface {
			v.storeRequiredFieldRef(aliasResult.reuseFieldRef)
		}
		if !fi.isLeaf {
			// push to OperationNodes so nested key fields are traversed,
			// but do NOT add to modifiedFieldRefs — the selection set was already
			// set up by the prior addRequiredFields call that created this alias
			v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: aliasResult.reuseFieldRef})
		}
		return
	}

	fieldNode := v.addRequiredField(fi.ref, fi.fieldName, fi.selectionSetRef, aliasResult.addAlias, aliasResult.includeDeferID)
	if !fi.isLeaf {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

// handleRequiredFieldNonDeferred handles @requires fields outside a deferred context.
func (v *requiredFieldsVisitor) handleRequiredFieldNonDeferred(field requiredFieldInfo) {
	operationHasField, operationFieldRef := v.config.operation.SelectionSetHasFieldSelectionWithExactName(field.selectionSetRef, field.fieldName)

	// @requires fields can carry arguments (e.g. price(currency: USD)).
	// If the same field already appears in the query with different arguments,
	// the two selections cannot share the same field node, so we must alias the
	// required copy to avoid clobbering the user's selection.
	// Key fields never have arguments, so this check is absent in handleKeyFieldNonDeferred.
	needAlias := v.key.FieldHasArguments(field.ref)

	// if the existing field is deferred but we are adding requirements for a non-deferred scope,
	// we must not reuse it — add an alias instead.
	// When deferInfo is set (deferred context) and we're nested inside a reused deferred field,
	// the nested field is already in the correct defer scope — reuse it directly.
	if operationHasField && v.config.deferInfo == nil && v.fieldHasDeferInternal(operationFieldRef) {
		needAlias = true
	}

	if operationHasField && !needAlias {
		// Skip storing __typename as a required field — we only want to depend on
		// the actual key fields, not __typename.
		// Exception: for interface objects the planner adds __typename via @requires
		// so we do need it as a real dependency in that case.
		// (handleKeyFieldNonDeferred always skips __typename because it handles __typename
		// through the representation variables builder instead.)
		if !field.isTypeName || v.config.isTypeNameForEntityInterface {
			v.storeRequiredFieldRef(operationFieldRef)
		}

		// do not add required field if the field is already present in the operation with the same name
		// but add an operation node from operation if the field has selections
		if field.isLeaf {
			return
		}

		v.modifiedFieldRefs = append(v.modifiedFieldRefs, operationFieldRef)
		v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: operationFieldRef})
		return
	}

	fieldNode := v.addRequiredField(field.ref, field.fieldName, field.selectionSetRef, operationHasField && needAlias, false)
	if !field.isLeaf {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

// handleKeyField is the EnterField entry point for key fields.
// It builds requiredFieldInfo and dispatches to the deferred or non-deferred path.
func (v *requiredFieldsVisitor) handleKeyField(keyFieldRef int) {
	fieldName := v.key.FieldNameBytes(keyFieldRef)

	field := requiredFieldInfo{
		ref:             keyFieldRef,
		fieldName:       fieldName,
		isTypeName:      bytes.Equal(fieldName, typeNameFieldBytes),
		isLeaf:          !v.key.FieldHasSelections(keyFieldRef),
		selectionSetRef: v.OperationNodes[len(v.OperationNodes)-1].Ref,
	}

	// Key fields must never alias __typename, even in a deferred context.
	// __typename is not part of the user-visible key field set; instead it is
	// always injected by the representation variables builder with the static
	// name "__typename". Aliasing it would break that builder.
	// (handleRequiredField does NOT exclude __typename here because for
	// interface objects __typename is fetched via @requires, not keys.)
	if v.config.deferInfo != nil && v.isRootLevel() && !field.isTypeName {
		v.handleKeyFieldDeferred(field)
		return
	}
	v.handleKeyFieldNonDeferred(field)
}

// handleKeyFieldDeferred handles key fields in a deferred context.
// Key fields are added to the initial (non-deferred) selection set so they can be
// used as entity representation inputs.  The first occurrence of a key field is
// always added as a plain field (no alias); subsequent callers from different defer
// scopes reuse it.  An alias is only needed when a plain field already exists but
// belongs to a specific defer scope (has @deferInternal) and therefore cannot be
// shared.
func (v *requiredFieldsVisitor) handleKeyFieldDeferred(field requiredFieldInfo) {
	// First preference: a plain (non-deferred) field that all scopes can share.
	plainExists, plainRef := v.config.operation.SelectionSetHasFieldSelectionWithExactName(field.selectionSetRef, field.fieldName)
	if plainExists && !v.fieldHasDeferInternal(plainRef) {
		v.storeRequiredFieldRef(plainRef)
		if !field.isLeaf {
			v.modifiedFieldRefs = append(v.modifiedFieldRefs, plainRef)
			v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: plainRef})
		}
		return
	}

	aliasResult := v.resolveDeferredAlias(field.fieldName, field.selectionSetRef)

	if aliasResult.reuseFieldRef != ast.InvalidRef {
		// reuse the existing aliased field from the same defer scope
		v.recordRemappedPathIfAliased(aliasResult.reuseFieldRef, field.fieldName)
		v.storeRequiredFieldRef(aliasResult.reuseFieldRef)
		if !field.isLeaf {
			v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: aliasResult.reuseFieldRef})
		}
		return
	}

	// No existing field to reuse.  An alias is only needed when a plain field
	// already exists but is deferred (has @deferInternal).  When no field exists
	// at all, add a plain field so subsequent callers from any scope can reuse it.
	addAlias := plainExists // true only when plain field exists but is deferred
	fieldNode := v.addRequiredField(field.ref, field.fieldName, field.selectionSetRef, addAlias, aliasResult.includeDeferID)
	if !field.isLeaf {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

// handleKeyFieldNonDeferred handles key fields outside a deferred context.
func (v *requiredFieldsVisitor) handleKeyFieldNonDeferred(field requiredFieldInfo) {
	operationHasField, operationFieldRef := v.config.operation.SelectionSetHasFieldSelectionWithExactName(field.selectionSetRef, field.fieldName)

	// If the existing field has @deferInternal it belongs to a specific defer scope;
	// the non-deferred planner must not reuse it — add an alias instead.
	existingFieldIsDeferred := operationHasField && v.config.deferInfo == nil && v.fieldHasDeferInternal(operationFieldRef)

	if operationHasField && !existingFieldIsDeferred {
		// Skip storing __typename as a required field.
		// Unlike handleRequiredFieldNonDeferred there is no isTypeNameForEntityInterface
		// exception here: for interface objects the real __typename is fetched
		// via @requires (handled by handleRequiredField), never as a key field.
		// Key fields cannot have arguments, so there is no needAlias check here
		// (unlike handleRequiredFieldNonDeferred).
		if !field.isTypeName {
			v.storeRequiredFieldRef(operationFieldRef)
		}

		// do not add the required field if the field is already present in the operation with the same name
		// but add an operation node from operation if the field has selections
		if field.isLeaf {
			return
		}

		v.modifiedFieldRefs = append(v.modifiedFieldRefs, operationFieldRef)
		v.OperationNodes = append(v.OperationNodes, ast.Node{Kind: ast.NodeKindField, Ref: operationFieldRef})
		return
	}

	fieldNode := v.addRequiredField(field.ref, field.fieldName, field.selectionSetRef, existingFieldIsDeferred, false)
	if !field.isLeaf {
		v.OperationNodes = append(v.OperationNodes, fieldNode)
	}
}

func (v *requiredFieldsVisitor) LeaveField(ref int) {
	if v.key.FieldHasSelections(ref) {
		v.OperationNodes = v.OperationNodes[:len(v.OperationNodes)-1]
	}
}

func (v *requiredFieldsVisitor) storeRequiredFieldRef(fieldRef int) {
	v.requiredFieldRefs = append(v.requiredFieldRefs, fieldRef)
}

// recordRemappedPathIfAliased records the path → alias mapping when reusing an
// existing aliased field.  Each AddRequiredFields call gets a fresh v.mapping,
// so every planner that reuses an alias must record the mapping itself.
func (v *requiredFieldsVisitor) recordRemappedPathIfAliased(fieldRef int, fieldName ast.ByteSlice) {
	if !v.config.operation.FieldAliasIsDefined(fieldRef) {
		return
	}
	currentPath := v.Walker.Path.DotDelimitedString() + "." + string(fieldName)
	v.mapping[currentPath] = string(v.config.operation.FieldAliasBytes(fieldRef))
}

func (v *requiredFieldsVisitor) addRequiredField(keyFieldRef int, fieldName ast.ByteSlice, selectionSet int, addAlias bool, includeDeferIDInAlias bool) ast.Node {
	field := ast.Field{
		Name:         v.config.operation.Input.AppendInputBytes(fieldName),
		SelectionSet: ast.InvalidRef,
	}

	if addAlias {
		var fullAliasName []byte
		if includeDeferIDInAlias && v.config.deferInfo != nil {
			fullAliasName = fmt.Appendf(nil, "__internal_%s_%s", v.config.deferInfo.ID, fieldName)
		} else {
			fullAliasName = append([]byte("__internal_"), fieldName...)
		}

		field.Alias = ast.Alias{
			IsDefined: true,
			Name:      v.config.operation.Input.AppendInputBytes(fullAliasName),
		}

		currentPath := v.Walker.Path.DotDelimitedString() + "." + string(fieldName)
		v.mapping[currentPath] = string(fullAliasName)
	}

	addedFieldNode := v.config.operation.AddField(field)

	if v.key.FieldHasArguments(keyFieldRef) {
		importedArgs := v.importer.ImportArguments(v.key.Fields[keyFieldRef].Arguments.Refs, v.key, v.config.operation)

		for _, arg := range importedArgs {
			v.config.operation.AddArgumentToField(addedFieldNode.Ref, arg)
		}
	}

	selection := ast.Selection{
		Kind: ast.SelectionKindField,
		Ref:  addedFieldNode.Ref,
	}
	v.config.operation.AddSelection(selectionSet, selection)

	v.skipFieldRefs = append(v.skipFieldRefs, addedFieldNode.Ref)

	// we are skipping adding __typename field to the required fields,
	// because we want to depend only on the regular key fields, not the __typename field
	if !bytes.Equal(fieldName, typeNameFieldBytes) || (bytes.Equal(fieldName, typeNameFieldBytes) && v.config.isTypeNameForEntityInterface) {
		v.storeRequiredFieldRef(addedFieldNode.Ref)
	}

	v.applyDeferInternalDirective(addedFieldNode.Ref)

	return addedFieldNode
}

func (v *requiredFieldsVisitor) applyDeferInternalDirective(fieldRef int) {
	if v.config.deferInfo == nil {
		return
	}

	// when we are adding required fields from the requires directive
	if !v.config.isKey {
		// required fields should land in the same scope as the current field
		// to be fetched in the same defer group, but not in the parent scope
		v.addDeferInternalDirective(fieldRef, v.config.deferInfo)
		return
	}

	// when we are adding key fields
	// and the parent field has the defer id
	if v.config.parentFieldDeferID != "" {
		// for key fields: use parentFieldDeferID as the id
		// key should be in scope of the parent defer id, not be the deferred inside the same fragment,
		// otherwise it can't be planned properly
		v.addDeferInternalDirective(fieldRef, &DeferInfo{ID: v.config.parentFieldDeferID})
	}

	// if the parent field does not have a defer id,
	// fields should be unscoped, as is the parent field itself
}

func (v *requiredFieldsVisitor) addDeferInternalDirective(fieldRef int, deferInfo *DeferInfo) {
	var argRefs []int

	argRefs = append(argRefs, v.addStringArgument("id", deferInfo.ID))

	if deferInfo.Label != "" {
		argRefs = append(argRefs, v.addStringArgument("label", deferInfo.Label))
	}
	if deferInfo.ParentID != "" {
		argRefs = append(argRefs, v.addStringArgument("parentDeferId", deferInfo.ParentID))
	}

	directive := ast.Directive{
		Name:         v.config.operation.Input.AppendInputBytes(literal.DEFER_INTERNAL),
		HasArguments: len(argRefs) > 0,
		Arguments: ast.ArgumentList{
			Refs: argRefs,
		},
	}
	directiveRef := v.config.operation.AddDirective(directive)
	v.config.operation.AddDirectiveToNode(directiveRef, ast.Node{
		Kind: ast.NodeKindField,
		Ref:  fieldRef,
	})
}

func (v *requiredFieldsVisitor) addStringArgument(name, value string) int {
	strValueRef := v.config.operation.AddStringValue(ast.StringValue{
		Content: v.config.operation.Input.AppendInputString(value),
	})

	arg := ast.Argument{
		Name:  v.config.operation.Input.AppendInputString(name),
		Value: ast.Value{Kind: ast.ValueKindString, Ref: strValueRef},
	}

	return v.config.operation.AddArgument(arg)
}
