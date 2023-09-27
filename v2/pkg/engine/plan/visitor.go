package plan

import (
	"bytes"
	"fmt"
	"reflect"
	"regexp"
	"strings"

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
	fetchConfigurations          []objectFetchConfiguration
	skipFieldsRefs               []int
	fieldConfigs                 map[int]*FieldConfiguration
	exportedVariables            map[string]struct{}
	skipIncludeFields            map[int]skipIncludeField
	disableResolveFieldPositions bool
}

func (v *Visitor) debugOnEnterNode(kind ast.NodeKind, ref int) {
	if !v.Config.Debug.PlanningVisitor {
		return
	}

	switch kind {
	case ast.NodeKindField:
		fieldName := v.Operation.FieldNameString(ref)
		v.debugPrint("EnterField : ", fieldName, " ref: ", ref)
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
		v.debugPrint("LeaveField : ", fieldName, " ref: ", ref)
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

type skipIncludeField struct {
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
				_ = fieldName

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

func (v *Visitor) currentFullPath() string {
	path := v.Walker.Path.DotDelimitedString()
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
	directives := v.Operation.InlineFragments[ref].Directives.Refs
	skip, skipVariableName := v.resolveSkip(directives)
	include, includeVariableName := v.resolveInclude(directives)
	set := v.Operation.InlineFragments[ref].SelectionSet
	if set == -1 {
		return
	}
	for _, selection := range v.Operation.SelectionSets[set].SelectionRefs {
		switch v.Operation.Selections[selection].Kind {
		case ast.SelectionKindField:
			ref := v.Operation.Selections[selection].Ref
			if skip || include {
				v.skipIncludeFields[ref] = skipIncludeField{
					skip:                skip,
					skipVariableName:    skipVariableName,
					include:             include,
					includeVariableName: includeVariableName,
				}
			}
		}
	}
}

func (v *Visitor) LeaveInlineFragment(ref int) {
	v.debugOnEnterNode(ast.NodeKindInlineFragment, ref)
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

	if v.skipField(ref) {
		return
	}

	skip, skipVariableName := v.resolveSkipForField(ref)
	include, includeVariableName := v.resolveIncludeForField(ref)

	fieldName := v.Operation.FieldNameBytes(ref)
	fieldAliasOrName := v.Operation.FieldAliasOrNameBytes(ref)
	if bytes.Equal(fieldName, literal.TYPENAME) {
		v.currentField = &resolve.Field{
			Name: fieldAliasOrName,
			Value: &resolve.String{
				Nullable:   false,
				Path:       []string{v.Operation.FieldAliasOrNameString(ref)},
				IsTypeName: true,
			},
			OnTypeNames:             v.resolveOnTypeNames(),
			Position:                v.resolveFieldPosition(ref),
			SkipDirectiveDefined:    skip,
			SkipVariableName:        skipVariableName,
			IncludeDirectiveDefined: include,
			IncludeVariableName:     includeVariableName,
		}
		*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, v.currentField)
		return
	}

	fieldDefinition, ok := v.Walker.FieldDefinition(ref)
	if !ok {
		return
	}

	path := v.resolveFieldPath(ref)
	fieldDefinitionType := v.Definition.FieldDefinitionType(fieldDefinition)

	v.currentField = &resolve.Field{
		Name:                    fieldAliasOrName,
		Value:                   v.resolveFieldValue(ref, fieldDefinitionType, true, path),
		OnTypeNames:             v.resolveOnTypeNames(),
		Position:                v.resolveFieldPosition(ref),
		SkipDirectiveDefined:    skip,
		SkipVariableName:        skipVariableName,
		IncludeDirectiveDefined: include,
		IncludeVariableName:     includeVariableName,
	}

	*v.currentFields[len(v.currentFields)-1].fields = append(*v.currentFields[len(v.currentFields)-1].fields, v.currentField)

	typeName := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
	fieldNameStr := v.Operation.FieldNameString(ref)
	fieldConfig := v.Config.Fields.ForTypeField(typeName, fieldNameStr)
	if fieldConfig == nil {
		return
	}
	v.fieldConfigs[ref] = fieldConfig
}

func (v *Visitor) linkFetchConfiguration(fieldRef int) {
	for i := range v.fetchConfigurations {
		if fieldRef == v.fetchConfigurations[i].fieldRef {
			if v.fetchConfigurations[i].isSubscription {
				plan, ok := v.plan.(*SubscriptionResponsePlan)
				if ok {
					v.fetchConfigurations[i].trigger = &plan.Response.Trigger
				}
			} else {
				v.fetchConfigurations[i].object = v.objects[len(v.objects)-1]
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

func (v *Visitor) resolveSkipForField(ref int) (bool, string) {
	skipInclude, ok := v.skipIncludeFields[ref]
	if ok {
		return skipInclude.skip, skipInclude.skipVariableName
	}
	return v.resolveSkip(v.Operation.Fields[ref].Directives.Refs)
}

func (v *Visitor) resolveIncludeForField(ref int) (bool, string) {
	skipInclude, ok := v.skipIncludeFields[ref]
	if ok {
		return skipInclude.include, skipInclude.includeVariableName
	}
	return v.resolveInclude(v.Operation.Fields[ref].Directives.Refs)
}

func (v *Visitor) resolveSkip(directiveRefs []int) (bool, string) {
	for _, i := range directiveRefs {
		if v.Operation.DirectiveNameString(i) != "skip" {
			continue
		}
		if value, ok := v.Operation.DirectiveArgumentValueByName(i, literal.IF); ok {
			if value.Kind == ast.ValueKindVariable {
				return true, v.Operation.VariableValueNameString(value.Ref)
			}
		}
	}
	return false, ""
}

func (v *Visitor) resolveInclude(directiveRefs []int) (bool, string) {
	for _, i := range directiveRefs {
		if v.Operation.DirectiveNameString(i) != "include" {
			continue
		}
		if value, ok := v.Operation.DirectiveArgumentValueByName(i, literal.IF); ok {
			if value.Kind == ast.ValueKindVariable {
				return true, v.Operation.VariableValueNameString(value.Ref)
			}
		}
	}
	return false, ""
}

func (v *Visitor) resolveOnTypeNames() [][]byte {
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
		return [][]byte{v.Config.Types.RenameTypeNameOnMatchBytes(typeName)}
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

	isSubscription, _, err := AnalyzePlanKind(v.Operation, v.Definition, v.OperationName)
	if err != nil {
		v.Walker.StopWithInternalErr(err)
		return
	}

	graphQLResponse := &resolve.GraphQLResponse{
		Data: rootObject,
	}

	if isSubscription {
		v.plan = &SubscriptionResponsePlan{
			FlushInterval: v.Config.DefaultFlushIntervalMillis,
			Response: &resolve.GraphQLSubscription{
				Response: graphQLResponse,
			},
		}
		return
	}

	/*if isStreaming {

	}*/

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
	v.skipIncludeFields = map[int]skipIncludeField{}
}

func (v *Visitor) LeaveDocument(_, _ *ast.Document) {
	for _, config := range v.fetchConfigurations {
		if config.isSubscription {
			v.configureSubscription(config)
		} else {
			v.configureObjectFetch(config)
		}
	}
}

var (
	templateRegex = regexp.MustCompile(`{{.*?}}`)
	selectorRegex = regexp.MustCompile(`{{\s*\.(.*?)\s*}}`)
)

func (v *Visitor) currentOrParentPlannerConfiguration() *plannerConfiguration {
	const none = -1
	currentPath := v.currentFullPath()
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

	switch f := fetch.(type) {
	case *resolve.SingleFetch:
		v.resolveInputTemplates(config, &f.Input, &f.Variables)
	}
	if config.object.Fetch == nil {
		config.object.Fetch = fetch
		return
	}
	switch existing := config.object.Fetch.(type) {
	case *resolve.SingleFetch:
		copyOfExisting := *existing
		if copyOfExisting.RequiresSerialFetch {
			serial := &resolve.SerialFetch{
				Fetches: []resolve.Fetch{&copyOfExisting, fetch},
			}
			config.object.Fetch = serial
		} else {
			parallel := &resolve.ParallelFetch{
				Fetches: []resolve.Fetch{&copyOfExisting, fetch},
			}
			config.object.Fetch = parallel
		}
	case *resolve.ParallelFetch:
		existing.Fetches = append(existing.Fetches, fetch)
	case *resolve.SerialFetch:
		existing.Fetches = append(existing.Fetches, fetch)
	}
}

func (v *Visitor) configureFetch(internal objectFetchConfiguration, external FetchConfiguration) resolve.Fetch {
	dataSourceType := reflect.TypeOf(external.DataSource).String()
	dataSourceType = strings.TrimPrefix(dataSourceType, "*")

	singleFetch := &resolve.SingleFetch{
		Input:                                 external.Input,
		DataSource:                            external.DataSource,
		Variables:                             external.Variables,
		DisallowSingleFlight:                  external.DisallowSingleFlight,
		RequiresSerialFetch:                   external.RequiresSerialFetch,
		RequiresBatchFetch:                    external.RequiresBatchFetch,
		RequiresParallelListItemFetch:         external.RequiresParallelListItemFetch,
		DataSourceIdentifier:                  []byte(dataSourceType),
		PostProcessing:                        external.PostProcessing,
		SetTemplateOutputToNullOnVariableNull: external.SetTemplateOutputToNullOnVariableNull,
		SerialID:                              internal.fetchID,
	}

	return singleFetch
}
