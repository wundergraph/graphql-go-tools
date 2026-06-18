package plan

import (
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type FederationMetaData struct {
	Keys                               FederationFieldConfigurations
	Requires                           FederationFieldConfigurations
	Provides                           FederationFieldConfigurations
	EntityInterfaces                   []EntityInterfaceConfiguration
	InterfaceObjects                   []EntityInterfaceConfiguration
	RequestScopedFields                []RequestScopedField                       `json:"request_scoped_fields,omitempty"`
	EntityCacheConfig                  EntityCacheConfigurations                  `json:"entity_cache_config,omitempty"`
	RootFieldCacheConfig               RootFieldCacheConfigurations               `json:"root_field_cache_config,omitempty"`
	MutationFieldCacheConfig           MutationFieldCacheConfigurations           `json:"mutation_field_cache_config,omitempty"`
	MutationCacheInvalidationConfig    MutationCacheInvalidationConfigurations    `json:"mutation_cache_invalidation_config,omitempty"`
	SubscriptionEntityPopulationConfig SubscriptionEntityPopulationConfigurations `json:"subscription_entity_population_config,omitempty"`

	entityTypeNames map[string]struct{}
}

type FederationInfo interface {
	HasKeyRequirement(typeName, requiresFields string) bool
	RequiredFieldsByKey(typeName string) []FederationFieldConfiguration
	RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool)
	RequestScopedFieldsForType(typeName string) []RequestScopedField
	RequestScopedExportsForField(typeName, fieldName string) []RequestScopedField
	RequestScopedRequiredFieldsByKey() map[string][]RequestScopedField
	HasEntity(typeName string) bool
	HasInterfaceObject(typeName string) bool
	HasEntityInterface(typeName string) bool
	EntityInterfaceNames() []string
}

func (d *FederationMetaData) HasKeyRequirement(typeName, requiresFields string) bool {
	return d.Keys.HasSelectionSet(typeName, "", requiresFields)
}

func (d *FederationMetaData) RequiredFieldsByKey(typeName string) []FederationFieldConfiguration {
	return d.Keys.FilterByTypeAndResolvability(typeName, true)
}

func (d *FederationMetaData) RequestScopedFieldsForType(typeName string) (out []RequestScopedField) {
	for i := range d.RequestScopedFields {
		if d.RequestScopedFields[i].TypeName == typeName {
			out = append(out, d.RequestScopedFields[i])
		}
	}
	return out
}

func (d *FederationMetaData) RequestScopedExportsForField(typeName, fieldName string) (out []RequestScopedField) {
	for i := range d.RequestScopedFields {
		if d.RequestScopedFields[i].TypeName == typeName && d.RequestScopedFields[i].FieldName == fieldName {
			out = append(out, d.RequestScopedFields[i])
		}
	}
	return out
}

func (d *FederationMetaData) RequestScopedRequiredFieldsByKey() map[string][]RequestScopedField {
	return RequestScopedFieldsByL1Key(d.RequestScopedFields)
}

func (d *FederationMetaData) HasEntity(typeName string) bool {
	_, ok := d.entityTypeNames[typeName]
	return ok
}

func (d *FederationMetaData) RequiredFieldsByRequires(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool) {
	return d.Requires.FirstByTypeAndField(typeName, fieldName)
}

func (d *FederationMetaData) HasInterfaceObject(typeName string) bool {
	return slices.ContainsFunc(d.InterfaceObjects, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) || interfaceObjCfg.InterfaceTypeName == typeName
	})
}

func (d *FederationMetaData) HasEntityInterface(typeName string) bool {
	return slices.ContainsFunc(d.EntityInterfaces, func(interfaceObjCfg EntityInterfaceConfiguration) bool {
		return slices.Contains(interfaceObjCfg.ConcreteTypeNames, typeName) || interfaceObjCfg.InterfaceTypeName == typeName
	})
}

func (d *FederationMetaData) EntityInterfaceNames() (out []string) {
	if len(d.EntityInterfaces) == 0 {
		return nil
	}

	for i := range d.EntityInterfaces {
		out = append(out, d.EntityInterfaces[i].InterfaceTypeName)
	}

	return out
}

type EntityInterfaceConfiguration struct {
	InterfaceTypeName string
	ConcreteTypeNames []string
}

type RequestScopedField struct {
	FieldName string `json:"field_name,omitempty"`
	TypeName  string `json:"type_name,omitempty"`
	L1Key     string `json:"l1_key,omitempty"`
}

func RequestScopedFieldsByL1Key(fields []RequestScopedField) map[string][]RequestScopedField {
	if len(fields) == 0 {
		return nil
	}

	out := make(map[string][]RequestScopedField)
	for i := range fields {
		out[fields[i].L1Key] = append(out[fields[i].L1Key], fields[i])
	}
	return out
}

func ValidateRequestScopedFields(fields []RequestScopedField) (warnings []string, err error) {
	keysByFirstOccurrence := make([]string, 0, len(fields))
	fieldsByKey := make(map[string][]RequestScopedField, len(fields))

	for i := range fields {
		if fields[i].L1Key == "" {
			return nil, fmt.Errorf("@requestScoped field %s has empty L1Key", fields[i].coordinate())
		}

		if _, exists := fieldsByKey[fields[i].L1Key]; !exists {
			keysByFirstOccurrence = append(keysByFirstOccurrence, fields[i].L1Key)
		}
		fieldsByKey[fields[i].L1Key] = append(fieldsByKey[fields[i].L1Key], fields[i])
	}

	for i := range keysByFirstOccurrence {
		key := keysByFirstOccurrence[i]
		group := fieldsByKey[key]
		if len(group) == 1 {
			warnings = append(warnings, fmt.Sprintf("@requestScoped key %q appears on only one field: %s", key, group[0].coordinate()))
		}
	}

	return warnings, nil
}

func (f RequestScopedField) coordinate() string {
	return fmt.Sprintf("%s.%s", f.TypeName, f.FieldName)
}

type EntityCacheConfiguration struct {
	TypeName                    string        `json:"type_name,omitempty"`
	CacheName                   string        `json:"cache_name,omitempty"`
	TTL                         time.Duration `json:"ttl,omitempty"`
	IncludeSubgraphHeaderPrefix bool          `json:"include_subgraph_header_prefix,omitempty"`
	EnablePartialCacheLoad      bool          `json:"enable_partial_cache_load,omitempty"`
	HashAnalyticsKeys           bool          `json:"hash_analytics_keys,omitempty"`
	ShadowMode                  bool          `json:"shadow_mode,omitempty"`
	NegativeCacheTTL            time.Duration `json:"negative_cache_ttl,omitempty"`
}

type EntityCacheConfigurations []EntityCacheConfiguration

func (c EntityCacheConfigurations) FindByTypeName(typeName string) (cfg EntityCacheConfiguration, exists bool) {
	for i := range c {
		if c[i].TypeName == typeName {
			return c[i], true
		}
	}
	return EntityCacheConfiguration{}, false
}

type RootFieldCacheConfiguration struct {
	TypeName                    string             `json:"type_name,omitempty"`
	FieldName                   string             `json:"field_name,omitempty"`
	CacheName                   string             `json:"cache_name,omitempty"`
	TTL                         time.Duration      `json:"ttl,omitempty"`
	IncludeSubgraphHeaderPrefix bool               `json:"include_subgraph_header_prefix,omitempty"`
	EntityKeyMappings           []EntityKeyMapping `json:"entity_key_mappings,omitempty"`
	ShadowMode                  bool               `json:"shadow_mode,omitempty"`
	PartialBatchLoad            bool               `json:"partial_batch_load,omitempty"`
}

type RootFieldCacheConfigurations []RootFieldCacheConfiguration

func (c RootFieldCacheConfigurations) FindByTypeAndField(typeName, fieldName string) (cfg RootFieldCacheConfiguration, exists bool) {
	for i := range c {
		if c[i].TypeName == typeName && c[i].FieldName == fieldName {
			return c[i], true
		}
	}
	return RootFieldCacheConfiguration{}, false
}

type EntityKeyMapping struct {
	EntityTypeName string         `json:"entity_type_name,omitempty"`
	FieldMappings  []FieldMapping `json:"field_mappings,omitempty"`
}

type FieldMapping struct {
	EntityKeyField      string   `json:"entity_key_field,omitempty"`
	ArgumentPath        []string `json:"argument_path,omitempty"`
	ArgumentIsEntityKey bool     `json:"argument_is_entity_key,omitempty"`
}

type MutationFieldCacheConfiguration struct {
	FieldName                     string        `json:"field_name,omitempty"`
	EnableEntityL2CachePopulation bool          `json:"enable_entity_l2_cache_population,omitempty"`
	TTL                           time.Duration `json:"ttl,omitempty"`
}

type MutationFieldCacheConfigurations []MutationFieldCacheConfiguration

func (c MutationFieldCacheConfigurations) FindByFieldName(fieldName string) (cfg MutationFieldCacheConfiguration, exists bool) {
	for i := range c {
		if c[i].FieldName == fieldName {
			return c[i], true
		}
	}
	return MutationFieldCacheConfiguration{}, false
}

type MutationCacheInvalidationConfiguration struct {
	FieldName      string `json:"field_name,omitempty"`
	EntityTypeName string `json:"entity_type_name,omitempty"`
}

type MutationCacheInvalidationConfigurations []MutationCacheInvalidationConfiguration

func (c MutationCacheInvalidationConfigurations) FindByFieldName(fieldName string) (cfg MutationCacheInvalidationConfiguration, exists bool) {
	for i := range c {
		if c[i].FieldName == fieldName {
			return c[i], true
		}
	}
	return MutationCacheInvalidationConfiguration{}, false
}

type SubscriptionEntityPopulationConfiguration struct {
	TypeName                    string        `json:"type_name,omitempty"`
	FieldName                   string        `json:"field_name,omitempty"`
	CacheName                   string        `json:"cache_name,omitempty"`
	TTL                         time.Duration `json:"ttl,omitempty"`
	IncludeSubgraphHeaderPrefix bool          `json:"include_subgraph_header_prefix,omitempty"`
	EnableInvalidationOnKeyOnly bool          `json:"enable_invalidation_on_key_only,omitempty"`
}

type SubscriptionEntityPopulationConfigurations []SubscriptionEntityPopulationConfiguration

func (c SubscriptionEntityPopulationConfigurations) FindByTypeAndFieldName(typeName, fieldName string) (cfg SubscriptionEntityPopulationConfiguration, exists bool) {
	if typeName == "" || fieldName == "" {
		return SubscriptionEntityPopulationConfiguration{}, false
	}

	for i := range c {
		if c[i].TypeName == typeName && c[i].FieldName == fieldName {
			return c[i], true
		}
	}
	return SubscriptionEntityPopulationConfiguration{}, false
}

type FederationFieldConfiguration struct {
	TypeName              string         `json:"type_name"`            // TypeName is the name of the Entity the Fragment is for
	FieldName             string         `json:"field_name,omitempty"` // FieldName is empty for key requirements, otherwise, it is the name of the field that has requires or provides directive
	SelectionSet          string         `json:"selection_set"`        // SelectionSet is the selection set that is required for the given field (keys, requires, provides)
	DisableEntityResolver bool           `json:"-"`                    // applicable only for the keys. If true it means that the given entity could not be resolved by this key.
	Conditions            []KeyCondition `json:"conditions,omitempty"` // conditions stores coordinates under which we could use implicit key, while on other paths this key is not available

	parsedSelectionSet *ast.Document
	RemappedPaths      map[string]string
}

type KeyCondition struct {
	Coordinates []FieldCoordinate `json:"coordinates"`
	FieldPath   []string          `json:"field_path"`
}

// FieldCoordinate contains coordinates of a field in a type
// TODO: rename to FieldCoordinates
type FieldCoordinate struct {
	TypeName  string `json:"type_name"`
	FieldName string `json:"field_name"`
}

func (f FieldCoordinate) String() string {
	return fmt.Sprintf("%s.%s", f.TypeName, f.FieldName)
}

// parseSelectionSet parses the selection set and stores the parsed AST in parsedSelectionSet.
// should have pointer receiver to preserve the value
func (f *FederationFieldConfiguration) parseSelectionSet() error {
	if f.parsedSelectionSet != nil {
		return nil
	}

	doc, report := RequiredFieldsFragment(f.TypeName, f.SelectionSet, false)
	if report.HasErrors() {
		return report
	}

	f.parsedSelectionSet = doc
	return nil
}

// String - implements fmt.Stringer
// NOTE: do not change to pointer receiver, it won't work for not pointer values
func (f FederationFieldConfiguration) String() string {
	b, _ := json.Marshal(f)
	return string(b)
}

type FederationFieldConfigurations []FederationFieldConfiguration

func (f *FederationFieldConfigurations) FilterByTypeAndResolvability(typeName string, skipUnresovable bool) (out []FederationFieldConfiguration) {
	for i := range *f {
		if (*f)[i].TypeName != typeName || (*f)[i].FieldName != "" {
			continue
		}
		if skipUnresovable && (*f)[i].DisableEntityResolver {
			continue
		}
		out = append(out, (*f)[i])
	}
	return out
}

func (f *FederationFieldConfigurations) UniqueTypes() (out []string) {
	seen := map[string]struct{}{}
	for i := range *f {
		seen[(*f)[i].TypeName] = struct{}{}
	}

	for k := range seen {
		out = append(out, k)
	}
	return out
}

func (f *FederationFieldConfigurations) FirstByTypeAndField(typeName, fieldName string) (cfg FederationFieldConfiguration, exists bool) {
	for i := range *f {
		if (*f)[i].TypeName == typeName && (*f)[i].FieldName == fieldName {
			return (*f)[i], true
		}
	}
	return FederationFieldConfiguration{}, false
}

func (f *FederationFieldConfigurations) HasSelectionSet(typeName, fieldName, selectionSet string) bool {
	for i := range *f {
		if typeName == (*f)[i].TypeName &&
			fieldName == (*f)[i].FieldName &&
			selectionSet == (*f)[i].SelectionSet {
			return true
		}
	}
	return false
}

func (f *FederationFieldConfigurations) AppendIfNotPresent(config FederationFieldConfiguration) (added bool) {
	ok := f.HasSelectionSet(config.TypeName, config.FieldName, config.SelectionSet)
	if ok {
		return false
	}

	*f = append(*f, config)

	return true
}
