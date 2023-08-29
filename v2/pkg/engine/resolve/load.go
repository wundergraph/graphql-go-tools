package resolve

import (
	"errors"
	"io"
	"sync"

	"github.com/buger/jsonparser"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
	"golang.org/x/sync/errgroup"
)

type Loader struct {
	parallelFetch bool
	parallelMu    sync.Mutex

	layers []*layer
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
	for i := range layer.items {
		if i != 0 {
			_, _ = out.Write([]byte(`,`))
		}
		_, _ = out.Write(layer.items[i])
	}
	_, _ = out.Write([]byte(`]`))
	return out.Bytes()
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, data []byte, out io.Writer) (err error) {
	l.layers = append(l.layers, &layer{
		data: data,
		kind: layerKindObject,
	})
	err = l.resolveNode(ctx, response.Data)
	if err != nil {
		return err
	}
	_, err = out.Write(l.layers[0].data)
	return err
}

func (l *Loader) getBuffer() *fastbuffer.FastBuffer {
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
		buf := l.getBuffer()
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
		data, _, _, err = jsonparser.Get(current.items[i], path...)
		if err != nil {
			return
		}
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
		buf := l.getBuffer()
		_, _ = buf.Write([]byte(`[`))
		for i := range child.items {
			if i != 0 {
				_, _ = buf.Write([]byte(`,`))
			}
			_, _ = buf.Write(child.items[i])
		}
		_, _ = buf.Write([]byte(`]`))
		parent.data, err = jsonparser.Set(parent.data, buf.Bytes(), child.path...)
		return err
	}
	for i := range parent.items {
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
	}
	return nil
}

func (l *Loader) resolveParallelFetch(ctx *Context, fetch *ParallelFetch) (err error) {
	l.parallelFetch = true
	g, gCtx := errgroup.WithContext(ctx.ctx)
	ctx.WithContext(gCtx)
	for i := range fetch.Fetches {
		f := fetch.Fetches[i]
		g.Go(func() error {
			return l.resolveFetch(ctx, f)
		})
	}
	err = g.Wait()
	l.parallelFetch = false
	return err
}

func (l *Loader) resolveSingleFetch(ctx *Context, fetch *SingleFetch) (err error) {
	input := l.getBuffer()
	inputBuf := pool.FastBuffer.Get()
	defer pool.FastBuffer.Put(inputBuf)
	inputData := l.inputData(l.currentLayer(), inputBuf)
	err = fetch.InputTemplate.Render(ctx, inputData, input)
	if err != nil {
		return err
	}
	out := l.getBuffer()
	err = fetch.DataSource.Load(ctx.ctx, input.Bytes(), out)
	if err != nil {
		return err
	}
	data := out.Bytes()
	if fetch.ProcessResponseConfig.ExtractGraphqlResponse {
		data, _, _, err = jsonparser.Get(data, "data")
		if err != nil {
			return err
		}
	}
	if fetch.ProcessResponseConfig.ExtractFederationEntities {
		data, _, _, err = jsonparser.Get(data, "_entities")
		if err != nil {
			return err
		}
		if !l.insideArray() {
			// _entities returns an array
			// if we are not inside an array, we want to merge the entity response into the parent object
			// if we are inside an array, we will merge the entity response into the array item
			data = data[1 : len(data)-1]
		}
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
	for i := 0; i < len(layer.items); i++ {
		layer.items[i], err = jsonpatch.MergePatch(layer.items[i], dataItems[i])
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
