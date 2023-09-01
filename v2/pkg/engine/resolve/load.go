package resolve

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/buger/jsonparser"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"golang.org/x/sync/errgroup"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

var (
	ErrOriginResponseError = errors.New("origin response error")
)

type Loader struct {
	parallelFetch bool
	parallelMu    sync.Mutex

	layers []*layer

	errors []byte
}

type layer struct {
	path    []string
	data    []byte
	items   [][]byte
	kind    layerKind
	buffers []*fastbuffer.FastBuffer
}

type layerKind int

const (
	layerKindObject layerKind = iota + 1
	layerKindArray
)

func (l *Loader) popLayer() {
	last := l.layers[len(l.layers)-1]
	for i := range last.buffers {
		pool.FastBuffer.Put(last.buffers[i])
	}
	l.layers = l.layers[:len(l.layers)-1]
}

func (l *Loader) inputData(layer *layer, out *fastbuffer.FastBuffer) []byte {
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
			_, err = out.Write([]byte(`{"errors":`))
			_, err = out.Write(l.errors)
			_, err = out.Write([]byte(`}`))
			return true, err
		}
		return false, err
	}
	_, err = out.Write(l.layers[0].data)
	return false, err
}

// getLayerBuffer returns a buffer that will live as long as the current layer
// it won't be re-used before the current layer is popped
func (l *Loader) getLayerBuffer() *fastbuffer.FastBuffer {
	buf := pool.FastBuffer.Get()
	if l.parallelFetch {
		l.parallelMu.Lock()
	}
	l.currentLayer().buffers = append(l.currentLayer().buffers, buf)
	if l.parallelFetch {
		l.parallelMu.Unlock()
	}
	return buf
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

func (l *Loader) resolveLayerData(path []string, isArray bool) (data []byte, items [][]byte, err error) {
	current := l.currentLayer()
	if !l.insideArray() && !isArray {
		buf := l.getLayerBuffer()
		_, _ = buf.Write(current.data)
		data = buf.Bytes()
		data, _, _, err = jsonparser.Get(data, path...)
		if errors.Is(err, jsonparser.KeyPathNotFoundError) {
			// we have no data for this path which is legit
			return nil, nil, nil
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
		return nil, items, err
	}
	for i := range current.items {
		data, _, _, _ = jsonparser.Get(current.items[i], path...)
		// we explicitly ignore the error and just append a nil slice
		items = append(items, data)
	}
	return nil, items, nil
}

func (l *Loader) resolveArray(ctx *Context, array *Array) (err error) {
	if !array.HasChildFetches() {
		return nil
	}
	if len(array.Path) != 0 {
		data, items, err := l.resolveLayerData(array.Path, true)
		if err != nil {
			return err
		}
		next := &layer{
			path:  array.Path,
			data:  data,
			items: items,
			kind:  layerKindArray,
		}
		l.layers = append(l.layers, next)
	}
	err = l.resolveNode(ctx, array.Item)
	if err != nil {
		return err
	}
	if len(array.Path) != 0 {
		err = l.mergeLayerIntoParent()
		if err != nil {
			return err
		}
		l.popLayer()
	}
	return nil
}

func (l *Loader) resolveObject(ctx *Context, object *Object) (err error) {
	hasChildFetches := object.HasChildFetches()
	if object.Fetch == nil && !hasChildFetches {
		return nil
	}
	if len(object.Path) != 0 {
		data, items, err := l.resolveLayerData(object.Path, false)
		if err != nil {
			return err
		}
		next := &layer{
			path:  object.Path,
			data:  data,
			items: items,
			kind:  layerKindObject,
		}
		if l.insideArray() {
			next.kind = layerKindArray
		}
		l.layers = append(l.layers, next)
	}
	if object.Fetch != nil {
		err = l.resolveFetch(ctx, object.Fetch)
		if err != nil {
			return err
		}
	}
	if hasChildFetches {
		for i := range object.Fields {
			err = l.resolveNode(ctx, object.Fields[i].Value)
			if err != nil {
				return err
			}
		}
	}
	if len(object.Path) != 0 {
		err = l.mergeLayerIntoParent()
		if err != nil {
			return err
		}
		l.popLayer()
	}
	return nil
}

func (l *Loader) mergeLayerIntoParent() (err error) {
	child := l.layers[len(l.layers)-1]
	parent := l.layers[len(l.layers)-2]
	if parent.kind == layerKindObject && child.kind == layerKindObject {
		patch, err := jsonpatch.MergePatch(parent.data, child.data)
		if err != nil {
			return err
		}
		parent.data, err = jsonparser.Set(parent.data, patch, child.path...)
		return err
	}
	if parent.kind == layerKindObject && child.kind == layerKindArray {
		buf := l.getLayerBuffer()
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
		return err
	}
	for i := range parent.items {
		if child.items[i] == nil {
			continue
		}
		existing, _, _, _ := jsonparser.Get(parent.items[i], child.path...)
		combined, err := jsonpatch.MergePatch(existing, child.items[i])
		if err != nil {
			return err
		}
		parent.items[i], err = jsonparser.Set(parent.items[i], combined, child.path...)
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) resolveFetch(ctx *Context, fetch Fetch) (err error) {
	switch f := fetch.(type) {
	case *SingleFetch:
		return l.resolveSingleFetch(ctx, f)
	case *SerialFetch:
		return l.resolveSerialFetch(ctx, f)
	case *ParallelFetch:
		return l.resolveParallelFetch(ctx, f)
	case *ParallelListItemFetch:
		return l.resolveParallelListItemFetch(ctx, f)
	case *BatchFetch:
		return l.resolveBatchFetch(ctx, f)
	}
	return nil
}

func (l *Loader) resolveBatchFetch(ctx *Context, fetch *BatchFetch) (err error) {
	input := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(input)
	inputBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(inputBuf)
	out := l.getLayerBuffer()

	lr := l.currentLayer()
	err = fetch.Input.Header.Render(ctx, nil, input)
	if err != nil {
		return err
	}
	addSeparator := false
	batchStats := make([][]int, len(lr.items))
	batchItemIndex := 0

	itemBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(itemBuf)

	for i := range lr.items {
		if lr.items[i] == nil {
			continue
		}
		for j := range fetch.Input.Items {
			if addSeparator {
				err = fetch.Input.Separator.Render(ctx, nil, input)
				if err != nil {
					return err
				}
			}
			itemBuf.Reset()
			err = fetch.Input.Items[j].Render(ctx, lr.items[i], itemBuf)
			if err != nil {
				if fetch.Input.SkipErrItems {
					err = nil
					batchStats[i] = append(batchStats[i], -1)
					addSeparator = false
					continue
				}
				return err
			}
			if fetch.Input.SkipNullItems && itemBuf.Len() == 4 && bytes.Equal(itemBuf.Bytes(), null) {
				batchStats[i] = append(batchStats[i], -1)
				addSeparator = false
				continue
			}
			input.WriteBytes(itemBuf.Bytes())
			batchStats[i] = append(batchStats[i], batchItemIndex)
			batchItemIndex++
			if !addSeparator {
				addSeparator = true
			}
		}
	}
	err = fetch.Input.Footer.Render(ctx, nil, input)
	if err != nil {
		return err
	}
	err = fetch.DataSource.Load(ctx.ctx, input.Bytes(), out)
	if err != nil {
		return err
	}
	data := out.Bytes()
	responseErrors, _, _, _ := jsonparser.Get(data, "errors")
	if responseErrors != nil {
		l.errors = responseErrors
		return ErrOriginResponseError
	}
	if fetch.PostProcessing.SelectResponseDataPath != nil {
		data, _, _, err = jsonparser.Get(data, fetch.PostProcessing.SelectResponseDataPath...)
		if err != nil {
			return err
		}
	}
	var (
		batchResponseItems [][]byte
	)
	_, err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		batchResponseItems = append(batchResponseItems, value)
	})
	if err != nil {
		return err
	}
	itemsData := make([][]byte, len(lr.items))
	for i, stats := range batchStats {
		buf := l.getLayerBuffer()
		buf.WriteBytes(lBrack)
		addCommaSeparator := false
		for j := range stats {
			if addCommaSeparator {
				buf.WriteBytes(comma)
			} else {
				addCommaSeparator = true
			}
			if stats[j] == -1 {
				buf.WriteBytes(null)
				continue
			}
			_, err = buf.Write(batchResponseItems[stats[j]])
			if err != nil {
				return err
			}
		}
		buf.WriteBytes(rBrack)
		itemsData[i] = buf.Bytes()
	}
	if fetch.PostProcessing.ResponseTemplate != nil {
		for i := range itemsData {
			out := l.getLayerBuffer()
			err = fetch.PostProcessing.ResponseTemplate.Render(ctx, itemsData[i], out)
			if err != nil {
				return err
			}
			itemsData[i] = out.Bytes()
		}
	}
	for i := range lr.items {
		if lr.items[i] == nil {
			continue
		}
		lr.items[i], err = jsonpatch.MergePatch(lr.items[i], itemsData[i])
		if err != nil {
			return err
		}
	}
	return
}

func (l *Loader) resolveParallelListItemFetch(ctx *Context, fetch *ParallelListItemFetch) (err error) {
	if !l.insideArray() {
		return fmt.Errorf("resolveParallelListItemFetch must be inside an array, this seems to be a bug in the planner")
	}
	layer := l.currentLayer()
	group, gCtx := errgroup.WithContext(ctx.ctx)
	l.parallelFetch = true
	defer func() {
		l.parallelFetch = false
	}()
	groupContext := ctx.WithContext(gCtx)
	for i := range layer.items {
		i := i
		// get a buffer before we start the goroutines
		// getLayerBuffer will append the buffer to the list of buffers of the current layer
		// this will ensure that the buffer is not re-used before this layer is merged into the parent
		// however, appending is not concurrency safe, so we need to do it before we start the goroutines
		out := l.getLayerBuffer()
		group.Go(func() error {
			input := pool.FastBuffer.Get()
			defer pool.FastBuffer.Put(input)
			err = fetch.Fetch.InputTemplate.Render(ctx, layer.items[i], input)
			if err != nil {
				return err
			}
			data, err := l.loadAndPostProcess(groupContext, input, fetch.Fetch, out)
			if err != nil {
				return err
			}
			layer.items[i], err = jsonpatch.MergePatch(layer.items[i], data)
			return err
		})
	}
	err = group.Wait()
	return err
}

func (l *Loader) resolveParallelFetch(ctx *Context, fetch *ParallelFetch) (err error) {
	l.parallelFetch = true
	group, grouptContext := errgroup.WithContext(ctx.ctx)
	groupCtx := ctx.WithContext(grouptContext)
	for i := range fetch.Fetches {
		f := fetch.Fetches[i]
		group.Go(func() error {
			return l.resolveFetch(groupCtx, f)
		})
	}
	err = group.Wait()
	l.parallelFetch = false
	return err
}

func (l *Loader) resolveSingleFetch(ctx *Context, fetch *SingleFetch) (err error) {
	input := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(input)
	inputBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(inputBuf)
	out := l.getLayerBuffer()
	inputData := l.inputData(l.currentLayer(), inputBuf)
	err = fetch.InputTemplate.Render(ctx, inputData, input)
	if err != nil {
		return err
	}
	data, err := l.loadAndPostProcess(ctx, input, fetch, out)
	if err != nil {
		return err
	}
	if l.parallelFetch {
		l.parallelMu.Lock()
	}
	err = l.mergeDataIntoLayer(l.currentLayer(), data)
	if err != nil {
		return err
	}
	if l.parallelFetch {
		l.parallelMu.Unlock()
	}
	if err != nil {
		return err
	}
	return
}

func (l *Loader) loadAndPostProcess(ctx *Context, input *fastbuffer.FastBuffer, fetch *SingleFetch, out *fastbuffer.FastBuffer) (data []byte, err error) {
	err = fetch.DataSource.Load(ctx.ctx, input.Bytes(), out)
	if err != nil {
		return nil, err
	}
	data = out.Bytes()
	responseErrors, _, _, _ := jsonparser.Get(data, "errors")
	if responseErrors != nil {
		if l.parallelFetch {
			l.parallelMu.Lock()
		}
		l.errors = responseErrors
		if l.parallelFetch {
			l.parallelMu.Unlock()
		}
		return nil, ErrOriginResponseError
	}
	if fetch.PostProcessing.SelectResponseDataPath != nil {
		data, _, _, err = jsonparser.Get(data, fetch.PostProcessing.SelectResponseDataPath...)
		if err != nil {
			return nil, err
		}
	}
	if fetch.PostProcessing.ResponseTemplate != nil {
		intermediate := pool.FastBuffer.Get()
		defer pool.FastBuffer.Put(intermediate)
		_, err = intermediate.Write(data)
		if err != nil {
			return nil, err
		}
		out.Reset()
		err = fetch.PostProcessing.ResponseTemplate.Render(ctx, intermediate.Bytes(), out)
		if err != nil {
			return nil, err
		}
		data = out.Bytes()
	}
	return data, nil
}

func (l *Loader) mergeDataIntoLayer(layer *layer, data []byte) (err error) {
	if layer.kind == layerKindObject {
		if layer.data == nil {
			layer.data = data
			return nil
		}
		layer.data, err = jsonpatch.MergePatch(layer.data, data)
		return err
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
		return err
	}
	skipped := 0
	for i := 0; i < len(layer.items); i++ {
		if layer.items[i] == nil {
			skipped++
			continue
		}
		layer.items[i], err = jsonpatch.MergePatch(layer.items[i], dataItems[i-skipped])
		if err != nil {
			return err
		}
	}
	return nil
}

func (l *Loader) resolveSerialFetch(ctx *Context, fetch *SerialFetch) (err error) {
	for i := range fetch.Fetches {
		err = l.resolveFetch(ctx, fetch.Fetches[i])
		if err != nil {
			return err
		}
	}
	return nil
}
