package resolve

import (
	"bytes"
	"fmt"
	"io"
	"reflect"
	"strconv"
	"sync"
	"unsafe"

	"github.com/buger/jsonparser"
	"github.com/pkg/errors"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/singleflight"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

var (
	ErrOriginResponseError = errors.New("origin response error")
)

type Loader struct {
	layers []*layer

	errors []byte

	sf        *singleflight.Group
	sfEnabled bool

	buffers []*bytes.Buffer
}

func (l *Loader) Free() {
	for i := range l.buffers {
		pool.BytesBuffer.Put(l.buffers[i])
	}
	l.buffers = l.buffers[:0]
}

type resultSet struct {
	mu        sync.Mutex
	data      []byte
	itemsData [][]byte
	mergePath []string
	buffers   []*bytes.Buffer
	errors    []byte
}

func (r *resultSet) getBuffer() *bytes.Buffer {
	buf := pool.BytesBuffer.Get()
	r.mu.Lock()
	r.buffers = append(r.buffers, buf)
	r.mu.Unlock()
	return buf
}

type layer struct {
	path            []string
	data            []byte
	items           [][]byte
	mapping         [][]int
	kind            layerKind
	hasFetches      bool
	hasResolvedData bool
}

func (l *layer) itemsSize() int {
	size := 0
	for i := range l.items {
		size += len(l.items[i])
	}
	return size
}

type layerKind int

const (
	layerKindObject layerKind = iota + 1
	layerKindArray
)

func (l *Loader) getBuffer() *bytes.Buffer {
	buf := pool.BytesBuffer.Get()
	l.buffers = append(l.buffers, buf)
	return buf
}

func (l *Loader) popLayer() {
	l.layers = l.layers[:len(l.layers)-1]
}

func (l *Loader) inputData(layer *layer, out *bytes.Buffer) []byte {
	if layer.data != nil || layer.kind == layerKindObject {
		return layer.data
	}
	_, _ = out.Write([]byte(`[`))
	addCommaSeparator := false
	for i := range layer.items {
		if layer.items[i] == nil {
			continue
		}
		if addCommaSeparator {
			_, _ = out.Write([]byte(`,`))
		} else {
			addCommaSeparator = true
		}
		_, _ = out.Write(layer.items[i])
	}
	_, _ = out.Write([]byte(`]`))
	return out.Bytes()
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, data []byte, out io.Writer) (hasErrors bool, err error) {
	l.layers = l.layers[:0]
	l.errors = l.errors[:0]
	l.layers = append(l.layers, &layer{
		data: data,
		kind: layerKindObject,
	})
	err = l.resolveNode(ctx, response.Data)
	if err != nil {
		if errors.Is(err, ErrOriginResponseError) {
			_, err1 := out.Write([]byte(`{"errors":`))
			_, err2 := out.Write(l.errors)
			_, err3 := out.Write([]byte(`}`))
			return true, multierr.Combine(err1, err2, err3)
		}
		return false, err
	}
	_, err = out.Write(l.layers[0].data)
	return false, err
}

func (l *Loader) resolveNode(ctx *Context, node Node) (err error) {
	switch node := node.(type) {
	case *Object:
		return l.resolveObject(ctx, node)
	case *Array:
		return l.resolveArray(ctx, node)
	}
	return nil
}

func (l *Loader) insideArray() bool {
	return l.currentLayer().kind == layerKindArray
}

func (l *Loader) currentLayer() *layer {
	return l.layers[len(l.layers)-1]
}

func (l *Loader) currentLayerData() []byte {
	return l.layers[len(l.layers)-1].data
}

func (l *Loader) setCurrentLayerData(data []byte) {
	l.layers[len(l.layers)-1].data = data
}

func (l *Loader) resolveLayerData(path []string, isArray bool) (data []byte, items [][]byte, itemsMapping [][]int, err error) {
	current := l.currentLayer()
	if !l.insideArray() && !isArray {
		data, _, _, err = jsonparser.Get(current.data, path...)
		if errors.Is(err, jsonparser.KeyPathNotFoundError) {
			// we have no data for this path which is legit
			return nil, nil, nil, nil
		}
		return
	}
	if current.data != nil {
		_, err = jsonparser.ArrayEach(current.data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			switch dataType {
			case jsonparser.String:
				// jsonparser.ArrayEach does not return the quotes so we need to add them
				items = append(items, current.data[offset-2:offset+len(value)])
			default:
				items = append(items, value)
			}
		}, path...)
		return nil, items, nil, errors.WithStack(err)
	}
	if isArray {
		itemsMapping = make([][]int, len(current.items))
		count := 0
		for i := range current.items {
			_, _ = jsonparser.ArrayEach(current.items[i], func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
				switch dataType {
				case jsonparser.String:
					// jsonparser.ArrayEach does not return the quotes so we need to add them
					items = append(items, current.items[i][offset-2:offset+len(value)])
				default:
					items = append(items, value)
				}
				itemsMapping[i] = append(itemsMapping[i], count)
				count++
			}, path...)
		}
	} else {
		for i := range current.items {
			data, _, _, _ = jsonparser.Get(current.items[i], path...)
			// we explicitly ignore the error and just append a nil slice
			items = append(items, data)
		}
	}
	return nil, items, itemsMapping, nil
}

func (l *Loader) resolveArray(ctx *Context, array *Array) (err error) {
	if !array.HasChildFetches() {
		return nil
	}
	data, items, mapping, err := l.resolveLayerData(array.Path, true)
	if err != nil {
		return errors.WithStack(err)
	}
	next := &layer{
		path:    array.Path,
		data:    data,
		items:   items,
		mapping: mapping,
		kind:    layerKindArray,
	}
	l.layers = append(l.layers, next)
	err = l.resolveNode(ctx, array.Item)
	if err != nil {
		return errors.WithStack(err)
	}
	err = l.mergeLayerIntoParent()
	if err != nil {
		return errors.WithStack(err)
	}
	l.popLayer()
	return nil
}

func (l *Loader) resolveObject(ctx *Context, object *Object) (err error) {
	if l.shouldSkipObject(object) {
		return nil
	}
	if len(object.Path) != 0 {
		data, items, mapping, err := l.resolveLayerData(object.Path, false)
		if err != nil {
			return errors.WithStack(err)
		}
		next := &layer{
			path:    object.Path,
			data:    data,
			items:   items,
			mapping: mapping,
			kind:    layerKindObject,
		}
		if l.insideArray() {
			next.kind = layerKindArray
		}
		l.layers = append(l.layers, next)
	}
	if object.Fetch != nil {
		err = l.resolveFetch(ctx, object.Fetch, nil)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	if l.shouldTraverseObjectChildren(object) {
		for i := range object.Fields {
			err = l.resolveNode(ctx, object.Fields[i].Value)
			if err != nil {
				return errors.WithStack(err)
			}
		}
	}
	if len(object.Path) != 0 {
		err = l.mergeLayerIntoParent()
		if err != nil {
			return errors.WithStack(err)
		}
		l.popLayer()
	}
	return nil
}

func (l *Loader) shouldSkipObject(object *Object) bool {
	if object.Fetch == nil && !object.HasChildFetches() {
		return true
	}
	return false
}

func (l *Loader) shouldTraverseObjectChildren(object *Object) bool {
	if !object.HasChildFetches() {
		return false
	}
	lr := l.currentLayer()
	if lr.hasFetches {
		return lr.hasResolvedData
	}
	return true
}

func (l *Loader) mergeLayerIntoParent() (err error) {
	child := l.layers[len(l.layers)-1]
	parent := l.layers[len(l.layers)-2]
	if child.mapping != nil {
		for i, indices := range child.mapping {
			buf := l.getBuffer()
			_, _ = buf.Write([]byte(`[`))
			for j := range indices {
				if j != 0 {
					_, _ = buf.Write([]byte(`,`))
				}
				_, _ = buf.Write(child.items[indices[j]])
			}
			_, _ = buf.Write([]byte(`]`))
			parent.items[i], err = jsonparser.Set(parent.items[i], buf.Bytes(), child.path...)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		return nil
	}
	if parent.kind == layerKindObject && child.kind == layerKindObject {
		parent.data, err = l.mergeJSONWithMergePath(parent.data, child.data, child.path)
		return errors.WithStack(err)
	}
	if parent.kind == layerKindObject && child.kind == layerKindArray {
		buf := l.getBuffer()
		_, _ = buf.Write([]byte(`[`))
		addCommaSeparator := false
		for i := range child.items {
			if child.items[i] == nil {
				continue
			}
			if addCommaSeparator {
				_, _ = buf.Write([]byte(`,`))
			} else {
				addCommaSeparator = true
			}
			_, _ = buf.Write(child.items[i])
		}
		_, _ = buf.Write([]byte(`]`))
		parent.data, err = jsonparser.Set(parent.data, buf.Bytes(), child.path...)
		return errors.WithStack(err)
	}
	for i := range parent.items {
		if child.items[i] == nil {
			continue
		}
		existing, _, _, _ := jsonparser.Get(parent.items[i], child.path...)
		combined, err := l.mergeJSON(existing, child.items[i])
		if err != nil {
			return errors.WithStack(err)
		}
		parent.items[i], err = jsonparser.Set(parent.items[i], combined, child.path...)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (l *Loader) mergeResultErr(res *resultSet) {
	if res == nil {
		return
	}
	if res.errors == nil {
		return
	}
	l.errors = res.errors
}

func (l *Loader) mergeResultSet(res *resultSet) (err error) {
	if res == nil {
		return nil
	}
	if res.buffers != nil {
		l.buffers = append(l.buffers, res.buffers...)
	}
	if res.data != nil {
		return l.mergeDataIntoLayer(l.currentLayer(), res.data, res.mergePath)
	}
	if res.itemsData != nil {
		lr := l.currentLayer()
		before := lr.itemsSize()
		for i := range lr.items {
			if lr.items[i] == nil {
				continue
			}
			lr.items[i], err = l.mergeJSONWithMergePath(lr.items[i], res.itemsData[i], res.mergePath)
			if err != nil {
				return errors.WithStack(err)
			}
		}
		after := lr.itemsSize()
		if after > before {
			lr.hasResolvedData = true
		}
	}
	return nil
}

func (l *Loader) resolveFetch(ctx *Context, fetch Fetch, res *resultSet) (err error) {
	parallel := res != nil
	lr := l.currentLayer()
	if !parallel {
		// would be a data race otherwise
		// we already set it to true for the root parallel fetch, so skip is fine
		lr.hasFetches = true
	}
	switch f := fetch.(type) {
	case *SingleFetch:
		if parallel {
			// TODO: single fetch is not possible inside parallel fetch
			return l.resolveSingleFetch(ctx, f, res)
		}
		res = &resultSet{}
		err = l.resolveSingleFetch(ctx, f, res)
		if err != nil {
			l.mergeResultErr(res)
			return err
		}
		return l.mergeResultSet(res)
	case *SerialFetch:
		return l.resolveSerialFetch(ctx, f)
	case *ParallelFetch:
		return l.resolveParallelFetch(ctx, f)
	case *ParallelListItemFetch:
		if parallel {
			return l.resolveParallelListItemFetch(ctx, f, res)
		}
		res = &resultSet{}
		err = l.resolveParallelListItemFetch(ctx, f, res)
		if err != nil {
			l.mergeResultErr(res)
			return err
		}
		return l.mergeResultSet(res)
	case *BatchFetch:
		if parallel {
			return l.resolveBatchFetch(ctx, f, res)
		}
		res = &resultSet{}
		err = l.resolveBatchFetch(ctx, f, res)
		if err != nil {
			l.mergeResultErr(res)
			return err
		}
		return l.mergeResultSet(res)
	}
	return nil
}

func (l *Loader) resolveBatchFetch(ctx *Context, fetch *BatchFetch, res *resultSet) (err error) {
	res.mergePath = fetch.PostProcessing.MergePath
	input := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(input)
	lr := l.currentLayer()
	err = fetch.Input.Header.Render(ctx, nil, input)
	if err != nil {
		return errors.WithStack(err)
	}
	batchStats := make([][]int, len(lr.items))
	batchItemIndex := 0

	itemBuf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(itemBuf)
	hash := pool.Hash64.Get()
	defer pool.Hash64.Put(hash)

	itemHashes := make([]uint64, 0, len(lr.items)*len(fetch.Input.Items))
	addSeparator := false

	for i := range lr.items {
		if lr.items[i] == nil {
			continue
		}
	WithNext:
		for j := range fetch.Input.Items {
			itemBuf.Reset()
			err = fetch.Input.Items[j].Render(ctx, lr.items[i], itemBuf)
			if err != nil {
				if fetch.Input.SkipErrItems {
					err = nil
					batchStats[i] = append(batchStats[i], -1)
					continue
				}
				return errors.WithStack(err)
			}
			if fetch.Input.SkipNullItems && itemBuf.Len() == 4 && bytes.Equal(itemBuf.Bytes(), null) {
				batchStats[i] = append(batchStats[i], -1)
				continue
			}
			hash.Reset()
			_, _ = hash.Write(itemBuf.Bytes())
			itemHash := hash.Sum64()
			for k := range itemHashes {
				if itemHashes[k] == itemHash {
					batchStats[i] = append(batchStats[i], k)
					continue WithNext
				}
			}
			itemHashes = append(itemHashes, itemHash)
			if addSeparator {
				err = fetch.Input.Separator.Render(ctx, nil, input)
				if err != nil {
					return errors.WithStack(err)
				}
			}
			_, _ = input.Write(itemBuf.Bytes())
			batchStats[i] = append(batchStats[i], batchItemIndex)
			batchItemIndex++
			addSeparator = true
		}
	}
	err = fetch.Input.Footer.Render(ctx, nil, input)
	if err != nil {
		return errors.WithStack(err)
	}
	data, err := l.loadWithSingleFlight(ctx, fetch.DataSource, fetch.DataSourceIdentifier, input.Bytes(), res)
	if err != nil {
		return errors.WithStack(err)
	}
	responseErrors, _, _, _ := jsonparser.Get(data, "errors")
	if responseErrors != nil {
		l.errors = responseErrors
		return errors.WithStack(ErrOriginResponseError)
	}
	if fetch.PostProcessing.SelectResponseDataPath != nil {
		data, _, _, err = jsonparser.Get(data, fetch.PostProcessing.SelectResponseDataPath...)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	var (
		batchResponseItems [][]byte
	)
	_, err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		batchResponseItems = append(batchResponseItems, value)
	})
	if err != nil {
		return errors.WithStack(err)
	}
	res.itemsData = make([][]byte, len(lr.items))
	if fetch.PostProcessing.ResponseTemplate != nil {
		buf := res.getBuffer()
		start := 0
		for i, stats := range batchStats {
			_, _ = buf.Write(lBrack)
			addCommaSeparator := false
			for j := range stats {
				if addCommaSeparator {
					_, _ = buf.Write(comma)
				} else {
					addCommaSeparator = true
				}
				if stats[j] == -1 {
					_, _ = buf.Write(null)
					continue
				}
				_, err = buf.Write(batchResponseItems[stats[j]])
				if err != nil {
					return errors.WithStack(err)
				}
			}
			_, _ = buf.Write(rBrack)
			res.itemsData[i] = buf.Bytes()[start:]
			start = buf.Len()
		}
		for i := range res.itemsData {
			err = fetch.PostProcessing.ResponseTemplate.Render(ctx, res.itemsData[i], buf)
			if err != nil {
				return errors.WithStack(err)
			}
			res.itemsData[i] = buf.Bytes()[start:]
			start = buf.Len()
		}
	} else {
		for i, stats := range batchStats {
			for j := range stats {
				if stats[j] == -1 {
					continue
				}
				res.itemsData[i], err = l.mergeJSON(res.itemsData[i], batchResponseItems[stats[j]])
				if err != nil {
					return errors.WithStack(err)
				}
			}
		}
	}
	return
}

func (l *Loader) resolveParallelListItemFetch(ctx *Context, fetch *ParallelListItemFetch, res *resultSet) (err error) {
	if !l.insideArray() {
		return errors.WithStack(fmt.Errorf("resolveParallelListItemFetch must be inside an array, this seems to be a bug in the planner"))
	}
	layer := l.currentLayer()
	group, gCtx := errgroup.WithContext(ctx.ctx)
	groupContext := ctx.WithContext(gCtx)
	res.itemsData = make([][]byte, len(layer.items))
	res.mergePath = fetch.Fetch.PostProcessing.MergePath
	for i := range layer.items {
		i := i
		// get a buffer before we start the goroutines
		// getLayerBuffer will append the buffer to the list of buffers of the current layer
		// this will ensure that the buffer is not re-used before this layer is merged into the parent
		// however, appending is not concurrency safe, so we need to do it before we start the goroutines
		out := res.getBuffer()
		input := res.getBuffer()
		err = fetch.Fetch.InputTemplate.Render(ctx, layer.items[i], input)
		if err != nil {
			return errors.WithStack(err)
		}
		group.Go(func() error {
			data, err := l.loadAndPostProcess(groupContext, input, fetch.Fetch, out, res)
			if err != nil {
				return errors.WithStack(err)
			}
			res.itemsData[i] = data
			return nil
		})
	}
	err = group.Wait()
	if err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func (l *Loader) resolveParallelFetch(ctx *Context, fetch *ParallelFetch) (err error) {
	group, groupContext := errgroup.WithContext(ctx.Context())
	groupCtx := ctx.WithContext(groupContext)
	isArray := l.insideArray()
	resultSets := make([]*resultSet, len(fetch.Fetches))
	for i := range fetch.Fetches {
		f := fetch.Fetches[i]
		res := &resultSet{}
		resultSets[i] = res
		if isArray && f.FetchKind() == FetchKindSingle {
			return fmt.Errorf("parallel fetches inside an array must not be of kind FetchKindSingle - this seems to be a bug in the planner")
		}
		group.Go(func() error {
			return l.resolveFetch(groupCtx, f, res)
		})
	}
	err = group.Wait()
	for i := range resultSets {
		err = l.mergeResultSet(resultSets[i])
		if err != nil {
			return err
		}
	}
	return errors.WithStack(err)
}

func (l *Loader) resolveSingleFetch(ctx *Context, fetch *SingleFetch, res *resultSet) (err error) {
	input := res.getBuffer()
	inputBuf := res.getBuffer()
	out := res.getBuffer()
	inputData := l.inputData(l.currentLayer(), inputBuf)
	err = fetch.InputTemplate.Render(ctx, inputData, input)
	if err != nil {
		return errors.WithStack(err)
	}
	res.mergePath = fetch.PostProcessing.MergePath
	res.data, err = l.loadAndPostProcess(ctx, input, fetch, out, res)
	if err != nil {
		return errors.WithStack(err)
	}
	return
}

func (l *Loader) loadWithSingleFlight(ctx *Context, source DataSource, identifier, input []byte, res *resultSet) ([]byte, error) {
	if !l.sfEnabled {
		out := res.getBuffer()
		err := source.Load(ctx.ctx, input, out)
		if err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}
	keyGen := pool.Hash64.Get()
	defer pool.Hash64.Put(keyGen)
	_, _ = keyGen.Write(identifier)
	_, _ = keyGen.Write(input)
	key := strconv.FormatUint(keyGen.Sum64(), 10)
	maybeData, err, shared := l.sf.Do(key, func() (interface{}, error) {
		out := &bytes.Buffer{}
		err := source.Load(ctx.ctx, input, out)
		if err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	})
	if err != nil {
		return nil, err
	}
	data := maybeData.([]byte)
	if shared {
		out := make([]byte, len(data))
		copy(out, data)
		return out, err
	}
	return data, nil
}

func (l *Loader) loadAndPostProcess(ctx *Context, input *bytes.Buffer, fetch *SingleFetch, out *bytes.Buffer, res *resultSet) (data []byte, err error) {
	data, err = l.loadWithSingleFlight(ctx, fetch.DataSource, fetch.DataSourceIdentifier, input.Bytes(), res)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	responseErrors, _, _, _ := jsonparser.Get(data, "errors")
	if responseErrors != nil {
		res.errors = responseErrors
		return nil, ErrOriginResponseError
	}
	if fetch.PostProcessing.SelectResponseDataPath != nil {
		data, _, _, err = jsonparser.Get(data, fetch.PostProcessing.SelectResponseDataPath...)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	}
	if fetch.PostProcessing.ResponseTemplate != nil {
		intermediate := pool.FastBuffer.Get()
		defer pool.FastBuffer.Put(intermediate)
		_, err = intermediate.Write(data)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		out.Reset()
		err = fetch.PostProcessing.ResponseTemplate.Render(ctx, intermediate.Bytes(), out)
		if err != nil {
			return nil, errors.WithStack(err)
		}
		data = out.Bytes()
	}
	return data, nil
}

func (l *Loader) mergeDataIntoLayer(layer *layer, data []byte, mergePath []string) (err error) {
	if bytes.Equal(data, null) {
		return
	}
	layer.hasResolvedData = true
	if layer.kind == layerKindObject {
		layer.data, err = l.mergeJSONWithMergePath(layer.data, data, mergePath)
		return errors.WithStack(err)
	}
	var (
		dataItems [][]byte
	)
	_, err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		switch dataType {
		case jsonparser.String:
			// jsonparser.ArrayEach does not return the quotes so we need to add them
			dataItems = append(dataItems, data[offset-2:offset+len(value)])
		default:
			dataItems = append(dataItems, value)
		}
	})
	if err != nil {
		return errors.WithStack(err)
	}
	skipped := 0
	for i := 0; i < len(layer.items); i++ {
		if layer.items[i] == nil {
			skipped++
			continue
		}
		layer.items[i], err = l.mergeJSONWithMergePath(layer.items[i], dataItems[i-skipped], mergePath)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

func (l *Loader) resolveSerialFetch(ctx *Context, fetch *SerialFetch) (err error) {
	for i := range fetch.Fetches {
		err = l.resolveFetch(ctx, fetch.Fetches[i], nil)
		if err != nil {
			return errors.WithStack(err)
		}
	}
	return nil
}

type fastJsonContext struct {
	keys, values               [][]byte
	types                      []jsonparser.ValueType
	missingKeys, missingValues [][]byte
	missingTypes               []jsonparser.ValueType
}

var (
	fastJsonPool = sync.Pool{
		New: func() interface{} {
			ctx := &fastJsonContext{}
			ctx.keys = make([][]byte, 0, 4)
			ctx.values = make([][]byte, 0, 4)
			ctx.types = make([]jsonparser.ValueType, 0, 4)
			ctx.missingKeys = make([][]byte, 0, 4)
			ctx.missingValues = make([][]byte, 0, 4)
			ctx.missingTypes = make([]jsonparser.ValueType, 0, 4)
			return ctx
		},
	}
)

func (l *Loader) mergeJSONWithMergePath(left, right []byte, mergePath []string) ([]byte, error) {
	if len(mergePath) == 0 {
		return l.mergeJSON(left, right)
	}
	element := mergePath[len(mergePath)-1]
	mergePath = mergePath[:len(mergePath)-1]
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	_, _ = buf.Write(lBrace)
	_, _ = buf.Write(quote)
	_, _ = buf.Write([]byte(element))
	_, _ = buf.Write(quote)
	_, _ = buf.Write(colon)
	_, _ = buf.Write(right)
	_, _ = buf.Write(rBrace)
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return l.mergeJSONWithMergePath(left, out, mergePath)
}

func (l *Loader) mergeJSON(left, right []byte) ([]byte, error) {
	if left == nil {
		return right, nil
	}
	if right == nil {
		return left, nil
	}
	if left == nil && right == nil {
		return nil, nil
	}
	leftIsNull := bytes.Equal(left, null)
	rightIsNull := bytes.Equal(right, null)
	switch {
	case leftIsNull && rightIsNull:
		return left, nil
	case !leftIsNull && rightIsNull:
		return left, nil
	case leftIsNull && !rightIsNull:
		return right, nil
	}
	ctx := fastJsonPool.Get().(*fastJsonContext)
	defer func() {
		ctx.keys = ctx.keys[:0]
		ctx.values = ctx.values[:0]
		ctx.types = ctx.types[:0]
		ctx.missingKeys = ctx.missingKeys[:0]
		ctx.missingValues = ctx.missingValues[:0]
		ctx.missingTypes = ctx.missingTypes[:0]
		fastJsonPool.Put(ctx)
	}()
	err := jsonparser.ObjectEach(left, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		ctx.keys = append(ctx.keys, key)
		ctx.values = append(ctx.values, value)
		ctx.types = append(ctx.types, dataType)
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	err = jsonparser.ObjectEach(right, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if i, exists := l.byteSliceContainsKey(ctx.keys, key); exists {
			if bytes.Equal(ctx.values[i], value) {
				return nil
			}
			switch ctx.types[i] {
			case jsonparser.Object:
				merged, err := l.mergeJSON(ctx.values[i], value)
				if err != nil {
					return errors.WithStack(err)
				}
				left, err = jsonparser.Set(left, merged, l.unsafeBytesToString(key))
				if err != nil {
					return errors.WithStack(err)
				}
			case jsonparser.String:
				update := right[offset-len(value)-2 : offset]
				left, err = jsonparser.Set(left, update, l.unsafeBytesToString(key))
				if err != nil {
					return errors.WithStack(err)
				}
			default:
				left, err = jsonparser.Set(left, value, l.unsafeBytesToString(key))
				if err != nil {
					return errors.WithStack(err)
				}
			}
			return nil
		}
		ctx.missingKeys = append(ctx.missingKeys, key)
		ctx.missingValues = append(ctx.missingValues, value)
		ctx.missingTypes = append(ctx.missingTypes, dataType)
		return nil
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if len(ctx.missingKeys) == 0 {
		return left, nil
	}
	buf := pool.BytesBuffer.Get()
	defer pool.BytesBuffer.Put(buf)
	_, _ = buf.Write(lBrace)
	for i := range ctx.missingKeys {
		_, _ = buf.Write(quote)
		_, _ = buf.Write(ctx.missingKeys[i])
		_, _ = buf.Write(quote)
		_, _ = buf.Write(colon)
		if ctx.missingTypes[i] == jsonparser.String {
			_, _ = buf.Write(quote)
		}
		_, _ = buf.Write(ctx.missingValues[i])
		if ctx.missingTypes[i] == jsonparser.String {
			_, _ = buf.Write(quote)
		}
		_, _ = buf.Write(comma)
	}
	start := bytes.Index(left, lBrace)
	_, _ = buf.Write(left[start+1:])
	combined := buf.Bytes()
	out := make([]byte, len(combined))
	copy(out, combined)
	return out, nil
}

func (l *Loader) byteSliceContainsKey(slice [][]byte, key []byte) (int, bool) {
	for i := range slice {
		if bytes.Equal(slice[i], key) {
			return i, true
		}
	}
	return -1, false
}

// unsafeBytesToString is a helper function to convert a byte slice to a string without copying the underlying data
func (l *Loader) unsafeBytesToString(bytes []byte) string {
	sliceHeader := (*reflect.SliceHeader)(unsafe.Pointer(&bytes))
	stringHeader := reflect.StringHeader{Data: sliceHeader.Data, Len: sliceHeader.Len}
	return *(*string)(unsafe.Pointer(&stringHeader)) // nolint: govet
}
