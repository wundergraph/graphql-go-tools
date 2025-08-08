package grpcdatasource

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/tidwall/gjson"
	"github.com/wundergraph/astjson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	protoref "google.golang.org/protobuf/reflect/protoreflect"
)

var (
	entityPath = "_entities"
	dataPath   = "data"
	errorsPath = "errors"
)

type entityIndex struct {
	representationIndex int
	resultIndex         int
}

type indexMap map[string][]entityIndex

func (i indexMap) getResultIndex(val *astjson.Value, representationIndex int) int {
	if i == nil {
		return representationIndex
	}

	if val == nil {
		return representationIndex
	}

	typeName := val.Get("__typename").GetStringBytes()

	for _, entityIndex := range i[string(typeName)] {
		if entityIndex.representationIndex == representationIndex {
			return entityIndex.resultIndex
		}
	}

	return representationIndex
}

func createRepresentationIndexMap(variables gjson.Result) indexMap {
	var representations []gjson.Result
	r := variables.Get("representations")
	if !r.Exists() {
		return nil
	}

	representations = r.Array()
	im := make(indexMap)
	indexSet := make(map[string]int)
	for i, representation := range representations {
		typeName := representation.Get("__typename").String()
		if _, ok := indexSet[typeName]; !ok {
			indexSet[typeName] = -1
		}

		indexSet[typeName]++

		im[typeName] = append(im[typeName], entityIndex{
			representationIndex: indexSet[typeName],
			resultIndex:         i,
		})
	}
	return im
}

type jsonBuilder struct {
	mapping   *GRPCMapping
	variables gjson.Result
	indexMap  indexMap
}

func newJSONBuilder(mapping *GRPCMapping, variables gjson.Result) *jsonBuilder {
	return &jsonBuilder{
		mapping:   mapping,
		variables: variables,
		indexMap:  createRepresentationIndexMap(variables),
	}
}

func (j *jsonBuilder) mergeValues(left *astjson.Value, right *astjson.Value) (*astjson.Value, error) {
	if len(j.indexMap) == 0 {
		// We don't have a representation index map, so we can just merge the values.
		root, _, err := astjson.MergeValues(left, right)
		if err != nil {
			return nil, err
		}
		return root, nil
	}

	// When we have an index map, we need to ensure to keep the order of the representations.
	leftObject, err := left.Object()
	if err != nil {
		return nil, err
	}

	if leftObject.Len() == 0 {
		return right, nil
	}

	return j.mergeEntities(left, right)
}

func (j *jsonBuilder) mergeEntities(left *astjson.Value, right *astjson.Value) (*astjson.Value, error) {

	root := astjson.Arena{}
	defer root.Reset()

	entities := root.NewObject()
	entities.Set(entityPath, root.NewArray())

	arr := entities.Get(entityPath)

	leftRepresentations, err := left.Get(entityPath).Array()
	if err != nil {
		return nil, err
	}

	rightRepresentations, err := right.Get(entityPath).Array()
	if err != nil {
		return nil, err
	}

	for index, lr := range leftRepresentations {
		resultIndex := j.indexMap.getResultIndex(lr, index)
		arr.SetArrayItem(resultIndex, lr)
	}

	for index, rr := range rightRepresentations {
		resultIndex := j.indexMap.getResultIndex(rr, index)
		arr.SetArrayItem(resultIndex, rr)
	}

	return entities, nil
}

func (j *jsonBuilder) marshalResponseJSON(arena *astjson.Arena, message *RPCMessage, data protoref.Message) (*astjson.Value, error) {
	if message == nil {
		return arena.NewNull(), nil
	}

	root := arena.NewObject()

	if message.IsOneOf() {
		oneof := data.Descriptor().Oneofs().ByName(protoref.Name(message.OneOfType.FieldName()))
		if oneof == nil {
			return nil, fmt.Errorf("unable to build response JSON: oneof %s not found in message %s", message.OneOfType.FieldName(), message.Name)
		}

		oneofDescriptor := data.WhichOneof(oneof)
		if oneofDescriptor == nil {
			return nil, fmt.Errorf("unable to build response JSON: oneof %s not found in message %s", message.OneOfType.FieldName(), message.Name)
		}

		if oneofDescriptor.Kind() == protoref.MessageKind {
			data = data.Get(oneofDescriptor).Message()
		}
	}

	validFields := message.Fields
	if message.IsOneOf() {
		validFields = append(validFields, message.FieldSelectionSet.SelectFieldsForTypes(message.SelectValidTypes(string(data.Type().Descriptor().Name())))...)
	}

	for _, field := range validFields {
		if field.StaticValue != "" {
			if len(message.MemberTypes) == 0 {
				root.Set(field.AliasOrPath(), arena.NewString(field.StaticValue))
				continue
			}

			for _, memberTypes := range message.MemberTypes {
				if memberTypes == string(data.Type().Descriptor().Name()) {
					root.Set(field.AliasOrPath(), arena.NewString(memberTypes))
					break
				}
			}

			continue
		}

		fd := data.Descriptor().Fields().ByName(protoref.Name(field.Name))
		if fd == nil {
			continue
		}

		if fd.IsList() {
			list := data.Get(fd).List()
			arr := arena.NewArray()
			root.Set(field.AliasOrPath(), arr)

			if !list.IsValid() {
				continue
			}

			for i := 0; i < list.Len(); i++ {
				switch fd.Kind() {
				case protoref.MessageKind:
					message := list.Get(i).Message()
					value, err := j.marshalResponseJSON(arena, field.Message, message)
					if err != nil {
						return nil, err
					}

					arr.SetArrayItem(i, value)
				default:
					j.setArrayItem(i, arena, arr, list.Get(i), fd)
				}

			}

			continue
		}

		if fd.Kind() == protoref.MessageKind {
			msg := data.Get(fd).Message()
			if !msg.IsValid() {
				root.Set(field.AliasOrPath(), arena.NewNull())
				continue
			}

			if field.IsListType {
				arr, err := j.flattenListStructure(arena, field.ListMetadata, msg, field.Message)
				if err != nil {
					return nil, fmt.Errorf("unable to flatten list structure for field %q: %w", field.AliasOrPath(), err)
				}

				root.Set(field.AliasOrPath(), arr)
				continue
			}

			if field.IsOptionalScalar() {
				err := j.resolveOptionalField(arena, root, field.AliasOrPath(), msg)
				if err != nil {
					return nil, err
				}

				continue
			}

			value, err := j.marshalResponseJSON(arena, field.Message, msg)
			if err != nil {
				return nil, err
			}

			if field.JSONPath == "" {
				root, _, err = astjson.MergeValues(root, value)
				if err != nil {
					return nil, err
				}
			} else {
				root.Set(field.AliasOrPath(), value)
			}

			continue
		}

		j.setJSONValue(arena, root, field.AliasOrPath(), data, fd)
	}

	return root, nil
}

func (j *jsonBuilder) flattenListStructure(arena *astjson.Arena, md *ListMetadata, data protoref.Message, message *RPCMessage) (*astjson.Value, error) {
	if md == nil {
		return arena.NewNull(), errors.New("list metadata not found")
	}

	if len(md.LevelInfo) < md.NestingLevel {
		return arena.NewNull(), errors.New("nesting level data does not match the number of levels in the list metadata")
	}

	if !data.IsValid() {
		if md.LevelInfo[0].Optional {
			return arena.NewNull(), nil
		}

		return arena.NewNull(), errors.New("cannot add null item to response for non nullable list")
	}

	root := arena.NewArray()
	return j.traverseList(0, arena, root, md, data, message)
}

func (j *jsonBuilder) traverseList(level int, arena *astjson.Arena, current *astjson.Value, md *ListMetadata, data protoref.Message, message *RPCMessage) (*astjson.Value, error) {
	if level > md.NestingLevel {
		return current, nil
	}

	// List wrappers always use field number 1
	fd := data.Descriptor().Fields().ByNumber(1)
	if fd == nil {
		return arena.NewNull(), fmt.Errorf("field with number %d not found in message %q", 1, data.Descriptor().Name())
	}

	if fd.Kind() != protoref.MessageKind {
		return arena.NewNull(), fmt.Errorf("field %q is not a message", fd.Name())
	}

	msg := data.Get(fd).Message()
	if !msg.IsValid() {
		// If the message is not valid we can either return null if the list is nullable or an error if it is non nullable.
		if md.LevelInfo[level].Optional {
			return arena.NewNull(), nil
		}

		return arena.NewArray(), fmt.Errorf("cannot add null item to response for non nullable list")
	}

	fd = msg.Descriptor().Fields().ByNumber(1)
	if !fd.IsList() {
		return arena.NewNull(), fmt.Errorf("field %q is not a list", fd.Name())
	}

	if level < md.NestingLevel-1 {
		list := msg.Get(fd).List()
		for i := 0; i < list.Len(); i++ {
			next := arena.NewArray()
			val, err := j.traverseList(level+1, arena, next, md, list.Get(i).Message(), message)
			if err != nil {
				return nil, err
			}

			current.SetArrayItem(i, val)
		}

		return current, nil
	}

	list := msg.Get(fd).List()
	if !list.IsValid() {
		// If the list is not valid, we return an empty array here as the
		// nullabilty is checked on the outer List wrapper type.
		return arena.NewArray(), nil
	}

	for i := 0; i < list.Len(); i++ {
		if message != nil {
			val, err := j.marshalResponseJSON(arena, message, list.Get(i).Message())
			if err != nil {
				return nil, err
			}

			current.SetArrayItem(i, val)
		} else {
			j.setArrayItem(i, arena, current, list.Get(i), fd)
		}
	}

	return current, nil
}

func (j *jsonBuilder) resolveOptionalField(arena *astjson.Arena, root *astjson.Value, name string, data protoref.Message) error {
	fd := data.Descriptor().Fields().ByName(protoref.Name("value"))
	if fd == nil {
		return fmt.Errorf("unable to resolve optional field: field %q not found in message %s", "value", data.Descriptor().Name())
	}

	j.setJSONValue(arena, root, name, data, fd)
	return nil
}

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
	case protoref.Int32Kind, protoref.Int64Kind:
		root.Set(name, arena.NewNumberInt(int(data.Get(fd).Int())))
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

		graphqlValue, ok := j.mapping.ResolveEnumValue(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			root.Set(name, arena.NewNull())
			return
		}

		root.Set(name, arena.NewString(graphqlValue))
	}
}

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
	case protoref.Int32Kind, protoref.Int64Kind:
		array.SetArrayItem(index, arena.NewNumberInt(int(data.Int())))
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

		graphqlValue, ok := j.mapping.ResolveEnumValue(string(enumDesc.Name()), string(enumValueDesc.Name()))
		if !ok {
			array.SetArrayItem(index, arena.NewNull())
			return
		}

		array.SetArrayItem(index, arena.NewString(graphqlValue))
	}
}

func (j *jsonBuilder) toDataObject(root *astjson.Value) *astjson.Value {
	a := astjson.Arena{}
	defer a.Reset()
	data := a.NewObject()
	data.Set(dataPath, root)
	return data
}

func (j *jsonBuilder) writeErrorBytes(err error) []byte {
	a := astjson.Arena{}
	defer a.Reset()
	errorRoot := a.NewObject()
	errorArray := a.NewArray()
	errorRoot.Set(errorsPath, errorArray)

	errorItem := a.NewObject()
	errorItem.Set("message", a.NewString(err.Error()))

	extensions := a.NewObject()
	if st, ok := status.FromError(err); ok {
		extensions.Set("code", a.NewString(st.Code().String()))
	} else {
		extensions.Set("code", a.NewString(codes.Internal.String()))
	}

	errorItem.Set("extensions", extensions)
	errorArray.SetArrayItem(0, errorItem)

	return errorRoot.MarshalTo(nil)
}
