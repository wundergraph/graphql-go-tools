package plan

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/astvisitor"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// cacheProvidesDataVisitor is the P1 caching walk (PLAN.md D9): a gated
// SECOND, filter-free walk over the operation, run on the planningWalker AFTER
// the main planning walk. Per planner (i.e. per fetch) it builds the exact
// field tree that fetch RETURNS — alias-aware (Field.OriginalName),
// argument-aware (Field.CacheArgs), with inline-fragment OnTypeNames and the
// entity-boundary reset (a nested entity fetch provides the ENTITY's fields,
// not the path down to it). It reads planningVisitor.fieldPlanners, which the
// main walk populates in LeaveField, so it must never run before the main
// walk; it never re-runs the planning visitor and never rebuilds the plan.
type cacheProvidesDataVisitor struct {
	*astvisitor.Walker

	operation, definition *ast.Document
	config                Configuration
	// planners / fieldPlanners are the main walk's outputs: the per-planner
	// configurations and the field→planner-IDs attribution.
	planners      []PlannerConfiguration
	fieldPlanners map[int][]int
	// objects is the per-planner tree under construction; currentFields the
	// per-planner frame stack; entityBoundaryPaths remembers where a nested
	// entity fetch's tree was reset so its direct children get OnTypeNames.
	objects             map[int]*resolve.Object
	currentFields       map[int][]objectFields
	entityBoundaryPaths map[int]string
}

// cacheProvidesDataFragmentMarkerRegex strips normalization fragment markers
// (e.g. ".$0User") from walker paths so they compare against fetch response
// paths.
var cacheProvidesDataFragmentMarkerRegex = regexp.MustCompile(`\.\$\d+\w+`)

// reset clears all per-plan state so a Planner can be reused across Plan calls.
func (v *cacheProvidesDataVisitor) reset() {
	v.objects = map[int]*resolve.Object{}
	v.currentFields = map[int][]objectFields{}
	v.entityBoundaryPaths = map[int]string{}
}

func (v *cacheProvidesDataVisitor) EnterField(ref int) {
	for _, plannerID := range v.fieldPlanners[ref] {
		v.trackField(plannerID, ref)
	}
}

func (v *cacheProvidesDataVisitor) LeaveField(ref int) {
	for _, plannerID := range v.fieldPlanners[ref] {
		v.popFields(plannerID, ref)
	}
}

// attachTo resolves each planner to its fetch's *FetchInfo (the
// identity-stable key across dedup, fetchID-append, and concrete-type
// conversion, which all copy Info by reference) and stashes the side-table on
// the plan's response. Planner IDs are walked in sorted order for determinism.
func (v *cacheProvidesDataVisitor) attachTo(p Plan) {
	resp := responseOf(p)
	if resp == nil {
		return
	}
	out := make(map[*resolve.FetchInfo]*resolve.Object, len(v.objects))
	for _, plannerID := range slices.Sorted(maps.Keys(v.objects)) {
		if plannerID < 0 || plannerID >= len(v.planners) {
			continue
		}
		fetchConfig := v.planners[plannerID].ObjectFetchConfiguration()
		if fetchConfig == nil || fetchConfig.fetchItem == nil || fetchConfig.fetchItem.Fetch == nil {
			continue
		}
		info := fetchConfig.fetchItem.Fetch.FetchInfo()
		if info == nil {
			continue
		}
		out[info] = v.objects[plannerID]
	}
	resp.SetCacheProvidesData(out)
}

// trackField appends the field to the planner's current frame; on the
// planner's entity boundary it RESETS the tree instead, so a nested entity
// fetch's ProvidesData starts at the entity, not at the query root.
func (v *cacheProvidesDataVisitor) trackField(plannerID, fieldRef int) {
	if !v.ensurePlanner(plannerID) {
		return
	}

	fieldName := v.operation.FieldNameBytes(fieldRef)
	fetchResponseKey := v.operation.FieldAliasOrNameString(fieldRef)

	if v.isEntityBoundaryField(plannerID, fieldRef) {
		entityObj := &resolve.Object{
			Fields: []*resolve.Field{},
		}
		v.Walker.RunAfterEnterField(func() {
			v.currentFields[plannerID] = append(v.currentFields[plannerID], objectFields{
				popOnField: fieldRef,
				fields:     &entityObj.Fields,
			})
		})
		v.objects[plannerID] = entityObj
		return
	}

	// __typename dedup: normalization can leave the same __typename selection
	// twice in one frame; a second entry would double-count coverage.
	if bytes.Equal(fieldName, literal.TYPENAME) && len(v.currentFields[plannerID]) > 0 {
		currentFields := v.currentFields[plannerID][len(v.currentFields[plannerID])-1]
		for _, existingField := range *currentFields.fields {
			if !bytes.Equal(existingField.Name, []byte(fetchResponseKey)) {
				continue
			}
			existingValue, ok := existingField.Value.(*resolve.Scalar)
			if ok && len(existingValue.Path) > 0 && existingValue.Path[0] == fetchResponseKey {
				return
			}
		}
	}

	fieldDefinition, ok := v.Walker.FieldDefinition(fieldRef)
	if !ok {
		return
	}
	fieldType := v.definition.FieldDefinitionType(fieldDefinition)
	fieldValue := v.createFieldValue(fieldType, []string{fetchResponseKey})
	field := &resolve.Field{
		Name:        []byte(fetchResponseKey),
		Value:       fieldValue,
		OnTypeNames: v.resolveEntityOnTypeNames(plannerID, fieldRef, fieldName),
	}
	if fetchResponseKey != string(fieldName) {
		field.OriginalName = v.operation.FieldNameBytes(fieldRef)
	}
	if v.operation.FieldHasArguments(fieldRef) {
		// Root-operation-field arguments are part of the ROOT-FIELD cache key
		// (rendered from the fetch input), not per-field CacheArgs.
		enclosingType := v.Walker.EnclosingTypeDefinition.NameString(v.definition)
		if !v.definition.Index.IsRootOperationTypeNameString(enclosingType) {
			field.CacheArgs = v.captureFieldCacheArgs(fieldRef)
		}
	}

	currentFields := v.currentFields[plannerID][len(v.currentFields[plannerID])-1]
	*currentFields.fields = append(*currentFields.fields, field)

	// Descend through list nesting to the object node (if any) and push its
	// field slice as the new frame for this planner.
	for {
		switch node := fieldValue.(type) {
		case *resolve.Array:
			fieldValue = node.Item
		case *resolve.Object:
			v.Walker.RunAfterEnterField(func() {
				v.currentFields[plannerID] = append(v.currentFields[plannerID], objectFields{
					popOnField: fieldRef,
					fields:     &node.Fields,
				})
			})
			return
		default:
			return
		}
	}
}

// ensurePlanner lazily creates the planner's root object and frame stack.
func (v *cacheProvidesDataVisitor) ensurePlanner(plannerID int) bool {
	if plannerID < 0 || plannerID >= len(v.planners) {
		return false
	}
	if _, ok := v.objects[plannerID]; ok {
		return true
	}
	v.objects[plannerID] = &resolve.Object{
		Fields: []*resolve.Field{},
	}
	v.currentFields[plannerID] = []objectFields{
		{
			popOnField: -1,
			fields:     &v.objects[plannerID].Fields,
		},
	}
	return true
}

// captureFieldCacheArgs records the field's variable-bound arguments, sorted
// by argument name so cache keys are order-independent. Literal (inline)
// argument values are intentionally skipped: they are part of the normalized
// operation and thus already part of the fetch input the key derives from.
func (v *cacheProvidesDataVisitor) captureFieldCacheArgs(fieldRef int) []resolve.CacheFieldArg {
	argRefs := v.operation.FieldArguments(fieldRef)
	if len(argRefs) == 0 {
		return nil
	}

	args := make([]resolve.CacheFieldArg, 0, len(argRefs))
	for _, argRef := range argRefs {
		argName := v.operation.ArgumentNameString(argRef)
		argValue := v.operation.ArgumentValue(argRef)
		if argValue.Kind != ast.ValueKindVariable {
			continue
		}
		args = append(args, resolve.CacheFieldArg{
			Name:         argName,
			VariableName: v.operation.VariableValueNameString(argValue.Ref),
		})
	}
	if len(args) == 0 {
		return nil
	}
	slices.SortFunc(args, func(a, b resolve.CacheFieldArg) int {
		return cmp.Compare(a.Name, b.Name)
	})
	return args
}

// createFieldValue maps a schema type to the resolve node shape the coverage
// walk consumes: scalars/enums to Scalar, composites to Object, lists to
// Array; nullability follows the schema.
func (v *cacheProvidesDataVisitor) createFieldValue(typeRef int, path []string) resolve.Node {
	ofType := v.definition.Types[typeRef].OfType

	switch v.definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		node := v.createFieldValue(ofType, path)
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
		return &resolve.Array{
			Nullable: true,
			Path:     path,
			Item:     v.createFieldValue(ofType, nil),
		}
	case ast.TypeKindNamed:
		typeName := v.definition.ResolveTypeNameString(typeRef)
		typeDefinitionNode, ok := v.definition.Index.FirstNodeByNameStr(typeName)
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
			return &resolve.Object{
				Nullable: true,
				Path:     path,
				Fields:   []*resolve.Field{},
			}
		default:
			return &resolve.Null{}
		}
	default:
		return &resolve.Null{}
	}
}

// isEntityBoundaryField reports whether the field is exactly the planner's
// fetch response path, i.e. the point where a nested entity fetch's own data
// begins.
func (v *cacheProvidesDataVisitor) isEntityBoundaryField(plannerID, fieldRef int) bool {
	fetchConfig := v.planners[plannerID].ObjectFetchConfiguration()
	if fetchConfig == nil || fetchConfig.fetchItem == nil || fetchConfig.fetchItem.ResponsePath == "" {
		return false
	}

	currentPath := v.currentFullPath(fieldRef)
	rootPrefix := "query"
	if idx := strings.IndexByte(currentPath, '.'); idx > 0 {
		rootPrefix = currentPath[:idx]
	}
	responsePath := strings.ReplaceAll(rootPrefix+"."+fetchConfig.fetchItem.ResponsePath, ".@", "")
	normalizedFieldPath := v.normalizePathRemovingFragments(currentPath)
	if normalizedFieldPath == responsePath {
		v.entityBoundaryPaths[plannerID] = normalizedFieldPath
		return true
	}
	return false
}

// isEntityRootField reports whether the field is a DIRECT child of the
// planner's entity boundary; those fields carry the entity type condition.
func (v *cacheProvidesDataVisitor) isEntityRootField(plannerID, fieldRef int) bool {
	boundaryPath, ok := v.entityBoundaryPaths[plannerID]
	if !ok {
		return false
	}
	return v.isEntityRootPath(boundaryPath, v.currentFullPath(fieldRef))
}

func (v *cacheProvidesDataVisitor) isEntityRootPath(boundaryPath, fullFieldPath string) bool {
	normalized := v.normalizePathRemovingFragments(fullFieldPath)
	rest, ok := strings.CutPrefix(normalized, boundaryPath+".")
	if !ok {
		return false
	}
	return !strings.Contains(rest, ".")
}

func (v *cacheProvidesDataVisitor) currentFullPath(fieldRef int) string {
	path := v.Walker.Path.DotDelimitedString()
	if v.Walker.CurrentKind == ast.NodeKindField {
		path += "." + v.operation.FieldAliasOrNameString(fieldRef)
	}
	return path
}

func (v *cacheProvidesDataVisitor) normalizePathRemovingFragments(path string) string {
	return cacheProvidesDataFragmentMarkerRegex.ReplaceAllString(path, "")
}

func (v *cacheProvidesDataVisitor) popFields(plannerID, fieldRef int) {
	fields, ok := v.currentFields[plannerID]
	if !ok || len(fields) == 0 {
		return
	}

	last := len(fields) - 1
	if fields[last].popOnField == fieldRef {
		v.currentFields[plannerID] = fields[:last]
	}
}

// resolveEntityOnTypeNames gives the entity boundary's direct children the
// enclosing entity type as their type condition (an _entities response is
// polymorphic); everything else falls through to the inline-fragment rules.
func (v *cacheProvidesDataVisitor) resolveEntityOnTypeNames(plannerID, fieldRef int, fieldName ast.ByteSlice) (onTypeNames [][]byte) {
	if v.isEntityRootField(plannerID, fieldRef) {
		enclosingTypeName := v.Walker.EnclosingTypeDefinition.NameBytes(v.definition)
		if enclosingTypeName != nil {
			return [][]byte{enclosingTypeName}
		}
	}
	return v.resolveOnTypeNames(fieldRef, fieldName)
}

// resolveOnTypeNames derives the field's type conditions from its enclosing
// inline fragment: concrete conditions pass through (plus a possible
// interfaceObject remap); abstract conditions expand to the implementing /
// member types, narrowed by the grandparent type where fragments nest.
func (v *cacheProvidesDataVisitor) resolveOnTypeNames(fieldRef int, fieldName ast.ByteSlice) (onTypeNames [][]byte) {
	if len(v.Walker.Ancestors) < 2 {
		return nil
	}
	inlineFragment := v.Walker.Ancestors[len(v.Walker.Ancestors)-2]
	if inlineFragment.Kind != ast.NodeKindInlineFragment {
		return nil
	}
	typeName := v.operation.InlineFragmentTypeConditionName(inlineFragment.Ref)
	if typeName == nil {
		typeName = v.Walker.EnclosingTypeDefinition.NameBytes(v.definition)
	}
	node, exists := v.definition.NodeByName(typeName)
	if !exists || !node.Kind.IsAbstractType() {
		return v.addInterfaceObjectNameToTypeNames(fieldRef, typeName, [][]byte{v.config.Types.RenameTypeNameOnMatchBytes(typeName)})
	}

	if node.Kind == ast.NodeKindUnionTypeDefinition {
		if !bytes.Equal(fieldName, literal.TYPENAME) {
			// Only __typename can be selected directly on a union.
			v.Walker.StopWithInternalErr(fmt.Errorf("resolveOnTypeNames called with a union type and field %s", fieldName))
			return nil
		}

		typeNames, ok := v.definition.UnionTypeDefinitionMemberTypeNamesAsBytes(node.Ref)
		if ok {
			onTypeNames = typeNames
		}
	} else {
		typeNames, ok := v.definition.InterfaceTypeDefinitionImplementedByObjectWithNamesAsBytes(node.Ref)
		if ok {
			onTypeNames = typeNames
		}
	}

	// Narrow the expansion by the grandparent scope where fragments nest.
	if len(v.Walker.TypeDefinitions) > 1 {
		grandParent := v.Walker.TypeDefinitions[len(v.Walker.TypeDefinitions)-2]
		switch grandParent.Kind {
		case ast.NodeKindUnionTypeDefinition:
			for i := 0; i < len(onTypeNames); i++ {
				possibleMember, exists := v.definition.Index.FirstNodeByNameStr(string(onTypeNames[i]))
				if !exists {
					continue
				}
				if !v.definition.NodeIsUnionMember(possibleMember, grandParent) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		case ast.NodeKindInterfaceTypeDefinition:
			objectTypesImplementingGrandParent, _ := v.definition.InterfaceTypeDefinitionImplementedByObjectWithNames(grandParent.Ref)
			for i := 0; i < len(onTypeNames); i++ {
				if !slices.Contains(objectTypesImplementingGrandParent, string(onTypeNames[i])) {
					onTypeNames = append(onTypeNames[:i], onTypeNames[i+1:]...)
					i--
				}
			}
		case ast.NodeKindObjectTypeDefinition:
			grandParentTypeName := grandParent.NameBytes(v.definition)
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

// addInterfaceObjectNameToTypeNames appends the interfaceObject interface name
// when a planner resolving this field declares the concrete type as an
// interfaceObject member, mirroring how such subgraphs respond with the
// interface type name.
func (v *cacheProvidesDataVisitor) addInterfaceObjectNameToTypeNames(fieldRef int, typeName []byte, onTypeNames [][]byte) [][]byte {
	for i := range v.planners {
		if !v.planners[i].HasPathWithFieldRef(fieldRef) {
			continue
		}

		for _, interfaceObjCfg := range v.planners[i].DataSourceConfiguration().FederationConfiguration().InterfaceObjects {
			if slices.Contains(interfaceObjCfg.ConcreteTypeNames, string(typeName)) {
				return append(onTypeNames, []byte(interfaceObjCfg.InterfaceTypeName))
			}
		}
	}
	return onTypeNames
}

// responseOf extracts the GraphQLResponse the side-table attaches to.
func responseOf(p Plan) *resolve.GraphQLResponse {
	switch t := p.(type) {
	case *SynchronousResponsePlan:
		return t.Response
	case *DeferResponsePlan:
		if t.Response == nil {
			return nil
		}
		return t.Response.Response
	case *SubscriptionResponsePlan:
		if t.Response == nil {
			return nil
		}
		return t.Response.Response
	default:
		return nil
	}
}
