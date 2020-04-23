//go:generate mockgen -self_package=github.com/jensneuse/go-data-resolver/pkg/resolve -destination=resolve_mock_test.go -package=resolve . DataSource

package resolve

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strconv"
	"sync"

	"github.com/buger/jsonparser"

	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
)

var (
	lBrace           = []byte("{")
	rBrace           = []byte("}")
	lBrack           = []byte("[")
	rBrack           = []byte("]")
	comma            = []byte(",")
	colon            = []byte(":")
	quote            = []byte("\"")
	null             = []byte("null")
	literalData      = []byte("data")
	literalErrors    = []byte("Errors")
	literalMessage   = []byte("message")
	literalLocations = []byte("locations")
	literalPath      = []byte("path")
)

var errNonNullableFieldValueIsNull = errors.New("non nullable field value is null")
var errTypeNameSkipped = errors.New("skipped because of __typename condition")

type Node interface {
	NodeKind() NodeKind
	Nullable() bool
}

type NodeKind int
type FetchKind int

const (
	NodeKindObject NodeKind = iota + 1
	NodeKindEmptyObject
	NodeKindArray
	NodeKindEmptyArray
	NodeKindNull
	NodeKindString
	NodeKindBoolean
	NodeKindInteger
	NodeKindFloat

	FetchKindSingle FetchKind = iota + 1
)

type Context struct {
	context.Context
	Variables []byte
}

type Fetch interface {
	FetchKind() FetchKind
}

type DataSource interface {
	Load(ctx context.Context, input []byte, bufPair *BufPair) (err error)
}

type Resolver struct {
	resultSetPool    sync.Pool
	byteSlicesPool   sync.Pool
	waitGroupPool    sync.Pool
	bufPairPool      sync.Pool
	bufPairSlicePool sync.Pool
	errChanPool      sync.Pool
}

func New() *Resolver {
	return &Resolver{
		resultSetPool: sync.Pool{
			New: func() interface{} {
				return &resultSet{
					buffers: make(map[uint8]*BufPair, 8),
				}
			},
		},
		byteSlicesPool: sync.Pool{
			New: func() interface{} {
				slice := make([][]byte, 0, 24)
				return &slice
			},
		},
		waitGroupPool: sync.Pool{
			New: func() interface{} {
				return &sync.WaitGroup{}
			},
		},
		bufPairPool: sync.Pool{
			New: func() interface{} {
				pair := BufPair{
					Data:   bytes.NewBuffer(make([]byte, 0, 1024)),
					Errors: bytes.NewBuffer(make([]byte, 0, 1024)),
				}
				return &pair
			},
		},
		bufPairSlicePool: sync.Pool{
			New: func() interface{} {
				slice := make([]*BufPair, 0, 24)
				return &slice
			},
		},
		errChanPool: sync.Pool{
			New: func() interface{} {
				return make(chan error, 1)
			},
		},
	}
}

func (r *Resolver) writeSafe(err error, writer io.Writer, data []byte) error {
	if err != nil {
		return err
	}
	_, err = writer.Write(data)
	return err
}

func (r *Resolver) writeErrSafe(err error, writer io.Writer, message, locations, path []byte) error {
	if err != nil {
		return err
	}
	_, err = writer.Write(lBrace)
	err = r.resolveObjectFieldSafe(err, writer, literalMessage, message)
	if locations != nil {
		_, err = writer.Write(comma)
		err = r.resolveObjectFieldSafe(err, writer, literalLocations, locations)
	}
	if locations != nil {
		_, err = writer.Write(comma)
		err = r.resolveObjectFieldSafe(err, writer, literalPath, locations)
	}
	_, err = writer.Write(rBrace)
	return err
}

func (r *Resolver) resolveObjectFieldSafe(err error, writer io.Writer, fieldName, fieldContent []byte) error {
	if err != nil {
		return err
	}
	_, err = writer.Write(quote)
	_, err = writer.Write(fieldName)
	_, err = writer.Write(quote)
	_, err = writer.Write(colon)
	_, err = writer.Write(fieldContent)
	return err
}

func (r *Resolver) resolveNode(ctx Context, node Node, data []byte, bufPair *BufPair) (err error) {
	switch n := node.(type) {
	case *Object:
		return r.resolveObject(ctx, n, data, bufPair)
	case *Array:
		return r.resolveArray(ctx, n, data, bufPair)
	case *Null:
		return r.resolveNull(bufPair.Data)
	case *String:
		return r.resolveString(n, data, bufPair)
	case *Boolean:
		return r.resolveBoolean(n, data, bufPair)
	case *Integer:
		return r.resolveInteger(n, data, bufPair)
	case *Float:
		return r.resolveFloat(n, data, bufPair)
	case *EmptyObject:
		return r.resolveEmptyObject(bufPair.Data)
	case *EmptyArray:
		return r.resolveEmptyArray(bufPair.Data)
	default:
		return
	}
}

func (r *Resolver) ResolveGraphQLResponse(ctx Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	err = r.resolveNode(ctx, response.Data, data, buf)
	if err != nil {
		return
	}

	hasErrors := buf.Errors.Len() != 0
	hasData := buf.Data.Len() != 0

	err = r.writeSafe(err, writer, lBrace)

	if hasErrors {
		err = r.writeSafe(err, writer, quote)
		err = r.writeSafe(err, writer, literalErrors)
		err = r.writeSafe(err, writer, quote)
		err = r.writeSafe(err, writer, colon)
		err = r.writeSafe(err, writer, lBrack)
		_, err = buf.Errors.WriteTo(writer)
		err = r.writeSafe(err, writer, rBrack)
	}

	if hasData {
		if hasErrors {
			err = r.writeSafe(err, writer, comma)
		}
		err = r.writeSafe(err, writer, quote)
		err = r.writeSafe(err, writer, literalData)
		err = r.writeSafe(err, writer, quote)
		err = r.writeSafe(err, writer, colon)
		_, err = buf.Data.WriteTo(writer)
	}

	err = r.writeSafe(err, writer, rBrace)

	return
}

func (r *Resolver) resolveEmptyArray(writer io.Writer) (err error) {
	err = r.writeSafe(nil, writer, lBrack)
	return r.writeSafe(nil, writer, rBrack)
}

func (r *Resolver) resolveEmptyObject(writer io.Writer) (err error) {
	err = r.writeSafe(err, writer, lBrace)
	return r.writeSafe(err, writer, rBrace)
}

func (r *Resolver) resolveArray(ctx Context, array *Array, data []byte, arrayBuf *BufPair) (err error) {

	arrayItems := r.byteSlicesPool.Get().(*[][]byte)
	defer func() {
		*arrayItems = (*arrayItems)[:0]
		r.byteSlicesPool.Put(arrayItems)
	}()

	_, err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		*arrayItems = append(*arrayItems, value)
	}, array.Path...)

	if len(*arrayItems) == 0 {
		if !array.nullable {
			return errNonNullableFieldValueIsNull
		}
		return r.resolveNull(arrayBuf.Data)
	}

	if array.ResolveAsynchronous {
		return r.resolveArrayAsynchronous(ctx, array, arrayItems, arrayBuf)
	}
	return r.resolveArraySynchronous(ctx, array, arrayItems, arrayBuf)
}

func (r *Resolver) resolveArraySynchronous(ctx Context, array *Array, arrayItems *[][]byte, arrayBuf *BufPair) (err error) {

	itemBuf := r.getBufPair()
	defer r.freeBufPair(itemBuf)

	err = r.writeSafe(err, arrayBuf.Data, lBrack)
	var (
		hasPreviousItem bool
		dataWritten     int
	)
	for i := range *arrayItems {
		err = r.resolveNode(ctx, array.Item, (*arrayItems)[i], itemBuf)
		if err != nil {
			if errors.Is(err, errNonNullableFieldValueIsNull) && array.nullable {
				arrayBuf.Data.Reset()
				return r.resolveNull(arrayBuf.Data)
			}
			if errors.Is(err, errTypeNameSkipped) {
				err = nil
				continue
			}
			return
		}
		dataWritten, _, err = r.MergeBufPairs(itemBuf, arrayBuf, hasPreviousItem)
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
		}
	}

	return r.writeSafe(err, arrayBuf.Data, rBrack)
}

func (r *Resolver) resolveArrayAsynchronous(ctx Context, array *Array, arrayItems *[][]byte, arrayBuf *BufPair) (err error) {

	err = r.writeSafe(err, arrayBuf.Data, lBrack)

	bufSlice := r.getBufPairSlice()
	defer r.freeBufPairSlice(bufSlice)

	wg := r.waitGroupPool.Get().(*sync.WaitGroup)
	defer r.waitGroupPool.Put(wg)

	errCh := r.getErrChan()
	defer r.freeErrChan(errCh)

	wg.Add(len(*arrayItems))

	for i := range *arrayItems {
		itemBuf := r.getBufPair()
		*bufSlice = append(*bufSlice, itemBuf)
		itemData := (*arrayItems)[i]
		go func() {
			if e := r.resolveNode(ctx, array.Item, itemData, itemBuf); e != nil && !errors.Is(e, errTypeNameSkipped) {
				select {
				case errCh <- e:
				default:
				}
			}
			wg.Done()
		}()
	}

	wg.Wait()

	select {
	case err = <-errCh:
	default:
	}

	if err != nil {
		if errors.Is(err, errNonNullableFieldValueIsNull) && array.nullable {
			arrayBuf.Data.Reset()
			return r.resolveNull(arrayBuf.Data)
		}
		return
	}

	var (
		hasPreviousItem bool
		dataWritten     int
	)
	for i := range *bufSlice {
		dataWritten, _, err = r.MergeBufPairs((*bufSlice)[i], arrayBuf, hasPreviousItem)
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
		}
	}

	return r.writeSafe(err, arrayBuf.Data, rBrack)
}

func (r *Resolver) resolveInteger(integer *Integer, data []byte, integerBuf *BufPair) (err error) {
	value, dataType, _, err := jsonparser.Get(data, integer.Path...)
	if err != nil || dataType != jsonparser.Number {
		if !integer.nullable {
			return errNonNullableFieldValueIsNull
		}
		return r.resolveNull(integerBuf.Data)
	}
	return r.writeSafe(nil, integerBuf.Data, value)
}

func (r *Resolver) resolveFloat(floatValue *Float, data []byte, integerBuf *BufPair) (err error) {
	value, dataType, _, err := jsonparser.Get(data, floatValue.Path...)
	if err != nil || dataType != jsonparser.Number {
		if !floatValue.nullable {
			return errNonNullableFieldValueIsNull
		}
		return r.resolveNull(integerBuf.Data)
	}
	return r.writeSafe(nil, integerBuf.Data, value)
}

func (r *Resolver) resolveBoolean(boolean *Boolean, data []byte, booleanBuf *BufPair) (err error) {
	value, valueType, _, err := jsonparser.Get(data, boolean.Path...)
	if err != nil || valueType != jsonparser.Boolean {
		if !boolean.nullable {
			return errNonNullableFieldValueIsNull
		}
		return r.resolveNull(booleanBuf.Data)
	}
	return r.writeSafe(nil, booleanBuf.Data, value)
}

func (r *Resolver) resolveString(str *String, data []byte, stringBuf *BufPair) (err error) {
	value, valueType, _, err := jsonparser.Get(data, str.Path...)
	if err != nil || valueType != jsonparser.String {
		if !str.nullable {
			return errNonNullableFieldValueIsNull
		}
		return r.resolveNull(stringBuf.Data)
	}
	err = r.writeSafe(nil, stringBuf.Data, quote)
	err = r.writeSafe(nil, stringBuf.Data, value)
	return r.writeSafe(nil, stringBuf.Data, quote)
}

func (r *Resolver) resolveNull(writer io.Writer) (err error) {
	return r.writeSafe(nil, writer, null)
}

func (r *Resolver) resolveObject(ctx Context, object *Object, data []byte, objectBuf *BufPair) (err error) {

	if len(object.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, object.Path...)
	}

	var set *resultSet
	if object.Fetch != nil {
		set = r.resultSetPool.Get().(*resultSet)
		defer r.freeResultSet(set)
		err = r.resolveFetch(ctx, object.Fetch, data, set)
		if err != nil {
			return
		}
		for i := range set.buffers {
			_, err = r.MergeBufPairErrors(set.buffers[i], objectBuf)
		}
	}

	fieldBuf := r.getBufPair()
	defer r.freeBufPair(fieldBuf)

	typeNameSkip := false
	first := true
	for i := range object.FieldSets {
		var fieldSetData []byte
		if set != nil && object.FieldSets[i].HasBuffer {
			buffer, ok := set.buffers[object.FieldSets[i].BufferId]
			if ok {
				fieldSetData = buffer.Data.Bytes()
			}
		} else {
			fieldSetData = data
		}

		if object.FieldSets[i].OnTypeName != nil {
			typeName, _, _, _ := jsonparser.Get(fieldSetData, "__typename")
			if !bytes.Equal(typeName, object.FieldSets[i].OnTypeName) {
				typeNameSkip = true
				continue
			}
		}

		for j := range object.FieldSets[i].Fields {
			if first {
				err = r.writeSafe(err, objectBuf.Data, lBrace)
				first = false
			} else {
				err = r.writeSafe(err, objectBuf.Data, comma)
			}
			err = r.writeSafe(err, objectBuf.Data, quote)
			err = r.writeSafe(err, objectBuf.Data, object.FieldSets[i].Fields[j].Name)
			err = r.writeSafe(err, objectBuf.Data, quote)
			err = r.writeSafe(err, objectBuf.Data, colon)
			if err != nil {
				return
			}
			err = r.resolveNode(ctx, object.FieldSets[i].Fields[j].Value, fieldSetData, fieldBuf)
			if err != nil {
				if errors.Is(err, errNonNullableFieldValueIsNull) && object.nullable {
					objectBuf.Data.Reset()
					return r.writeSafe(nil, objectBuf.Data, null)
				}
				return
			}
			_, _, err = r.MergeBufPairs(fieldBuf, objectBuf, false)
		}
	}
	if first {
		if !object.nullable {
			if typeNameSkip {
				return errTypeNameSkipped
			}
			return errNonNullableFieldValueIsNull
		}
		return r.resolveNull(objectBuf.Data)
	}
	return r.writeSafe(err, objectBuf.Data, rBrace)
}

func (r *Resolver) freeResultSet(set *resultSet) {
	for i := range set.buffers {
		set.buffers[i].Reset()
		r.bufPairPool.Put(set.buffers[i])
		delete(set.buffers, i)
	}
	r.resultSetPool.Put(set)
}

func (r *Resolver) resolveFetch(ctx Context, fetch Fetch, data []byte, set *resultSet) (err error) {
	switch f := fetch.(type) {
	case *SingleFetch:
		err = r.resolveSingleFetch(ctx, f, data, set)
	}
	return
}

func (r *Resolver) resolveSingleFetch(ctx Context, fetch *SingleFetch, data []byte, set *resultSet) (err error) {

	if len(fetch.Variables.variables) != 0 {
		fetch.Input = r.resolveVariables(ctx, fetch.Variables.variables, data, fetch.Input)
	}

	buf := r.getBufPair()
	set.buffers[fetch.BufferId] = buf
	return fetch.DataSource.Load(ctx.Context, fetch.Input, buf)
}

func (r *Resolver) resolveVariables(ctx Context, variables []Variable, data, input []byte) []byte {
	for i := range variables {
		variableName := []byte("$$" + strconv.Itoa(i) + "$$")
		switch v := variables[i].(type) {
		case *ContextVariable:
			value := r.resolveContextVariable(ctx, v)
			input = bytes.ReplaceAll(input, variableName, value)
		case *ObjectVariable:
			value := r.resolveObjectVariable(data, v)
			input = bytes.ReplaceAll(input, variableName, value)
		}
	}
	return input
}

func (r *Resolver) resolveObjectVariable(data []byte, variable *ObjectVariable) []byte {
	value, _, _, err := jsonparser.Get(data, variable.Path...)
	if err != nil {
		return null
	}
	return value
}

func (r *Resolver) resolveContextVariable(ctx Context, variable *ContextVariable) []byte {
	value, _, _, err := jsonparser.Get(ctx.Variables, variable.Path...)
	if err != nil {
		return null
	}
	return value
}

type Object struct {
	nullable  bool
	Path      []string
	FieldSets []FieldSet
	Fetch     Fetch
}

func (o *Object) Nullable() bool {
	return o.nullable
}

func (_ *Object) NodeKind() NodeKind {
	return NodeKindObject
}

type EmptyObject struct{}

func (_ *EmptyObject) Nullable() bool {
	return false
}

func (_ *EmptyObject) NodeKind() NodeKind {
	return NodeKindEmptyObject
}

type EmptyArray struct{}

func (_ *EmptyArray) Nullable() bool {
	return false
}

func (_ *EmptyArray) NodeKind() NodeKind {
	return NodeKindEmptyArray
}

type FieldSet struct {
	OnTypeName []byte
	BufferId   uint8
	HasBuffer  bool
	Fields     []Field
}

type Field struct {
	Name  []byte
	Value Node
}

type Null struct {
}

func (_ *Null) Nullable() bool {
	return true
}

func (_ *Null) NodeKind() NodeKind {
	return NodeKindNull
}

type resultSet struct {
	buffers map[uint8]*BufPair
}

type SingleFetch struct {
	BufferId   uint8
	Input      []byte
	DataSource DataSource
	Variables  Variables
}

func (_ *SingleFetch) FetchKind() FetchKind {
	return FetchKindSingle
}

type String struct {
	Path     []string
	nullable bool
}

func (s *String) Nullable() bool {
	return s.nullable
}

func (_ *String) NodeKind() NodeKind {
	return NodeKindString
}

type Boolean struct {
	Path     []string
	nullable bool
}

func (b *Boolean) Nullable() bool {
	return b.nullable
}

func (_ *Boolean) NodeKind() NodeKind {
	return NodeKindBoolean
}

type Float struct {
	Path     []string
	nullable bool
}

func (_ *Float) NodeKind() NodeKind {
	return NodeKindFloat
}

func (f *Float) Nullable() bool {
	return f.nullable
}

type Integer struct {
	Path     []string
	nullable bool
}

func (i *Integer) Nullable() bool {
	return i.nullable
}

func (_ *Integer) NodeKind() NodeKind {
	return NodeKindInteger
}

type Array struct {
	Path                []string
	nullable            bool
	ResolveAsynchronous bool
	Item                Node
}

func (a *Array) Nullable() bool {
	return a.nullable
}

func (_ *Array) NodeKind() NodeKind {
	return NodeKindArray
}

type Variable interface {
	VariableKind() VariableKind
}

type Variables struct {
	variables []Variable
}

func NewVariables(variables ...Variable) Variables {
	var out Variables
	for i := range variables {
		out.AddVariable(variables[i])
	}
	return out
}

var (
	variablePrefixSuffix = []byte("$$")
)

func (v *Variables) AddVariable(variable Variable) (name []byte) {
	v.variables = append(v.variables, variable)
	index := unsafebytes.StringToBytes(strconv.Itoa(len(v.variables) - 1))
	return append(variablePrefixSuffix, append(index, variablePrefixSuffix...)...)
}

type VariableKind int

const (
	VariableKindContext VariableKind = iota + 1
	VariableKindObject
)

type ContextVariable struct {
	Path []string
}

func (_ *ContextVariable) VariableKind() VariableKind {
	return VariableKindContext
}

type ObjectVariable struct {
	Path []string
}

func (_ *ObjectVariable) VariableKind() VariableKind {
	return VariableKindObject
}

type GraphQLResponse struct {
	Data Node
}

type BufPair struct {
	Data   *bytes.Buffer
	Errors *bytes.Buffer
}

func (b *BufPair) HasData() bool {
	return b.Data.Len() != 0
}

func (b *BufPair) HasErrors() bool {
	return b.Errors.Len() != 0
}

func (b *BufPair) Reset() {
	b.Data.Reset()
	b.Errors.Reset()
}

func (b *BufPair) writeErrors(err error, data []byte) error {
	if err != nil {
		return err
	}
	_, err = b.Errors.Write(data)
	return err
}

func (b *BufPair) WriteErr(message, locations, path []byte) (err error) {
	if b.HasErrors() {
		err = b.writeErrors(err, comma)
	}
	err = b.writeErrors(err, lBrace)
	err = b.writeErrors(err, quote)
	err = b.writeErrors(err, literalMessage)
	err = b.writeErrors(err, quote)
	err = b.writeErrors(err, colon)
	err = b.writeErrors(err, quote)
	err = b.writeErrors(err, message)
	err = b.writeErrors(err, quote)
	if locations != nil {
		err = b.writeErrors(err, comma)
		err = b.writeErrors(err, quote)
		err = b.writeErrors(err, literalLocations)
		err = b.writeErrors(err, quote)
		err = b.writeErrors(err, colon)
		err = b.writeErrors(err, locations)
	}
	if path != nil {
		err = b.writeErrors(err, comma)
		err = b.writeErrors(err, quote)
		err = b.writeErrors(err, literalPath)
		err = b.writeErrors(err, quote)
		err = b.writeErrors(err, colon)
		err = b.writeErrors(err, path)
	}
	err = b.writeErrors(err, rBrace)
	return
}

func (r *Resolver) MergeBufPairs(from, to *BufPair, prefixDataWithComma bool) (dataWritten, errorsWritten int, err error) {
	dataWritten, err = r.MergeBufPairData(from, to, prefixDataWithComma)
	if err != nil {
		return
	}
	errorsWritten, err = r.MergeBufPairErrors(from, to)
	return
}

func (r *Resolver) MergeBufPairData(from, to *BufPair, prefixDataWithComma bool) (dataWritten int, err error) {
	if !from.HasData() {
		return
	}
	var written int64
	if prefixDataWithComma {
		dataWritten, err = to.Data.Write(comma)
	}
	written, err = from.Data.WriteTo(to.Data)
	dataWritten += int(written)
	return
}

func (r *Resolver) MergeBufPairErrors(from, to *BufPair) (errorsWritten int, err error) {
	if !from.HasErrors() {
		return
	}
	var written int64
	if to.HasErrors() {
		errorsWritten, err = to.Errors.Write(comma)
	}
	written, err = from.Errors.WriteTo(to.Errors)
	errorsWritten += int(written)
	return
}

func (r *Resolver) freeBufPair(pair *BufPair) {
	pair.Data.Reset()
	pair.Errors.Reset()
	r.bufPairPool.Put(pair)
}

func (r *Resolver) getBufPair() *BufPair {
	return r.bufPairPool.Get().(*BufPair)
}

func (r *Resolver) getBufPairSlice() *[]*BufPair {
	return r.bufPairSlicePool.Get().(*[]*BufPair)
}

func (r *Resolver) freeBufPairSlice(slice *[]*BufPair) {
	for i := range *slice {
		r.freeBufPair((*slice)[i])
	}
	*slice = (*slice)[:0]
	r.bufPairSlicePool.Put(slice)
}

func (r *Resolver) getErrChan() chan error {
	return r.errChanPool.Get().(chan error)
}

func (r *Resolver) freeErrChan(ch chan error) {
	r.errChanPool.Put(ch)
}
