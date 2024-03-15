package plan

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"slices"
	"strings"

	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/ast"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astimport"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/TykTechnologies/graphql-go-tools/v2/pkg/lexer/literal"
)

type DataSourceDebugger interface {
	astvisitor.VisitorIdentifier
	DebugPrint(args ...interface{})
	EnableDebug()
	EnableQueryPlanLogging()
}

type Visitor struct {
	Operation, Definition        *ast.Document
	Walker                       *astvisitor.Walker
	Importer                     astimport.Importer
	Config                       Configuration
	plan                         Plan
	OperationName                string
	operationDefinition          int
	objects                      []*resolve.Object
	currentFields                []objectFields
	currentField                 *resolve.Field
	planners                     []*plannerConfiguration
	skipFieldsRefs               []int
	fieldConfigs                 map[int]*FieldConfiguration
	exportedVariables            map[string]struct{}
	skipIncludeOnFragments       map[int]skipIncludeInfo
	disableResolveFieldPositions bool

	fieldByPaths    map[string]*resolve.Field
	allowFieldMerge bool
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

func (v *Visitor) AllowVisitor(kind astvisitor.VisitorKind, ref int, visitor interface{}, skipFor astvisitor.SkipVisitors) bool {
	if visitor == v {
		return true
	}
	path := v.Walker.Path.DotDelimitedString()

	switch kind {
	case astvisitor.EnterField, astvisitor.LeaveField:
		fieldAliasOrName := v.Operation.FieldAliasOrNameString(ref)
		path = path + "." + fieldAliasOrName
	}
	if !strings.Contains(path, ".") {
		return true
	}
	for _, config := range v.planners {
		if config.planner == visitor && config.hasPath(path) {
			switch kind {
			case astvisitor.EnterField, astvisitor.LeaveField:
				fieldName := v.Operation.FieldNameString(ref)
				enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)

				shouldWalkFieldsOnPath :=
					// check if the field path has type condition and matches the enclosing type
					config.shouldWalkFieldsOnPath(path, enclosingTypeName) ||
						// check if the planner has path without type condition
						// this could happen in case of union type
						// or if there was added missing parent path
						config.shouldWalkFieldsOnPath(path, "")

				if pp, ok := config.planner.(DataSourceDebugger); ok {
					pp.DebugPrint("allow:", shouldWalkFieldsOnPath, " AllowVisitor: Field", " ref:", ref, " enclosingTypeName:", enclosingTypeName, " field:", fieldName, " path:", path)
				}

				return shouldWalkFieldsOnPath
			case astvisitor.EnterInlineFragment, astvisitor.LeaveInlineFragment:
				typeCondition := v.Operation.InlineFragmentTypeConditionNameString(ref)
				hasRootOrHasChildNode := config.dataSourceConfiguration.HasRootNodeWithTypename(typeCondition) ||
					config.dataSourceConfiguration.HasChildNodeWithTypename(typeCondition)

				if pp, ok := config.planner.(DataSourceDebugger); ok {
					pp.DebugPrint("allow:", hasRootOrHasChildNode, " AllowVisitor: InlineFragment", " ref:", ref, " typeCondition:", typeCondition)
				}

				return hasRootOrHasChildNode
			case astvisitor.EnterSelectionSet, astvisitor.LeaveSelectionSet:
				allowedByParent := skipFor.Allow(config.planner)

				if !allowedByParent {
					if pp, ok := config.planner.(DataSourceDebugger); ok {
						pp.DebugPrint("allow:", false, " AllowVisitor: SelectionSet", " ref:", ref)
					}

					// do not override a parent's decision
					return false
				}

				allow := !config.isExitPath(path)

				if pp, ok := config.planner.(DataSourceDebugger); ok {
					pp.DebugPrint("allow:", allow, " AllowVisitor: SelectionSet", " ref:", ref)
				}

				return allow
			default:
				return skipFor.Allow(config.planner)
			}
		}
	}
	return false
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

	v.linkFetchConfiguration(ref)

	// check if we have to skip the field in the response
	// it means it was requested by the planner not the user
	if v.skipField(ref) {
		return
	}

	fieldName := v.Operation.FieldNameBytes(ref)
	fieldAliasOrName := v.Operation.FieldAliasOrNameBytes(ref)

	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}
	fieldDefinitionTypeRef := v.Definition.FieldDefinitionType(fieldDefinition)

	fullFieldPathWithoutFragments := v.currentFullPath(true)

	// if we already have a field with the same path we merge existing field with the current one
	if v.allowFieldMerge && v.handleExistingField(ref, fieldDefinitionTypeRef, fullFieldPathWithoutFragments) {
		return
	}

	skipIncludeInfo := v.resolveSkipIncludeForField(ref)

	onTypeNames := v.resolveOnTypeNames(ref)

	if bytes.Equal(fieldName, literal.TYPENAME) {
		v.currentField = &resolve.Field{
			Name: fieldAliasOrName,
			Value: &resolve.String{
				Nullable:   false,
				Path:       []string{v.Operation.FieldAliasOrNameString(ref)},
				IsTypeName: true,
			},
			OnTypeNames:             onTypeNames,
			Position:                v.resolveFieldPosition(ref),
			SkipDirectiveDefined:    skipIncludeInfo.skip,
			SkipVariableName:        skipIncludeInfo.skipVariableName,
			IncludeDirectiveDefined: skipIncludeInfo.include,
			IncludeVariableName:     skipIncludeInfo.includeVariableName,
			Info:                    v.resolveFieldInfo(ref, fieldDefinitionTypeRef, onTypeNames),
		}
	} else {
		path := v.resolveFieldPath(ref)
		v.currentField = &resolve.Field{
			Name:                    fieldAliasOrName,
			Value:                   v.resolveFieldValue(ref, fieldDefinitionTypeRef, true, path),
			OnTypeNames:             onTypeNames,
			Position:                v.resolveFieldPosition(ref),
			SkipDirectiveDefined:    skipIncludeInfo.skip,
			SkipVariableName:        skipIncludeInfo.skipVariableName,
			IncludeDirectiveDefined: skipIncludeInfo.include,
			IncludeVariableName:     skipIncludeInfo.includeVariableName,
			Info:                    v.resolveFieldInfo(ref, fieldDefinitionTypeRef, onTypeNames),
		}
	}

	// append the field to the current object
	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, v.currentField)

	// track the added field by its path
	v.fieldByPaths[fullFieldPathWithoutFragments] = v.currentField

	v.mapFieldConfig(ref)
}

func (v *Visitor) handleExistingField(currentFieldRef int, fieldDefinitionTypeRef int, fullFieldPathWithoutFragments string) (exists bool) {
	resolveField := v.fieldByPaths[fullFieldPathWithoutFragments]
	if resolveField == nil {
		return false
	}

	// merge on type names
	onTypeNames := v.resolveOnTypeNames(currentFieldRef)
	hasOnTypeNames := len(resolveField.OnTypeNames) > 0 && len(onTypeNames) > 0
	if hasOnTypeNames {
		for _, t := range onTypeNames {
			if !slices.ContainsFunc(resolveField.OnTypeNames, func(existingT []byte) bool {
				return bytes.Equal(existingT, t)
			}) {
				resolveField.OnTypeNames = append(resolveField.OnTypeNames, t)
			}
		}
	} else {
		// if one of the duplicates request the field unconditionally, we remove the on type names
		resolveField.OnTypeNames = nil
	}

	// merge field info
	maybeAdditionalInfo := v.resolveFieldInfo(currentFieldRef, fieldDefinitionTypeRef, onTypeNames)
	if resolveField.Info != nil && maybeAdditionalInfo != nil {
		resolveField.Info.Merge(maybeAdditionalInfo)
	}

	// set field as current field
	v.currentField = resolveField

	var obj *resolve.Object
	switch node := v.currentField.Value.(type) {
	case *resolve.Array:
		obj, _ = node.Item.(*resolve.Object)
	case *resolve.Object:
		obj = node
	}

	if obj != nil {
		v.objects = append(v.objects, obj)
		v.Walker.DefferOnEnterField(func() {
			v.currentFields = append(v.currentFields, objectFields{
				popOnField: currentFieldRef,
				fields:     &obj.Fields,
			})
		})
	}

	// we have to map field config again with a different ref because it is used in input templates rendering
	v.mapFieldConfig(currentFieldRef)

	return true
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
	if !v.Config.IncludeInfo {
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

	sourceIDs := make([]string, 0, 1)

	for i := range v.planners {
		for j := range v.planners[i].paths {
			if v.planners[i].paths[j].fieldRef == ref {
				sourceIDs = append(sourceIDs, v.planners[i].dataSourceConfiguration.ID)
			}
		}
	}
	return &resolve.FieldInfo{
		Name:            fieldName,
		NamedType:       typeName,
		ParentTypeNames: parentTypeNames,
		Source: resolve.TypeFieldSource{
			IDs: sourceIDs,
		},
		ExactParentTypeName:  enclosingTypeName,
		HasAuthorizationRule: fieldHasAuthorizationRule,
	}
}

func (v *Visitor) fieldHasAuthorizationRule(typeName, fieldName string) bool {
	fieldConfig := v.Config.Fields.ForTypeField(typeName, fieldName)
	return fieldConfig != nil && fieldConfig.HasAuthorizationRule
}

func (v *Visitor) linkFetchConfiguration(fieldRef int) {
	for i := range v.planners {
		if fieldRef == v.planners[i].objectFetchConfiguration.fieldRef {
			if v.planners[i].objectFetchConfiguration.isSubscription {
				plan, ok := v.plan.(*SubscriptionResponsePlan)
				if ok {
					v.planners[i].objectFetchConfiguration.trigger = &plan.Response.Trigger
				}
			} else {
				v.planners[i].objectFetchConfiguration.object = v.objects[len(v.objects)-1]
			}
		}
	}
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

func (v *Visitor) resolveSkipIncludeForField(fieldRef int) skipIncludeInfo {
	if info, ok := v.resolveSkipIncludeOnParent(); ok {
		return info
	}

	directiveRefs := v.Operation.Fields[fieldRef].Directives.Refs
	skipVariableName, skip := v.Operation.ResolveSkipDirectiveVariable(directiveRefs)
	includeVariableName, include := v.Operation.ResolveIncludeDirectiveVariable(directiveRefs)

	if skip || include {
		return skipIncludeInfo{
			skip:                skip,
			skipVariableName:    skipVariableName,
			include:             include,
			includeVariableName: includeVariableName,
		}
	}

	return skipIncludeInfo{}
}

func (v *Visitor) resolveOnTypeNames(fieldRef int) [][]byte {
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
		// This should never be true. If it is, it's an error
		panic("resolveOnTypeNames called with a union type")
	}
	// We're dealing with an interface, so add all objects that implement this interface to the slice
	onTypeNames := make([][]byte, 0, 2)
	for objectTypeDefinitionRef := range v.Definition.ObjectTypeDefinitions {
		if v.Definition.ObjectTypeDefinitionImplementsInterface(objectTypeDefinitionRef, typeName) {
			onTypeNames = append(onTypeNames, v.Definition.ObjectTypeDefinitionNameBytes(objectTypeDefinitionRef))
		}
	}
	if len(onTypeNames) < 1 {
		return nil
	}

	return onTypeNames
}

func (v *Visitor) addInterfaceObjectNameToTypeNames(fieldRef int, typeName []byte, onTypeNames [][]byte) [][]byte {
	includeInterfaceObjectName := false
	var interfaceObjectName string
	for i := range v.planners {
		if !slices.ContainsFunc(v.planners[i].paths, func(path pathConfiguration) bool {
			return path.fieldRef == fieldRef
		}) {
			continue
		}

		for _, interfaceObjCfg := range v.planners[i].dataSourceConfiguration.FederationMetaData.InterfaceObjects {
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
		}
	case ast.TypeKindNamed:
		typeName := v.Definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.Definition.Index.FirstNodeByNameStr(typeName)
		if !ok {
			return &resolve.Null{}
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
			return &resolve.String{
				Path:                 path,
				Nullable:             nullable,
				UnescapeResponseJson: unescapeResponseJson,
			}
		case ast.NodeKindObjectTypeDefinition, ast.NodeKindInterfaceTypeDefinition, ast.NodeKindUnionTypeDefinition:
			object := &resolve.Object{
				Nullable:             nullable,
				Path:                 path,
				Fields:               []*resolve.Field{},
				UnescapeResponseJson: unescapeResponseJson,
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

	graphQLResponse := &resolve.GraphQLResponse{
		Data: rootObject,
	}

	if v.Config.IncludeInfo {
		graphQLResponse.Info = &resolve.GraphQLResponseInfo{
			OperationType: operationKind,
		}
	}

	if operationKind == ast.OperationTypeSubscription {
		v.plan = &SubscriptionResponsePlan{
			FlushInterval: v.Config.DefaultFlushIntervalMillis,
			Response: &resolve.GraphQLSubscription{
				Response: graphQLResponse,
			},
		}
		return
	}

	v.plan = &SynchronousResponsePlan{
		Response: graphQLResponse,
	}
}

func (v *Visitor) resolveFieldPath(ref int) []string {
	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldName := v.Operation.FieldNameUnsafeString(ref)
	plannerConfig := v.currentOrParentPlannerConfiguration()

	aliasOverride := false
	if plannerConfig != nil && plannerConfig.planner != nil {
		aliasOverride = plannerConfig.planner.DataSourcePlanningBehavior().OverrideFieldPathFromAlias
	}

	for i := range v.Config.Fields {
		if v.Config.Fields[i].TypeName == typeName && v.Config.Fields[i].FieldName == fieldName {
			if aliasOverride {
				override, exists := plannerConfig.planner.DownstreamResponseFieldAlias(ref)
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
	v.fieldByPaths = map[string]*resolve.Field{}
}

func (v *Visitor) LeaveDocument(_, _ *ast.Document) {
	for i := range v.planners {
		if v.planners[i].objectFetchConfiguration.isSubscription {
			v.configureSubscription(v.planners[i].objectFetchConfiguration)
		} else {
			v.configureObjectFetch(v.planners[i].objectFetchConfiguration)
		}
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s*\.(.*?)\s*}}`)
)

func (v *Visitor) currentOrParentPlannerConfiguration() *plannerConfiguration {
	const none = -1
	currentPath := v.currentFullPath(false)
	plannerIndex := none
	plannerPathDeepness := none

	for i := range v.planners {
		for _, plannerPath := range v.planners[i].paths {
			if v.isCurrentOrParentPath(currentPath, plannerPath.path) {
				currentPlannerPathDeepness := v.pathDeepness(plannerPath.path)
				if currentPlannerPathDeepness > plannerPathDeepness {
					plannerPathDeepness = currentPlannerPathDeepness
					plannerIndex = i
					break
				}
			}
		}
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

func (v *Visitor) resolveInputTemplates(config objectFetchConfiguration, input *string, variables *resolve.Variables) {
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
						renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(v.Operation, v.Definition, variableTypeRef, variablePath...)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					case RenderArgumentAsGraphQLValue:
						renderer, err := resolve.NewGraphQLVariableRendererFromTypeRef(v.Operation, v.Definition, variableTypeRef)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					case RenderArgumentAsJSONValue:
						renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(v.Operation, v.Definition, variableTypeRef)
						if err != nil {
							break
						}
						variable.Renderer = renderer
					}
				}
			}

			if variable.Renderer == nil {
				renderer, err := resolve.NewPlainVariableRendererWithValidationFromTypeRef(v.Operation, v.Definition, variableTypeRef, variablePath...)
				if err != nil {
					break
				}
				variable.Renderer = renderer
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
		inputType := v.Definition.InputValueDefinitions[inputValueDefinition].Type
		renderer, err := resolve.NewJSONVariableRendererWithValidationFromTypeRef(v.Definition, v.Definition, inputType)
		if err != nil {
			renderer = resolve.NewJSONVariableRenderer()
		}
		variableName, _ := variables.AddVariable(&resolve.ContextVariable{
			Path:     []string{variablePath},
			Renderer: renderer,
		})
		out += variableName
	}
	return
}

func (v *Visitor) configureSubscription(config objectFetchConfiguration) {
	subscription := config.planner.ConfigureSubscription()
	config.trigger.Variables = subscription.Variables
	config.trigger.Source = subscription.DataSource
	config.trigger.PostProcessing = subscription.PostProcessing
	v.resolveInputTemplates(config, &subscription.Input, &config.trigger.Variables)
	config.trigger.Input = []byte(subscription.Input)
}

func (v *Visitor) configureObjectFetch(config objectFetchConfiguration) {
	if config.object == nil {
		v.Walker.StopWithInternalErr(fmt.Errorf("object fetch configuration has empty object"))
		return
	}
	fetchConfig := config.planner.ConfigureFetch()
	fetch := v.configureFetch(config, fetchConfig)
	v.resolveInputTemplates(config, &fetch.Input, &fetch.Variables)

	if config.object.Fetch == nil {
		config.object.Fetch = fetch
		return
	}

	switch existing := config.object.Fetch.(type) {
	case *resolve.SingleFetch:
		copyOfExisting := *existing
		multi := &resolve.MultiFetch{
			Fetches: []*resolve.SingleFetch{&copyOfExisting, fetch},
		}
		config.object.Fetch = multi
	case *resolve.MultiFetch:
		existing.Fetches = append(existing.Fetches, fetch)
	}
}

func (v *Visitor) configureFetch(internal objectFetchConfiguration, external resolve.FetchConfiguration) *resolve.SingleFetch {
	dataSourceType := reflect.TypeOf(external.DataSource).String()
	dataSourceType = strings.TrimPrefix(dataSourceType, "*")

	singleFetch := &resolve.SingleFetch{
		FetchConfiguration:   external,
		FetchID:              internal.fetchID,
		DependsOnFetchIDs:    internal.dependsOnFetchIDs,
		DataSourceIdentifier: []byte(dataSourceType),
	}

	if v.Config.IncludeInfo {
		singleFetch.Info = &resolve.FetchInfo{
			DataSourceID: internal.sourceID,
			RootFields:   internal.rootFields,
		}
	}

	return singleFetch
}
