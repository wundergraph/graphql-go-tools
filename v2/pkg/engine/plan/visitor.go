package plan

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astimport"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

type DataSourceDebugger interface {
	astvisitor.VisitorIdentifier
	DebugPrint(args ...interface{})
	EnableDebug()
	EnableDebugQueryPlanLogging()
}

type QueryPlanProvider interface {
	IncludeQueryPlanInFetchConfiguration()
}

type Visitor struct {
	Operation, Definition        *ast.Document
	Walker                       *astvisitor.Walker
	Importer                     astimport.Importer
	Config                       Configuration
	plan                         Plan
	response                     *resolve.GraphQLResponse
	subscription                 *resolve.GraphQLSubscription
	OperationName                string
	operationDefinition          int
	objects                      []*resolve.Object
	currentFields                []objectFields
	currentField                 *resolve.Field
	planners                     []PlannerConfiguration
	skipFieldsRefs               []int
	fieldRefDependsOnFieldRefs   map[int][]int
	fieldDependencyKind          map[fieldDependencyKey]fieldDependencyKind
	fieldConfigs                 map[int]*FieldConfiguration
	exportedVariables            map[string]struct{}
	skipIncludeOnFragments       map[int]skipIncludeInfo
	disableResolveFieldPositions bool
	includeQueryPlans            bool
	indirectInterfaceFields      map[int]indirectInterfaceField
	pathCache                    map[astvisitor.VisitorKind]map[int]string

	// plannerFields tells which fields are planned on which planners
	// map plannerID -> []fieldRef
	plannerFields map[int][]int
	// fieldPlanners tells which planners a field was planned on
	// map fieldRef -> []plannerID
	fieldPlanners map[int][]int
	// fieldEnclosingTypeNames stores the enclosing type names for each field ref
	fieldEnclosingTypeNames map[int]string
	// plannerObjects stores the root object for each planner's ProvidesData
	// map plannerID -> root object
	plannerObjects map[int]*resolve.Object
	// plannerCurrentFields stores the current field stack for each planner
	// map plannerID -> field stack
	plannerCurrentFields map[int][]objectFields
	// plannerResponsePaths stores the response paths relative to each planner's root
	// map plannerID -> response path stack
	plannerResponsePaths map[int][]string
	// plannerEntityBoundaryPaths stores the entity boundary paths for each planner
	// map plannerID -> entity boundary path
	plannerEntityBoundaryPaths map[int]string
}

type indirectInterfaceField struct {
	interfaceName string
	node          ast.Node
}

func (v *Visitor) debugOnEnterNode(kind ast.NodeKind, ref int) {
	if !v.Config.Debug.PlanningVisitor {
		return
	}

	switch kind {
	case ast.NodeKindField:
		fieldName := v.Operation.FieldNameString(ref)
		fullPath := v.currentFullPath(false)
		v.debugPrint("EnterField : ", fieldName, " ref: ", ref, " path: ", fullPath)
	case ast.NodeKindInlineFragment:
		fragmentTypeCondition := v.Operation.InlineFragmentTypeConditionNameString(ref)
		v.debugPrint("EnterInlineFragment : ", fragmentTypeCondition, " ref: ", ref)
	case ast.NodeKindSelectionSet:
		v.debugPrint("EnterSelectionSet", " ref: ", ref)
	}
}

func (v *Visitor) debugOnLeaveNode(kind ast.NodeKind, ref int) {
	if !v.Config.Debug.PlanningVisitor {
		return
	}

	switch kind {
	case ast.NodeKindField:
		fieldName := v.Operation.FieldNameString(ref)
		fullPath := v.currentFullPath(false)
		v.debugPrint("LeaveField : ", fieldName, " ref: ", ref, " path: ", fullPath)
	case ast.NodeKindInlineFragment:
		fragmentTypeCondition := v.Operation.InlineFragmentTypeConditionNameString(ref)
		v.debugPrint("LeaveInlineFragment : ", fragmentTypeCondition, " ref: ", ref)
	case ast.NodeKindSelectionSet:
		v.debugPrint("LeaveSelectionSet", " ref: ", ref)
	}
}

func (v *Visitor) debugPrint(args ...interface{}) {
	if !v.Config.Debug.PlanningVisitor {
		return
	}

	allArgs := []interface{}{"[Visitor]: "}
	allArgs = append(allArgs, args...)
	fmt.Println(allArgs...)
}

type skipIncludeInfo struct {
	skip                bool
	skipVariableName    string
	include             bool
	includeVariableName string
}

type objectFields struct {
	popOnField int
	fields     *[]*resolve.Field
}

func (v *Visitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor any, skipFor astvisitor.SkipVisitors) bool {
	if visitor == v {
		// main planner visitor should always be allowed
		return true
	}
	var (
		path           string
		isFragmentPath bool
	)

	if entry, ok := v.pathCache[kind]; ok {
		path = entry[ref]
	} else {
		v.pathCache[kind] = make(map[int]string)
	}
	if path == "" {
		path = v.Walker.Path.DotDelimitedString()
		if kind == astvisitor.EnterField || kind == astvisitor.LeaveField {
			path = path + "." + v.Operation.FieldAliasOrNameString(ref)
		}
		v.pathCache[kind][ref] = path
	}

	if kind == astvisitor.EnterInlineFragment || kind == astvisitor.LeaveInlineFragment {
		isFragmentPath = true
	}

	isRootPath := !strings.Contains(path, ".")
	if isRootPath && !isFragmentPath {
		// if path is a root query (e.g. query, mutation) path we always allow visiting
		//
		// but if it is a fragment path on a query type like `... on Query`, we need to check if visiting is allowed
		// AllowVisitor callback is called before firing Enter/Leave callbacks, but we append ancestor and update path after enter callback,
		// so we will get path as `query` instead of `query.$Query` in case of fragment path
		return true
	}

	idVisitor, ok := visitor.(Identifyable)
	if !ok {
		return false
	}
	visitorID := idVisitor.ID()
	config := v.planners[visitorID]
	if !config.HasPath(path) {
		return false
	}
	switch kind {
	case astvisitor.EnterField, astvisitor.LeaveField:
		fieldName := v.Operation.FieldNameString(ref)
		enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)

		allow := config.HasPathWithFieldRef(ref) || config.HasParent(path)

		if !allow {
			if v.Config.Debug.PlanningVisitor {
				if pp, ok := config.Debugger(); ok {
					pp.DebugPrint("allow:", false, " AllowVisitor: Field", " ref:", ref, " enclosingTypeName:", enclosingTypeName, " field:", fieldName, " path:", path)
				}
			}
			return false
		}

		shouldWalkFieldsOnPath :=
			// check if the field path has type condition and matches the enclosing type
			config.ShouldWalkFieldsOnPath(path, enclosingTypeName) ||
				// check if the planner has path without type condition
				// this could happen in case of union type
				// or if there was added missing parent path
				config.ShouldWalkFieldsOnPath(path, "")

		if v.Config.Debug.PlanningVisitor {
			if pp, ok := config.Debugger(); ok {
				pp.DebugPrint("allow:", shouldWalkFieldsOnPath, " AllowVisitor: Field", " ref:", ref, " enclosingTypeName:", enclosingTypeName, " field:", fieldName, " path:", path)
			}
		}

		if !v.Config.DisableIncludeFieldDependencies && kind == astvisitor.LeaveField {
			// we don't need to do this twice, so we only do it on leave

			// store which fields are planned on which planners
			if v.plannerFields[visitorID] == nil {
				v.plannerFields[visitorID] = []int{ref}
			} else {
				v.plannerFields[visitorID] = append(v.plannerFields[visitorID], ref)
			}

			// store which planners a field was planned on
			if v.fieldPlanners[ref] == nil {
				v.fieldPlanners[ref] = []int{visitorID}
			} else {
				v.fieldPlanners[ref] = append(v.fieldPlanners[ref], visitorID)
			}
		}

		return shouldWalkFieldsOnPath
	case astvisitor.EnterInlineFragment, astvisitor.LeaveInlineFragment:
		// we allow visiting inline fragments only if particular planner has path for the fragment

		hasFragmentPath := config.HasFragmentPath(ref)

		if v.Config.Debug.PlanningVisitor {
			if pp, ok := config.Debugger(); ok {
				typeCondition := v.Operation.InlineFragmentTypeConditionNameString(ref)
				pp.DebugPrint("allow:", hasFragmentPath, " AllowVisitor: InlineFragment", " ref:", ref, " typeCondition:", typeCondition)
			}
		}

		return hasFragmentPath
	case astvisitor.EnterSelectionSet, astvisitor.LeaveSelectionSet:
		allowedByParent := skipFor.Allow(config.Planner())

		if v.Config.Debug.PlanningVisitor {
			if pp, ok := config.Debugger(); ok {
				pp.DebugPrint("allow:", allowedByParent, " AllowVisitor: SelectionSet", " ref:", ref, " parent allowance check")
			}
		}

		return allowedByParent
	default:
		return skipFor.Allow(config.Planner())
	}
}

func (v *Visitor) currentFullPath(skipFragments bool) string {
	path := v.Walker.Path.DotDelimitedString()
	if skipFragments {
		path = v.Walker.Path.WithoutInlineFragmentNames().DotDelimitedString()
	}

	if v.Walker.CurrentKind == ast.NodeKindField {
		fieldAliasOrName := v.Operation.FieldAliasOrNameString(v.Walker.CurrentRef)
		path = path + "." + fieldAliasOrName
	}
	return path
}

func (v *Visitor) EnterDirective(ref int) {
	directiveName := v.Operation.DirectiveNameString(ref)
	ancestor := v.Walker.Ancestors[len(v.Walker.Ancestors)-1]
	switch ancestor.Kind {
	case ast.NodeKindOperationDefinition:
		switch directiveName {
		case "flushInterval":
			if value, ok := v.Operation.DirectiveArgumentValueByName(ref, literal.MILLISECONDS); ok {
				if value.Kind == ast.ValueKindInteger {
					v.plan.SetFlushInterval(v.Operation.IntValueAsInt(value.Ref))
				}
			}
		}
	case ast.NodeKindField:
		switch directiveName {
		case "stream":
			initialBatchSize := 0
			if value, ok := v.Operation.DirectiveArgumentValueByName(ref, literal.INITIAL_BATCH_SIZE); ok {
				if value.Kind == ast.ValueKindInteger {
					initialBatchSize = int(v.Operation.IntValueAsInt32(value.Ref))
				}
			}
			v.currentField.Stream = &resolve.StreamField{
				InitialBatchSize: initialBatchSize,
			}
		case "defer":
			v.currentField.Defer = &resolve.DeferField{}
		}
	}
}

func (v *Visitor) EnterInlineFragment(ref int) {
	v.debugOnEnterNode(ast.NodeKindInlineFragment, ref)

	if v.Walker.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		field := indirectInterfaceField{
			interfaceName: v.Walker.EnclosingTypeDefinition.NameString(v.Definition),
			node:          v.Walker.EnclosingTypeDefinition,
		}
		v.indirectInterfaceFields[v.Operation.InlineFragments[ref].SelectionSet] = field
	}

	directives := v.Operation.InlineFragments[ref].Directives.Refs
	skipVariableName, skip := v.Operation.ResolveSkipDirectiveVariable(directives)
	includeVariableName, include := v.Operation.ResolveIncludeDirectiveVariable(directives)
	setRef := v.Operation.InlineFragments[ref].SelectionSet
	if setRef == ast.InvalidRef {
		return
	}

	if skip || include {
		v.skipIncludeOnFragments[ref] = skipIncludeInfo{
			skip:                skip,
			skipVariableName:    skipVariableName,
			include:             include,
			includeVariableName: includeVariableName,
		}
	}
}

func (v *Visitor) LeaveInlineFragment(ref int) {
	v.debugOnLeaveNode(ast.NodeKindInlineFragment, ref)
}

func (v *Visitor) EnterSelectionSet(ref int) {
	v.debugOnEnterNode(ast.NodeKindSelectionSet, ref)
}

func (v *Visitor) LeaveSelectionSet(ref int) {
	v.debugOnLeaveNode(ast.NodeKindSelectionSet, ref)
}

func (v *Visitor) EnterField(ref int) {
	v.debugOnEnterNode(ast.NodeKindField, ref)

	if !v.Config.DisableIncludeFieldDependencies {
		v.fieldEnclosingTypeNames[ref] = strings.Clone(v.Walker.EnclosingTypeDefinition.NameString(v.Definition))
	}

	// Track field for each planner that should handle it
	for plannerID := range v.planners {
		v.trackFieldForPlanner(plannerID, ref)
	}

	// check if we have to skip the field in the response
	// it means it was requested by the planner not the user
	if v.skipField(ref) {
		return
	}

	fieldName := v.Operation.FieldNameBytes(ref)
	fieldAliasOrName := v.Operation.FieldAliasOrNameBytes(ref)

	if bytes.Equal(fieldAliasOrName, []byte("__internal__typename_placeholder")) {
		// we should skip such typename as it was added as a placeholder to keep query valid
		return
	}

	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionTypeRef := v.Definition.FieldDefinitionType(fieldDefinition)

	onTypeNames := v.resolveOnTypeNames(ref, fieldName)

	v.currentField = &resolve.Field{
		Name:        fieldAliasOrName,
		OnTypeNames: onTypeNames,
		Position:    v.resolveFieldPosition(ref),
		Info:        v.resolveFieldInfo(ref, fieldDefinitionTypeRef, onTypeNames),
	}

	if bytes.Equal(fieldName, literal.TYPENAME) {
		typeName := v.Walker.EnclosingTypeDefinition.NameBytes(v.Definition)
		isRootQueryType := v.Definition.Index.IsRootOperationTypeNameBytes(typeName)

		if isRootQueryType {
			str := &resolve.StaticString{
				Path:  []string{v.Operation.FieldAliasOrNameString(ref)},
				Value: string(typeName),
			}
			v.currentField.Value = str
		} else {
			str := &resolve.String{
				Nullable:   false,
				Path:       []string{v.Operation.FieldAliasOrNameString(ref)},
				IsTypeName: true,
			}
			v.currentField.Value = str
		}
	} else {
		path := v.resolveFieldPath(ref)
		v.currentField.Value = v.resolveFieldValue(ref, fieldDefinitionTypeRef, true, path)
	}

	// append the field to the current object
	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, v.currentField)

	v.mapFieldConfig(ref)
}

func (v *Visitor) mapFieldConfig(ref int) {
	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldNameStr := v.Operation.FieldNameString(ref)
	fieldConfig := v.Config.Fields.ForTypeField(typeName, fieldNameStr)
	if fieldConfig == nil {
		return
	}
	v.fieldConfigs[ref] = fieldConfig
}

func (v *Visitor) resolveFieldInfo(ref, typeRef int, onTypeNames [][]byte) *resolve.FieldInfo {
	if v.Config.DisableIncludeInfo {
		return nil
	}

	enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldName := v.Operation.FieldNameString(ref)
	fieldHasAuthorizationRule := v.fieldHasAuthorizationRule(enclosingTypeName, fieldName)
	underlyingType := v.Definition.ResolveUnderlyingType(typeRef)
	typeName := v.Definition.ResolveTypeNameString(typeRef)

	// if the value is not a named type, try to resolve the underlying type
	if underlyingType != -1 {
		typeName = v.Definition.ResolveTypeNameString(underlyingType)
	}

	parentTypeNames := []string{enclosingTypeName}
	for i := range onTypeNames {
		onTypeName := string(onTypeNames[i])
		// in the case of a union, this will be equal, so do not append duplicates
		if onTypeName == enclosingTypeName {
			continue
		}
		parentTypeNames = append(parentTypeNames, onTypeName)
	}

	if v.Walker.EnclosingTypeDefinition.Kind == ast.NodeKindInterfaceTypeDefinition {
		// get all the type names that implement this interface
		implementingTypeNames, ok := v.Definition.InterfaceTypeDefinitionImplementedByObjectWithNames(v.Walker.EnclosingTypeDefinition.Ref)
		if ok {
			parentTypeNames = append(parentTypeNames, implementingTypeNames...)
		}
	}
	slices.Sort(parentTypeNames)
	parentTypeNames = slices.Compact(parentTypeNames)

	sourceNames := make([]string, 0, 1)
	sourceIDs := make([]string, 0, 1)

	for i := range v.planners {
		if v.planners[i].HasPathWithFieldRef(ref) {
			sourceIDs = append(sourceIDs, v.planners[i].DataSourceConfiguration().Id())
			sourceNames = append(sourceNames, v.planners[i].DataSourceConfiguration().Name())
		}
	}
	fieldInfo := &resolve.FieldInfo{
		Name:            fieldName,
		NamedType:       typeName,
		ParentTypeNames: parentTypeNames,
		Source: resolve.TypeFieldSource{
			IDs:   sourceIDs,
			Names: sourceNames,
		},
		ExactParentTypeName:  enclosingTypeName,
		HasAuthorizationRule: fieldHasAuthorizationRule,
	}

	if value, ok := v.indirectInterfaceFields[v.Walker.Ancestors[len(v.Walker.Ancestors)-1].Ref]; ok {
		_, defined := v.Definition.NodeFieldDefinitionByName(value.node, v.Operation.FieldNameBytes(ref))
		if defined && value.node.Kind == ast.NodeKindInterfaceTypeDefinition {
			fieldInfo.IndirectInterfaceNames = append(fieldInfo.IndirectInterfaceNames, value.interfaceName)
		}
	}

	return fieldInfo
}

func (v *Visitor) fieldHasAuthorizationRule(typeName, fieldName string) bool {
	fieldConfig := v.Config.Fields.ForTypeField(typeName, fieldName)
	return fieldConfig != nil && fieldConfig.HasAuthorizationRule
}

func (v *Visitor) resolveFieldPosition(ref int) resolve.Position {
	if v.disableResolveFieldPositions {
		return resolve.Position{}
	}
	return resolve.Position{
		Line:   v.Operation.Fields[ref].Position.LineStart,
		Column: v.Operation.Fields[ref].Position.CharStart,
	}
}

func (v *Visitor) resolveSkipIncludeOnParent() (info skipIncludeInfo, ok bool) {
	if len(v.skipIncludeOnFragments) == 0 {
		return skipIncludeInfo{}, false
	}

	for i := len(v.Walker.Ancestors) - 1; i >= 0; i-- {
		ancestor := v.Walker.Ancestors[i]
		if ancestor.Kind != ast.NodeKindInlineFragment {
			continue
		}
		if info, ok := v.skipIncludeOnFragments[ancestor.Ref]; ok {
			return info, true
		}
	}

	return skipIncludeInfo{}, false
}

func (v *Visitor) resolveOnTypeNames(fieldRef int, fieldName ast.ByteSlice) (onTypeNames [][]byte) {
	if len(v.Walker.Ancestors) < 2 {
		return nil
	}
	inlineFragment := v.Walker.Ancestors[len(v.Walker.Ancestors)-2]
	if inlineFragment.Kind != ast.NodeKindInlineFragment {
		return nil
	}
	typeName := v.Operation.InlineFragmentTypeConditionName(inlineFragment.Ref)
	if typeName == nil {
		typeName = v.Walker.EnclosingTypeDefinition.NameBytes(v.Definition)
	}
	node, exists := v.Definition.NodeByName(typeName)
	// If not an interface, return the concrete type
	if !exists || !node.Kind.IsAbstractType() {
		return v.addInterfaceObjectNameToTypeNames(fieldRef, typeName, [][]byte{v.Config.Types.RenameTypeNameOnMatchBytes(typeName)})
	}

	if node.Kind == ast.NodeKindUnionTypeDefinition {
		if !bytes.Equal(fieldName, literal.TYPENAME) {
			// Union can't have field selections other than __typename
			v.Walker.StopWithInternalErr(fmt.Errorf("resolveOnTypeNames called with a union type and field %s", fieldName))
			return nil
		}

		typeNames, ok := v.Definition.UnionTypeDefinitionMemberTypeNamesAsBytes(node.Ref)
		if ok {
			onTypeNames = typeNames
		}
	} else {
		// We're dealing with an interface, so add all objects that implement this interface to the slice
		typeNames, ok := v.Definition.InterfaceTypeDefinitionImplementedByObjectWithNamesAsBytes(node.Ref)
		if ok {
			onTypeNames = typeNames
		}
	}

	if len(v.Walker.TypeDefinitions) > 1 {
		// filter obtained onTypeNames to only those that are allowed by the grandparent type
		grandParent := v.Walker.TypeDefinitions[len(v.Walker.TypeDefinitions)-2]
		switch grandParent.Kind {
		case ast.NodeKindUnionTypeDefinition:
			for i := 0; i < len(onTypeNames); i++ {
				possibleMember, exists := v.Definition.Index.FirstNodeByNameStr(string(onTypeNames[i]))
				if !exists {
					continue
				}
				if !v.Definition.NodeIsUnionMember(possibleMember, grandParent) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		case ast.NodeKindInterfaceTypeDefinition:
			objectTypesImplementingGrandParent, _ := v.Definition.InterfaceTypeDefinitionImplementedByObjectWithNames(grandParent.Ref)
			for i := 0; i < len(onTypeNames); i++ {
				if !slices.Contains(objectTypesImplementingGrandParent, string(onTypeNames[i])) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		case ast.NodeKindObjectTypeDefinition:
			// if the grandparent is an object type, we only want to keep the onTypeNames that match the grandparent type
			grandParentTypeName := grandParent.NameBytes(v.Definition)
			for i := 0; i < len(onTypeNames); i++ {
				if !bytes.Equal(onTypeNames[i], grandParentTypeName) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		default:
		}
	}

	return onTypeNames
}

func (v *Visitor) addInterfaceObjectNameToTypeNames(fieldRef int, typeName []byte, onTypeNames [][]byte) [][]byte {
	includeInterfaceObjectName := false
	var interfaceObjectName string
	for i := range v.planners {
		if !v.planners[i].HasPathWithFieldRef(fieldRef) {
			continue
		}

		for _, interfaceObjCfg := range v.planners[i].DataSourceConfiguration().FederationConfiguration().InterfaceObjects {
			if slices.Contains(interfaceObjCfg.ConcreteTypeNames, string(typeName)) {
				includeInterfaceObjectName = true
				interfaceObjectName = interfaceObjCfg.InterfaceTypeName
				break
			}
		}
	}
	if includeInterfaceObjectName {
		onTypeNames = append(onTypeNames, []byte(interfaceObjectName))
	}

	return onTypeNames
}

func (v *Visitor) LeaveField(ref int) {
	v.debugOnLeaveNode(ast.NodeKindField, ref)

	// Pop fields for each planner that tracked this field
	for plannerID := range v.planners {
		v.popFieldsForPlanner(plannerID, ref)
	}

	if v.skipField(ref) {
		// we should also check skips on field leave
		// cause on nested keys we could mistakenly remove wrong object
		// from the stack of the current objects
		return
	}

	if v.currentFields[len(v.currentFields)-1].popOnField == ref {
		v.currentFields = v.currentFields[:len(v.currentFields)-1]
	}
	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionTypeNode := v.Definition.FieldDefinitionTypeNode(fieldDefinition)
	switch fieldDefinitionTypeNode.Kind {
	case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
		v.objects = v.objects[:len(v.objects)-1]
	}
}

func (v *Visitor) skipField(ref int) bool {
	for _, skipRef := range v.skipFieldsRefs {
		if skipRef == ref {
			return true
		}
	}
	return false
}

func (v *Visitor) introspectionShouldEvaluateIncludeDeprecated(fieldName string, enclosingTypeName string) bool {
	var introspectionEvaluateIncludeDeprecated bool

	switch enclosingTypeName {
	case "__Directive", "__Field":
		introspectionEvaluateIncludeDeprecated = fieldName == "args"
	case "__Type":
		switch fieldName {
		case "fields", "enumValues", "inputFields":
			introspectionEvaluateIncludeDeprecated = true
		}
	}

	return introspectionEvaluateIncludeDeprecated
}

func (v *Visitor) includeDeprecatedVariableName(fieldRef int) (name string) {
	if !v.Operation.FieldHasArguments(fieldRef) {
		return
	}

	argRef, ok := v.Operation.FieldArgument(fieldRef, []byte("includeDeprecated"))
	if !ok {
		return
	}

	argValue := v.Operation.ArgumentValue(argRef)
	if argValue.Kind != ast.ValueKindVariable {
		return
	}

	return string(v.Operation.VariableValueNameBytes(argValue.Ref))
}

func (v *Visitor) resolveSkipArrayItem(fieldRef int, fieldName string, enclosingTypeName string) resolve.SkipArrayItem {
	if !v.introspectionShouldEvaluateIncludeDeprecated(fieldName, enclosingTypeName) {
		return nil
	}

	return func(includeDeprecatedVariableName string) resolve.SkipArrayItem {
		return func(ctx *resolve.Context, itemValue *astjson.Value) bool {
			shouldIncludeDeprecated := false

			if includeDeprecatedVariableName != "" {
				shouldIncludeDeprecated = ctx.Variables.GetBool(includeDeprecatedVariableName)
			}

			isDeprecated := itemValue.GetBool("isDeprecated")

			return isDeprecated && !shouldIncludeDeprecated
		}
	}(v.includeDeprecatedVariableName(fieldRef))
}

func (v *Visitor) resolveFieldValue(fieldRef, typeRef int, nullable bool, path []string) resolve.Node {
	ofType := v.Definition.Types[typeRef].OfType

	fieldName := v.Operation.FieldNameString(fieldRef)
	enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldConfig := v.Config.Fields.ForTypeField(enclosingTypeName, fieldName)
	unescapeResponseJson := false
	if fieldConfig != nil {
		unescapeResponseJson = fieldConfig.UnescapeResponseJson
	}

	switch v.Definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		return v.resolveFieldValue(fieldRef, ofType, false, path)
	case ast.TypeKindList:
		listItem := v.resolveFieldValue(fieldRef, ofType, true, nil)

		return &resolve.Array{
			Nullable: nullable,
			Path:     path,
			Item:     listItem,
			SkipItem: v.resolveSkipArrayItem(fieldRef, fieldName, enclosingTypeName),
		}
	case ast.TypeKindNamed:
		typeName := v.Definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			v.Walker.StopWithInternalErr(fmt.Errorf("type %s not found in the definition", typeName))
			return &resolve.Null{}
		}

		customResolve, ok := v.Config.CustomResolveMap[typeName]
		if ok {
			return &resolve.CustomNode{
				CustomResolve: customResolve,
				Path:          path,
				Nullable:      nullable,
			}
		}

		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition:
			fieldExport := v.resolveFieldExport(fieldRef)
			switch typeName {
			case "String":
				return &resolve.String{
					Path:                 path,
					Nullable:             nullable,
					Export:               fieldExport,
					UnescapeResponseJson: unescapeResponseJson,
				}
			case "Boolean":
				return &resolve.Boolean{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "Int":
				return &resolve.Integer{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "Float":
				return &resolve.Float{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "BigInt":
				return &resolve.BigInt{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			case "JSON":
				if unescapeResponseJson {
					return &resolve.String{
						Path:                 path,
						Nullable:             nullable,
						Export:               fieldExport,
						UnescapeResponseJson: unescapeResponseJson,
					}
				}
				fallthrough
			default:
				return &resolve.Scalar{
					Path:     path,
					Nullable: nullable,
					Export:   fieldExport,
				}
			}
		case ast.NodeKindEnumTypeDefinition:
			values := make([]string, 0, len(v.Definition.EnumTypeDefinitions[typeDefinitionNode.Ref].EnumValuesDefinition.Refs))
			inaccessibleValues := make([]string, 0)
			for _, valueRef := range v.Definition.EnumTypeDefinitions[typeDefinitionNode.Ref].EnumValuesDefinition.Refs {
				valueName := v.Definition.EnumValueDefinitionNameString(valueRef)
				values = append(values, valueName)

				if _, isInaccessible := v.Definition.EnumValueDefinitionDirectiveByName(valueRef, []byte("inaccessible")); isInaccessible {
					inaccessibleValues = append(inaccessibleValues, valueName)
				}
			}
			return &resolve.Enum{
				Path:               path,
				Nullable:           nullable,
				TypeName:           typeName,
				Values:             values,
				InaccessibleValues: inaccessibleValues,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
			object := &resolve.Object{
				Nullable:      nullable,
				Path:          path,
				Fields:        []*resolve.Field{},
				TypeName:      typeName,
				PossibleTypes: map[string]struct{}{},
			}

			switch typeDefinitionNode.Kind {
			case ast.NodeKindObjectTypeDefinition:
				object.PossibleTypes[typeName] = struct{}{}
			case ast.NodeKindInterfaceTypeDefinition:
				objectTypesImplementingInterface, _ := v.Definition.InterfaceTypeDefinitionImplementedByObjectWithNames(typeDefinitionNode.Ref)
				for _, implementingTypeName := range objectTypesImplementingInterface {
					// exlude inaccessible types from possible types
					if v.isInaccesibleType(implementingTypeName) {
						continue
					}

					object.PossibleTypes[implementingTypeName] = struct{}{}

				}

				if slices.Contains(v.Config.EntityInterfaceNames, typeName) {
					object.PossibleTypes[typeName] = struct{}{}
				}

			case ast.NodeKindUnionTypeDefinition:
				if unionMembers, ok := v.Definition.UnionTypeDefinitionMemberTypeNames(typeDefinitionNode.Ref); ok {
					for _, unionMember := range unionMembers {
						// exlude inaccessible types from possible types
						if v.isInaccesibleType(unionMember) {
							continue
						}
						object.PossibleTypes[unionMember] = struct{}{}
					}
				}
			default:
			}

			if v.currentField.Info != nil {
				if len(v.currentField.Info.Source.Names) > 0 {
					object.SourceName = v.currentField.Info.Source.Names[0]
				} else if len(v.currentField.Info.Source.IDs) > 0 {
					object.SourceName = v.currentField.Info.Source.IDs[0]
				}
			}

			v.objects = append(v.objects, object)
			v.Walker.DefferOnEnterField(func() {
				v.currentFields = append(v.currentFields, objectFields{
					popOnField: fieldRef,
					fields:     &object.Fields,
				})
			})
			return object
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}

func (v *Visitor) isInaccesibleType(typeName string) bool {
	typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
	if !ok {
		return false
	}

	if typeDefinitionNode.Kind != ast.NodeKindObjectTypeDefinition {
		return false
	}

	if !v.Definition.ObjectTypeDefinitions[typeDefinitionNode.Ref].HasDirectives {
		return false
	}

	return v.Definition.ObjectTypeDefinitions[typeDefinitionNode.Ref].Directives.HasDirectiveByName(v.Definition, "inaccessible")
}

func (v *Visitor) resolveFieldExport(fieldRef int) *resolve.FieldExport {
	if !v.Operation.Fields[fieldRef].HasDirectives {
		return nil
	}
	exportAs := ""
	for _, ref := range v.Operation.Fields[fieldRef].Directives.Refs {
		if v.Operation.Input.ByteSliceString(v.Operation.Directives[ref].Name) == "export" {
			value, ok := v.Operation.DirectiveArgumentValueByName(ref, []byte("as"))
			if !ok {
				continue
			}
			if value.Kind != ast.ValueKindString {
				continue
			}
			exportAs = v.Operation.StringValueContentString(value.Ref)
		}
	}
	if exportAs == "" {
		return nil
	}
	variableDefinition, ok := v.Operation.VariableDefinitionByNameAndOperation(v.Walker.Ancestors[0].Ref, []byte(exportAs))
	if !ok {
		return nil
	}
	v.exportedVariables[exportAs] = struct{}{}

	typeName := v.Operation.ResolveTypeNameString(v.Operation.VariableDefinitions[variableDefinition].Type)
	switch typeName {
	case "Int", "Float", "Boolean":
		return &resolve.FieldExport{
			Path: []string{exportAs},
		}
	default:
		return &resolve.FieldExport{
			Path:     []string{exportAs},
			AsString: true,
		}
	}
}

func (v *Visitor) fieldRequiresExportedVariable(fieldRef int) bool {
	for _, arg := range v.Operation.Fields[fieldRef].Arguments.Refs {
		if v.valueRequiresExportedVariable(v.Operation.Arguments[arg].Value) {
			return true
		}
	}
	return false
}

func (v *Visitor) valueRequiresExportedVariable(value ast.Value) bool {
	switch value.Kind {
	case ast.ValueKindVariable:
		name := v.Operation.VariableValueNameString(value.Ref)
		if _, ok := v.exportedVariables[name]; ok {
			return true
		}
		return false
	case ast.ValueKindList:
		for _, ref := range v.Operation.ListValues[value.Ref].Refs {
			if v.valueRequiresExportedVariable(v.Operation.Values[ref]) {
				return true
			}
		}
		return false
	case ast.ValueKindObject:
		for _, ref := range v.Operation.ObjectValues[value.Ref].Refs {
			if v.valueRequiresExportedVariable(v.Operation.ObjectFieldValue(ref)) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func (v *Visitor) EnterOperationDefinition(ref int) {
	operationName := v.Operation.OperationDefinitionNameString(ref)
	if v.OperationName != operationName {
		v.Walker.SkipNode()
		return
	}

	v.operationDefinition = ref

	rootObject := &resolve.Object{
		Fields: []*resolve.Field{},
	}

	v.objects = append(v.objects, rootObject)
	v.currentFields = append(v.currentFields, objectFields{
		fields:     &rootObject.Fields,
		popOnField: -1,
	})

	operationKind, _, err := AnalyzePlanKind(v.Operation, v.Definition, v.OperationName)
	if err != nil {
		v.Walker.StopWithInternalErr(err)
		return
	}

	v.response = &resolve.GraphQLResponse{
		Data:       rootObject,
		RawFetches: make([]*resolve.FetchItem, 0, len(v.planners)),
	}
	if !v.Config.DisableIncludeInfo {
		v.response.Info = &resolve.GraphQLResponseInfo{
			OperationType: operationKind,
		}
	}

	// Initialize per-planner structures for ProvidesData tracking
	v.initializePlannerStructures()

	if operationKind == ast.OperationTypeSubscription {
		v.subscription = &resolve.GraphQLSubscription{
			Response: v.response,
		}
		v.plan = &SubscriptionResponsePlan{
			FlushInterval: v.Config.DefaultFlushIntervalMillis,
			Response:      v.subscription,
		}
		return
	}

	v.plan = &SynchronousResponsePlan{
		Response: v.response,
	}
}

func (v *Visitor) resolveFieldPath(ref int) []string {
	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldName := v.Operation.FieldNameUnsafeString(ref)
	plannerConfig := v.currentOrParentPlannerConfiguration(ref)

	aliasOverride := false
	if plannerConfig != nil && plannerConfig.Planner() != nil {
		aliasOverride = plannerConfig.DataSourcePlanningBehavior().OverrideFieldPathFromAlias
	}

	for i := range v.Config.Fields {
		if v.Config.Fields[i].TypeName == typeName && v.Config.Fields[i].FieldName == fieldName {
			if aliasOverride {
				override, exists := plannerConfig.DownstreamResponseFieldAlias(ref)
				if exists {
					return []string{override}
				}
			}
			if aliasOverride && v.Operation.FieldAliasIsDefined(ref) {
				return []string{v.Operation.FieldAliasString(ref)}
			}
			if v.Config.Fields[i].DisableDefaultMapping {
				return nil
			}
			if len(v.Config.Fields[i].Path) != 0 {
				return v.Config.Fields[i].Path
			}
			return []string{fieldName}
		}
	}

	if aliasOverride {
		return []string{v.Operation.FieldAliasOrNameString(ref)}
	}

	return []string{fieldName}
}

func (v *Visitor) EnterDocument(operation, definition *ast.Document) {
	v.Operation, v.Definition = operation, definition
	v.fieldConfigs = map[int]*FieldConfiguration{}
	v.exportedVariables = map[string]struct{}{}
	v.skipIncludeOnFragments = map[int]skipIncludeInfo{}
	v.indirectInterfaceFields = map[int]indirectInterfaceField{}
	v.pathCache = map[astvisitor.VisitorKind]map[int]string{}
	v.plannerFields = map[int][]int{}
	v.fieldPlanners = map[int][]int{}
	v.fieldEnclosingTypeNames = map[int]string{}
	v.plannerObjects = map[int]*resolve.Object{}
	v.plannerCurrentFields = map[int][]objectFields{}
	v.plannerResponsePaths = map[int][]string{}
}

func (v *Visitor) LeaveDocument(_, _ *ast.Document) {
	for i := range v.planners {
		if v.planners[i].ObjectFetchConfiguration().isSubscription {
			v.configureSubscription(v.planners[i].ObjectFetchConfiguration())
		} else {
			v.configureObjectFetch(v.planners[i].ObjectFetchConfiguration())
		}
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s*\.(.*?)\s*}}`)
)

func (v *Visitor) currentOrParentPlannerConfiguration(fieldRef int) PlannerConfiguration {
	// TODO: this method should be dropped it is unnecessary expensive

	const none = -1
	currentPath := v.currentFullPath(false)
	plannerIndex := none
	plannerPathDeepness := none

	for i := range v.planners {
		v.planners[i].ForEachPath(func(plannerPath *pathConfiguration) bool {
			if v.isCurrentOrParentPath(currentPath, plannerPath.path) {
				currentPlannerPathDeepness := v.pathDeepness(plannerPath.path)
				if currentPlannerPathDeepness > plannerPathDeepness {
					plannerPathDeepness = currentPlannerPathDeepness
					plannerIndex = i
					return true
				}
			}
			return false
		})
	}

	if plannerIndex != none {
		return v.planners[plannerIndex]
	}

	return nil
}

func (v *Visitor) isCurrentOrParentPath(currentPath string, parentPath string) bool {
	return strings.HasPrefix(currentPath, parentPath)
}

func (v *Visitor) pathDeepness(path string) int {
	return strings.Count(path, ".")
}

func (v *Visitor) initializePlannerStructures() {
	// Initialize root objects and field stacks for each potential planner
	// We'll populate these as we traverse fields
	if v.planners == nil {
		return
	}

	for i := range v.planners {
		v.plannerObjects[i] = &resolve.Object{
			Fields: []*resolve.Field{},
		}
		v.plannerCurrentFields[i] = []objectFields{{
			fields:     &v.plannerObjects[i].Fields,
			popOnField: -1,
		}}
		v.plannerResponsePaths[i] = []string{}
	}
	v.plannerEntityBoundaryPaths = map[int]string{}
}

func (v *Visitor) trackFieldForPlanner(plannerID int, fieldRef int) {
	// Safety checks
	if v.planners == nil || plannerID >= len(v.planners) {
		return
	}
	if v.plannerObjects == nil || v.plannerCurrentFields == nil {
		return
	}

	// Check if this planner should handle this field
	if !v.shouldPlannerHandleField(plannerID, fieldRef) {
		return
	}

	// Get field information
	fieldName := v.Operation.FieldNameBytes(fieldRef)
	fieldAliasOrName := v.Operation.FieldAliasOrNameString(fieldRef)

	// For nested entity fetches, check if this field represents the entity boundary
	// If so, we should skip adding this field to ProvidesData and instead add its children
	if v.isEntityBoundaryField(plannerID, fieldRef) {
		// Add a __typename field to the current object for entity boundary
		v.addTypenameFieldForPlanner(plannerID)
		return
	}

	// Check if this is a __typename field and if we already have one with the same name and path
	if bytes.Equal(fieldName, literal.TYPENAME) && len(v.plannerCurrentFields[plannerID]) > 0 {
		currentFields := v.plannerCurrentFields[plannerID][len(v.plannerCurrentFields[plannerID])-1]

		// Check if we already have a __typename field with the same name and path
		for _, existingField := range *currentFields.fields {
			if bytes.Equal(existingField.Name, []byte(fieldAliasOrName)) {
				// For __typename fields, the path is [fieldAliasOrName]
				// Check if the existing field has the same path
				if existingValue, ok := existingField.Value.(*resolve.Scalar); ok {
					if len(existingValue.Path) > 0 && existingValue.Path[0] == fieldAliasOrName {
						// We already have this __typename field with the same name and path, skip it
						return
					}
				}
			}
		}
	}

	// Get the field definition
	fieldDefinition, ok := v.Walker.FieldDefinition(fieldRef)
	if !ok {
		return
	}
	fieldType := v.Definition.FieldDefinitionType(fieldDefinition)

	// Create a simple field value for tracking purposes
	fieldValue := v.createFieldValueForPlanner(fieldRef, fieldType, []string{fieldAliasOrName})

	onTypeNames := v.resolveEntityOnTypeNames(plannerID, fieldRef, fieldName)

	// Create the field
	field := &resolve.Field{
		Name:        []byte(fieldAliasOrName),
		Value:       fieldValue,
		OnTypeNames: onTypeNames,
	}

	// Add the field to the current object for this planner
	if len(v.plannerCurrentFields[plannerID]) > 0 {
		currentFields := v.plannerCurrentFields[plannerID][len(v.plannerCurrentFields[plannerID])-1]
		*currentFields.fields = append(*currentFields.fields, field)
	}

	// If the field value is an object, push it onto the stack for this planner
	if obj, ok := fieldValue.(*resolve.Object); ok {
		v.Walker.DefferOnEnterField(func() {
			v.plannerCurrentFields[plannerID] = append(v.plannerCurrentFields[plannerID], objectFields{
				popOnField: fieldRef,
				fields:     &obj.Fields,
			})
		})
	}
}

func (v *Visitor) resolveEntityOnTypeNames(plannerID, fieldRef int, fieldName ast.ByteSlice) (onTypeNames [][]byte) {
	// If this is an entity root field, return the enclosing type name
	if v.isEntityRootField(plannerID, fieldRef) {
		enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameBytes(v.Definition)
		if enclosingTypeName != nil {
			return [][]byte{enclosingTypeName}
		}
	}

	// Otherwise, use the regular resolution logic
	onTypeNames = v.resolveOnTypeNames(fieldRef, fieldName)
	return onTypeNames
}

// createFieldValueForPlanner creates a simplified field value for planner tracking
// without relying on the full visitor state like resolveFieldValue does
func (v *Visitor) createFieldValueForPlanner(fieldRef, typeRef int, path []string) resolve.Node {
	ofType := v.Definition.Types[typeRef].OfType

	switch v.Definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		node := v.createFieldValueForPlanner(fieldRef, ofType, path)
		// Set nullable to false for the returned node
		switch n := node.(type) {
		case *resolve.Scalar:
			n.Nullable = false
		case *resolve.Object:
			n.Nullable = false
		case *resolve.Array:
			n.Nullable = false
		}
		return node
	case ast.TypeKindList:
		listItem := v.createFieldValueForPlanner(fieldRef, ofType, nil)
		return &resolve.Array{
			Nullable: true,
			Path:     path,
			Item:     listItem,
		}
	case ast.TypeKindNamed:
		typeName := v.Definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
		}
		switch typeDefinitionNode.Kind {
		case ast.NodeKindScalarTypeDefinition, ast.NodeKindEnumTypeDefinition:
			return &resolve.Scalar{
				Nullable: true,
				Path:     path,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
			// For object types, create a new object that will be populated by child fields
			obj := &resolve.Object{
				Nullable: true,
				Path:     path,
				Fields:   []*resolve.Field{},
			}
			return obj
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}

// isEntityBoundaryField checks if this field represents the entity boundary for a nested entity fetch
// For nested entity fetches, the field at the response path boundary should be skipped in ProvidesData
func (v *Visitor) isEntityBoundaryField(plannerID int, fieldRef int) bool {
	config := v.planners[plannerID]
	fetchConfig := config.ObjectFetchConfiguration()
	if fetchConfig == nil || fetchConfig.fetchItem == nil {
		return false
	}

	// Check if this is a nested fetch (has "." in response path)
	responsePath := "query." + fetchConfig.fetchItem.ResponsePath
	if !strings.Contains(responsePath, ".") {
		return false // Root fetch, no boundary field to skip
	}

	// For nested fetches, check if this field is at the entity boundary
	currentPath := v.Walker.Path.DotDelimitedString()
	fieldName := v.Operation.FieldAliasOrNameString(fieldRef)
	fullFieldPath := currentPath + "." + fieldName

	// If this field path matches the response path, it's the entity boundary
	if fullFieldPath == responsePath {
		// Store the entity boundary path for this planner
		v.plannerEntityBoundaryPaths[plannerID] = fullFieldPath
		return true
	}
	return false
}

// isEntityRootField checks if this field is at the root of an entity
// This means it has one additional path element compared to the stored entity boundary path
func (v *Visitor) isEntityRootField(plannerID int, fieldRef int) bool {
	// Check if we have a stored entity boundary path for this planner
	boundaryPath, hasBoundary := v.plannerEntityBoundaryPaths[plannerID]
	if !hasBoundary {
		return false
	}

	// Get the current field path
	currentPath := v.Walker.Path.DotDelimitedString()
	fieldName := v.Operation.FieldAliasOrNameString(fieldRef)
	fullFieldPath := currentPath + "." + fieldName

	// Check if this field is a direct child of the entity boundary
	// It should start with the boundary path and have exactly one more segment
	if !strings.HasPrefix(fullFieldPath, boundaryPath+".") {
		return false
	}

	// Remove the boundary path prefix and check if there's exactly one segment left
	remainingPath := strings.TrimPrefix(fullFieldPath, boundaryPath+".")
	// If there are no more dots, this is a root field of the entity
	return !strings.Contains(remainingPath, ".")
}

// addTypenameFieldForPlanner adds a __typename field to the current object for entity boundary fields
func (v *Visitor) addTypenameFieldForPlanner(plannerID int) {

	// Create a __typename field
	typenameField := &resolve.Field{
		Name: []byte("__typename"),
		Value: &resolve.Scalar{
			Path: []string{"__typename"},
		},
	}

	// Add the __typename field to the current object for this planner
	if len(v.plannerCurrentFields[plannerID]) > 0 {
		currentFields := v.plannerCurrentFields[plannerID][len(v.plannerCurrentFields[plannerID])-1]
		*currentFields.fields = append(*currentFields.fields, typenameField)
	}
}

func (v *Visitor) shouldPlannerHandleField(plannerID int, fieldRef int) bool {
	// Safety checks
	if v.planners == nil || plannerID >= len(v.planners) {
		return false
	}

	// Use the same logic as AllowVisitor to check if a planner handles a field
	path := v.Walker.Path.DotDelimitedString()
	if v.Walker.CurrentKind == ast.NodeKindField {
		path = path + "." + v.Operation.FieldAliasOrNameString(fieldRef)
	}

	config := v.planners[plannerID]
	if !config.HasPath(path) {
		return false
	}

	enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)

	allow := config.HasPathWithFieldRef(fieldRef) || config.HasParent(path)
	if !allow {
		return false
	}

	shouldWalkFieldsOnPath := config.ShouldWalkFieldsOnPath(path, enclosingTypeName) ||
		config.ShouldWalkFieldsOnPath(path, "")

	return shouldWalkFieldsOnPath
}

func (v *Visitor) popFieldsForPlanner(plannerID int, fieldRef int) {
	// Safety checks
	if v.plannerCurrentFields == nil || plannerID >= len(v.plannerCurrentFields) {
		return
	}

	if len(v.plannerCurrentFields[plannerID]) > 0 {
		last := len(v.plannerCurrentFields[plannerID]) - 1
		if v.plannerCurrentFields[plannerID][last].popOnField == fieldRef {
			v.plannerCurrentFields[plannerID] = v.plannerCurrentFields[plannerID][:last]
		}
	}
}

func (v *Visitor) resolveInputTemplates(config *objectFetchConfiguration, input *string, variables *resolve.Variables) {
	*input = templateRegex.ReplaceAllStringFunc(*input, func(s string) string {
		selectors := selectorRegex.FindStringSubmatch(s)
		if len(selectors) != 2 {
			return s
		}
		selector := strings.TrimPrefix(selectors[1], ".")
		parts := strings.Split(selector, ".")
		if len(parts) < 2 {
			return s
		}
		path := parts[1:]
		var (
			variableName string
		)
		switch parts[0] {
		case "object":
			variable := &resolve.ObjectVariable{
				Path:     path,
				Renderer: resolve.NewPlainVariableRenderer(),
			}
			variableName, _ = variables.AddVariable(variable)
		case "arguments":
			argumentName := path[0]
			arg, ok := v.Operation.FieldArgument(config.fieldRef, []byte(argumentName))
			if !ok {
				break
			}
			value := v.Operation.ArgumentValue(arg)
			if value.Kind != ast.ValueKindVariable {
				inputValueDefinition := -1
				for _, ref := range v.Definition.FieldDefinitions[config.fieldDefinitionRef].ArgumentsDefinition.Refs {
					inputFieldName := v.Definition.Input.ByteSliceString(v.Definition.InputValueDefinitions[ref].Name)
					if inputFieldName == argumentName {
						inputValueDefinition = ref
						break
					}
				}
				if inputValueDefinition == -1 {
					return "null"
				}
				return v.renderJSONValueTemplate(value, variables, inputValueDefinition)
			}
			variableValue := v.Operation.VariableValueNameString(value.Ref)
			if !v.Operation.OperationDefinitionHasVariableDefinition(v.operationDefinition, variableValue) {
				break // omit optional argument when variable is not defined
			}
			variableDefinition, exists := v.Operation.VariableDefinitionByNameAndOperation(v.operationDefinition, v.Operation.VariableValueNameBytes(value.Ref))
			if !exists {
				break
			}
			variableTypeRef := v.Operation.VariableDefinitions[variableDefinition].Type
			typeName := v.Operation.ResolveTypeNameBytes(v.Operation.VariableDefinitions[variableDefinition].Type)
			node, exists := v.Definition.Index.FirstNodeByNameBytes(typeName)
			if !exists {
				break
			}

			var variablePath []string
			if len(parts) > 2 && node.Kind == ast.NodeKindInputObjectTypeDefinition {
				variablePath = append(variablePath, path...)
			} else {
				variablePath = append(variablePath, variableValue)
			}

			variable := &resolve.ContextVariable{
				Path: variablePath,
			}

			if fieldConfig, ok := v.fieldConfigs[config.fieldRef]; ok {
				if argumentConfig := fieldConfig.Arguments.ForName(argumentName); argumentConfig != nil {
					switch argumentConfig.RenderConfig {
					case RenderArgumentAsArrayCSV:
						variable.Renderer = resolve.NewCSVVariableRendererFromTypeRef(v.Operation, v.Definition, variableTypeRef)
					case RenderArgumentDefault:
						variable.Renderer = resolve.NewPlainVariableRenderer()
					case RenderArgumentAsGraphQLValue:
						renderer, err := resolve.NewGraphQLVariableRendererFromTypeRefWithoutValidation(v.Operation, v.Definition, variableTypeRef)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					case RenderArgumentAsJSONValue:
						variable.Renderer = resolve.NewJSONVariableRenderer()
					}
				}
			}

			if variable.Renderer == nil {
				variable.Renderer = resolve.NewPlainVariableRenderer()
			}

			variableName, _ = variables.AddVariable(variable)
		case "request":
			if len(path) != 2 {
				break
			}
			switch path[0] {
			case "headers":
				key := path[1]
				variableName, _ = variables.AddVariable(&resolve.HeaderVariable{
					Path: []string{key},
				})
			}
		}
		return variableName
	})
}

func (v *Visitor) renderJSONValueTemplate(value ast.Value, variables *resolve.Variables, inputValueDefinition int) (out string) {
	switch value.Kind {
	case ast.ValueKindList:
		out += "["
		addComma := false
		for _, ref := range v.Operation.ListValues[value.Ref].Refs {
			if addComma {
				out += ","
			} else {
				addComma = true
			}
			listValue := v.Operation.Values[ref]
			out += v.renderJSONValueTemplate(listValue, variables, inputValueDefinition)
		}
		out += "]"
	case ast.ValueKindObject:
		out += "{"
		addComma := false
		for _, ref := range v.Operation.ObjectValues[value.Ref].Refs {
			fieldName := v.Operation.Input.ByteSlice(v.Operation.ObjectFields[ref].Name)
			fieldValue := v.Operation.ObjectFields[ref].Value
			typeName := v.Definition.ResolveTypeNameString(v.Definition.InputValueDefinitions[inputValueDefinition].Type)
			typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
			if !ok {
				continue
			}
			objectFieldDefinition, ok := v.Definition.NodeInputFieldDefinitionByName(typeDefinitionNode, fieldName)
			if !ok {
				continue
			}
			if addComma {
				out += ","
			} else {
				addComma = true
			}
			out += fmt.Sprintf("\"%s\":", string(fieldName))
			out += v.renderJSONValueTemplate(fieldValue, variables, objectFieldDefinition)
		}
		out += "}"
	case ast.ValueKindVariable:
		variablePath := v.Operation.VariableValueNameString(value.Ref)
		variableName, _ := variables.AddVariable(&resolve.ContextVariable{
			Path:     []string{variablePath},
			Renderer: resolve.NewJSONVariableRenderer(),
		})
		out += variableName
	}
	return
}

func (v *Visitor) configureSubscription(config *objectFetchConfiguration) {
	subscription := config.planner.ConfigureSubscription()
	v.subscription.Trigger.Variables = subscription.Variables
	v.subscription.Trigger.Source = subscription.DataSource
	v.subscription.Trigger.PostProcessing = subscription.PostProcessing
	v.resolveInputTemplates(config, &subscription.Input, &v.subscription.Trigger.Variables)
	v.subscription.Trigger.Input = []byte(subscription.Input)
	v.subscription.Filter = config.filter
}

func (v *Visitor) configureObjectFetch(config *objectFetchConfiguration) {
	fetchConfig := config.planner.ConfigureFetch()
	// If the datasource is missing, we can anticipate that configure fetch failed
	if fetchConfig.DataSource == nil {
		return
	}

	if v.includeQueryPlans && fetchConfig.QueryPlan == nil {
		fetchConfig.QueryPlan = &resolve.QueryPlan{}
	}
	fetch := v.configureFetch(config, fetchConfig)
	v.resolveInputTemplates(config, &fetch.Input, &fetch.Variables)

	fetchItem := config.fetchItem
	fetchItem.Fetch = fetch

	v.response.RawFetches = append(v.response.RawFetches, fetchItem)
}

func (v *Visitor) configureFetch(internal *objectFetchConfiguration, external resolve.FetchConfiguration) *resolve.SingleFetch {
	dataSourceType := reflect.TypeOf(external.DataSource).String()
	dataSourceType = strings.TrimPrefix(dataSourceType, "*")

	singleFetch := &resolve.SingleFetch{
		FetchConfiguration: external,
		FetchDependencies: resolve.FetchDependencies{
			FetchID:           internal.fetchID,
			DependsOnFetchIDs: internal.dependsOnFetchIDs,
		},
		DataSourceIdentifier: []byte(dataSourceType),
	}

	if !v.Config.DisableIncludeInfo {
		singleFetch.Info = &resolve.FetchInfo{
			DataSourceID:   internal.sourceID,
			DataSourceName: internal.sourceName,
			RootFields:     internal.rootFields,
			OperationType:  internal.operationType,
			QueryPlan:      external.QueryPlan,
		}

		// Set ProvidesData from the planner's object structure
		if providesData, ok := v.plannerObjects[internal.fetchID]; ok {
			singleFetch.Info.ProvidesData = providesData
		}
	}

	if !v.Config.DisableIncludeFieldDependencies {
		singleFetch.FetchConfiguration.CoordinateDependencies = v.resolveFetchDependencies(internal.fetchID)
	}

	return singleFetch
}

func (v *Visitor) resolveFetchDependencies(fetchID int) []resolve.FetchDependency {
	fields, ok := v.plannerFields[fetchID]
	if !ok {
		return nil
	}
	dependencies := make([]resolve.FetchDependency, 0, len(fields))
	for _, fieldRef := range fields {
		// skipField returns true if the field is a dependency and should not be included in the response
		// consequently, if we don't skip the field, the client requested it in the query
		userRequestedField := !v.skipField(fieldRef)
		deps, ok := v.fieldRefDependsOnFieldRefs[fieldRef]
		if !ok {
			continue
		}
		dependency := resolve.FetchDependency{
			Coordinate: resolve.GraphCoordinate{
				FieldName: strings.Clone(v.Operation.FieldNameString(fieldRef)),
				TypeName:  v.fieldEnclosingTypeNames[fieldRef],
			},
			IsUserRequested: userRequestedField,
		}
		for _, depFieldRef := range deps {
			depFieldPlanners, ok := v.fieldPlanners[depFieldRef]
			if !ok {
				continue
			}
			for _, fetchID := range depFieldPlanners { // planner index == fetchID
				ofc := v.planners[fetchID].ObjectFetchConfiguration()
				if ofc == nil {
					continue
				}
				origin := resolve.FetchDependencyOrigin{
					FetchID:  fetchID,
					Subgraph: ofc.sourceName,
					Coordinate: resolve.GraphCoordinate{
						FieldName: strings.Clone(v.Operation.FieldNameString(depFieldRef)),
						TypeName:  v.fieldEnclosingTypeNames[depFieldRef],
					},
				}
				dependencyKind, ok := v.fieldDependencyKind[fieldDependencyKey{field: fieldRef, dependsOn: depFieldRef}]
				if !ok {
					continue
				}
				switch dependencyKind {
				case fieldDependencyKindKey:
					origin.IsKey = true
				case fieldDependencyKindRequires:
					origin.IsRequires = true
				}
				dependency.DependsOn = append(dependency.DependsOn, origin)
			}
		}
		dependencies = append(dependencies, dependency)
	}
	return dependencies
}
