package grpcdatasource

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protoref "google.golang.org/protobuf/reflect/protoreflect"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// Standard GraphQL response paths
const (
	entityPath          = "_entities" // Path for federated entities in response
	dataPath            = "data"      // Standard GraphQL data wrapper
	errorsPath          = "errors"    // Standard GraphQL errors array
	resolveResponsePath = "result"    // Path for resolve response
)

// entityIndex represents the mapping between representation order and result order
// for GraphQL federation entities. This is crucial for maintaining correct entity
// order when multiple subgraphs return entities in different orders.
type entityIndex struct {
	representationIndex int // Index in the original representation array
	resultIndex         int // Index where this entity should appear in the final result
}

// indexMap maps GraphQL type names to their corresponding entity indices
// This allows proper ordering of federated entities by type
type indexMap map[string][]entityIndex

// getResultIndex returns the correct result index for an entity based on its type
// and representation index. This ensures federated entities maintain proper ordering
// across multiple subgraph responses.
func (i indexMap) getResultIndex(val *astjson.Value, representationIndex int) int {
	if i == nil {
		return representationIndex
	}

	if val == nil {
		return representationIndex
	}

	// Extract the __typename field to determine entity type
	typeName := val.Get("__typename").GetStringBytes()

	// Find the correct result index for this type and representation index
	for _, entityIndex := range i[string(typeName)] {
		if entityIndex.representationIndex == representationIndex {
			return entityIndex.resultIndex
		}
	}

	// Fallback to representation index if no mapping found
	return representationIndex
}

// createRepresentationIndexMap builds an index mapping for GraphQL federation entities
// from the variables containing entity representations. This map is used to ensure
// that entities are returned in the correct order when merging responses from multiple
// subgraphs, which is critical for GraphQL federation correctness.
func createRepresentationIndexMap(variables gjson.Result) indexMap {
	var representations []gjson.Result
	r := variables.Get("representations")
	if !r.Exists() {
		return nil
	}

	representations = r.Array()
	im := make(indexMap)
	indexSet := make(map[string]int) // Track count per type name

	// Build mapping for each representation
	for i, representation := range representations {
		typeName := representation.Get("__typename").String()

		// Initialize counter for new type names
		if _, ok := indexSet[typeName]; !ok {
			indexSet[typeName] = -1
		}

		// Increment index for this type
		indexSet[typeName]++

		// Create mapping entry for this entity
		im[typeName] = append(im[typeName], entityIndex{
			representationIndex: indexSet[typeName], // Position within entities of this type
			resultIndex:         i,                  // Position in the overall result array
		})
	}
	return im
}

// jsonBuilder is the core component responsible for converting gRPC protobuf responses
// into GraphQL-compatible JSON format. It handles complex scenarios including:
// - GraphQL federation entity merging and ordering
// - Nested list structures with proper nullability handling
// - Protobuf to GraphQL type conversion
// - Error response formatting
type jsonBuilder struct {
	mapping   *GRPCMapping // Mapping configuration for GraphQL to gRPC translation
	variables gjson.Result // GraphQL variables containing entity representations
	indexMap  indexMap     // Entity index mapping for federation ordering
}

// newJSONBuilder creates a new JSON builder instance with the provided mapping
// and variables. The builder automatically creates an index map for proper
// federation entity ordering if representations are present in the variables.
func newJSONBuilder(mapping *GRPCMapping, variables gjson.Result) *jsonBuilder {
	return &jsonBuilder{
		mapping:   mapping,
		variables: variables,
		indexMap:  createRepresentationIndexMap(variables),
	}
}

// validateFederatedResponse validates that the federated response is valid
// by checking that the number of entities per type is correct.
// For non-federated responses, this function is a no-op.
func (j *jsonBuilder) validateFederatedResponse(response *astjson.Value) error {
	if j.indexMap == nil {
		return nil
	}

	// Get the entities array from the response
	// If we have an index map, we expect it to be a federated response
	entities, err := response.Get(entityPath).Array()
	if err != nil {
		return err
	}

	// Count the number of entities per type
	entitiyCountPerType := make(map[string]int)
	for _, entity := range entities {
		entityType := entity.Get("__typename").GetStringBytes()
		entitiyCountPerType[string(entityType)]++
	}

	// Check that the number of entities per type is correct and exists in the index map.
	for typeName, count := range entitiyCountPerType {
		em, found := j.indexMap[typeName]
		if !found {
			return fmt.Errorf("entity type %s received in the subgraph response, but was not expected", typeName)
		}

		if len(em) != count {
			return fmt.Errorf("entity type %s received %d entities in the subgraph response, but %d are expected", typeName, count, len(em))
		}
	}
	return nil
}

// mergeValues combines two JSON values while preserving proper federation entity ordering.
// This is a critical function for GraphQL federation where multiple subgraphs may
// return entities that need to be merged in the correct order.
func (j *jsonBuilder) mergeValues(left *astjson.Value, right *astjson.Value) (*astjson.Value, error) {
	if len(j.indexMap) == 0 {
		// No federation index map available - use simple merge
		// This path is taken for non-federated queries
		root, _, err := astjson.MergeValues(left, right)
		if err != nil {
			return nil, err
		}
		return root, nil
	}

	// Federation entities present - must preserve representation order
	leftObject, err := left.Object()
	if err != nil {
		return nil, err
	}

	// If left side is empty, just return right side
	if leftObject.Len() == 0 {
		return right, nil
	}

	// Perform federation-aware entity merging
	return j.mergeEntities(left, right)
}

// mergeEntities performs federation-aware merging of entity arrays from multiple subgraph responses.
// This function ensures that entities are placed in the correct positions in the final response
// array based on their original representation order, which is critical for GraphQL federation.
func (j *jsonBuilder) mergeEntities(left *astjson.Value, right *astjson.Value) (*astjson.Value, error) {
	arena := astjson.Arena{}

	// Create the response structure with _entities array
	entities := arena.NewObject()
	entities.Set(entityPath, arena.NewArray())
	arr := entities.Get(entityPath)

	// Extract entity arrays from both responses
	leftRepresentations, err := left.Get(entityPath).Array()
	if err != nil {
		return nil, err
	}

	rightRepresentations, err := right.Get(entityPath).Array()
	if err != nil {
		return nil, err
	}

	// Merge left entities using index mapping to preserve order
	for index, lr := range leftRepresentations {
		arr.SetArrayItem(j.indexMap.getResultIndex(lr, index), lr)
	}

	// Merge right entities using index mapping to preserve order
	for index, rr := range rightRepresentations {
		arr.SetArrayItem(j.indexMap.getResultIndex(rr, index), rr)
	}

	return entities, nil
}

func (j *jsonBuilder) mergeWithPath(base *astjson.Value, resolved *astjson.Value, path ast.Path) error {
	if len(path) == 0 {
		return errors.New("path is empty")
	}

	resolvedValues := resolved.GetArray(resolveResponsePath)

	searchPath := path[:len(path)-1]
	elementName := path[len(path)-1].FieldName.String()

	responseValues := make([]*astjson.Value, 0, len(resolvedValues))

	current := base
	current = current.Get(searchPath[0].FieldName.String())
	switch current.Type() {
	case astjson.TypeArray:
		arr := current.GetArray()
		values, err := j.flattenList(arr, searchPath[1:])
		if err != nil {
			return err
		}
		responseValues = append(responseValues, values...)
	default:
		values, err := j.flattenObject(current, searchPath[1:])
		if err != nil {
			return err
		}
		responseValues = append(responseValues, values...)
	}

	if len(resolvedValues) != len(responseValues) {
		return fmt.Errorf("length of values doesn't match the length of the result array, expected %d, got %d", len(resolvedValues), len(responseValues))
	}

	for i := range responseValues {
		responseValues[i].Set(elementName, resolvedValues[i].Get(elementName))
	}

	return nil
}

func (j *jsonBuilder) flattenObject(value *astjson.Value, path ast.Path) ([]*astjson.Value, error) {
	if path.Len() == 0 {
		return []*astjson.Value{value}, nil
	}

	segment := path[0]
	current := value.Get(segment.FieldName.String())
	result := make([]*astjson.Value, 0)
	switch current.Type() {
	case astjson.TypeObject:
		values, err := j.flattenObject(current, path[1:])
		if err != nil {
			return nil, err
		}
		result = append(result, values...)
	case astjson.TypeArray:
		values, err := j.flattenList(current.GetArray(), path[1:])
		if err != nil {
			return nil, err
		}
		result = append(result, values...)
	default:
		return nil, fmt.Errorf("expected array or object, got %s", current.Type())
	}

	return result, nil
}

func (j *jsonBuilder) flattenList(items []*astjson.Value, path ast.Path) ([]*astjson.Value, error) {
	if path.Len() == 0 {
		return items, nil
	}

	result := make([]*astjson.Value, 0)
	for _, item := range items {
		values, err := j.flattenObject(item, path)
		if err != nil {
			return nil, err
		}
		result = append(result, values...)
	}

	return result, nil
}

// marshalResponseJSON converts a protobuf message into a GraphQL-compatible JSON response.
// This is the core marshaling function that handles all the complex type conversions,
// including oneOf types, nested messages, lists, and scalar values.
func (j *jsonBuilder) marshalResponseJSON(arena *astjson.Arena, message *RPCMessage, data protoref.Message) (*astjson.Value, error) {
	if message == nil {
		return arena.NewNull(), nil
	}

	root := arena.NewObject()

	// Handle protobuf oneOf types - these represent GraphQL union/interface types
	if message.IsOneOf() {
		oneof := data.Descriptor().Oneofs().ByName(protoref.Name(message.OneOfType.FieldName()))
		if oneof == nil {
			return nil, fmt.Errorf("unable to build response JSON: oneof %s not found in message %s", message.OneOfType.FieldName(), message.Name)
		}

		// Determine which oneOf field is actually set
		oneofDescriptor := data.WhichOneof(oneof)
		if oneofDescriptor == nil {
			return nil, fmt.Errorf("unable to build response JSON: oneof %s not found in message %s", message.OneOfType.FieldName(), message.Name)
		}

		// Extract the actual message data from the oneOf wrapper
		if oneofDescriptor.Kind() == protoref.MessageKind {
			data = data.Get(oneofDescriptor).Message()
		}
	}

	// Determine which fields to include in the response
	validFields := message.Fields
	if message.IsOneOf() {
		// For oneOf types, add type-specific fields based on the actual concrete type
		validFields = append(validFields, message.FieldSelectionSet.SelectFieldsForTypes(message.SelectValidTypes(string(data.Type().Descriptor().Name())))...)
	}

	// Process each field in the message
	for _, field := range validFields {
		// Handle static values (like __typename fields)
		if field.StaticValue != "" {
			if len(message.MemberTypes) == 0 {
				// Simple static value - use as-is
				root.Set(field.AliasOrPath(), arena.NewString(field.StaticValue))
				continue
			}

			// Type-specific static value - match against member types
			for _, memberTypes := range message.MemberTypes {
				if memberTypes == string(data.Type().Descriptor().Name()) {
					root.Set(field.AliasOrPath(), arena.NewString(memberTypes))
					break
				}
			}

			continue
		}

		// Get the protobuf field descriptor for this GraphQL field
		fd := data.Descriptor().Fields().ByName(protoref.Name(field.Name))
		if fd == nil {
			// Field not found in protobuf message - skip it
			continue
		}

		// Handle list fields (repeated in protobuf)
		if fd.IsList() {
			list := data.Get(fd).List()
			arr := arena.NewArray()
			root.Set(field.AliasOrPath(), arr)

			if !list.IsValid() {
				// Invalid list - leave as empty array
				continue
			}

			// Process each list item
			for i := 0; i < list.Len(); i++ {
				switch fd.Kind() {
				case protoref.MessageKind:
					// List of messages - recursively marshal each message
					message := list.Get(i).Message()
					value, err := j.marshalResponseJSON(arena, field.Message, message)
					if err != nil {
						return nil, err
					}

					arr.SetArrayItem(i, value)
				default:
					// List of scalar values - convert directly
					j.setArrayItem(i, arena, arr, list.Get(i), fd)
				}
			}

			continue
		}

		// Handle message fields (nested objects)
		if fd.Kind() == protoref.MessageKind {
			msg := data.Get(fd).Message()
			if !msg.IsValid() {
				// Invalid message - set to null
				root.Set(field.AliasOrPath(), arena.NewNull())
				continue
			}

			// Handle special list wrapper types for complex nested lists
			if field.IsListType {
				arr, err := j.flattenListStructure(arena, field.ListMetadata, msg, field.Message)
				if err != nil {
					return nil, fmt.Errorf("unable to flatten list structure for field %q: %w", field.AliasOrPath(), err)
				}

				root.Set(field.AliasOrPath(), arr)
				continue
			}

			// Handle optional scalar wrapper types (e.g., google.protobuf.StringValue)
			if field.IsOptionalScalar() {
				err := j.resolveOptionalField(arena, root, field.AliasOrPath(), msg)
				if err != nil {
					return nil, err
				}

				continue
			}

			// Regular nested message - recursively marshal
			value, err := j.marshalResponseJSON(arena, field.Message, msg)
			if err != nil {
				return nil, err
			}

			if field.JSONPath == "" {
				// Field should be merged into parent object (flattened)
				root, _, err = astjson.MergeValues(root, value)
				if err != nil {
					return nil, err
				}
			} else {
				// Field should be nested under its own key
				root.Set(field.AliasOrPath(), value)
			}

			continue
		}

		// Handle scalar fields (string, int, bool, etc.)
		j.setJSONValue(arena, root, field.AliasOrPath(), data, fd)
	}

	return root, nil
}

// flattenListStructure handles complex nested list structures that are wrapped in protobuf
// messages to support nullable and multi-dimensional lists. This is necessary because
// protobuf doesn't directly support nullable list items or complex nesting scenarios
// that GraphQL allows.
func (j *jsonBuilder) flattenListStructure(arena *astjson.Arena, md *ListMetadata, data protoref.Message, message *RPCMessage) (*astjson.Value, error) {
	if md == nil {
		return arena.NewNull(), errors.New("list metadata not found")
	}

	// Validate metadata consistency
	if len(md.LevelInfo) < md.NestingLevel {
		return arena.NewNull(), errors.New("nesting level data does not match the number of levels in the list metadata")
	}

	// Handle null data with proper nullability checking
	if !data.IsValid() {
		if md.LevelInfo[0].Optional {
			return arena.NewNull(), nil
		}

		return arena.NewNull(), errors.New("cannot add null item to response for non nullable list")
	}

	// Start recursive traversal of the nested list structure
	root := arena.NewArray()
	return j.traverseList(0, arena, root, md, data, message)
}

// traverseList recursively traverses nested list wrapper structures to extract the actual
// list data. This handles multi-dimensional lists like [[String]] or [[[User]]] by
// unwrapping the protobuf message wrappers at each level.
func (j *jsonBuilder) traverseList(level int, arena *astjson.Arena, current *astjson.Value, md *ListMetadata, data protoref.Message, message *RPCMessage) (*astjson.Value, error) {
	if level > md.NestingLevel {
		return current, nil
	}

	// List wrappers always use field number 1 in the generated protobuf
	fd := data.Descriptor().Fields().ByNumber(1)
	if fd == nil {
		return arena.NewNull(), fmt.Errorf("field with number %d not found in message %q", 1, data.Descriptor().Name())
	}

	if fd.Kind() != protoref.MessageKind {
		return arena.NewNull(), fmt.Errorf("field %q is not a message", fd.Name())
	}

	// Get the wrapper message containing the list
	msg := data.Get(fd).Message()
	if !msg.IsValid() {
		// Handle null wrapper based on nullability rules
		if md.LevelInfo[level].Optional {
			return arena.NewNull(), nil
		}

		return arena.NewArray(), errors.New("cannot add null item to response for non nullable list")
	}

	// The actual list is always at field number 1 in the wrapper
	fd = msg.Descriptor().Fields().ByNumber(1)
	if !fd.IsList() {
		return arena.NewNull(), fmt.Errorf("field %q is not a list", fd.Name())
	}

	// Handle intermediate nesting levels (not the final level)
	if level < md.NestingLevel-1 {
		list := msg.Get(fd).List()
		for i := 0; i < list.Len(); i++ {
			// Create nested array for next level
			next := arena.NewArray()
			val, err := j.traverseList(level+1, arena, next, md, list.Get(i).Message(), message)
			if err != nil {
				return nil, err
			}

			current.SetArrayItem(i, val)
		}

		return current, nil
	}

	// Handle the final nesting level - extract actual data
	list := msg.Get(fd).List()
	if !list.IsValid() {
		// Invalid list at final level - return empty array
		// Nullability is checked at the wrapper level, not the list level
		return arena.NewArray(), nil
	}

	// Process each item in the final list
	for i := 0; i < list.Len(); i++ {
		if message != nil {
			// List of complex objects - recursively marshal each item
			val, err := j.marshalResponseJSON(arena, message, list.Get(i).Message())
			if err != nil {
				return nil, err
			}

			current.SetArrayItem(i, val)
		} else {
			// List of scalar values - convert directly
			j.setArrayItem(i, arena, current, list.Get(i), fd)
		}
	}

	return current, nil
}

// resolveOptionalField extracts the value from optional scalar wrapper types like
// google.protobuf.StringValue, google.protobuf.Int32Value, etc. These wrappers
// are used to represent nullable scalar values in protobuf.
func (j *jsonBuilder) resolveOptionalField(arena *astjson.Arena, root *astjson.Value, name string, data protoref.Message) error {
	// Optional scalar wrappers always have a "value" field
	fd := data.Descriptor().Fields().ByName(protoref.Name("value"))
	if fd == nil {
		return fmt.Errorf("unable to resolve optional field: field %q not found in message %s", "value", data.Descriptor().Name())
	}

	// Extract and set the wrapped value
	j.setJSONValue(arena, root, name, data, fd)
	return nil
}

// setJSONValue converts a protobuf field value to the appropriate JSON representation
// and sets it on the provided JSON object. This handles all protobuf scalar types
// and enum values with proper GraphQL mapping.
func (j *jsonBuilder) setJSONValue(arena *astjson.Arena, root *astjson.Value, name string, data protoref.Message, fd protoref.FieldDescriptor) {
	if !data.IsValid() {
		root.Set(name, arena.NewNull())
		return
	}

	switch fd.Kind() {
	case protoref.BoolKind:
		boolValue := data.Get(fd).Bool()
		if boolValue {
			root.Set(name, arena.NewTrue())
		} else {
			root.Set(name, arena.NewFalse())
		}
	case protoref.StringKind:
		root.Set(name, arena.NewString(data.Get(fd).String()))
	case protoref.Int32Kind:
		root.Set(name, arena.NewNumberInt(int(data.Get(fd).Int())))
	case protoref.Int64Kind:
		root.Set(name, arena.NewNumberString(strconv.FormatInt(data.Get(fd).Int(), 10)))
	case protoref.Uint32Kind, protoref.Uint64Kind:
		root.Set(name, arena.NewNumberString(strconv.FormatUint(data.Get(fd).Uint(), 10)))
	case protoref.FloatKind, protoref.DoubleKind:
		root.Set(name, arena.NewNumberFloat64(data.Get(fd).Float()))
	case protoref.BytesKind:
		root.Set(name, arena.NewStringBytes(data.Get(fd).Bytes()))
	case protoref.EnumKind:
		enumDesc := fd.Enum()
		enumValueDesc := enumDesc.Values().ByNumber(data.Get(fd).Enum())
		if enumValueDesc == nil {
			root.Set(name, arena.NewNull())
			return
		}

		// Look up the GraphQL enum value mapping
		graphqlValue, ok := j.mapping.FindEnumValueMapping(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			// No mapping found - set to null
			root.Set(name, arena.NewNull())
			return
		}

		root.Set(name, arena.NewString(graphqlValue))
	}
}

// setArrayItem converts a protobuf list item value to JSON and sets it at the specified
// array index. This is similar to setJSONValue but operates on array elements rather
// than object properties, and works with protobuf Value types rather than Message types.
func (j *jsonBuilder) setArrayItem(index int, arena *astjson.Arena, array *astjson.Value, data protoref.Value, fd protoref.FieldDescriptor) {
	if !data.IsValid() {
		array.SetArrayItem(index, arena.NewNull())
		return
	}

	switch fd.Kind() {
	case protoref.BoolKind:
		boolValue := data.Bool()
		if boolValue {
			array.SetArrayItem(index, arena.NewTrue())
		} else {
			array.SetArrayItem(index, arena.NewFalse())
		}
	case protoref.StringKind:
		array.SetArrayItem(index, arena.NewString(data.String()))
	case protoref.Int32Kind:
		array.SetArrayItem(index, arena.NewNumberInt(int(data.Int())))
	case protoref.Int64Kind:
		array.SetArrayItem(index, arena.NewNumberString(strconv.FormatInt(data.Int(), 10)))
	case protoref.Uint32Kind, protoref.Uint64Kind:
		array.SetArrayItem(index, arena.NewNumberString(strconv.FormatUint(data.Uint(), 10)))
	case protoref.FloatKind, protoref.DoubleKind:
		array.SetArrayItem(index, arena.NewNumberFloat64(data.Float()))
	case protoref.BytesKind:
		array.SetArrayItem(index, arena.NewStringBytes(data.Bytes()))
	case protoref.EnumKind:
		enumDesc := fd.Enum()
		enumValueDesc := enumDesc.Values().ByNumber(data.Enum())
		if enumValueDesc == nil {
			array.SetArrayItem(index, arena.NewNull())
			return
		}

		// Look up GraphQL enum mapping
		graphqlValue, ok := j.mapping.FindEnumValueMapping(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			// No mapping found - use null
			array.SetArrayItem(index, arena.NewNull())
			return
		}

		array.SetArrayItem(index, arena.NewString(graphqlValue))
	}
}

// toDataObject wraps a response value in the standard GraphQL data envelope.
// This creates the top-level structure { "data": ... } that GraphQL clients expect.
func (j *jsonBuilder) toDataObject(root *astjson.Value) *astjson.Value {
	a := astjson.Arena{}
	data := a.NewObject()
	data.Set(dataPath, root)
	return data
}

// writeErrorBytes creates a properly formatted GraphQL error response in JSON format.
// This includes the error message and gRPC status code information in the extensions
// field, following GraphQL error specification standards.
func (j *jsonBuilder) writeErrorBytes(err error) []byte {
	a := astjson.Arena{}
	defer a.Reset()

	// Create standard GraphQL error structure
	errorRoot := a.NewObject()
	errorArray := a.NewArray()
	errorRoot.Set(errorsPath, errorArray)

	// Create individual error object
	errorItem := a.NewObject()
	errorItem.Set("message", a.NewString(err.Error()))

	// Add gRPC status code information to extensions
	extensions := a.NewObject()
	if st, ok := status.FromError(err); ok {
		// gRPC error - include the specific status code
		extensions.Set("code", a.NewString(st.Code().String()))
	} else {
		// Generic error - default to INTERNAL status
		extensions.Set("code", a.NewString(codes.Internal.String()))
	}

	errorItem.Set("extensions", extensions)
	errorArray.SetArrayItem(0, errorItem)

	return errorRoot.MarshalTo(nil)
}
