package resolve

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/wundergraph/astjson"
	"github.com/wundergraph/go-arena"
)

// KeyField is the resolve-side representation of an entity @key field set.
type KeyField struct {
	Name     string
	Children []KeyField
}

// ParseKeyFields converts a GraphQL field-set selection string into KeyFields.
func ParseKeyFields(selectionSet string) []KeyField {
	tokens := tokenizeKeyFields(selectionSet)
	index := 0
	return parseKeyFields(tokens, &index)
}

// EntityQueryCacheKeyTemplate renders entity-fetch cache keys from @key fields.
type EntityQueryCacheKeyTemplate struct {
	Keys     *ResolvableObjectVariable
	TypeName string
}

func (t *EntityQueryCacheKeyTemplate) RenderCacheKeys(a arena.Arena, _ *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error) {
	if t == nil {
		return nil, nil
	}
	keyFields := t.KeyFields()
	keys := make([]*CacheKey, 0, len(items))
	for _, item := range items {
		key, ok := renderEntityCacheKey(a, item, t.TypeName, keyFields, prefix)
		if !ok {
			continue
		}
		keys = append(keys, &CacheKey{
			Item: item,
			Keys: []string{
				key,
			},
		})
	}
	return keys, nil
}

func (*EntityQueryCacheKeyTemplate) IsEntityFetch() bool {
	return true
}

func (*EntityQueryCacheKeyTemplate) BatchEntityKeyArgumentPath() []string {
	return nil
}

func (*EntityQueryCacheKeyTemplate) EntityMergePath(PostProcessingConfiguration) []string {
	return nil
}

// KeyFields returns the template's @key field tree, excluding __typename.
func (t *EntityQueryCacheKeyTemplate) KeyFields() []KeyField {
	if t == nil || t.Keys == nil || t.Keys.Renderer == nil {
		return nil
	}
	object, ok := t.Keys.Renderer.Node.(*Object)
	if !ok {
		return nil
	}
	return objectKeyFields(object)
}

// RootField describes a cacheable root field and the argument values that seed its key.
type RootField struct {
	TypeName    string
	FieldName   string
	ResponseKey string
	Arguments   []RootFieldArgument
}

// RootFieldArgument describes one root-field argument rendered into the root key.
type RootFieldArgument struct {
	Name         string
	Value        *astjson.Value
	VariablePath []string
}

// EntityKeyMappingConfig maps root-field arguments to entity @key fields.
type EntityKeyMappingConfig struct {
	EntityTypeName string
	FieldMappings  []EntityFieldMappingConfig
}

// EntityFieldMappingConfig maps one root argument path to one entity @key field.
type EntityFieldMappingConfig struct {
	EntityKeyField      string
	ArgumentPath        []string
	ArgumentIsEntityKey bool
}

// RootQueryCacheKeyTemplate renders root-field cache keys or mapped entity keys.
type RootQueryCacheKeyTemplate struct {
	RootFields        []RootField
	EntityKeyMappings []EntityKeyMappingConfig

	batchEntityKeyArgumentPath []string
}

func NewRootQueryCacheKeyTemplate(rootFields []RootField, entityKeyMappings []EntityKeyMappingConfig) *RootQueryCacheKeyTemplate {
	template := &RootQueryCacheKeyTemplate{
		RootFields:        rootFields,
		EntityKeyMappings: entityKeyMappings,
	}
	for _, mapping := range entityKeyMappings {
		for _, fieldMapping := range mapping.FieldMappings {
			if fieldMapping.ArgumentIsEntityKey {
				template.batchEntityKeyArgumentPath = append([]string(nil), fieldMapping.ArgumentPath...)
				return template
			}
		}
	}
	return template
}

func (t *RootQueryCacheKeyTemplate) RenderCacheKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) ([]*CacheKey, error) {
	if t == nil {
		return nil, nil
	}
	if len(items) == 0 {
		items = []*astjson.Value{nil}
	}
	if len(t.EntityKeyMappings) > 0 {
		if len(t.batchEntityKeyArgumentPath) > 0 {
			return t.renderBatchEntityKeys(a, ctx, items[0], prefix), nil
		}
		return t.renderMappedEntityKeys(a, ctx, items, prefix), nil
	}
	return t.renderRootFieldKeys(a, ctx, items, prefix), nil
}

func (t *RootQueryCacheKeyTemplate) IsEntityFetch() bool {
	return t != nil && len(t.EntityKeyMappings) > 0
}

func (t *RootQueryCacheKeyTemplate) BatchEntityKeyArgumentPath() []string {
	if t == nil || len(t.batchEntityKeyArgumentPath) == 0 {
		return nil
	}
	return append([]string(nil), t.batchEntityKeyArgumentPath...)
}

func (t *RootQueryCacheKeyTemplate) EntityMergePath(pp PostProcessingConfiguration) []string {
	if t == nil || len(t.EntityKeyMappings) == 0 {
		return nil
	}
	if len(pp.MergePath) > 0 {
		return append([]string(nil), pp.MergePath...)
	}
	if len(t.RootFields) == 0 {
		return nil
	}
	responseKey := t.RootFields[0].ResponseKey
	if responseKey == "" {
		responseKey = t.RootFields[0].FieldName
	}
	if responseKey == "" {
		return nil
	}
	return []string{responseKey}
}

func (t *RootQueryCacheKeyTemplate) renderRootFieldKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) []*CacheKey {
	rendered := make([]string, 0, len(t.RootFields))
	for _, field := range t.RootFields {
		rendered = append(rendered, renderRootFieldCacheKey(a, ctx, field, prefix))
	}
	cacheKeys := make([]*CacheKey, 0, len(items))
	for _, item := range items {
		cacheKeys = append(cacheKeys, &CacheKey{
			Item: item,
			Keys: append([]string(nil), rendered...),
		})
	}
	return cacheKeys
}

func (t *RootQueryCacheKeyTemplate) renderMappedEntityKeys(a arena.Arena, ctx *Context, items []*astjson.Value, prefix string) []*CacheKey {
	rendered := make([]string, 0, len(t.EntityKeyMappings))
	for _, mapping := range t.EntityKeyMappings {
		key, ok := renderMappedEntityKey(a, ctx, mapping, prefix)
		if !ok {
			continue
		}
		rendered = append(rendered, key)
	}
	if len(rendered) == 0 {
		return nil
	}
	cacheKeys := make([]*CacheKey, 0, len(items))
	for _, item := range items {
		cacheKeys = append(cacheKeys, &CacheKey{
			Item: item,
			Keys: append([]string(nil), rendered...),
		})
	}
	return cacheKeys
}

func (t *RootQueryCacheKeyTemplate) renderBatchEntityKeys(a arena.Arena, ctx *Context, item *astjson.Value, prefix string) []*CacheKey {
	var batchMapping EntityFieldMappingConfig
	var batchMappingFound bool
	var mappingConfig EntityKeyMappingConfig
	for _, mapping := range t.EntityKeyMappings {
		for _, fieldMapping := range mapping.FieldMappings {
			if fieldMapping.ArgumentIsEntityKey {
				batchMapping = fieldMapping
				batchMappingFound = true
				mappingConfig = mapping
				break
			}
		}
		if batchMappingFound {
			break
		}
	}
	if !batchMappingFound {
		return nil
	}
	batchValue, batchPath, batchRelativePath := resolveBatchValue(ctx, batchMapping.ArgumentPath)
	if batchValue == nil || batchValue.Type() == astjson.TypeNull || batchValue.Type() != astjson.TypeArray {
		return nil
	}
	items := batchValue.GetArray()
	cacheKeys := make([]*CacheKey, 0, len(items))
	for index, batchItem := range items {
		keyObject := astjson.ObjectValue(a)
		ok := true
		for _, fieldMapping := range mappingConfig.FieldMappings {
			var value *astjson.Value
			if fieldMapping.ArgumentIsEntityKey {
				value = batchItem
				if len(batchRelativePath) > 0 {
					value = valueAtPath(batchItem, batchRelativePath)
				}
			} else if pathHasPrefix(fieldMapping.ArgumentPath, batchPath) {
				value = valueAtPath(batchItem, fieldMapping.ArgumentPath[len(batchPath):])
			} else {
				value = contextValueAtPath(ctx, fieldMapping.ArgumentPath)
			}
			if value == nil || value.Type() == astjson.TypeNull {
				ok = false
				break
			}
			setMappedKeyValue(a, keyObject, fieldMapping.EntityKeyField, value)
		}
		if !ok || objectLen(keyObject) == 0 {
			continue
		}
		key := buildEntityKeyString(a, mappingConfig.EntityTypeName, keyObject, prefix)
		cacheKeys = append(cacheKeys, &CacheKey{
			Item:       item,
			BatchIndex: index,
			Keys: []string{
				key,
			},
		})
	}
	return cacheKeys
}

func renderMappedEntityKey(a arena.Arena, ctx *Context, mapping EntityKeyMappingConfig, prefix string) (string, bool) {
	keyObject := astjson.ObjectValue(a)
	for _, fieldMapping := range mapping.FieldMappings {
		value := contextValueAtPath(ctx, fieldMapping.ArgumentPath)
		if value == nil || value.Type() == astjson.TypeNull {
			return "", false
		}
		setMappedKeyValue(a, keyObject, fieldMapping.EntityKeyField, value)
	}
	if objectLen(keyObject) == 0 {
		return "", false
	}
	return buildEntityKeyString(a, mapping.EntityTypeName, keyObject, prefix), true
}

func renderEntityCacheKey(a arena.Arena, item *astjson.Value, fallbackTypeName string, keyFields []KeyField, prefix string) (string, bool) {
	if len(keyFields) == 0 || item == nil || item.Type() == astjson.TypeNull {
		return "", false
	}
	keyObject := astjson.ObjectValue(a)
	for _, field := range keyFields {
		value, ok := extractKeyFieldValue(a, item, field)
		if !ok {
			return "", false
		}
		keyObject.Set(a, field.Name, value)
	}
	if objectLen(keyObject) == 0 {
		return "", false
	}
	typeName := fallbackTypeName
	if value := item.Get("__typename"); value != nil && value.Type() != astjson.TypeNull {
		if value.Type() == astjson.TypeString {
			typeName = string(value.GetStringBytes())
		} else {
			typeName = string(value.MarshalTo(nil))
		}
	}
	return buildEntityKeyString(a, typeName, keyObject, prefix), true
}

func extractKeyFieldValue(a arena.Arena, item *astjson.Value, field KeyField) (*astjson.Value, bool) {
	value := item.Get(field.Name)
	if value == nil || value.Type() == astjson.TypeNull {
		return nil, false
	}
	if len(field.Children) == 0 {
		return copyEntityKeyValue(a, value), true
	}
	if value.Type() != astjson.TypeObject {
		return nil, false
	}
	object := astjson.ObjectValue(a)
	for _, child := range field.Children {
		childValue, ok := extractKeyFieldValue(a, value, child)
		if !ok {
			return nil, false
		}
		object.Set(a, child.Name, childValue)
	}
	return object, objectLen(object) > 0
}

func buildEntityKeyString(a arena.Arena, typeName string, keyObject *astjson.Value, prefix string) string {
	root := astjson.ObjectValue(a)
	root.Set(a, "__typename", astjson.StringValue(a, typeName))
	root.Set(a, "key", keyObject)
	return prefixCacheKey(prefix, string(root.MarshalTo(nil)))
}

func renderRootFieldCacheKey(a arena.Arena, ctx *Context, field RootField, prefix string) string {
	root := astjson.ObjectValue(a)
	root.Set(a, "__typename", astjson.StringValue(a, field.TypeName))
	root.Set(a, "field", astjson.StringValue(a, field.FieldName))
	if len(field.Arguments) > 0 {
		args := astjson.ObjectValue(a)
		arguments := append([]RootFieldArgument(nil), field.Arguments...)
		sort.SliceStable(arguments, func(i, j int) bool {
			return arguments[i].Name < arguments[j].Name
		})
		for _, argument := range arguments {
			value := argument.Value
			if value == nil && len(argument.VariablePath) > 0 {
				value = contextValueAtPath(ctx, argument.VariablePath)
			}
			args.Set(a, argument.Name, copyRootArgumentValue(a, value))
		}
		root.Set(a, "args", args)
	}
	return prefixCacheKey(prefix, string(root.MarshalTo(nil)))
}

func copyEntityKeyValue(a arena.Arena, value *astjson.Value) *astjson.Value {
	if value == nil {
		return nil
	}
	switch value.Type() {
	case astjson.TypeString:
		return astjson.StringValueBytes(a, value.GetStringBytes())
	case astjson.TypeNumber:
		return astjson.StringValueBytes(a, value.MarshalTo(nil))
	case astjson.TypeObject:
		object := astjson.ObjectValue(a)
		value.GetObject().Visit(func(key []byte, child *astjson.Value) {
			object.Set(a, string(key), copyEntityKeyValue(a, child))
		})
		return object
	case astjson.TypeArray:
		array := astjson.ArrayValue(a)
		for index, child := range value.GetArray() {
			array.SetArrayItem(a, index, copyEntityKeyValue(a, child))
		}
		return array
	default:
		var parser astjson.Parser
		return parser.DeepCopy(a, value)
	}
}

func copyRootArgumentValue(a arena.Arena, value *astjson.Value) *astjson.Value {
	if value == nil {
		return nil
	}
	switch value.Type() {
	case astjson.TypeObject:
		object := astjson.ObjectValue(a)
		keys := make([]string, 0)
		values := make(map[string]*astjson.Value)
		value.GetObject().Visit(func(key []byte, child *astjson.Value) {
			name := string(key)
			keys = append(keys, name)
			values[name] = child
		})
		sort.Strings(keys)
		for _, key := range keys {
			object.Set(a, key, copyRootArgumentValue(a, values[key]))
		}
		return object
	case astjson.TypeArray:
		array := astjson.ArrayValue(a)
		for index, child := range value.GetArray() {
			array.SetArrayItem(a, index, copyRootArgumentValue(a, child))
		}
		return array
	default:
		var parser astjson.Parser
		return parser.DeepCopy(a, value)
	}
}

func setMappedKeyValue(a arena.Arena, keyObject *astjson.Value, entityKeyField string, value *astjson.Value) {
	path := strings.Split(entityKeyField, ".")
	if len(path) == 0 || path[0] == "" {
		return
	}
	current := keyObject
	for _, segment := range path[:len(path)-1] {
		if segment == "" {
			return
		}
		child := current.Get(segment)
		if child == nil || child.Type() != astjson.TypeObject {
			child = astjson.ObjectValue(a)
			current.Set(a, segment, child)
		}
		current = child
	}
	current.Set(a, path[len(path)-1], copyEntityKeyValue(a, value))
}

func objectKeyFields(object *Object) []KeyField {
	if object == nil {
		return nil
	}
	fields := make([]KeyField, 0, len(object.Fields))
	for _, field := range object.Fields {
		if field == nil {
			continue
		}
		name := string(field.Name)
		if name == "" || name == "__typename" {
			continue
		}
		keyField := KeyField{Name: name}
		if childObject, ok := field.Value.(*Object); ok {
			keyField.Children = objectKeyFields(childObject)
		}
		fields = append(fields, keyField)
	}
	return fields
}

func contextValueAtPath(ctx *Context, path []string) *astjson.Value {
	if ctx == nil || ctx.Variables == nil || len(path) == 0 {
		return nil
	}
	head := path[0]
	if ctx.RemapVariables != nil {
		if remapped, ok := ctx.RemapVariables[head]; ok {
			head = remapped
		}
	}
	return valueAtPath(ctx.Variables, append([]string{head}, path[1:]...))
}

func valueAtPath(value *astjson.Value, path []string) *astjson.Value {
	current := value
	for _, segment := range path {
		if current == nil {
			return nil
		}
		switch current.Type() {
		case astjson.TypeObject:
			current = current.Get(segment)
		case astjson.TypeArray:
			index, err := strconv.Atoi(segment)
			if err != nil {
				return nil
			}
			array := current.GetArray()
			if index < 0 || index >= len(array) {
				return nil
			}
			current = array[index]
		default:
			return nil
		}
	}
	return current
}

func resolveBatchValue(ctx *Context, path []string) (*astjson.Value, []string, []string) {
	for i := len(path); i > 0; i-- {
		prefix := path[:i]
		value := contextValueAtPath(ctx, prefix)
		if value != nil && value.Type() == astjson.TypeArray {
			return value, append([]string(nil), prefix...), append([]string(nil), path[i:]...)
		}
	}
	return nil, nil, nil
}

func pathHasPrefix(path []string, prefix []string) bool {
	if len(prefix) > len(path) {
		return false
	}
	for i := range prefix {
		if path[i] != prefix[i] {
			return false
		}
	}
	return true
}

func objectLen(value *astjson.Value) int {
	if value == nil || value.Type() != astjson.TypeObject {
		return 0
	}
	count := 0
	value.GetObject().Visit(func(_ []byte, _ *astjson.Value) {
		count++
	})
	return count
}

func prefixCacheKey(prefix string, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + ":" + key
}

func tokenizeKeyFields(selectionSet string) []string {
	tokens := make([]string, 0)
	for i := 0; i < len(selectionSet); {
		r := rune(selectionSet[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if selectionSet[i] == '{' || selectionSet[i] == '}' {
			tokens = append(tokens, selectionSet[i:i+1])
			i++
			continue
		}
		start := i
		for i < len(selectionSet) {
			r = rune(selectionSet[i])
			if unicode.IsSpace(r) || selectionSet[i] == '{' || selectionSet[i] == '}' {
				break
			}
			i++
		}
		tokens = append(tokens, selectionSet[start:i])
	}
	return tokens
}

func parseKeyFields(tokens []string, index *int) []KeyField {
	fields := make([]KeyField, 0)
	for *index < len(tokens) {
		token := tokens[*index]
		*index++
		if token == "}" {
			return fields
		}
		if token == "{" {
			continue
		}
		field := KeyField{Name: token}
		if *index < len(tokens) && tokens[*index] == "{" {
			*index++
			field.Children = parseKeyFields(tokens, index)
		}
		if field.Name != "__typename" {
			fields = append(fields, field)
		}
	}
	return fields
}
