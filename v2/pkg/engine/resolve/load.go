package resolve

import (
	"io"
	"sync"

	"github.com/buger/jsonparser"
	jsonpatch "github.com/evanphx/json-patch/v5"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
	"golang.org/x/sync/errgroup"
)

type Loader struct {
	path      []string
	mergePath []string
	data      []byte
	pathData  []byte
	buffers   []*fastbuffer.FastBuffer

	parallelFetch bool
	parallelMu    sync.Mutex
}

func (l *Loader) LoadGraphQLResponseData(ctx *Context, response *GraphQLResponse, data []byte, out io.Writer) (err error) {
	err = l.resolveNode(ctx, response.Data)
	if err != nil {
		return err
	}
	_, err = out.Write(l.data)
	return err
}

func (l *Loader) Free() {
	for i := range l.buffers {
		pool.FastBuffer.Put(l.buffers[i])
	}
	l.buffers = l.buffers[:0]
}

func (l *Loader) getBuffer() *fastbuffer.FastBuffer {
	buf := pool.FastBuffer.Get()
	if l.parallelFetch {
		l.parallelMu.Lock()
	}
	l.buffers = append(l.buffers, buf)
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

func (l *Loader) resolveArray(ctx *Context, array *Array) (err error) {
	if len(array.Path) != 0 {
		l.path = append(l.path, array.Path...)
	}
	err = l.resolveNode(ctx, array.Item)
	if err != nil {
		return err
	}
	if len(array.Path) != 0 {
		l.path = l.path[:len(l.path)-len(array.Path)]
	}
	return nil
}

func (l *Loader) resolveObject(ctx *Context, object *Object) (err error) {
	if len(object.Path) != 0 {
		err = l.mergeIntoParent()
		if err != nil {
			return err
		}
		l.path = append(l.path, object.Path...)
		if l.pathData == nil {
			l.pathData = l.data
		}
		l.pathData, _, _, err = jsonparser.Get(l.pathData, object.Path...)
		if err != nil {
			return err
		}
	}
	if object.Fetch != nil {
		err = l.resolveFetch(ctx, object.Fetch)
		if err != nil {
			return err
		}
	}
	for i := range object.Fields {
		err = l.resolveNode(ctx, object.Fields[i].Value)
		if err != nil {
			return err
		}
	}
	if len(object.Path) != 0 {
		err = l.mergeIntoParent()
		if err != nil {
			return err
		}
		l.path = l.path[:len(l.path)-len(object.Path)]
	}
	return nil
}

func (l *Loader) mergeIntoParent() (err error) {
	if len(l.path) != len(l.mergePath) {
		return nil
	}
	for i := range l.path {
		if l.path[i] != l.mergePath[i] {
			return nil
		}
	}
	if len(l.data) == 0 && len(l.pathData) != 0 {
		l.data = l.pathData
		l.mergePath = l.mergePath[:0]
		return nil
	}
	l.data, err = jsonparser.Set(l.data, l.pathData, l.path...)
	if err != nil {
		return err
	}
	l.mergePath = l.mergePath[:0]
	return nil
}

func (l *Loader) resolveFetch(ctx *Context, fetch Fetch) (err error) {
	switch f := fetch.(type) {
	case *SingleFetch:
		return l.resolveSingleFetch(ctx, f)
	case *EntityFetch:
		return l.resolveEntityFetch(ctx, f)
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
	err = fetch.InputTemplate.Render(ctx, l.pathData, input)
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
	if l.parallelFetch {
		l.parallelMu.Lock()
	}
	if l.pathData == nil {
		l.pathData = data
	} else {
		l.pathData, err = jsonpatch.MergePatch(l.pathData, data)
		l.mergePath = l.path
	}
	if l.parallelFetch {
		l.parallelMu.Unlock()
	}
	if err != nil {
		return err
	}
	return
}

func (l *Loader) resolveEntityFetch(ctx *Context, fetch *EntityFetch) (err error) {
	err = l.resolveFetch(ctx, fetch.Fetch)
	if err != nil {
		return err
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
