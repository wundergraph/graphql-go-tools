package plan

import (
	"bytes"
	"cmp"
	"regexp"
	"slices"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
)

// cachingPlannerState owns the cache-planning state extracted from Visitor.
// Keeps the entity-caching planner additions separate from the core
// response-shaping visitor.
type cachingPlannerState struct {
	visitor                          *Visitor
	entityAnalyticsCache             map[string]*resolve.ObjectCacheAnalytics
	requestScopedVisibleResponseKeys map[int]string
	requestScopedFetchAliases        map[int]string
	// plannerObjects stores the root object for each planner's ProvidesData
	// map plannerID -> root object
	plannerObjects map[int]*resolve.Object
	// plannerCurrentFields stores the current field stack for each planner
	// map plannerID -> field stack
	plannerCurrentFields map[int][]objectFields
	// plannerResponsePaths stores the response paths relative to each planner's root.
	// Paths are normalized: inline-fragment markers like ".$0User" are stripped so
	// prefix comparisons against plannerEntityBoundaryPaths match regardless of fragments.
	// map plannerID -> response path stack
	plannerResponsePaths map[int][]string
	// plannerEntityBoundaryPaths stores the entity boundary paths for each planner.
	// Stored in normalized form (no inline-fragment markers) so that isEntityRootField
	// can match regardless of how the query wraps the boundary in a fragment.
	// map plannerID -> entity boundary path
	plannerEntityBoundaryPaths map[int]string
}

func newCachingPlannerState(visitor *Visitor) *cachingPlannerState {
	return &cachingPlannerState{
		visitor: visitor,
	}
}

func (s *cachingPlannerState) setRequestScopedMaps(visibleResponseKeys, fetchAliases map[int]string) {
	s.requestScopedVisibleResponseKeys = visibleResponseKeys
	s.requestScopedFetchAliases = fetchAliases
}

func (s *cachingPlannerState) visibleResponseKey(fieldRef int) (string, bool) {
	visible, ok := s.requestScopedVisibleResponseKeys[fieldRef]
	return visible, ok
}

func (s *cachingPlannerState) fetchAlias(fieldRef int) (string, bool) {
	alias, ok := s.requestScopedFetchAliases[fieldRef]
	return alias, ok
}

func (s *cachingPlannerState) resetPlannerStructures() {
	s.plannerObjects = map[int]*resolve.Object{}
	s.plannerCurrentFields = map[int][]objectFields{}
	s.plannerResponsePaths = map[int][]string{}
}

// initializePlannerStructures seeds per-planner ProvidesData state so field tracking
// during the walk can push/pop onto a stable root. Safe to call when no planners
// are configured: the range over a nil slice is a no-op.
func (s *cachingPlannerState) initializePlannerStructures() {
	v := s.visitor
	for i := range v.planners {
		s.plannerObjects[i] = &resolve.Object{
			Fields: []*resolve.Field{},
		}
		s.plannerCurrentFields[i] = []objectFields{{
			fields:     &s.plannerObjects[i].Fields,
			popOnField: -1,
		}}
		s.plannerResponsePaths[i] = []string{}
	}
	s.plannerEntityBoundaryPaths = map[int]string{}
}

// trackFieldForPlanner adds field information to the planner's tracked object structure.
// It handles entity boundary detection, __typename field deduplication, and creates
// the appropriate field value nodes for the planner's representation of the query.
// The caller may pass any plannerID; shouldPlannerHandleField validates bounds and
// ownership in one place.
func (s *cachingPlannerState) trackFieldForPlanner(plannerID int, fieldRef int) {
	v := s.visitor
	if !v.shouldPlannerHandleField(plannerID, fieldRef) {
		return
	}

	fieldName := v.Operation.FieldNameBytes(fieldRef)
	fieldAliasOrName := v.Operation.FieldAliasOrNameString(fieldRef)
	fetchResponseKey := fieldAliasOrName
	if fetchAlias, ok := s.fetchAlias(fieldRef); ok {
		fetchResponseKey = fetchAlias
	}

	// For nested entity fetches, check if this field represents the entity boundary
	// If so, we should skip adding this field to ProvidesData and instead add its children
	if s.isEntityBoundaryField(plannerID, fieldRef) {
		// Create a new object for the entity fields (children of the boundary)
		// This ensures entity fields like id, username are added to this object, not the parent
		entityObj := &resolve.Object{
			Fields: []*resolve.Field{},
		}
		// Push the entity object onto the stack so child fields get added to it
		v.Walker.DefferOnEnterField(func() {
			s.plannerCurrentFields[plannerID] = append(s.plannerCurrentFields[plannerID], objectFields{
				popOnField: fieldRef,
				fields:     &entityObj.Fields,
			})
		})
		// Replace the root object for this planner with the entity object
		// This makes the entity fields the top-level fields in ProvidesData
		s.plannerObjects[plannerID] = entityObj
		return
	}

	// Check if this is a __typename field and if we already have one with the same name and path
	if bytes.Equal(fieldName, literal.TYPENAME) && len(s.plannerCurrentFields[plannerID]) > 0 {
		currentFields := s.plannerCurrentFields[plannerID][len(s.plannerCurrentFields[plannerID])-1]

		// Check if we already have a __typename field with the same name and path
		for _, existingField := range *currentFields.fields {
			if bytes.Equal(existingField.Name, []byte(fetchResponseKey)) {
				// For __typename fields, the path is [fieldAliasOrName]
				// Check if the existing field has the same path
				if existingValue, ok := existingField.Value.(*resolve.Scalar); ok {
					if len(existingValue.Path) > 0 && existingValue.Path[0] == fetchResponseKey {
						// We already have this __typename field with the same name and path, skip it
						return
					}
				}
			}
		}
	}

	fieldDefinition, ok := v.Walker.FieldDefinition(fieldRef)
	if !ok {
		return
	}
	fieldType := v.Definition.FieldDefinitionType(fieldDefinition)

	fieldValue := s.createFieldValueForPlanner(fieldType, []string{fetchResponseKey})

	onTypeNames := v.resolveEntityOnTypeNames(plannerID, fieldRef, fieldName)

	field := &resolve.Field{
		Name:        []byte(fetchResponseKey),
		Value:       fieldValue,
		OnTypeNames: onTypeNames,
	}
	if fetchResponseKey != string(fieldName) {
		field.OriginalName = v.Operation.FieldNameBytes(fieldRef)
	}
	// Capture field arguments for cache suffix computation at resolve time.
	// Skip root query fields (Query/Mutation/Subscription) — their args are already
	// part of the cache key, and suffixing would break entity key mapping.
	if v.Operation.FieldHasArguments(fieldRef) {
		enclosingType := v.Walker.EnclosingTypeDefinition.NameString(v.Definition)
		if !v.Definition.Index.IsRootOperationTypeNameString(enclosingType) {
			field.CacheArgs = s.captureFieldCacheArgs(fieldRef)
		}
	}

	if len(s.plannerCurrentFields[plannerID]) > 0 {
		currentFields := s.plannerCurrentFields[plannerID][len(s.plannerCurrentFields[plannerID])-1]
		*currentFields.fields = append(*currentFields.fields, field)
	}

	for {
		// for loop to unwrap array item
		switch node := fieldValue.(type) {
		case *resolve.Array:
			// unwrap and check type again
			fieldValue = node.Item
		case *resolve.Object:
			// if the field value is an object, add it to the current fields stack
			v.Walker.DefferOnEnterField(func() {
				s.plannerCurrentFields[plannerID] = append(s.plannerCurrentFields[plannerID], objectFields{
					popOnField: fieldRef,
					fields:     &node.Fields,
				})
			})
			return
		default:
			// field value is a scalar or null, we don't add it to the stack
			return
		}
	}
}

// captureFieldCacheArgs extracts argument metadata from a field for cache suffix computation.
// After normalization, all argument values are variable references (e.g., friends(first: $a)).
// We capture the arg name and variable path so the resolve-time suffix can look up actual values.
func (s *cachingPlannerState) captureFieldCacheArgs(fieldRef int) []resolve.CacheFieldArg {
	v := s.visitor
	argRefs := v.Operation.FieldArguments(fieldRef)
	if len(argRefs) == 0 {
		return nil
	}
	args := make([]resolve.CacheFieldArg, 0, len(argRefs))
	for _, argRef := range argRefs {
		argName := v.Operation.ArgumentNameString(argRef)
		argValue := v.Operation.ArgumentValue(argRef)
		if argValue.Kind == ast.ValueKindVariable {
			variableName := v.Operation.VariableValueNameString(argValue.Ref)
			args = append(args, resolve.CacheFieldArg{
				ArgName:      argName,
				VariableName: variableName,
			})
		}
	}
	if len(args) == 0 {
		return nil
	}
	// Sort by ArgName for deterministic suffix
	slices.SortFunc(args, func(a, b resolve.CacheFieldArg) int {
		return cmp.Compare(a.ArgName, b.ArgName)
	})
	return args
}

// createFieldValueForPlanner builds the resolve.Node shape used for ProvidesData
// tracking on a given planner. Unlike resolveFieldValue it does not mutate walker
// state (objects list, currentFields stack, etc.), so it can be invoked from
// trackFieldForPlanner during EnterField without side-effects on the main walk.
func (s *cachingPlannerState) createFieldValueForPlanner(typeRef int, path []string) resolve.Node {
	v := s.visitor
	ofType := v.Definition.Types[typeRef].OfType

	switch v.Definition.Types[typeRef].TypeKind {
	case ast.TypeKindNonNull:
		node := s.createFieldValueForPlanner(ofType, path)
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
		listItem := s.createFieldValueForPlanner(ofType, nil)
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
func (s *cachingPlannerState) isEntityBoundaryField(plannerID int, fieldRef int) bool {
	v := s.visitor
	config := v.planners[plannerID]
	fetchConfig := config.ObjectFetchConfiguration()
	if fetchConfig == nil || fetchConfig.fetchItem == nil {
		return false
	}

	// Check if this is a nested fetch (has "." in response path)
	if fetchConfig.fetchItem.ResponsePath == "" {
		return false // Root fetch, no boundary field to skip
	}

	// Determine the root path prefix from the walker path.
	// For queries this is "query", for mutations "mutation", for subscriptions "subscription".
	currentPath := v.Walker.Path.DotDelimitedString()
	rootPrefix := "query"
	if idx := strings.IndexByte(currentPath, '.'); idx > 0 {
		rootPrefix = currentPath[:idx]
	}
	responsePath := rootPrefix + "." + fetchConfig.fetchItem.ResponsePath

	// Normalize the response path by removing array index markers (@.)
	// e.g., "query.topProducts.@.reviews.@.author" -> "query.topProducts.reviews.author"
	normalizedResponsePath := strings.ReplaceAll(responsePath, ".@", "")

	// For nested fetches, check if this field is at the entity boundary
	fieldName := v.Operation.FieldAliasOrNameString(fieldRef)
	fullFieldPath := currentPath + "." + fieldName

	// Normalize the field path by removing inline fragment type conditions
	// e.g., "query.meInterface.$0User.reviews" -> "query.meInterface.reviews"
	// The walker path includes $N<TypeName> markers for inline fragments
	normalizedFieldPath := s.normalizePathRemovingFragments(fullFieldPath)

	// If this normalized field path matches the normalized response path, it's the entity boundary
	if normalizedFieldPath == normalizedResponsePath {
		// Store the entity boundary path for this planner (use normalized path)
		s.plannerEntityBoundaryPaths[plannerID] = normalizedFieldPath
		return true
	}
	return false
}

// normalizePathRemovingFragments removes inline fragment type condition markers from the path
// e.g., "query.meInterface.$0User.reviews" -> "query.meInterface.reviews"
// The walker path includes $N<TypeName> markers for inline fragments (e.g., $0User, $1Admin)
var fragmentMarkerRegex = regexp.MustCompile(`\.\$\d+\w+`)

func (s *cachingPlannerState) normalizePathRemovingFragments(path string) string {
	return fragmentMarkerRegex.ReplaceAllString(path, "")
}

// isEntityRootField checks if this field is at the root of an entity.
// It returns true when the field path is a direct child of the stored entity
// boundary path. The current walker path is normalized (inline-fragment markers
// stripped) before the prefix check — boundary paths are stored normalized by
// isEntityBoundaryField, so comparing a raw path here would miss queries that
// wrap the boundary in an inline fragment such as `... on User { reviews }`.
func (s *cachingPlannerState) isEntityRootField(plannerID int, fieldRef int) bool {
	v := s.visitor
	boundaryPath, hasBoundary := s.plannerEntityBoundaryPaths[plannerID]
	if !hasBoundary {
		return false
	}

	currentPath := v.Walker.Path.DotDelimitedString()
	fieldName := v.Operation.FieldAliasOrNameString(fieldRef)
	return s.isEntityRootPath(boundaryPath, currentPath+"."+fieldName)
}

// isEntityRootPath is the pure, walker-free core of isEntityRootField. It
// normalizes the candidate field path (stripping inline-fragment markers) and
// returns true when that path is a direct child of boundaryPath. Extracted so
// the inline-fragment / fragment-wrapping invariant from A42 can be unit-tested
// without staging a real walker.
func (s *cachingPlannerState) isEntityRootPath(boundaryPath, fullFieldPath string) bool {
	normalized := s.normalizePathRemovingFragments(fullFieldPath)
	if !strings.HasPrefix(normalized, boundaryPath+".") {
		return false
	}
	return !strings.Contains(strings.TrimPrefix(normalized, boundaryPath+"."), ".")
}

func (s *cachingPlannerState) popFieldsForPlanner(plannerID int, fieldRef int) {
	fields, ok := s.plannerCurrentFields[plannerID]
	if !ok {
		return
	}

	if len(fields) > 0 {
		last := len(fields) - 1
		if fields[last].popOnField == fieldRef {
			s.plannerCurrentFields[plannerID] = fields[:last]
		}
	}
}

// configureSubscriptionEntityCachePopulation determines whether the subscription
// should populate or invalidate L2 cache entries for root entities.
func (s *cachingPlannerState) configureSubscriptionEntityCachePopulation(config *objectFetchConfiguration) {
	v := s.visitor
	if len(config.rootFields) == 0 {
		return
	}

	ds := s.findDataSourceByID(config.sourceID)
	if ds == nil {
		return
	}

	fedConfigVal := ds.FederationConfiguration()
	fedConfig := &fedConfigVal
	if len(fedConfig.SubscriptionEntityPopulation) == 0 {
		return
	}

	// Get the subscription field's return type from the definition
	subscriptionField := config.rootFields[0]
	entityTypeName := s.subscriptionFieldReturnTypeName(subscriptionField.TypeName, subscriptionField.FieldName)
	if entityTypeName == "" {
		return
	}

	// Look up subscription entity population config with a 2-tier fallback:
	// 1. Exact match: type + field name (disambiguates when multiple subscription fields return the same entity type)
	// 2. Union/interface resolution: check member/implementor types
	resolvedTypeName, popConfig := s.resolveSubscriptionEntityPopulationConfig(entityTypeName, subscriptionField.FieldName, fedConfig)
	if popConfig == nil {
		return
	}
	entityTypeName = resolvedTypeName
	// Build EntityQueryCacheKeyTemplate from entity's @key fields
	entityKeys := fedConfig.RequiredFieldsByKey(entityTypeName)
	if len(entityKeys) == 0 {
		return
	}

	var objects []*resolve.Object
	for _, key := range entityKeys {
		node, err := BuildRepresentationVariableNode(v.Definition, key, *fedConfig)
		if err != nil {
			continue
		}
		objects = append(objects, node)
	}
	if len(objects) == 0 {
		return
	}

	mergedObject := MergeRepresentationVariableNodes(objects)
	cacheKeyTemplate := &resolve.EntityQueryCacheKeyTemplate{
		Keys:     resolve.NewResolvableObjectVariable(mergedObject),
		TypeName: entityTypeName,
	}

	// Determine populate vs invalidate mode:
	// Check if the subscription selects any non-key fields from this datasource for the entity type
	keyFieldNames := s.entityKeyFieldNames(entityKeys)
	hasNonKeyFields := s.subscriptionSelectsNonKeyFields(ds, entityTypeName, keyFieldNames)

	mode := resolve.SubscriptionCacheModePopulate
	if !hasNonKeyFields {
		if popConfig.EnableInvalidationOnKeyOnly {
			mode = resolve.SubscriptionCacheModeInvalidate
		} else {
			// No non-key fields and invalidation not enabled — nothing to do
			return
		}
	}

	// Use the alias (or name if no alias) from the operation AST, because
	// resolvable.data uses the response field name (alias) as the JSON key.
	subscriptionResponseFieldName := v.Operation.FieldAliasOrNameString(config.fieldRef)

	v.subscription.EntityCachePopulation = &resolve.SubscriptionEntityCachePopulation{
		Mode:                        mode,
		CacheKeyTemplate:            cacheKeyTemplate,
		CacheName:                   popConfig.CacheName,
		TTL:                         popConfig.TTL,
		IncludeSubgraphHeaderPrefix: popConfig.IncludeSubgraphHeaderPrefix,
		DataSourceName:              config.sourceName,
		SubscriptionFieldName:       subscriptionResponseFieldName,
		EntityTypeName:              entityTypeName,
	}
}

// resolveSubscriptionEntityPopulationConfig performs a 2-tier lookup for subscription
// entity population config:
//  1. Exact match by type name + subscription field name
//  2. Union/interface member resolution (when the subscription returns an abstract type)
//
// Returns the resolved entity type name (may differ from input if an abstract type was
// resolved to a concrete member) and the config. Returns ("", nil) if no match found.
func (s *cachingPlannerState) resolveSubscriptionEntityPopulationConfig(entityTypeName, fieldName string, fedConfig *FederationMetaData) (string, *SubscriptionEntityPopulationConfiguration) {
	// Tier 1: exact match on both type and field name
	if config := fedConfig.SubscriptionEntityPopulation.FindByTypeAndFieldName(entityTypeName, fieldName); config != nil {
		return entityTypeName, config
	}
	// Tier 2: abstract type resolution — check union members and interface implementors.
	if resolvedName, config := s.resolveAbstractEntityPopulation(entityTypeName, fieldName, fedConfig); config != nil {
		return resolvedName, config
	}
	return "", nil
}

// resolveAbstractEntityPopulation checks if typeName is a union or interface type and
// returns the first member/implementor that has a SubscriptionEntityPopulation config.
func (s *cachingPlannerState) resolveAbstractEntityPopulation(typeName, fieldName string, fedConfig *FederationMetaData) (string, *SubscriptionEntityPopulationConfiguration) {
	v := s.visitor
	node, exists := v.Definition.Index.FirstNodeByNameStr(typeName)
	if !exists {
		return "", nil
	}
	var candidates []string
	var ok bool
	switch node.Kind {
	case ast.NodeKindUnionTypeDefinition:
		candidates, ok = v.Definition.UnionTypeDefinitionMemberTypeNames(node.Ref)
	case ast.NodeKindInterfaceTypeDefinition:
		candidates, ok = v.Definition.InterfaceTypeDefinitionImplementedByObjectWithNames(node.Ref)
	default:
		return "", nil
	}
	if !ok {
		return "", nil
	}
	for _, name := range candidates {
		if cfg := fedConfig.SubscriptionEntityPopulation.FindByTypeAndFieldName(name, fieldName); cfg != nil {
			return name, cfg
		}
	}
	return "", nil
}

// subscriptionFieldReturnTypeName returns the named return type of a subscription field.
func (s *cachingPlannerState) subscriptionFieldReturnTypeName(typeName, fieldName string) string {
	v := s.visitor
	node, exists := v.Definition.Index.FirstNodeByNameStr(typeName)
	if !exists {
		return ""
	}
	if node.Kind != ast.NodeKindObjectTypeDefinition {
		return ""
	}
	for _, fieldDefRef := range v.Definition.ObjectTypeDefinitions[node.Ref].FieldsDefinition.Refs {
		if v.Definition.FieldDefinitionNameString(fieldDefRef) == fieldName {
			return v.Definition.FieldDefinitionTypeNameString(fieldDefRef)
		}
	}
	return ""
}

// entityKeyFieldNames extracts top-level field names from @key configurations.
// It walks the parsed field-set AST so nested keys like "org { id }" correctly
// yield only "org" rather than the previous superset {"org", "id"}.
func (s *cachingPlannerState) entityKeyFieldNames(keys []FederationFieldConfiguration) map[string]struct{} {
	result := make(map[string]struct{})
	for i := range keys {
		if err := keys[i].parseSelectionSet(); err != nil {
			continue
		}
		doc := keys[i].parsedSelectionSet
		if doc == nil || len(doc.FragmentDefinitions) == 0 {
			continue
		}

		selectionSetRef := doc.FragmentDefinitions[0].SelectionSet
		for _, fieldRef := range doc.SelectionSetFieldRefs(selectionSetRef) {
			fieldName := doc.FieldNameString(fieldRef)
			if fieldName == "" {
				continue
			}
			result[fieldName] = struct{}{}
		}
	}
	return result
}

// subscriptionSelectsNonKeyFields checks if the operation selects any fields
// from the given datasource for the entity type that are NOT @key fields.
// It iterates the fieldEnclosingTypeNames map (already narrowed to fields we
// have type info for) rather than every operation field ref.
func (s *cachingPlannerState) subscriptionSelectsNonKeyFields(ds DataSource, entityTypeName string, keyFieldNames map[string]struct{}) bool {
	v := s.visitor
	for fieldRef, enclosingType := range v.fieldEnclosingTypeNames {
		if enclosingType != entityTypeName {
			continue
		}
		opFieldName := v.Operation.FieldNameString(fieldRef)
		if opFieldName == "__typename" {
			continue
		}
		if _, isKey := keyFieldNames[opFieldName]; isKey {
			continue
		}
		if ds.HasChildNode(entityTypeName, opFieldName) || ds.HasRootNode(entityTypeName, opFieldName) {
			return true
		}
	}
	return false
}

// configureFetchCaching determines the cache configuration for a fetch.
// For entity fetches, it looks up per-entity configuration from FederationMetaData.
// Returns disabled caching if no configuration exists or if caching is globally disabled.
func (s *cachingPlannerState) configureFetchCaching(internal *objectFetchConfiguration, external resolve.FetchConfiguration) resolve.FetchCacheConfiguration {
	v := s.visitor
	// Populate ProvidesData on requestScoped fields using the planner's response
	// Object tree. This enables alias-aware normalization/denormalization (same
	// pipeline as entity L1 / L2 caches). Fields without aliases or args get a
	// fast path via Object.HasAliases.
	plannerObj := s.plannerObjects[internal.fetchID]
	requestScopedFields := s.populateRequestScopedFieldsProvidesData(external.Caching.RequestScopedFields, plannerObj)

	// Always preserve CacheKeyTemplate for L1 cache - L1 cache works independently of L2 cache.
	// The Enabled flag controls L2 cache only, not L1 cache.
	// L1 cache uses CacheKeyTemplate.Keys and is controlled by ctx.ExecutionOptions.Caching.EnableL1Cache.
	// UseL1Cache defaults to false - the postprocessor (optimizeL1Cache) will enable it when beneficial.
	result := resolve.FetchCacheConfiguration{
		CacheKeyTemplate:                   external.Caching.CacheKeyTemplate,
		RootFieldL1EntityCacheKeyTemplates: external.Caching.RootFieldL1EntityCacheKeyTemplates,
		RequestScopedFields:                requestScopedFields,
	}
	if rootTemplate, ok := external.Caching.CacheKeyTemplate.(*resolve.RootQueryCacheKeyTemplate); ok {
		result.BatchEntityKeyArgumentPathHint = rootTemplate.BatchEntityKeyArgumentPath()
	}

	// For mutations returning cached entities: enable mutation impact detection.
	// This runs before the L2 caching checks because mutations don't have CacheKeyTemplate
	// (they go through a separate path), but we still want to annotate the fetch for
	// runtime mutation impact detection.
	if internal.operationType == ast.OperationTypeMutation && len(internal.rootFields) > 0 {
		if !v.Config.DisableEntityCaching {
			s.configureMutationEntityImpact(internal, &result)
		}
		// Look up per-mutation-field cache config from the subgraph that owns the mutation
		ds := s.findDataSourceByID(internal.sourceID)
		if ds != nil {
			if mutConfig := ds.MutationFieldCacheConfig(internal.rootFields[0].FieldName); mutConfig != nil {
				result.EnableMutationL2CachePopulation = mutConfig.EnableEntityL2CachePopulation
				result.MutationCacheTTLOverride = mutConfig.TTL
			}
		}
	}

	// Global disable takes precedence for L2 cache
	if v.Config.DisableEntityCaching {
		return result
	}

	// No cache key template = caching not applicable
	if external.Caching.CacheKeyTemplate == nil {
		return result
	}

	// Must have at least 1 root field to determine cache config
	if len(internal.rootFields) == 0 {
		return result
	}

	// Find the datasource by ID to access FederationMetaData
	ds := s.findDataSourceByID(internal.sourceID)
	if ds == nil {
		return result
	}

	fedConfig := ds.FederationConfiguration()

	// Check if this is an entity fetch or a root field fetch
	if external.RequiresEntityFetch || external.RequiresEntityBatchFetch {
		// Entity fetch: look up cache config for the entity type
		// All root fields in an entity fetch belong to the same entity type
		entityTypeName := internal.rootFields[0].TypeName
		cacheConfig := fedConfig.EntityCacheConfig(entityTypeName)

		// Extract key fields from cache key template (plan time)
		var keyFields []resolve.KeyField
		if entityTemplate, ok := external.Caching.CacheKeyTemplate.(*resolve.EntityQueryCacheKeyTemplate); ok {
			keyFields = entityTemplate.KeyFields()
		}

		if cacheConfig == nil {
			// No config = L2 caching disabled for this entity (opt-in model)
			// L1 cache can still work since CacheKeyTemplate is preserved
			// Still provide key fields for analytics
			result.KeyFields = keyFields
			return result
		}

		// L2 cache is enabled for this entity type
		// UseL1Cache is set by the postprocessor (optimizeL1Cache) when beneficial
		return resolve.FetchCacheConfiguration{
			Enabled:                        true,
			CacheName:                      cacheConfig.CacheName,
			TTL:                            cacheConfig.TTL,
			CacheKeyTemplate:               external.Caching.CacheKeyTemplate,
			IncludeSubgraphHeaderPrefix:    cacheConfig.IncludeSubgraphHeaderPrefix,
			EnablePartialCacheLoad:         cacheConfig.EnablePartialCacheLoad,
			HashAnalyticsKeys:              cacheConfig.HashAnalyticsKeys,
			KeyFields:                      keyFields,
			ShadowMode:                     cacheConfig.ShadowMode,
			NegativeCacheTTL:               cacheConfig.NegativeCacheTTL,
			BatchEntityKeyArgumentPathHint: result.BatchEntityKeyArgumentPathHint,
			// Preserve requestScoped hints/exports through the entity-cache-enabled path.
			RequestScopedFields: requestScopedFields,
		}
	}

	// Root field fetch: find common cache config for all root fields
	// All root fields in the fetch must have the same cache config for L2 caching to be enabled

	// Root field caching only applies to queries - mutations and subscriptions
	// should never cache root field responses in L2 (they would never be read).
	if internal.operationType != ast.OperationTypeQuery {
		return result
	}

	var commonConfig *RootFieldCacheConfiguration
	for i := range internal.rootFields {
		rootField := internal.rootFields[i]
		cacheConfig := fedConfig.RootFieldCacheConfig(rootField.TypeName, rootField.FieldName)
		if cacheConfig == nil {
			// No config for this field = L2 caching disabled for this fetch
			return result
		}
		if commonConfig == nil {
			commonConfig = cacheConfig
		} else {
			// Check if config matches the common config
			if commonConfig.CacheName != cacheConfig.CacheName ||
				commonConfig.TTL != cacheConfig.TTL ||
				commonConfig.IncludeSubgraphHeaderPrefix != cacheConfig.IncludeSubgraphHeaderPrefix {
				// Different configs = can't enable L2 caching for this fetch
				return result
			}
		}
	}

	if commonConfig == nil {
		return result
	}

	// L2 cache is enabled - all root fields have the same cache config
	// UseL1Cache is set by the postprocessor (optimizeL1Cache) when beneficial
	return resolve.FetchCacheConfiguration{
		Enabled:                            true,
		CacheName:                          commonConfig.CacheName,
		TTL:                                commonConfig.TTL,
		CacheKeyTemplate:                   external.Caching.CacheKeyTemplate,
		IncludeSubgraphHeaderPrefix:        commonConfig.IncludeSubgraphHeaderPrefix,
		RootFieldL1EntityCacheKeyTemplates: external.Caching.RootFieldL1EntityCacheKeyTemplates,
		ShadowMode:                         commonConfig.ShadowMode,
		PartialBatchLoad:                   commonConfig.PartialBatchLoad,
		BatchEntityKeyArgumentPathHint:     result.BatchEntityKeyArgumentPathHint,
		// Preserve requestScoped fields through the L2-enabled root field path.
		RequestScopedFields: requestScopedFields,
	}
}

// populateRequestScopedFieldsProvidesData fills in ProvidesData by locating the
// matching sub-Object in the planner's response tree. The match is by response
// key (field.Name), since the datasource planner already resolves aliases.
//
// If plannerObj is nil or no matching field is found, ProvidesData is left nil
// (resolver falls back to raw byte storage, loses alias awareness).
func (s *cachingPlannerState) populateRequestScopedFieldsProvidesData(fields []resolve.RequestScopedField, plannerObj *resolve.Object) []resolve.RequestScopedField {
	if len(fields) == 0 || plannerObj == nil {
		return fields
	}
	out := make([]resolve.RequestScopedField, len(fields))
	for i, f := range fields {
		out[i] = f
		sub := s.findObjectFieldByResponseKey(plannerObj, f.FieldName)
		if sub != nil {
			resolve.ComputeHasAliases(sub)
			out[i].ProvidesData = sub
		}
	}
	return out
}

// findObjectFieldByResponseKey walks the Object's top-level fields looking for one
// whose response key (field.Name) matches, and returns its value Object (if the
// value is an Object). Returns nil if not found or if the value is not an Object.
func (s *cachingPlannerState) findObjectFieldByResponseKey(obj *resolve.Object, responseKey string) *resolve.Object {
	if obj == nil {
		return nil
	}
	for _, field := range obj.Fields {
		if string(field.Name) == responseKey {
			if sub, ok := field.Value.(*resolve.Object); ok {
				return sub
			}
			return nil
		}
	}
	return nil
}

// findDataSourceByID finds the datasource configuration for a given source ID
func (s *cachingPlannerState) findDataSourceByID(sourceID string) DataSource {
	v := s.visitor
	for i := range v.Config.DataSources {
		if v.Config.DataSources[i].Id() == sourceID {
			return v.Config.DataSources[i]
		}
	}
	return nil
}

// configureMutationEntityImpact checks if a mutation returns a cached entity and annotates
// the fetch config with MutationEntityImpactConfig for runtime cache staleness detection.
func (s *cachingPlannerState) configureMutationEntityImpact(internal *objectFetchConfiguration, result *resolve.FetchCacheConfiguration) {
	returnTypeName := s.resolveMutationReturnType(internal.fieldDefinitionRef)
	if returnTypeName == "" {
		return
	}

	ds := s.findDataSourceByID(internal.sourceID)
	if ds == nil {
		return
	}

	fedConfig := ds.FederationConfiguration()
	entityCacheConfig := fedConfig.EntityCacheConfig(returnTypeName)
	if entityCacheConfig == nil {
		return
	}

	// Merge key fields from ALL @key configurations so entities with multiple keys
	// keep every invalidation-relevant field (top-level fields deduped by name).
	keyConfigs := fedConfig.RequiredFieldsByKey(returnTypeName)
	keyFields := extractKeyFields(keyConfigs, returnTypeName)

	result.MutationEntityImpactConfig = &resolve.MutationEntityImpactConfig{
		EntityTypeName:              returnTypeName,
		KeyFields:                   keyFields,
		CacheName:                   entityCacheConfig.CacheName,
		IncludeSubgraphHeaderPrefix: entityCacheConfig.IncludeSubgraphHeaderPrefix,
	}

	// Check if this specific mutation field is configured for cache invalidation
	// or populate. A field is annotated with one or the other in composition.
	if len(internal.rootFields) > 0 {
		mutationFieldName := internal.rootFields[0].FieldName
		if fedConfig.MutationCacheInvalidationConfig(mutationFieldName) != nil {
			result.MutationEntityImpactConfig.InvalidateCache = true
		}
		// `@cachePopulate` arrives via MutationFieldCacheConfig with EnableEntityL2CachePopulation.
		// The flag was originally added to thread the populate intent through to follow-up entity
		// fetches in federated mutations; here we extend it to single-subgraph mutations where the
		// entity is returned directly and there is no follow-up fetch to inherit it.
		if mutCfg := fedConfig.MutationFieldCacheConfig(mutationFieldName); mutCfg != nil && mutCfg.EnableEntityL2CachePopulation {
			result.MutationEntityImpactConfig.PopulateCache = true
			result.MutationEntityImpactConfig.PopulateTTL = mutCfg.TTL
		}
	}
}

// resolveMutationReturnType resolves the return type name of a mutation field definition.
func (s *cachingPlannerState) resolveMutationReturnType(fieldDefinitionRef int) string {
	v := s.visitor
	if fieldDefinitionRef < 0 {
		return ""
	}
	typeRef := v.Definition.FieldDefinitionType(fieldDefinitionRef)
	underlyingType := v.Definition.ResolveUnderlyingType(typeRef)
	if underlyingType != -1 {
		return v.Definition.ResolveTypeNameString(underlyingType)
	}
	return v.Definition.ResolveTypeNameString(typeRef)
}

// entityCacheAnalytics returns the ObjectCacheAnalytics for a given type name.
// Uses a lazy cache to avoid repeated scans across datasources.
// Returns nil if the type is not an entity.
func (s *cachingPlannerState) entityCacheAnalytics(typeName string) *resolve.ObjectCacheAnalytics {
	if s.entityAnalyticsCache == nil {
		s.entityAnalyticsCache = make(map[string]*resolve.ObjectCacheAnalytics)
	}
	if cached, ok := s.entityAnalyticsCache[typeName]; ok {
		return cached // may be nil (not entity)
	}

	// Scan all datasources for this entity type
	for i := range s.visitor.Config.DataSources {
		ds := s.visitor.Config.DataSources[i]
		fedConfig := ds.FederationConfiguration()
		if !fedConfig.HasEntity(typeName) {
			continue
		}
		// Extract full key structure from @key SelectionSets
		keys := fedConfig.Keys.FilterByTypeAndResolvability(typeName, true)
		keyFields := extractKeyFields(keys, typeName)
		// Get hash mode from entity cache config (default false), then OR
		// with the planner's global ForceHashAnalyticsKeys flag. The global
		// flag exists so router operators can guarantee that no raw entity
		// key ever leaves the engine via analytics, regardless of what each
		// subgraph SDL declares.
		var hashKeys bool
		if cacheConfig := fedConfig.EntityCacheConfig(typeName); cacheConfig != nil {
			hashKeys = cacheConfig.HashAnalyticsKeys
		}
		if s.visitor.Config.ForceHashAnalyticsKeys {
			hashKeys = true
		}
		result := &resolve.ObjectCacheAnalytics{
			KeyFields: keyFields,
			HashKeys:  hashKeys,
		}
		s.entityAnalyticsCache[typeName] = result
		return result
	}

	s.entityAnalyticsCache[typeName] = nil // not an entity
	return nil
}

// polymorphicEntityCacheAnalytics returns per-concrete-type cache analytics for an
// interface/union object. Returns nil when none of the possible types is an entity
// (so the caller can assign unconditionally).
func (s *cachingPlannerState) polymorphicEntityCacheAnalytics(possibleTypes map[string]struct{}) *resolve.ObjectCacheAnalytics {
	byTypeName := make(map[string]*resolve.ObjectCacheAnalytics, len(possibleTypes))
	for possibleType := range possibleTypes {
		if analytics := s.entityCacheAnalytics(possibleType); analytics != nil {
			byTypeName[possibleType] = analytics
		}
	}
	if len(byTypeName) == 0 {
		return nil
	}
	return &resolve.ObjectCacheAnalytics{ByTypeName: byTypeName}
}

// extractKeyFields extracts the full structured key from @key SelectionSets.
// Merges all @key directives for the type, deduplicating top-level names.
func extractKeyFields(keys []FederationFieldConfiguration, typeName string) []resolve.KeyField {
	var result []resolve.KeyField
	seen := make(map[string]struct{})
	for i := range keys {
		if keys[i].TypeName != typeName || keys[i].FieldName != "" {
			continue
		}
		for _, kf := range resolve.ParseKeyFields(keys[i].SelectionSet) {
			if kf.Name == "__typename" {
				continue
			}
			if _, ok := seen[kf.Name]; !ok {
				seen[kf.Name] = struct{}{}
				result = append(result, kf)
			}
		}
	}
	return result
}
