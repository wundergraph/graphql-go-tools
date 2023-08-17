//go:generate mockgen --build_flags=--mod=mod -self_package=github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve -destination=resolve_mock_test.go -package=resolve . DataSource,BeforeFetchHook,AfterFetchHook,DataSourceBatch,DataSourceBatchFactory

package resolve

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"sync"
	"time"

	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash/v2"
	"github.com/tidwall/gjson"
	errors "golang.org/x/xerrors"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type Resolver struct {
	ctx               context.Context
	dataLoaderEnabled bool
	resultSetPool     sync.Pool
	byteSlicesPool    sync.Pool
	waitGroupPool     sync.Pool
	bufPairPool       sync.Pool
	bufPairSlicePool  sync.Pool
	errChanPool       sync.Pool
	hash64Pool        sync.Pool
	dataloaderFactory *dataLoaderFactory
	fetcher           *Fetcher
}

// New returns a new Resolver, ctx.Done() is used to cancel all active subscriptions & streams
func New(ctx context.Context, fetcher *Fetcher, enableDataLoader bool) *Resolver {
	return &Resolver{
		ctx: ctx,
		resultSetPool: sync.Pool{
			New: func() interface{} {
				return &resultSet{
					buffers: make(map[int]*BufPair, 8),
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
		dataloaderFactory: newDataloaderFactory(fetcher),
		fetcher:           fetcher,
		dataLoaderEnabled: enableDataLoader,
	}
}

func (r *Resolver) resolveNode(ctx *Context, node Node, data []byte, bufPair *BufPair) (err error) {
	switch n := node.(type) {
	case *Object:
		return r.resolveObject(ctx, n, data, bufPair)
	case *Array:
		return r.resolveArray(ctx, n, data, bufPair)
	case *Null:
		if n.Defer.Enabled {
			r.preparePatch(ctx, n.Defer.PatchIndex, nil, data)
		}
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

func (r *Resolver) validateContext(ctx *Context) (err error) {
	if ctx.maxPatch != -1 || ctx.currentPatch != -1 {
		return fmt.Errorf("context must be resetted using Free() before re-using it")
	}
	return nil
}

func (r *Resolver) ResolveGraphQLResponse(ctx *Context, response *GraphQLResponse, data []byte, writer io.Writer) (err error) {

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	if data != nil {
		ctx.lastFetchID = initialValueID
	}

	if r.dataLoaderEnabled {
		ctx.dataLoader = r.dataloaderFactory.newDataLoader(data)
		defer func() {
			r.dataloaderFactory.freeDataLoader(ctx.dataLoader)
			ctx.dataLoader = nil
		}()
	}

	ignoreData := false
	err = r.resolveNode(ctx, response.Data, data, buf)
	if err != nil {
		if !errors.Is(err, errNonNullableFieldValueIsNull) {
			return
		}
		ignoreData = true
	}

	return writeGraphqlResponse(buf, writer, ignoreData)
}

func (r *Resolver) resolveGraphQLSubscriptionResponse(ctx *Context, response *GraphQLResponse, subscriptionData *BufPair, writer io.Writer) (err error) {

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	if subscriptionData.HasData() {
		ctx.lastFetchID = initialValueID
	}

	if r.dataLoaderEnabled {
		ctx.dataLoader = r.dataloaderFactory.newDataLoader(subscriptionData.Data.Bytes())
		defer func() {
			r.dataloaderFactory.freeDataLoader(ctx.dataLoader)
			ctx.dataLoader = nil
		}()
	}

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

	buf := r.getBufPair()
	err = subscription.Trigger.InputTemplate.Render(ctx, nil, buf.Data)
	if err != nil {
		return
	}
	rendered := buf.Data.Bytes()
	subscriptionInput := make([]byte, len(rendered))
	copy(subscriptionInput, rendered)
	r.freeBufPair(buf)

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
			extractResponse(data, responseBuf, subscription.Trigger.ProcessResponseConfig)
			err = r.resolveGraphQLSubscriptionResponse(ctx, subscription.Response, responseBuf, writer)
			if err != nil {
				return err
			}
			writer.Flush()
		}
	}
}

func (r *Resolver) ResolveGraphQLStreamingResponse(ctx *Context, response *GraphQLStreamingResponse, data []byte, writer FlushWriter) (err error) {

	if err := r.validateContext(ctx); err != nil {
		return err
	}

	err = r.ResolveGraphQLResponse(ctx, response.InitialResponse, data, writer)
	if err != nil {
		return err
	}
	writer.Flush()

	nextFlush := time.Now().Add(time.Millisecond * time.Duration(response.FlushInterval))

	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)

	buf.Write(literal.LBRACK)

	done := ctx.Context().Done()

Loop:
	for {
		select {
		case <-done:
			return
		default:
			patch, ok := ctx.popNextPatch()
			if !ok {
				break Loop
			}

			if patch.index > len(response.Patches)-1 {
				continue
			}

			if buf.Len() != 1 {
				buf.Write(literal.COMMA)
			}

			preparedPatch := response.Patches[patch.index]
			err = r.ResolveGraphQLResponsePatch(ctx, preparedPatch, patch.data, patch.path, patch.extraPath, buf)
			if err != nil {
				return err
			}

			now := time.Now()
			if now.After(nextFlush) {
				buf.Write(literal.RBRACK)
				_, err = writer.Write(buf.Bytes())
				if err != nil {
					return err
				}
				writer.Flush()
				buf.Reset()
				buf.Write(literal.LBRACK)
				nextFlush = time.Now().Add(time.Millisecond * time.Duration(response.FlushInterval))
			}
		}
	}

	if buf.Len() != 1 {
		buf.Write(literal.RBRACK)
		_, err = writer.Write(buf.Bytes())
		if err != nil {
			return err
		}
		writer.Flush()
	}

	return
}

func (r *Resolver) ResolveGraphQLResponsePatch(ctx *Context, patch *GraphQLResponsePatch, data, path, extraPath []byte, writer io.Writer) (err error) {

	buf := r.getBufPair()
	defer r.freeBufPair(buf)

	ctx.pathPrefix = append(path, extraPath...)

	if patch.Fetch != nil {
		set := r.getResultSet()
		defer r.freeResultSet(set)
		err = r.resolveFetch(ctx, patch.Fetch, data, set)
		if err != nil {
			return err
		}
		_, ok := set.buffers[0]
		if ok {
			r.MergeBufPairErrors(set.buffers[0], buf)
			data = set.buffers[0].Data.Bytes()
		}
	}

	err = r.resolveNode(ctx, patch.Value, data, buf)
	if err != nil {
		return
	}

	hasErrors := buf.Errors.Len() != 0
	hasData := buf.Data.Len() != 0

	if hasErrors {
		return
	}

	if hasData {
		if hasErrors {
			err = writeSafe(err, writer, comma)
		}
		err = writeSafe(err, writer, lBrace)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literal.OP)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, patch.Operation)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, comma)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literal.PATH)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, path)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, comma)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, literal.VALUE)
		err = writeSafe(err, writer, quote)
		err = writeSafe(err, writer, colon)
		_, err = writer.Write(buf.Data.Bytes())
		err = writeSafe(err, writer, rBrace)
	}

	return
}

func (r *Resolver) resolveEmptyArray(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrack)
	b.WriteBytes(rBrack)
}

func (r *Resolver) resolveEmptyObject(b *fastbuffer.FastBuffer) {
	b.WriteBytes(lBrace)
	b.WriteBytes(rBrace)
}

func (r *Resolver) resolveArray(ctx *Context, array *Array, data []byte, arrayBuf *BufPair) (err error) {
	if len(array.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, array.Path...)
	}

	if bytes.Equal(data, emptyArray) {
		r.resolveEmptyArray(arrayBuf.Data)
		return
	}

	arrayItems := r.byteSlicesPool.Get().(*[][]byte)
	defer func() {
		*arrayItems = (*arrayItems)[:0]
		r.byteSlicesPool.Put(arrayItems)
	}()

	_, _ = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if err == nil && dataType == jsonparser.String {
			value = data[offset-2 : offset+len(value)] // add quotes to string values
		}

		*arrayItems = append(*arrayItems, value)
	})

	if len(*arrayItems) == 0 {
		if !array.Nullable {
			r.resolveEmptyArray(arrayBuf.Data)
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(arrayBuf.Data)
		return nil
	}

	ctx.addResponseArrayElements(array.Path)
	defer func() { ctx.removeResponseArrayLastElements(array.Path) }()

	if array.ResolveAsynchronous && !array.Stream.Enabled && !r.dataLoaderEnabled {
		return r.resolveArrayAsynchronous(ctx, array, arrayItems, arrayBuf)
	}
	return r.resolveArraySynchronous(ctx, array, arrayItems, arrayBuf)
}

func (r *Resolver) resolveArraySynchronous(ctx *Context, array *Array, arrayItems *[][]byte, arrayBuf *BufPair) (err error) {

	itemBuf := r.getBufPair()
	defer r.freeBufPair(itemBuf)

	arrayBuf.Data.WriteBytes(lBrack)
	var (
		hasPreviousItem bool
		dataWritten     int
	)
	for i := range *arrayItems {

		if array.Stream.Enabled {
			if i > array.Stream.InitialBatchSize-1 {
				ctx.addIntegerPathElement(i)
				r.preparePatch(ctx, array.Stream.PatchIndex, nil, (*arrayItems)[i])
				ctx.removeLastPathElement()
				continue
			}
		}

		ctx.addIntegerPathElement(i)
		err = r.resolveNode(ctx, array.Item, (*arrayItems)[i], itemBuf)
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
		dataWritten += itemBuf.Data.Len()
		r.MergeBufPairs(itemBuf, arrayBuf, hasPreviousItem)
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
		}
	}

	arrayBuf.Data.WriteBytes(rBrack)
	return
}

func (r *Resolver) resolveArrayAsynchronous(ctx *Context, array *Array, arrayItems *[][]byte, arrayBuf *BufPair) (err error) {

	arrayBuf.Data.WriteBytes(lBrack)

	bufSlice := r.getBufPairSlice()
	defer r.freeBufPairSlice(bufSlice)

	wg := r.getWaitGroup()
	defer r.freeWaitGroup(wg)

	errCh := r.getErrChan()
	defer r.freeErrChan(errCh)

	wg.Add(len(*arrayItems))

	for i := range *arrayItems {
		itemBuf := r.getBufPair()
		*bufSlice = append(*bufSlice, itemBuf)
		itemData := (*arrayItems)[i]
		cloned := ctx.clone()
		go func(ctx Context, i int) {
			ctx.addPathElement([]byte(strconv.Itoa(i)))
			if e := r.resolveNode(&ctx, array.Item, itemData, itemBuf); e != nil && !errors.Is(e, errTypeNameSkipped) {
				select {
				case errCh <- e:
				default:
				}
			}
			ctx.Free()
			wg.Done()
		}(cloned, i)
	}

	wg.Wait()

	select {
	case err = <-errCh:
	default:
	}

	if err != nil {
		if errors.Is(err, errNonNullableFieldValueIsNull) && array.Nullable {
			arrayBuf.Data.Reset()
			r.resolveNull(arrayBuf.Data)
			return nil
		}
		return
	}

	var (
		hasPreviousItem bool
		dataWritten     int
	)
	for i := range *bufSlice {
		dataWritten += (*bufSlice)[i].Data.Len()
		r.MergeBufPairs((*bufSlice)[i], arrayBuf, hasPreviousItem)
		if !hasPreviousItem && dataWritten != 0 {
			hasPreviousItem = true
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
		value = append(literal.QUOTE, append(value, literal.QUOTE...)...)
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
			value = append(literal.QUOTE, append(value, literal.QUOTE...)...)
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

func (r *Resolver) preparePatch(ctx *Context, patchIndex int, extraPath, data []byte) {
	buf := pool.BytesBuffer.Get()
	ctx.usedBuffers = append(ctx.usedBuffers, buf)
	_, _ = buf.Write(data)
	path, data := ctx.path(), buf.Bytes()
	ctx.addPatch(patchIndex, path, extraPath, data)
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

func (r *Resolver) resolveObject(ctx *Context, object *Object, data []byte, objectBuf *BufPair) (err error) {
	if len(object.Path) != 0 {
		data, _, _, _ = jsonparser.Get(data, object.Path...)

		if len(data) == 0 || bytes.Equal(data, literal.NULL) {
			// we will not traverse the children if the object is null
			// therefore, we must "pop" the null element from the batch
			r.recursivelySkipBatchResults(ctx, object, data)
			if object.Nullable {
				r.resolveNull(objectBuf.Data)
				return
			}

			r.addResolveError(ctx, objectBuf)
			return errNonNullableFieldValueIsNull
		}

		ctx.addResponseElements(object.Path)
		defer ctx.removeResponseLastElements(object.Path)
	}

	if object.UnescapeResponseJson {
		data = bytes.ReplaceAll(data, []byte(`\"`), []byte(`"`))
	}

	var set *resultSet
	if object.Fetch != nil {
		set = r.getResultSet()
		defer r.freeResultSet(set)
		err = r.resolveFetch(ctx, object.Fetch, data, set)
		if err != nil {
			return
		}
		for i := range set.buffers {
			r.MergeBufPairErrors(set.buffers[i], objectBuf)
		}
	}

	fieldBuf := r.getBufPair()
	defer r.freeBufPair(fieldBuf)

	responseElements := ctx.responseElements
	lastFetchID := ctx.lastFetchID

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

		var fieldData []byte
		if set != nil && object.Fields[i].HasBuffer {
			buffer, ok := set.buffers[object.Fields[i].BufferID]
			if ok {
				fieldData = buffer.Data.Bytes()
				ctx.resetResponsePathElements()
				ctx.lastFetchID = object.Fields[i].BufferID
			}
		} else {
			fieldData = data
		}

		if object.Fields[i].OnTypeNames != nil {
			typeName, _, _, _ := jsonparser.Get(fieldData, "__typename")
			hasMatch := false
			for _, onTypeName := range object.Fields[i].OnTypeNames {
				if bytes.Equal(typeName, onTypeName) {
					hasMatch = true
					break
				}
			}
			if !hasMatch {
				typeNameSkip = true
				// Restore the response elements that may have been reset above.
				ctx.responseElements = responseElements
				ctx.lastFetchID = lastFetchID
				continue
			}
		}

		if first {
			objectBuf.Data.WriteBytes(lBrace)
			first = false
		} else {
			objectBuf.Data.WriteBytes(comma)
		}
		objectBuf.Data.WriteBytes(quote)
		objectBuf.Data.WriteBytes(object.Fields[i].Name)
		objectBuf.Data.WriteBytes(quote)
		objectBuf.Data.WriteBytes(colon)
		ctx.addPathElement(object.Fields[i].Name)
		ctx.setPosition(object.Fields[i].Position)
		err = r.resolveNode(ctx, object.Fields[i].Value, fieldData, fieldBuf)
		ctx.removeLastPathElement()
		ctx.responseElements = responseElements
		ctx.lastFetchID = lastFetchID
		if err != nil {
			if errors.Is(err, errTypeNameSkipped) {
				objectBuf.Data.Reset()
				r.resolveEmptyObject(objectBuf.Data)
				return nil
			}
			if errors.Is(err, errNonNullableFieldValueIsNull) {
				objectBuf.Data.Reset()
				r.MergeBufPairErrors(fieldBuf, objectBuf)

				if object.Nullable {
					r.resolveNull(objectBuf.Data)
					return nil
				}

				// if fied is of object type than we should not add resolve error here
				if _, ok := object.Fields[i].Value.(*Object); !ok {
					r.addResolveError(ctx, objectBuf)
				}
			}

			return
		}
		r.MergeBufPairs(fieldBuf, objectBuf, false)
	}
	allSkipped := len(object.Fields) != 0 && len(object.Fields) == skipCount
	if allSkipped {
		// return empty object if all fields have been skipped
		r.resolveEmptyObject(objectBuf.Data)
		return
	}
	if first {
		if typeNameSkip {
			r.resolveEmptyObject(objectBuf.Data)
			return
		}
		if !object.Nullable {
			r.addResolveError(ctx, objectBuf)
			return errNonNullableFieldValueIsNull
		}
		r.resolveNull(objectBuf.Data)
		return
	}
	objectBuf.Data.WriteBytes(rBrace)
	return
}

// recursivelySkipBatchResults traverses an object and skips all batch results by triggering fetch
// when a fetch is attached to an object using batch fetch, only the first object will actually trigger the fetch
// subsequent objects (siblings) will load the result from the cache, filled by the first sibling
// if one sibling has no data (null), we have to "pop" the null result (generated by the batch resolver) from the cache
// this is because the "null" sibling will not trigger a fetch by itself, as it has no data and will not resolve any fields
func (r *Resolver) recursivelySkipBatchResults(ctx *Context, object *Object, data []byte) {
	if object.Fetch != nil && object.Fetch.FetchKind() == FetchKindBatch {
		set := r.getResultSet()
		defer r.freeResultSet(set)
		_ = r.resolveFetch(ctx, object.Fetch, data, set)
	}
	for i := range object.Fields {
		value := object.Fields[i].Value
		switch v := value.(type) {
		case *Object:
			r.recursivelySkipBatchResults(ctx, v, data)
		case *Array:
			switch av := v.Item.(type) {
			case *Object:
				r.recursivelySkipBatchResults(ctx, av, data)
			}
		}
	}
}

func (r *Resolver) resolveFetch(ctx *Context, fetch Fetch, data []byte, set *resultSet) (err error) {
	// if context is cancelled, we should not resolve the fetch
	if errors.Is(ctx.Context().Err(), context.Canceled) {
		return nil
	}

	switch f := fetch.(type) {
	case *SingleFetch:
		preparedInput := r.getBufPair()
		defer r.freeBufPair(preparedInput)
		err = r.prepareSingleFetch(ctx, f, data, set, preparedInput.Data)
		if err != nil {
			return err
		}
		err = r.resolveSingleFetch(ctx, f, preparedInput.Data, set.buffers[f.BufferId])
	case *BatchFetch:
		preparedInput := r.getBufPair()
		defer r.freeBufPair(preparedInput)
		err = r.prepareSingleFetch(ctx, f.Fetch, data, set, preparedInput.Data)
		if err != nil {
			return err
		}
		err = r.resolveBatchFetch(ctx, f, preparedInput.Data, set.buffers[f.Fetch.BufferId])
	case *ParallelFetch:
		err = r.resolveParallelFetch(ctx, f, data, set)
	case *SerialFetch:
		// TODO: implement
		panic("implement me")
	}
	return
}

func (r *Resolver) resolveParallelFetch(ctx *Context, fetch *ParallelFetch, data []byte, set *resultSet) (err error) {
	preparedInputs := r.getBufPairSlice()
	defer r.freeBufPairSlice(preparedInputs)

	resolvers := make([]func() error, 0, len(fetch.Fetches))

	wg := r.getWaitGroup()
	defer r.freeWaitGroup(wg)

	for i := range fetch.Fetches {
		wg.Add(1)
		switch f := fetch.Fetches[i].(type) {
		case *SingleFetch:
			preparedInput := r.getBufPair()
			err = r.prepareSingleFetch(ctx, f, data, set, preparedInput.Data)
			if err != nil {
				return err
			}
			*preparedInputs = append(*preparedInputs, preparedInput)
			buf := set.buffers[f.BufferId]
			resolvers = append(resolvers, func() error {
				return r.resolveSingleFetch(ctx, f, preparedInput.Data, buf)
			})
		case *BatchFetch:
			preparedInput := r.getBufPair()
			err = r.prepareSingleFetch(ctx, f.Fetch, data, set, preparedInput.Data)
			if err != nil {
				return err
			}
			*preparedInputs = append(*preparedInputs, preparedInput)
			buf := set.buffers[f.Fetch.BufferId]
			resolvers = append(resolvers, func() error {
				return r.resolveBatchFetch(ctx, f, preparedInput.Data, buf)
			})
		}
	}

	for _, resolver := range resolvers {
		go func(r func() error) {
			_ = r()
			wg.Done()
		}(resolver)
	}

	wg.Wait()

	return
}

func (r *Resolver) prepareSingleFetch(ctx *Context, fetch *SingleFetch, data []byte, set *resultSet, preparedInput *fastbuffer.FastBuffer) (err error) {
	err = fetch.InputTemplate.Render(ctx, data, preparedInput)
	buf := r.getBufPair()
	set.buffers[fetch.BufferId] = buf
	return
}

func (r *Resolver) resolveBatchFetch(ctx *Context, fetch *BatchFetch, preparedInput *fastbuffer.FastBuffer, buf *BufPair) error {
	if r.dataLoaderEnabled {
		return ctx.dataLoader.LoadBatch(ctx, fetch, buf)
	}

	if err := r.fetcher.FetchBatch(ctx, fetch, []*fastbuffer.FastBuffer{preparedInput}, []*BufPair{buf}); err != nil {
		return err
	}

	return nil
}

func (r *Resolver) resolveSingleFetch(ctx *Context, fetch *SingleFetch, preparedInput *fastbuffer.FastBuffer, buf *BufPair) error {
	if r.dataLoaderEnabled && !fetch.DisableDataLoader {
		return ctx.dataLoader.Load(ctx, fetch, buf)
	}
	return r.fetcher.Fetch(ctx, fetch, preparedInput, buf)
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

func (r *Resolver) getResultSet() *resultSet {
	return r.resultSetPool.Get().(*resultSet)
}

func (r *Resolver) freeResultSet(set *resultSet) {
	for i := range set.buffers {
		set.buffers[i].Reset()
		r.bufPairPool.Put(set.buffers[i])
		delete(set.buffers, i)
	}
	r.resultSetPool.Put(set)
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
