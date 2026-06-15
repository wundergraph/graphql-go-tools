package plan

import (
	"bytes"
	"slices"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

type cachingPlannerState struct {
	operation  *ast.Document
	definition *ast.Document
	config     *Configuration

	plannerObjects map[int]*resolve.Object
	rootFields     map[int][]resolve.RootField
	fieldStack     []*cachingPlannerField
	responsePaths  []string
}

type cachingPlannerField struct {
	ref     int
	field   *resolve.Field
	objects map[int]*resolve.Object
}

type fetchCachingInput struct {
	fetchID        int
	sourceName     string
	operationType  ast.OperationType
	federation     FederationMetaData
	requiredFields FederationFieldConfigurations
	rootFields     []resolve.GraphCoordinate
}

func newCachingPlannerState(operation, definition *ast.Document, config *Configuration) *cachingPlannerState {
	if config == nil || config.DisableEntityCaching {
		return nil
	}
	return &cachingPlannerState{
		operation:      operation,
		definition:     definition,
		config:         config,
		plannerObjects: map[int]*resolve.Object{},
		rootFields:     map[int][]resolve.RootField{},
		responsePaths:  make([]string, 0, 8),
		fieldStack:     make([]*cachingPlannerField, 0, 8),
	}
}

func hasCachingConfiguration(dataSources []DataSource) bool {
	for _, dataSource := range dataSources {
		federation := dataSource.FederationConfiguration()
		if len(federation.EntityCacheConfig) > 0 ||
			len(federation.RootFieldCacheConfig) > 0 ||
			len(federation.MutationFieldCacheConfig) > 0 ||
			len(federation.MutationCacheInvalidationConfig) > 0 {
			return true
		}
	}
	return false
}

func (s *cachingPlannerState) trackFieldForPlanner(fieldRef int, field *resolve.Field) {
	if s == nil || field == nil {
		return
	}
	s.responsePaths = append(s.responsePaths, string(field.Name))
	s.fieldStack = append(s.fieldStack, &cachingPlannerField{
		ref:     fieldRef,
		field:   s.copyFieldForPlanner(fieldRef, field),
		objects: map[int]*resolve.Object{},
	})
}

func (s *cachingPlannerState) finishFieldForPlanner(fieldRef int, plannerIDs []int) {
	if s == nil || len(s.fieldStack) == 0 {
		return
	}
	frame := s.fieldStack[len(s.fieldStack)-1]
	if frame.ref != fieldRef {
		s.responsePaths = s.responsePaths[:len(s.responsePaths)-1]
		return
	}
	s.fieldStack = s.fieldStack[:len(s.fieldStack)-1]
	s.responsePaths = s.responsePaths[:len(s.responsePaths)-1]

	for _, plannerID := range plannerIDs {
		if len(s.fieldStack) == 0 {
			s.rootFields[plannerID] = append(s.rootFields[plannerID], s.rootFieldCacheKeyField(frame.ref, frame.field))
		}
		field := s.copyFieldForPlanner(fieldRef, frame.field)
		if childObject, ok := frame.objects[plannerID]; ok {
			field.Value = childObject
		}
		parent := s.currentPlannerObject(plannerID)
		addPlannerField(parent, field)
	}
}

func (s *cachingPlannerState) captureFieldCacheArgs(fieldRef int) {
	if s == nil || len(s.fieldStack) == 0 || s.operation == nil {
		return
	}
	frame := s.fieldStack[len(s.fieldStack)-1]
	if frame.ref != fieldRef || len(s.operation.FieldArguments(fieldRef)) == 0 {
		return
	}
	for _, argRef := range s.operation.FieldArguments(fieldRef) {
		value := s.operation.ArgumentValue(argRef)
		if value.Kind != ast.ValueKindVariable {
			continue
		}
		frame.field.CacheArgs = append(frame.field.CacheArgs, resolve.CacheFieldArg{
			ArgName:      s.operation.ArgumentNameString(argRef),
			VariableName: s.operation.VariableValueNameString(value.Ref),
		})
	}
}

func (s *cachingPlannerState) isEntityBoundaryField(fieldRef int) bool {
	return s != nil && s.operation != nil && len(s.responsePaths) == 1 && fieldRef != ast.InvalidRef
}

func (s *cachingPlannerState) configureFetchCaching(input fetchCachingInput) *resolve.FetchCacheConfiguration {
	if s == nil || s.config == nil || s.config.DisableEntityCaching {
		return nil
	}

	providesData := s.providesData(input.fetchID)
	if input.operationType == ast.OperationTypeMutation {
		return s.configureMutationEntityImpact(input, providesData)
	}
	if input.operationType != ast.OperationTypeQuery {
		return nil
	}
	if len(input.requiredFields) > 0 {
		return s.configureEntityFetchCaching(input, providesData)
	}
	return s.configureRootFetchCaching(input, providesData)
}

func (s *cachingPlannerState) configureMutationEntityImpact(input fetchCachingInput, providesData *resolve.Object) *resolve.FetchCacheConfiguration {
	rootFieldName := s.mutationRootFieldName(input)
	if rootFieldName == "" {
		return nil
	}
	fieldCfg, hasFieldCfg := input.federation.MutationFieldCacheConfig.FindByFieldName(rootFieldName)
	invalidationCfg, hasInvalidationCfg := input.federation.MutationCacheInvalidationConfig.FindByFieldName(rootFieldName)
	if !hasFieldCfg && !hasInvalidationCfg {
		return nil
	}

	cache := &resolve.FetchCacheConfiguration{
		ProvidesData: providesData,
	}
	if hasFieldCfg {
		cache.EnableMutationL2CachePopulation = fieldCfg.EnableEntityL2CachePopulation
		cache.MutationCacheTTLOverride = fieldCfg.TTL
	}

	entityTypeName := invalidationCfg.EntityTypeName
	if entityTypeName == "" && len(input.requiredFields) > 0 {
		entityTypeName = input.requiredFields[0].TypeName
	}
	if entityTypeName == "" {
		return cache
	}
	entityCfg, hasEntityCfg := input.federation.EntityCacheConfig.FindByTypeName(entityTypeName)
	if !hasEntityCfg {
		return cache
	}
	keyFields := s.mutationKeyFields(input.federation, entityTypeName)
	if len(keyFields) == 0 {
		return cache
	}
	cache.CacheName = entityCfg.CacheName
	cache.MutationEntityImpactConfig = &resolve.MutationEntityImpactConfig{
		EntityTypeName:              entityTypeName,
		KeyFields:                   keyFields,
		CacheName:                   entityCfg.CacheName,
		IncludeSubgraphHeaderPrefix: entityCfg.IncludeSubgraphHeaderPrefix,
		InvalidateCache:             hasInvalidationCfg,
		PopulateTTL:                 fieldCfg.TTL,
	}
	return cache
}

func (s *cachingPlannerState) configureEntityFetchCaching(input fetchCachingInput, providesData *resolve.Object) *resolve.FetchCacheConfiguration {
	entityTypeName, template := s.entityCacheKeyTemplate(input.requiredFields, input.federation)
	if template == nil {
		return nil
	}

	cache := &resolve.FetchCacheConfiguration{
		KeyTemplate:  template,
		ProvidesData: providesData,
	}
	cfg, exists := input.federation.EntityCacheConfig.FindByTypeName(entityTypeName)
	if !exists {
		return cache
	}
	cache.CacheName = cfg.CacheName
	cache.EnableL2Cache = true
	cache.IncludeSubgraphHeaderPrefix = cfg.IncludeSubgraphHeaderPrefix
	cache.TTL = cfg.TTL
	cache.NegativeCacheTTL = cfg.NegativeCacheTTL
	cache.EnablePartialCacheLoad = cfg.EnablePartialCacheLoad
	return cache
}

func (s *cachingPlannerState) configureRootFetchCaching(input fetchCachingInput, providesData *resolve.Object) *resolve.FetchCacheConfiguration {
	if len(input.rootFields) == 0 {
		return nil
	}

	rootFields := s.rootFields[input.fetchID]
	if len(rootFields) == 0 {
		rootFields = make([]resolve.RootField, 0, len(input.rootFields))
		for _, coordinate := range input.rootFields {
			rootFields = append(rootFields, resolve.RootField{
				TypeName:  coordinate.TypeName,
				FieldName: coordinate.FieldName,
			})
		}
	} else {
		for i := range rootFields {
			if rootFields[i].TypeName != "" {
				continue
			}
			for _, coordinate := range input.rootFields {
				if coordinate.FieldName == rootFields[i].FieldName {
					rootFields[i].TypeName = coordinate.TypeName
					break
				}
			}
		}
	}
	var sharedCfg RootFieldCacheConfiguration
	for i, field := range rootFields {
		cfg, exists := input.federation.RootFieldCacheConfig.FindByTypeAndField(field.TypeName, field.FieldName)
		if !exists {
			return nil
		}
		if i == 0 {
			sharedCfg = cfg
		} else if !rootFieldCacheConfigsEqual(sharedCfg, cfg) {
			return &resolve.FetchCacheConfiguration{
				KeyTemplate:  s.rootCacheKeyTemplate(input.rootFields, nil),
				ProvidesData: providesData,
			}
		}
	}

	cache := &resolve.FetchCacheConfiguration{
		CacheName:                   sharedCfg.CacheName,
		EnableL2Cache:               true,
		IncludeSubgraphHeaderPrefix: sharedCfg.IncludeSubgraphHeaderPrefix,
		TTL:                         sharedCfg.TTL,
		KeyTemplate:                 resolve.NewRootQueryCacheKeyTemplate(rootFields, entityKeyMappings(sharedCfg.EntityKeyMappings)),
		ProvidesData:                providesData,
	}
	return cache
}

func (s *cachingPlannerState) providesData(fetchID int) *resolve.Object {
	if s.config.DisableFetchProvidesData {
		return nil
	}
	object := s.plannerObjects[fetchID]
	if object == nil {
		return nil
	}
	resolve.ComputeHasAliases(object)
	return object
}

func (s *cachingPlannerState) entityCacheKeyTemplate(requiredFields FederationFieldConfigurations, federation FederationMetaData) (string, *resolve.EntityQueryCacheKeyTemplate) {
	if len(requiredFields) == 0 {
		return "", nil
	}
	cfg := requiredFields[0]
	keyCfg := cfg
	for _, candidate := range federation.Keys.FilterByTypeAndResolvability(cfg.TypeName, true) {
		keyCfg = candidate
		break
	}
	return keyCfg.TypeName, &resolve.EntityQueryCacheKeyTemplate{
		TypeName: keyCfg.TypeName,
		Keys:     resolve.NewResolvableObjectVariable(keyFieldsObject(resolve.ParseKeyFields(keyCfg.SelectionSet))),
	}
}

func (s *cachingPlannerState) rootCacheKeyTemplate(rootFieldCoordinates []resolve.GraphCoordinate, mappings []resolve.EntityKeyMappingConfig) *resolve.RootQueryCacheKeyTemplate {
	rootFields := make([]resolve.RootField, 0, len(rootFieldCoordinates))
	for _, coordinate := range rootFieldCoordinates {
		rootFields = append(rootFields, resolve.RootField{
			TypeName:  coordinate.TypeName,
			FieldName: coordinate.FieldName,
		})
	}
	return resolve.NewRootQueryCacheKeyTemplate(rootFields, mappings)
}

func (s *cachingPlannerState) mutationRootFieldName(input fetchCachingInput) string {
	if rootFields := s.rootFields[input.fetchID]; len(rootFields) > 0 {
		return rootFields[0].FieldName
	}
	for _, coordinate := range input.rootFields {
		if coordinate.FieldName != "" {
			return coordinate.FieldName
		}
	}
	return ""
}

func (s *cachingPlannerState) mutationKeyFields(federation FederationMetaData, entityTypeName string) []resolve.KeyField {
	for _, keyCfg := range federation.Keys.FilterByTypeAndResolvability(entityTypeName, true) {
		return resolve.ParseKeyFields(keyCfg.SelectionSet)
	}
	return nil
}

func (s *cachingPlannerState) rootFieldCacheKeyField(fieldRef int, responseField *resolve.Field) resolve.RootField {
	enclosingTypeName := ""
	if s.definition != nil && len(s.fieldStack) == 0 {
		enclosingTypeName = "Query"
	}
	field := resolve.RootField{
		TypeName:    enclosingTypeName,
		FieldName:   s.operation.FieldNameString(fieldRef),
		ResponseKey: string(responseField.Name),
	}
	if s.operation == nil {
		return field
	}
	for _, argRef := range s.operation.FieldArguments(fieldRef) {
		arg := s.rootFieldArgument(argRef)
		if arg.Name != "" {
			field.Arguments = append(field.Arguments, arg)
		}
	}
	return field
}

func (s *cachingPlannerState) rootFieldArgument(argRef int) resolve.RootFieldArgument {
	arg := resolve.RootFieldArgument{Name: s.operation.ArgumentNameString(argRef)}
	value := s.operation.ArgumentValue(argRef)
	if value.Kind == ast.ValueKindVariable {
		arg.VariablePath = []string{s.operation.VariableValueNameString(value.Ref)}
		return arg
	}
	jsonValue, err := s.operation.ValueToJSON(value)
	if err != nil {
		return arg
	}
	parsed, err := astjson.ParseBytes(jsonValue)
	if err != nil {
		return arg
	}
	arg.Value = parsed
	return arg
}

func (s *cachingPlannerState) currentPlannerObject(plannerID int) *resolve.Object {
	for _, frame := range slices.Backward(s.fieldStack) {
		if object, ok := frame.objects[plannerID]; ok {
			return object
		}
		if frame.field.Value != nil && frame.field.Value.NodeKind() == resolve.NodeKindObject {
			object := cloneObjectShape(frame.field.Value.(*resolve.Object))
			frame.objects[plannerID] = object
			return object
		}
		if frame.field.Value != nil && frame.field.Value.NodeKind() == resolve.NodeKindArray {
			if object := arrayItemObject(frame.field.Value); object != nil {
				cloned := cloneObjectShape(object)
				frame.objects[plannerID] = cloned
				return cloned
			}
		}
	}
	object := s.plannerObjects[plannerID]
	if object == nil {
		object = &resolve.Object{Fields: []*resolve.Field{}}
		s.plannerObjects[plannerID] = object
	}
	return object
}

func (s *cachingPlannerState) copyFieldForPlanner(fieldRef int, field *resolve.Field) *resolve.Field {
	copied := field.Copy()
	if s.operation != nil && fieldRef != ast.InvalidRef && !bytes.Equal(field.Name, s.operation.FieldNameBytes(fieldRef)) {
		copied.OriginalName = append([]byte(nil), s.operation.FieldNameBytes(fieldRef)...)
	}
	return copied
}

func addPlannerField(object *resolve.Object, field *resolve.Field) {
	if object == nil || field == nil {
		return
	}
	for i := range object.Fields {
		if bytes.Equal(object.Fields[i].Name, field.Name) && bytes.Equal(object.Fields[i].OriginalName, field.OriginalName) {
			object.Fields[i] = mergePlannerFields(object.Fields[i], field)
			return
		}
	}
	object.Fields = append(object.Fields, field)
}

func mergePlannerFields(left, right *resolve.Field) *resolve.Field {
	if left.Value == nil || right.Value == nil {
		return left
	}
	switch left.Value.NodeKind() {
	case resolve.NodeKindObject:
		if right.Value.NodeKind() == resolve.NodeKindObject {
			left.Value = mergePlannerObjects(left.Value.(*resolve.Object), right.Value.(*resolve.Object))
		}
	case resolve.NodeKindArray:
		if right.Value.NodeKind() == resolve.NodeKindArray {
			leftArray := left.Value.(*resolve.Array)
			rightArray := right.Value.(*resolve.Array)
			if leftArray.Item.NodeKind() == resolve.NodeKindObject && rightArray.Item.NodeKind() == resolve.NodeKindObject {
				leftArray.Item = mergePlannerObjects(leftArray.Item.(*resolve.Object), rightArray.Item.(*resolve.Object))
			}
		}
	}
	return left
}

func mergePlannerObjects(left, right *resolve.Object) *resolve.Object {
	for _, field := range right.Fields {
		addPlannerField(left, field)
	}
	return left
}

func cloneObjectShape(object *resolve.Object) *resolve.Object {
	return &resolve.Object{
		Nullable:      object.Nullable,
		Path:          append([]string(nil), object.Path...),
		Fields:        []*resolve.Field{},
		PossibleTypes: object.PossibleTypes,
		SourceName:    object.SourceName,
		TypeName:      object.TypeName,
	}
}

func arrayItemObject(node resolve.Node) *resolve.Object {
	array, ok := node.(*resolve.Array)
	if !ok {
		return nil
	}
	switch item := array.Item.(type) {
	case *resolve.Object:
		return item
	case *resolve.Array:
		return arrayItemObject(item)
	default:
		return nil
	}
}

func rootFieldCacheConfigsEqual(left, right RootFieldCacheConfiguration) bool {
	return left.CacheName == right.CacheName &&
		left.TTL == right.TTL &&
		left.IncludeSubgraphHeaderPrefix == right.IncludeSubgraphHeaderPrefix &&
		left.ShadowMode == right.ShadowMode &&
		left.PartialBatchLoad == right.PartialBatchLoad &&
		slices.EqualFunc(left.EntityKeyMappings, right.EntityKeyMappings, entityKeyMappingEqual)
}

func entityKeyMappingEqual(left, right EntityKeyMapping) bool {
	return left.EntityTypeName == right.EntityTypeName &&
		slices.EqualFunc(left.FieldMappings, right.FieldMappings, fieldMappingEqual)
}

func fieldMappingEqual(left, right FieldMapping) bool {
	return left.EntityKeyField == right.EntityKeyField &&
		left.ArgumentIsEntityKey == right.ArgumentIsEntityKey &&
		slices.Equal(left.ArgumentPath, right.ArgumentPath)
}

func entityKeyMappings(mappings []EntityKeyMapping) []resolve.EntityKeyMappingConfig {
	out := make([]resolve.EntityKeyMappingConfig, 0, len(mappings))
	for _, mapping := range mappings {
		fields := make([]resolve.EntityFieldMappingConfig, 0, len(mapping.FieldMappings))
		for _, field := range mapping.FieldMappings {
			fields = append(fields, resolve.EntityFieldMappingConfig{
				EntityKeyField:      field.EntityKeyField,
				ArgumentPath:        append([]string(nil), field.ArgumentPath...),
				ArgumentIsEntityKey: field.ArgumentIsEntityKey,
			})
		}
		out = append(out, resolve.EntityKeyMappingConfig{
			EntityTypeName: mapping.EntityTypeName,
			FieldMappings:  fields,
		})
	}
	return out
}

func keyFieldsObject(fields []resolve.KeyField) *resolve.Object {
	object := &resolve.Object{Fields: make([]*resolve.Field, 0, len(fields))}
	for _, field := range fields {
		object.Fields = append(object.Fields, keyFieldObjectField(field))
	}
	return object
}

func keyFieldObjectField(field resolve.KeyField) *resolve.Field {
	out := &resolve.Field{Name: []byte(field.Name)}
	if len(field.Children) == 0 {
		out.Value = &resolve.String{Path: []string{field.Name}}
		return out
	}
	child := keyFieldsObject(field.Children)
	child.Path = []string{field.Name}
	out.Value = child
	return out
}
