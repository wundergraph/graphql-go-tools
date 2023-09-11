package resolve

import (
	"bytes"
	"context"
	"net/http"
	"strconv"

	"github.com/wundergraph/graphql-go-tools/v2/internal/pkg/unsafebytes"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/pool"
)

type Context struct {
	ctx              context.Context
	Variables        []byte
	Request          Request
	pathElements     [][]byte
	responseElements []string
	usedBuffers      []*bytes.Buffer
	pathPrefix       []byte
	beforeFetchHook  BeforeFetchHook
	afterFetchHook   AfterFetchHook
	position         Position
	RenameTypeNames  []RenameTypeName
}

type Request struct {
	Header http.Header
}

func NewContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("nil context.Context")
	}
	return &Context{
		ctx:          ctx,
		Variables:    make([]byte, 0, 4096),
		pathPrefix:   make([]byte, 0, 4096),
		pathElements: make([][]byte, 0, 16),
		usedBuffers:  make([]*bytes.Buffer, 0, 48),
		position:     Position{},
	}
}

func (c *Context) Context() context.Context {
	return c.ctx
}

func (c *Context) WithContext(ctx context.Context) *Context {
	if ctx == nil {
		panic("nil context.Context")
	}
	cpy := *c
	cpy.ctx = ctx
	return &cpy
}

func (c *Context) clone() Context {
	variables := make([]byte, len(c.Variables))
	copy(variables, c.Variables)
	pathPrefix := make([]byte, len(c.pathPrefix))
	copy(pathPrefix, c.pathPrefix)
	pathElements := make([][]byte, len(c.pathElements))
	for i := range pathElements {
		pathElements[i] = make([]byte, len(c.pathElements[i]))
		copy(pathElements[i], c.pathElements[i])
	}
	return Context{
		ctx:             c.ctx,
		Variables:       variables,
		Request:         c.Request,
		pathElements:    pathElements,
		usedBuffers:     make([]*bytes.Buffer, 0, 48),
		pathPrefix:      pathPrefix,
		beforeFetchHook: c.beforeFetchHook,
		afterFetchHook:  c.afterFetchHook,
		position:        c.position,
	}
}

func (c *Context) Free() {
	c.ctx = nil
	c.Variables = c.Variables[:0]
	c.pathPrefix = c.pathPrefix[:0]
	c.pathElements = c.pathElements[:0]
	for i := range c.usedBuffers {
		pool.BytesBuffer.Put(c.usedBuffers[i])
	}
	c.usedBuffers = c.usedBuffers[:0]
	c.beforeFetchHook = nil
	c.afterFetchHook = nil
	c.Request.Header = nil
	c.position = Position{}
	c.RenameTypeNames = nil
}

func (c *Context) SetBeforeFetchHook(hook BeforeFetchHook) {
	c.beforeFetchHook = hook
}

func (c *Context) SetAfterFetchHook(hook AfterFetchHook) {
	c.afterFetchHook = hook
}

func (c *Context) setPosition(position Position) {
	c.position = position
}

func (c *Context) addResponseElements(elements []string) {
	c.responseElements = append(c.responseElements, elements...)
}

func (c *Context) addResponseArrayElements(elements []string) {
	c.responseElements = append(c.responseElements, elements...)
}

func (c *Context) removeResponseLastElements(elements []string) {
	c.responseElements = c.responseElements[:len(c.responseElements)-len(elements)]
}
func (c *Context) removeResponseArrayLastElements(elements []string) {
	c.responseElements = c.responseElements[:len(c.responseElements)-(len(elements))]
}

func (c *Context) resetResponsePathElements() {
	c.responseElements = nil
}

func (c *Context) addPathElement(elem []byte) {
	c.pathElements = append(c.pathElements, elem)
}

func (c *Context) addIntegerPathElement(elem int) {
	b := unsafebytes.StringToBytes(strconv.Itoa(elem))
	c.pathElements = append(c.pathElements, b)
}

func (c *Context) removeLastPathElement() {
	c.pathElements = c.pathElements[:len(c.pathElements)-1]
}

func (c *Context) path() []byte {
	buf := pool.BytesBuffer.Get()
	c.usedBuffers = append(c.usedBuffers, buf)
	if len(c.pathPrefix) != 0 {
		buf.Write(c.pathPrefix)
	} else {
		buf.Write(literal.SLASH)
		buf.Write(literal.DATA)
	}
	for i := range c.pathElements {
		if i == 0 && bytes.Equal(literal.DATA, c.pathElements[0]) {
			continue
		}
		_, _ = buf.Write(literal.SLASH)
		_, _ = buf.Write(c.pathElements[i])
	}
	return buf.Bytes()
}

type HookContext struct {
	CurrentPath []byte
}

type BeforeFetchHook interface {
	OnBeforeFetch(ctx HookContext, input []byte)
}

type AfterFetchHook interface {
	OnData(ctx HookContext, output []byte, singleFlight bool)
	OnError(ctx HookContext, output []byte, singleFlight bool)
}
