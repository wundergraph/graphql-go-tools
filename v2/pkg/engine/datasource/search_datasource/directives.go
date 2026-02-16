package search_datasource

import (
	"fmt"
	"strings"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/searchindex"
)

// ParsedConfig holds the complete parsed configuration from the config schema.
type ParsedConfig struct {
	Indices       []IndexDirective
	Entities      []SearchableEntity
	Populations   []PopulateDirective
	Subscriptions []SubscribeDirective
}

// IndexDirective represents @index on the schema extension.
type IndexDirective struct {
	Name                  string
	Backend               string
	ConfigJSON            string
	CursorBasedPagination bool // from @index(cursorBasedPagination: true)
}

// SearchableEntity represents @searchable on an object type.
type SearchableEntity struct {
	TypeName               string
	IndexName              string
	SearchField            string
	SuggestField           string // e.g. "suggestProducts" — omit to disable autocomplete
	KeyFields              []string
	Fields                 []IndexedField
	EmbeddingFields        []EmbeddingField
	ResultsMetaInformation bool // renamed from UseResultWrapper
	CursorBasedPagination  bool // propagated from IndexDirective
	CursorBidirectional    bool // true if backend supports last/before
}

// NeedsResponseWrapper returns true if the entity needs wrapper types in the SDL.
func (e *SearchableEntity) NeedsResponseWrapper() bool {
	return e.ResultsMetaInformation || e.CursorBasedPagination
}

// IndexedField represents @indexed on a field definition.
type IndexedField struct {
	FieldName    string
	GraphQLType  string
	IndexType    searchindex.FieldType
	Filterable   bool
	Sortable     bool
	Dimensions   int
	Weight       float64 // search boost for TEXT fields; 0 = default (1.0)
	Autocomplete bool    // opt-in for term autocomplete
}

// EmbeddingField represents @embedding on a virtual field.
type EmbeddingField struct {
	FieldName    string
	SourceFields []string
	Template     string
	Model        string
}

// PopulateDirective represents @populate on a schema extension or query operation.
type PopulateDirective struct {
	IndexName      string
	EntityTypeName string
	Path           string
	Query          string // GraphQL query to execute for population
	ResyncInterval string
}

// SubscribeDirective represents @subscribe on a schema extension or subscription operation.
type SubscribeDirective struct {
	IndexName      string
	EntityTypeName string
	Path           string
	DeletionPath   string
	Subscription   string // GraphQL subscription operation to execute
}

// HasVectorSearch returns true if the entity has any VECTOR or embedding fields.
func (e *SearchableEntity) HasVectorSearch() bool {
	for _, f := range e.Fields {
		if f.IndexType == searchindex.FieldTypeVector {
			return true
		}
	}
	return len(e.EmbeddingFields) > 0
}

// HasGeoSearch returns true if the entity has any GEO fields.
func (e *SearchableEntity) HasGeoSearch() bool {
	for _, f := range e.Fields {
		if f.IndexType == searchindex.FieldTypeGeo {
			return true
		}
	}
	return false
}

// HasDateField returns true if the entity has any DATE or DATETIME fields.
func (e *SearchableEntity) HasDateField() bool {
	for _, f := range e.Fields {
		if f.IndexType == searchindex.FieldTypeDate || f.IndexType == searchindex.FieldTypeDateTime {
			return true
		}
	}
	return false
}

// HasTextSearch returns true if the entity has any TEXT fields.
func (e *SearchableEntity) HasTextSearch() bool {
	for _, f := range e.Fields {
		if f.IndexType == searchindex.FieldTypeText {
			return true
		}
	}
	return false
}

// HasAutocomplete returns true if the entity has any fields with autocomplete enabled.
func (e *SearchableEntity) HasAutocomplete() bool {
	for _, f := range e.Fields {
		if f.Autocomplete {
			return true
		}
	}
	return false
}

// ParseConfigSchema parses a config schema document and extracts all directives.
func ParseConfigSchema(doc *ast.Document) (*ParsedConfig, error) {
	config := &ParsedConfig{}

	// Parse @index, @populate, @subscribe directives from schema extensions
	if err := parseSchemaExtensionDirectives(doc, config); err != nil {
		return nil, fmt.Errorf("parsing schema extension directives: %w", err)
	}

	// Parse @searchable, @indexed, @embedding directives from object types
	if err := parseEntityDirectives(doc, config); err != nil {
		return nil, fmt.Errorf("parsing entity directives: %w", err)
	}

	// Propagate cursor flags from IndexDirective to matching SearchableEntity.
	indexByName := make(map[string]*IndexDirective, len(config.Indices))
	for i := range config.Indices {
		indexByName[config.Indices[i].Name] = &config.Indices[i]
	}
	for i := range config.Entities {
		if idx, ok := indexByName[config.Entities[i].IndexName]; ok && idx.CursorBasedPagination {
			config.Entities[i].CursorBasedPagination = true
			caps, hasCaps := cursorBackendCaps[idx.Backend]
			if hasCaps {
				config.Entities[i].CursorBidirectional = caps.Bidirectional
			}
		}
	}

	return config, nil
}

// cursorBackendCap describes cursor support for a backend.
type cursorBackendCap struct {
	Supported     bool
	Bidirectional bool
}

// cursorBackendCaps maps backend names to their cursor capabilities.
var cursorBackendCaps = map[string]cursorBackendCap{
	"bleve":         {Supported: true, Bidirectional: true},
	"elasticsearch": {Supported: true, Bidirectional: false},
	"pgvector":      {Supported: true, Bidirectional: true},
}

func parseSchemaExtensionDirectives(doc *ast.Document, config *ParsedConfig) error {
	for i := range doc.SchemaExtensions {
		for _, dirRef := range doc.SchemaExtensions[i].Directives.Refs {
			dirName := doc.DirectiveNameString(dirRef)
			switch dirName {
			case "index":
				idx := IndexDirective{}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("name")); ok {
					idx.Name = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("backend")); ok {
					idx.Backend = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("config")); ok {
					idx.ConfigJSON = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("cursorBasedPagination")); ok {
					if val.Kind == ast.ValueKindBoolean {
						idx.CursorBasedPagination = bool(doc.BooleanValues[val.Ref])
					}
				}
				if idx.Name == "" || idx.Backend == "" {
					return fmt.Errorf("@index requires 'name' and 'backend' arguments")
				}
				config.Indices = append(config.Indices, idx)
			case "populate":
				p := PopulateDirective{}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("index")); ok {
					p.IndexName = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("entity")); ok {
					p.EntityTypeName = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("path")); ok {
					p.Path = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("query")); ok {
					p.Query = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("resyncInterval")); ok {
					p.ResyncInterval = doc.StringValueContentString(val.Ref)
				}
				config.Populations = append(config.Populations, p)
			case "subscribe":
				s := SubscribeDirective{}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("index")); ok {
					s.IndexName = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("entity")); ok {
					s.EntityTypeName = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("path")); ok {
					s.Path = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("deletionPath")); ok {
					s.DeletionPath = doc.StringValueContentString(val.Ref)
				}
				if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("subscription")); ok {
					s.Subscription = doc.StringValueContentString(val.Ref)
				}
				config.Subscriptions = append(config.Subscriptions, s)
			}
		}
	}
	return nil
}

func parseEntityDirectives(doc *ast.Document, config *ParsedConfig) error {
	for i := range doc.ObjectTypeDefinitions {
		def := &doc.ObjectTypeDefinitions[i]
		entity, err := parseSearchableType(doc, def, i)
		if err != nil {
			return err
		}
		if entity != nil {
			config.Entities = append(config.Entities, *entity)
		}
	}
	// Also check type extensions
	for i := range doc.ObjectTypeExtensions {
		ext := &doc.ObjectTypeExtensions[i]
		entity, err := parseSearchableTypeExtension(doc, ext, i)
		if err != nil {
			return err
		}
		if entity != nil {
			config.Entities = append(config.Entities, *entity)
		}
	}
	return nil
}

func parseSearchableType(doc *ast.Document, def *ast.ObjectTypeDefinition, defIdx int) (*SearchableEntity, error) {
	// Look for @searchable directive
	searchableDir := -1
	for _, dirRef := range def.Directives.Refs {
		if doc.DirectiveNameString(dirRef) == "searchable" {
			searchableDir = dirRef
			break
		}
	}
	if searchableDir == -1 {
		return nil, nil
	}

	entity := &SearchableEntity{
		TypeName:               doc.ObjectTypeDefinitionNameString(defIdx),
		ResultsMetaInformation: true, // default
	}

	// Parse @searchable arguments
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("index")); ok {
		entity.IndexName = doc.StringValueContentString(val.Ref)
	}
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("searchField")); ok {
		entity.SearchField = doc.StringValueContentString(val.Ref)
	}
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("suggestField")); ok {
		entity.SuggestField = doc.StringValueContentString(val.Ref)
	}
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("resultsMetaInformation")); ok {
		if val.Kind == ast.ValueKindBoolean {
			entity.ResultsMetaInformation = bool(doc.BooleanValues[val.Ref])
		}
	}

	// Parse @key directive for key fields
	for _, dirRef := range def.Directives.Refs {
		if doc.DirectiveNameString(dirRef) == "key" {
			if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("fields")); ok {
				fieldsStr := doc.StringValueContentString(val.Ref)
				entity.KeyFields = strings.Fields(fieldsStr)
			}
		}
	}

	// Parse fields with @indexed and @embedding directives
	for _, fieldRef := range def.FieldsDefinition.Refs {
		fieldName := doc.FieldDefinitionNameString(fieldRef)
		fieldType := doc.FieldDefinitionTypeNameString(fieldRef)

		// Check for @indexed
		for _, dirRef := range doc.FieldDefinitions[fieldRef].Directives.Refs {
			switch doc.DirectiveNameString(dirRef) {
			case "indexed":
				field, err := parseIndexedDirective(doc, dirRef, fieldName, fieldType)
				if err != nil {
					return nil, err
				}
				entity.Fields = append(entity.Fields, *field)
			case "embedding":
				emb, err := parseEmbeddingDirective(doc, dirRef, fieldName)
				if err != nil {
					return nil, err
				}
				entity.EmbeddingFields = append(entity.EmbeddingFields, *emb)
			}
		}
	}

	return entity, nil
}

func parseSearchableTypeExtension(doc *ast.Document, ext *ast.ObjectTypeExtension, extIdx int) (*SearchableEntity, error) {
	searchableDir := -1
	for _, dirRef := range ext.Directives.Refs {
		if doc.DirectiveNameString(dirRef) == "searchable" {
			searchableDir = dirRef
			break
		}
	}
	if searchableDir == -1 {
		return nil, nil
	}

	entity := &SearchableEntity{
		TypeName:               doc.ObjectTypeExtensionNameString(extIdx),
		ResultsMetaInformation: true, // default
	}

	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("index")); ok {
		entity.IndexName = doc.StringValueContentString(val.Ref)
	}
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("searchField")); ok {
		entity.SearchField = doc.StringValueContentString(val.Ref)
	}
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("suggestField")); ok {
		entity.SuggestField = doc.StringValueContentString(val.Ref)
	}
	if val, ok := doc.DirectiveArgumentValueByName(searchableDir, []byte("resultsMetaInformation")); ok {
		if val.Kind == ast.ValueKindBoolean {
			entity.ResultsMetaInformation = bool(doc.BooleanValues[val.Ref])
		}
	}

	for _, dirRef := range ext.Directives.Refs {
		if doc.DirectiveNameString(dirRef) == "key" {
			if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("fields")); ok {
				fieldsStr := doc.StringValueContentString(val.Ref)
				entity.KeyFields = strings.Fields(fieldsStr)
			}
		}
	}

	for _, fieldRef := range ext.FieldsDefinition.Refs {
		fieldName := doc.FieldDefinitionNameString(fieldRef)
		fieldType := doc.FieldDefinitionTypeNameString(fieldRef)

		for _, dirRef := range doc.FieldDefinitions[fieldRef].Directives.Refs {
			switch doc.DirectiveNameString(dirRef) {
			case "indexed":
				field, err := parseIndexedDirective(doc, dirRef, fieldName, fieldType)
				if err != nil {
					return nil, err
				}
				entity.Fields = append(entity.Fields, *field)
			case "embedding":
				emb, err := parseEmbeddingDirective(doc, dirRef, fieldName)
				if err != nil {
					return nil, err
				}
				entity.EmbeddingFields = append(entity.EmbeddingFields, *emb)
			}
		}
	}

	return entity, nil
}

func parseIndexedDirective(doc *ast.Document, dirRef int, fieldName, fieldType string) (*IndexedField, error) {
	field := &IndexedField{
		FieldName:   fieldName,
		GraphQLType: fieldType,
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("type")); ok {
		enumStr := doc.EnumValueNameString(val.Ref)
		ft, ok := searchindex.ParseFieldType(enumStr)
		if !ok {
			return nil, fmt.Errorf("unknown indexed field type %q on field %s", enumStr, fieldName)
		}
		field.IndexType = ft
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("filterable")); ok {
		field.Filterable = val.Kind == ast.ValueKindBoolean && bool(doc.BooleanValues[val.Ref])
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("sortable")); ok {
		field.Sortable = val.Kind == ast.ValueKindBoolean && bool(doc.BooleanValues[val.Ref])
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("dimensions")); ok {
		if val.Kind == ast.ValueKindInteger {
			field.Dimensions = int(doc.IntValueAsInt32(val.Ref))
		}
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("weight")); ok {
		switch val.Kind {
		case ast.ValueKindFloat:
			field.Weight = float64(doc.FloatValueAsFloat32(val.Ref))
		case ast.ValueKindInteger:
			field.Weight = float64(doc.IntValueAsInt32(val.Ref))
		}
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("autocomplete")); ok {
		field.Autocomplete = val.Kind == ast.ValueKindBoolean && bool(doc.BooleanValues[val.Ref])
	}

	return field, nil
}

func parseEmbeddingDirective(doc *ast.Document, dirRef int, fieldName string) (*EmbeddingField, error) {
	emb := &EmbeddingField{
		FieldName: fieldName,
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("fields")); ok {
		fieldsStr := doc.StringValueContentString(val.Ref)
		emb.SourceFields = strings.Fields(fieldsStr)
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("template")); ok {
		emb.Template = doc.StringValueContentString(val.Ref)
	}

	if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("model")); ok {
		emb.Model = doc.StringValueContentString(val.Ref)
	}

	if len(emb.SourceFields) == 0 || emb.Template == "" || emb.Model == "" {
		return nil, fmt.Errorf("@embedding on field %s requires 'fields', 'template', and 'model' arguments", fieldName)
	}

	return emb, nil
}

// ParsePopulateDirective parses @populate from a query operation document.
// Deprecated: prefer placing @populate on schema extensions and using ParseConfigSchema.
func ParsePopulateDirective(doc *ast.Document, operationRef int) (*PopulateDirective, error) {
	for _, dirRef := range doc.OperationDefinitions[operationRef].Directives.Refs {
		if doc.DirectiveNameString(dirRef) != "populate" {
			continue
		}
		p := &PopulateDirective{}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("index")); ok {
			p.IndexName = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("entity")); ok {
			p.EntityTypeName = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("path")); ok {
			p.Path = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("query")); ok {
			p.Query = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("resyncInterval")); ok {
			p.ResyncInterval = doc.StringValueContentString(val.Ref)
		}
		return p, nil
	}
	return nil, nil
}

// ParseSubscribeDirective parses @subscribe from a subscription operation document.
// Deprecated: prefer placing @subscribe on schema extensions and using ParseConfigSchema.
func ParseSubscribeDirective(doc *ast.Document, operationRef int) (*SubscribeDirective, error) {
	for _, dirRef := range doc.OperationDefinitions[operationRef].Directives.Refs {
		if doc.DirectiveNameString(dirRef) != "subscribe" {
			continue
		}
		s := &SubscribeDirective{}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("index")); ok {
			s.IndexName = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("entity")); ok {
			s.EntityTypeName = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("path")); ok {
			s.Path = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("deletionPath")); ok {
			s.DeletionPath = doc.StringValueContentString(val.Ref)
		}
		if val, ok := doc.DirectiveArgumentValueByName(dirRef, []byte("subscription")); ok {
			s.Subscription = doc.StringValueContentString(val.Ref)
		}
		return s, nil
	}
	return nil, nil
}
