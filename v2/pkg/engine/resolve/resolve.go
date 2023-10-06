//go:generate mockgen --build_flags=--mod=mod -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource,BeforeFetchHook,AfterFetchHook

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/pkg/errors"
	"github.com/tidwall/gjson"
	"golang.org/x/sync/singleflight"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type Resolver struct {
	ctx              context.Context
	byteSlicesPool   sync.Pool
	waitGroupPool    sync.Pool
	bufPairPool      sync.Pool
	bufPairSlicePool sync.Pool
	errChanPool      sync.Pool
	hash64Pool       sync.Pool
	loaders          sync.Pool

	enableSingleFlightLoader bool
	sf                       *singleflight.Group
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context, enableSingleFlightLoader bool) *Resolver {
	return &Resolver{
		ctx: ctx,
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
					Data:   fastbuffer.New(),
					Errors: fastbuffer.New(),
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
		hash64Pool: sync.Pool{
			New: func() interface{} {
				return xxhash.New()
			},
		},
		loaders: sync.Pool{
			New: func() interface{} {
				return &Loader{}
			},
		},
		enableSingleFlightLoader: enableSingleFlightLoader,
		sf:                       &singleflight.Group{},
	}
}

func (r *Resolver) resolveNode(ctx *Context, node Node, data []byte, bufPair *BufPair) (err error) {
	switch n := node.(type) {
	case *Object:
		return r.resolveObject(ctx, n, data, bufPair)
	case *Array:
		return r.resolveArray(ctx, n, data, bufPair)
	case *Null:
		r.resolveNull(bufPair.Data)
		return
	case *String:
		return r.resolveString(ctx, n, data, bufPair)
	case *Boolean:
		return r.resolveBoolean(ctx, n, data, bufPair)
	case *Integer:
		return r.resolveInteger(ctx, n, data, bufPair)
	case *Float:
		return r.resolveFloat(ctx, n, data, bufPair)
	case *BigInt:
		return r.resolveBigInt(ctx, n, data, bufPair)
	case *Scalar:
		return r.resolveScalar(ctx, n, data, bufPair)
	case *EmptyObject:
		r.resolveEmptyObject(bufPair.Data)
		return
	case *EmptyArray:
		r.resolveEmptyArray(bufPair.Data)
		return
	case *CustomNode:
		return r.resolveCustom(ctx, n, data, bufPair)
	default:
		return
	}
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {

	dataBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(dataBuf)

	loader := r.loaders.Get().(*Loader)
	defer func() {
		loader.Free()
		r.loaders.Put(loader)
	}()
	loader.sf = r.sf
	loader.sfEnabled = r.enableSingleFlightLoader

	hasErrors, err := loader.LoadGraphQLResponseData(ctx, response, data, dataBuf)
	if err != nil {
		return err
	}

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	if hasErrors {
		_, err = writer.Write(dataBuf.Bytes())
		return
	}

	ignoreData := false
	err = r.resolveNode(ctx, response.Data, dataBuf.Bytes(), buf)
	if err != nil {
		if !errors.Is(err, errNonNullableFieldValueIsNull) {
			return
		}
		ignoreData = true
	}

	return writeGraphqlResponse(buf, writer, ignoreData)
}

func (r *Resolver) resolveGraphQLSubscriptionResponse(ctx *Context, response *GraphQLResponse, subscriptionData *BufPair, writer io.Writer) (err error) {

	dataBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(dataBuf)

	loader := r.loaders.Get().(*Loader)
	defer func() {
		loader.Free()
		r.loaders.Put(loader)
	}()
	loader.sf = r.sf
	loader.sfEnabled = r.enableSingleFlightLoader

	hasErrors, err := loader.LoadGraphQLResponseData(ctx, response, subscriptionData.Data.Bytes(), dataBuf)
	if err != nil {
		return err
	}

	if hasErrors {
		_, err = writer.Write(dataBuf.Bytes())
		return
	}

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	ignoreData := false
	err = r.resolveNode(ctx, response.Data, subscriptionData.Data.Bytes(), buf)
	if err != nil {
		if !errors.Is(err, errNonNullableFieldValueIsNull) {
			return
		}
		ignoreData = true
	}
	if subscriptionData.HasErrors() {
		r.MergeBufPairErrors(subscriptionData, buf)
	}

	return writeGraphqlResponse(buf, writer, ignoreData)
}

func (r *Resolver) ResolveGraphQLSubscription(ctx *Context, subscription *GraphQLSubscription, writer FlushWriter) (err error) {

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	err = subscription.Trigger.InputTemplate.Render(ctx, nil, buf)
	if err != nil {
		return
	}
	rendered := buf.Bytes()
	subscriptionInput := make([]byte, len(rendered))
	copy(subscriptionInput, rendered)

	c, cancel := context.WithCancel(ctx.Context())
	defer cancel()
	resolverDone := r.ctx.Done()

	next := make(chan []byte)
	if subscription.Trigger.Source == nil {
		msg := []byte(`{"errors":[{"message":"no data source found"}]}`)
		return writeAndFlush(writer, msg)
	}

	err = subscription.Trigger.Source.Start(c, subscriptionInput, next)
	if err != nil {
		if errors.Is(err, ErrUnableToResolve) {
			msg := []byte(`{"errors":[{"message":"unable to resolve"}]}`)
			return writeAndFlush(writer, msg)
		}
		return err
	}

	responseBuf := r.getBufPair()
	defer r.freeBufPair(responseBuf)

	for {
		select {
		case <-resolverDone:
			return nil
		default:
			data, ok := <-next
			if !ok {
				return nil
			}
			responseBuf.Reset()
			extractResponse(data, responseBuf, subscription.Trigger.PostProcessing)
			err = r.resolveGraphQLSubscriptionResponse(ctx, subscription.Response, responseBuf, writer)
			if err != nil {
				return err
			}
			writer.Flush()
		}
	}
}

func (r *Resolver) resolveEmptyArray(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrack)
	b.WriteBytes(rBrack)
}

func (r *Resolver) resolveEmptyObject(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrace)
	b.WriteBytes(rBrace)
}

func (r *Resolver) resolveArray(ctx *Context, array *Array, data []byte, arrayBuf *BufPair) (parentErr error) {
	if len(array.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, array.Path...)
	}

	if bytes.Equal(data, emptyArray) {
		r.resolveEmptyArray(arrayBuf.Data)
		return
	}

	itemBuf := r.getBufPair()
	defer r.freeBufPair(itemBuf)

	reset := arrayBuf.Data.Len()
	i := 0
	hasData := false
	resolveArrayAsNull := false

	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if parentErr != nil {
			return
		}
		if err == nil && dataType == jsonparser.String {
			value = data[offset-2 : offset+len(value)] // add quotes to string values
		}
		itemBuf.Reset()
		ctx.addIntegerPathElement(i)
		err = r.resolveNode(ctx, array.Item, value, itemBuf)
		ctx.removeLastPathElement()
		if err != nil {
			if errors.Is(err, errNonNullableFieldValueIsNull) && array.Nullable {
				resolveArrayAsNull = true
				return
			}
			parentErr = err
			return
		}
		if !hasData {
			arrayBuf.Data.WriteBytes(lBrack)
		}
		r.MergeBufPairs(itemBuf, arrayBuf, hasData)
		hasData = true
		i++
	})
	if err != nil {
		if array.Nullable {
			arrayBuf.Data.Reslice(0, reset)
			r.resolveNull(arrayBuf.Data)
			return nil
		}
		return errNonNullableFieldValueIsNull
	}
	if resolveArrayAsNull {
		arrayBuf.Data.Reslice(0, reset)
		r.resolveNull(arrayBuf.Data)
		return nil
	}
	if hasData {
		arrayBuf.Data.WriteBytes(rBrack)
	}
	return
}

func (r *Resolver) resolveArraySynchronous(ctx *Context, array *Array, arrayItems *[][]byte, arrayBuf *BufPair) (err error) {
	arrayBuf.Data.WriteBytes(lBrack)
	start := arrayBuf.Data.Len()

	itemBuf := r.getBufPair()
	defer r.freeBufPair(itemBuf)

	for i := range *arrayItems {
		ctx.addIntegerPathElement(i)
		if arrayBuf.Data.Len() > start {
			arrayBuf.Data.WriteBytes(comma)
		}
		err = r.resolveNode(ctx, array.Item, (*arrayItems)[i], arrayBuf)
		ctx.removeLastPathElement()
		if err != nil {
			if errors.Is(err, errNonNullableFieldValueIsNull) && array.Nullable {
				arrayBuf.Data.Reset()
				r.resolveNull(arrayBuf.Data)
				return nil
			}
			if errors.Is(err, errTypeNameSkipped) {
				err = nil
				continue
			}
			return
		}
	}

	arrayBuf.Data.WriteBytes(rBrack)
	return
}

func (r *Resolver) exportField(ctx *Context, export *FieldExport, value []byte) {
	if export == nil {
		return
	}
	if export.AsString {
		value = append(quote, append(value, quote...)...)
	}
	ctx.Variables, _ = jsonparser.Set(ctx.Variables, value, export.Path...)
}

func (r *Resolver) resolveInteger(ctx *Context, integer *Integer, data []byte, integerBuf *BufPair) error {
	value, dataType, _, err := jsonparser.Get(data, integer.Path...)
	if err != nil || dataType != jsonparser.Number {
		if !integer.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(integerBuf.Data)
		return nil
	}
	integerBuf.Data.WriteBytes(value)
	r.exportField(ctx, integer.Export, value)
	return nil
}

func (r *Resolver) resolveFloat(ctx *Context, floatValue *Float, data []byte, floatBuf *BufPair) error {
	value, dataType, _, err := jsonparser.Get(data, floatValue.Path...)
	if err != nil || dataType != jsonparser.Number {
		if !floatValue.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(floatBuf.Data)
		return nil
	}
	floatBuf.Data.WriteBytes(value)
	r.exportField(ctx, floatValue.Export, value)
	return nil
}

func (r *Resolver) resolveBigInt(ctx *Context, bigIntValue *BigInt, data []byte, bigIntBuf *BufPair) error {
	value, valueType, _, err := jsonparser.Get(data, bigIntValue.Path...)
	switch {
	case err != nil, valueType == jsonparser.Null:
		if !bigIntValue.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(bigIntBuf.Data)
		return nil
	case valueType == jsonparser.Number:
		bigIntBuf.Data.WriteBytes(value)
	case valueType == jsonparser.String:
		bigIntBuf.Data.WriteBytes(quote)
		bigIntBuf.Data.WriteBytes(value)
		bigIntBuf.Data.WriteBytes(quote)
	default:
		return fmt.Errorf("invalid value type '%s' for path %s, expecting number or string, got: %v", valueType, string(ctx.path()), string(value))

	}
	r.exportField(ctx, bigIntValue.Export, value)
	return nil
}

func (r *Resolver) resolveScalar(ctx *Context, scalarValue *Scalar, data []byte, scalarBuf *BufPair) error {
	value, valueType, _, err := jsonparser.Get(data, scalarValue.Path...)
	switch {
	case err != nil, valueType == jsonparser.Null:
		if !scalarValue.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(scalarBuf.Data)
		return nil
	case valueType == jsonparser.String:
		scalarBuf.Data.WriteBytes(quote)
		scalarBuf.Data.WriteBytes(value)
		scalarBuf.Data.WriteBytes(quote)
	default:
		scalarBuf.Data.WriteBytes(value)
	}
	r.exportField(ctx, scalarValue.Export, value)
	return nil
}

func (r *Resolver) resolveCustom(ctx *Context, customValue *CustomNode, data []byte, customBuf *BufPair) error {
	value, dataType, _, _ := jsonparser.Get(data, customValue.Path...)
	if dataType == jsonparser.Null && !customValue.Nullable {
		return errNonNullableFieldValueIsNull
	}
	resolvedValue, err := customValue.Resolve(value)
	if err != nil {
		return fmt.Errorf("failed to resolve value type %s for path %s via custom resolver", dataType, string(ctx.path()))
	}
	customBuf.Data.WriteBytes(resolvedValue)
	return nil
}

func (r *Resolver) resolveBoolean(ctx *Context, boolean *Boolean, data []byte, booleanBuf *BufPair) error {
	value, valueType, _, err := jsonparser.Get(data, boolean.Path...)
	if err != nil || valueType != jsonparser.Boolean {
		if !boolean.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(booleanBuf.Data)
		return nil
	}
	booleanBuf.Data.WriteBytes(value)
	r.exportField(ctx, boolean.Export, value)
	return nil
}

func (r *Resolver) resolveString(ctx *Context, str *String, data []byte, stringBuf *BufPair) error {
	var (
		value     []byte
		valueType jsonparser.ValueType
		err       error
	)

	value, valueType, _, err = jsonparser.Get(data, str.Path...)
	if err != nil || valueType != jsonparser.String {
		if err == nil && str.UnescapeResponseJson {
			switch valueType {
			case jsonparser.Object, jsonparser.Array, jsonparser.Boolean, jsonparser.Number, jsonparser.Null:
				stringBuf.Data.WriteBytes(value)
				return nil
			}
		}
		if value != nil && valueType != jsonparser.Null {
			return fmt.Errorf("invalid value type '%s' for path %s, expecting string, got: %v. You can fix this by configuring this field as Int/Float/JSON Scalar", valueType, string(ctx.path()), string(value))
		}
		if !str.Nullable {
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(stringBuf.Data)
		return nil
	}

	if value == nil && !str.Nullable {
		return errNonNullableFieldValueIsNull
	}

	if str.UnescapeResponseJson {
		value = bytes.ReplaceAll(value, []byte(`\"`), []byte(`"`))

		// Do not modify values which was strings
		// When the original value from upstream response was a plain string value `"hello"`, `"true"`, `"1"`, `"2.0"`,
		// after getting it via jsonparser.Get we will get unquoted values `hello`, `true`, `1`, `2.0`
		// which is not string anymore, so we need to quote it again
		if !(bytes.ContainsAny(value, `{}[]`) && gjson.ValidBytes(value)) {
			// wrap value in quotes to make it valid json
			value = append(quote, append(value, quote...)...)
		}

		stringBuf.Data.WriteBytes(value)
		r.exportField(ctx, str.Export, value)
		return nil
	}

	value = r.renameTypeName(ctx, str, value)

	stringBuf.Data.WriteBytes(quote)
	stringBuf.Data.WriteBytes(value)
	stringBuf.Data.WriteBytes(quote)
	r.exportField(ctx, str.Export, value)
	return nil
}

func (r *Resolver) renameTypeName(ctx *Context, str *String, typeName []byte) []byte {
	if !str.IsTypeName {
		return typeName
	}
	for i := range ctx.RenameTypeNames {
		if bytes.Equal(ctx.RenameTypeNames[i].From, typeName) {
			return ctx.RenameTypeNames[i].To
		}
	}
	return typeName
}

func (r *Resolver) resolveNull(b *fastbuffer.FastBuffer) {
	b.WriteBytes(null)
}

func (r *Resolver) addResolveError(ctx *Context, objectBuf *BufPair) {
	locations, path := pool.BytesBuffer.Get(), pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(locations)
	defer pool.BytesBuffer.Put(path)

	var pathBytes []byte

	locations.Write(lBrack)
	locations.Write(lBrace)
	locations.Write(quote)
	locations.Write(literalLine)
	locations.Write(quote)
	locations.Write(colon)
	locations.Write([]byte(strconv.Itoa(int(ctx.position.Line))))
	locations.Write(comma)
	locations.Write(quote)
	locations.Write(literalColumn)
	locations.Write(quote)
	locations.Write(colon)
	locations.Write([]byte(strconv.Itoa(int(ctx.position.Column))))
	locations.Write(rBrace)
	locations.Write(rBrack)

	if len(ctx.pathElements) > 0 {
		path.Write(lBrack)
		path.Write(quote)
		path.Write(bytes.Join(ctx.pathElements, quotedComma))
		path.Write(quote)
		path.Write(rBrack)

		pathBytes = path.Bytes()
	}

	objectBuf.WriteErr(unableToResolveMsg, locations.Bytes(), pathBytes, nil)
}

func (r *Resolver) resolveObject(ctx *Context, object *Object, data []byte, parentBuf *BufPair) (err error) {
	if len(object.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, object.Path...)
		if len(data) == 0 || bytes.Equal(data, null) {
			if object.Nullable {
				r.resolveNull(parentBuf.Data)
				return
			}

			r.addResolveError(ctx, parentBuf)
			return errNonNullableFieldValueIsNull
		}
	}

	if object.UnescapeResponseJson {
		data = bytes.ReplaceAll(data, []byte(`\"`), []byte(`"`))
	}

	fieldBuf := r.getBufPair()
	defer r.freeBufPair(fieldBuf)

	typeNameSkip := false
	first := true
	skipCount := 0
	for i := range object.Fields {
		if object.Fields[i].SkipDirectiveDefined {
			skip, err := jsonparser.GetBoolean(ctx.Variables, object.Fields[i].SkipVariableName)
			if err == nil && skip {
				skipCount++
				continue
			}
		}

		if object.Fields[i].IncludeDirectiveDefined {
			include, err := jsonparser.GetBoolean(ctx.Variables, object.Fields[i].IncludeVariableName)
			if err != nil || !include {
				skipCount++
				continue
			}
		}

		if object.Fields[i].OnTypeNames != nil {
			typeName, _, _, _ := jsonparser.Get(data, "__typename")
			hasMatch := false
			for _, onTypeName := range object.Fields[i].OnTypeNames {
				if bytes.Equal(typeName, onTypeName) {
					hasMatch = true
					break
				}
			}
			if !hasMatch {
				typeNameSkip = true
				continue
			}
		}

		if first {
			fieldBuf.Data.WriteBytes(lBrace)
			first = false
		} else {
			fieldBuf.Data.WriteBytes(comma)
		}
		fieldBuf.Data.WriteBytes(quote)
		fieldBuf.Data.WriteBytes(object.Fields[i].Name)
		fieldBuf.Data.WriteBytes(quote)
		fieldBuf.Data.WriteBytes(colon)
		ctx.addPathElement(object.Fields[i].Name)
		ctx.setPosition(object.Fields[i].Position)
		err = r.resolveNode(ctx, object.Fields[i].Value, data, fieldBuf)
		ctx.removeLastPathElement()
		if err != nil {
			if errors.Is(err, errTypeNameSkipped) {
				fieldBuf.Data.Reset()
				r.resolveEmptyObject(parentBuf.Data)
				return nil
			}
			if errors.Is(err, errNonNullableFieldValueIsNull) {
				fieldBuf.Data.Reset()
				r.MergeBufPairErrors(fieldBuf, parentBuf)

				if object.Nullable {
					r.resolveNull(parentBuf.Data)
					return nil
				}

				// if field is of object type than we should not add resolve error here
				if _, ok := object.Fields[i].Value.(*Object); !ok {
					r.addResolveError(ctx, parentBuf)
				}
			}

			return
		}
		r.MergeBufPairs(fieldBuf, parentBuf, false)
	}
	allSkipped := len(object.Fields) != 0 && len(object.Fields) == skipCount
	if allSkipped {
		// return empty object if all fields have been skipped
		r.resolveEmptyObject(parentBuf.Data)
		return
	}
	if first {
		if typeNameSkip {
			r.resolveEmptyObject(parentBuf.Data)
			return
		}
		if !object.Nullable {
			r.addResolveError(ctx, parentBuf)
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(parentBuf.Data)
		return
	}
	parentBuf.Data.WriteBytes(rBrace)
	return
}

func (r *Resolver) MergeBufPairs(from, to *BufPair, prefixDataWithComma bool) {
	r.MergeBufPairData(from, to, prefixDataWithComma)
	r.MergeBufPairErrors(from, to)
}

func (r *Resolver) MergeBufPairData(from, to *BufPair, prefixDataWithComma bool) {
	if !from.HasData() {
		return
	}
	if prefixDataWithComma {
		to.Data.WriteBytes(comma)
	}
	to.Data.WriteBytes(from.Data.Bytes())
	from.Data.Reset()
}

func (r *Resolver) MergeBufPairErrors(from, to *BufPair) {
	if !from.HasErrors() {
		return
	}
	if to.HasErrors() {
		to.Errors.WriteBytes(comma)
	}
	to.Errors.WriteBytes(from.Errors.Bytes())
	from.Errors.Reset()
}

func (r *Resolver) getBufPair() *BufPair {
	return r.bufPairPool.Get().(*BufPair)
}

func (r *Resolver) freeBufPair(pair *BufPair) {
	pair.Data.Reset()
	pair.Errors.Reset()
	r.bufPairPool.Put(pair)
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

func (r *Resolver) getWaitGroup() *sync.WaitGroup {
	return r.waitGroupPool.Get().(*sync.WaitGroup)
}

func (r *Resolver) freeWaitGroup(wg *sync.WaitGroup) {
	r.waitGroupPool.Put(wg)
}
