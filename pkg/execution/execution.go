package execution

import (
	"bytes"
	"context"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
	"strconv"
	"sync"
)

type Executor struct {
	context      Context
	out          io.Writer
	err          error
	buffers      LockableBufferMap
	instruction  Instruction
	streamBuffer bytes.Buffer
}

type LockableBufferMap struct {
	sync.Mutex
	Buffers map[uint64]*bytes.Buffer
}

func NewExecutor() *Executor {
	return &Executor{
		buffers: LockableBufferMap{
			Buffers: map[uint64]*bytes.Buffer{},
		},
	}
}

type Instruction int

const (
	KeepStreamAlive Instruction = iota + 1
	CloseConnection
)

func (e *Executor) Execute(ctx Context, node RootNode, w io.Writer) (instruction Instruction, err error) {
	e.context = ctx
	e.out = w
	e.err = nil
	var path string
	switch node.OperationType() {
	case ast.OperationTypeQuery:
		path = "query"
		e.instruction = CloseConnection
	case ast.OperationTypeMutation:
		path = "mutation"
		e.instruction = CloseConnection
	case ast.OperationTypeSubscription:
		path = "subscription"
		e.instruction = KeepStreamAlive
	}
	e.resolveNode(node, nil, path, nil, true)
	return e.instruction, e.err
}

func (e *Executor) write(data []byte) {
	if e.err != nil {
		return
	}
	_, e.err = e.out.Write(data)
}

func (e *Executor) resolveNode(node Node, data []byte, path string, prefetch *sync.WaitGroup, shouldFetch bool) {
	switch node := node.(type) {
	case *Stream:
		e.streamBuffer.Reset()
		e.instruction = node.SourceInvocation.DataSource.Resolve(e.context, e.ResolveArgs(node.SourceInvocation.Args, data, ""), &e.streamBuffer)
		if e.instruction == CloseConnection {
			return
		}
		data = e.streamBuffer.Bytes()
		e.resolveNode(node.Value, data, path, nil, true)
	case *Object:

		if data != nil && node.Path != nil {
			data, _, _, e.err = jsonparser.Get(data, node.Path...)
			if e.err == jsonparser.KeyPathNotFoundError {
				e.err = nil
				e.write(literal.NULL)
				return
			}
		}

		if shouldFetch && node.Fetch != nil {
			node.Fetch.Fetch(e.context, data, e, path, &e.buffers)
		}

		if prefetch != nil {
			prefetch.Done()
			return
		}

		if bytes.Equal(data, literal.NULL) {
			e.write(literal.NULL)
			return
		}
		e.write(literal.LBRACE)

		for i := 0; i < len(node.Fields); i++ {
			if node.Fields[i].Skip != nil {
				if node.Fields[i].Skip.Evaluate(e.context, data) {
					continue
				}
			}
			if i != 0 {
				e.write(literal.COMMA)
			}
			e.resolveNode(&node.Fields[i], data, path, nil, true)
		}
		e.write(literal.RBRACE)
	case *Field:
		path = path + "." + unsafebytes.BytesToString(node.Name)
		if node.HasResolver {
			buffer, ok := e.buffers.Buffers[xxhash.Sum64String(path)]
			if !ok {
				e.write(literal.QUOTE)
				e.write(node.Name)
				e.write(literal.QUOTE)
				e.write(literal.COLON)
				e.write(literal.NULL)
				return
			}
			data = buffer.Bytes()
		}
		e.write(literal.QUOTE)
		e.write(node.Name)
		e.write(literal.QUOTE)
		e.write(literal.COLON)
		if len(data) == 0 && !node.Value.HasResolvers() {
			e.write(literal.NULL)
			return
		}
		e.resolveNode(node.Value, data, path, nil, true)
	case *Value:
		if bytes.Equal(data, literal.NULL) {
			e.write(literal.NULL)
			return
		}
		if len(node.Path) == 0 {
			if node.QuoteValue {
				e.write(literal.QUOTE)
			}
			e.write(data)
			if node.QuoteValue {
				e.write(literal.QUOTE)
			}
			return
		}
		data, _, _, e.err = jsonparser.Get(data, node.Path...)
		if e.err == jsonparser.KeyPathNotFoundError {
			e.err = nil
			e.write(literal.NULL)
			return
		}
		if node.QuoteValue {
			e.write(literal.QUOTE)
		}
		e.write(data)
		if node.QuoteValue {
			e.write(literal.QUOTE)
		}
	case *List:
		if len(data) == 0 {
			e.write(literal.NULL)
			return
		}
		shouldPrefetch := false
		switch object := node.Value.(type) {
		case *Object:
			if object.Fetch != nil {
				shouldPrefetch = true
			}
		}
		var listItems [][]byte
		_, e.err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			listItems = append(listItems, value)
		}, node.Path...)

		path = path + "."

		if shouldPrefetch {
			wg := &sync.WaitGroup{}
			for i := 0; i < len(listItems); i++ {
				wg.Add(1)
				go e.resolveNode(node.Value, listItems[i], path+strconv.Itoa(i), wg, true)
			}
			wg.Wait()
		}
		i := 0
		for i = 0; i < len(listItems); i++ {
			if i == 0 {
				e.write(literal.LBRACK)
			} else {
				e.write(literal.COMMA)
			}
			e.resolveNode(node.Value, listItems[i], path+strconv.Itoa(i), nil, false)
		}
		if i == 0 || e.err == jsonparser.KeyPathNotFoundError {
			e.err = nil
			e.write(literal.LBRACK)
		}
		e.write(literal.RBRACK)
	}
}

func (e *Executor) ResolveArgs(args []Argument, data []byte, prefix string) ResolvedArgs {
	/*
		TODO: optimize later
		var resolved ResolvedArgs
		if len(e.args) >= len(args) {
			resolved = e.args[:len(args)]
		} else {
			resolved = make(ResolvedArgs, len(args))
		}
	*/

	resolved := make(ResolvedArgs, len(args))
	for i := 0; i < len(args); i++ {
		switch arg := args[i].(type) {
		case *StaticVariableArgument:
			resolved[i].Key = arg.Name
			resolved[i].Value = arg.Value
		case *ObjectVariableArgument:
			resolved[i].Key = arg.Name
			resolved[i].Value, _, _, _ = jsonparser.Get(data, arg.Path...)
		case *ContextVariableArgument:
			resolved[i].Key = arg.Name
			resolved[i].Value = e.context.Variables[xxhash.Sum64(arg.VariableName)]
		}
	}
	return resolved
}

const (
	ObjectKind NodeKind = iota + 1
	FieldKind
	ListKind
	ValueKind
	StreamKind
)

type NodeKind int

type Node interface {
	Kind() NodeKind
	HasResolvers() bool
}

type RootNode interface {
	Node
	OperationType() ast.OperationType
}

type Context struct {
	context.Context
	Variables Variables
}

type Variables map[uint64][]byte

type Argument interface {
	ArgName() []byte
}

type ResolvedArgument struct {
	Key   []byte
	Value []byte
}

type ResolvedArgs []ResolvedArgument

func (r ResolvedArgs) ByKey(key []byte) []byte {
	for i := 0; i < len(r); i++ {
		if bytes.Equal(r[i].Key, key) {
			return r[i].Value
		}
	}
	return nil
}

func (r ResolvedArgs) Dump() []string {
	out := make([]string, len(r))
	for i := range r {
		out[i] = string(r[i].Key) + "=" + string(r[i].Value)
	}
	return out
}

type ContextVariableArgument struct {
	Name         []byte
	VariableName []byte
}

func (c *ContextVariableArgument) ArgName() []byte {
	return c.Name
}

type ObjectVariableArgument struct {
	Name []byte
	Path []string
}

func (o *ObjectVariableArgument) ArgName() []byte {
	return o.Name
}

type StaticVariableArgument struct {
	Name  []byte
	Value []byte
}

func (s *StaticVariableArgument) ArgName() []byte {
	return s.Name
}

type Object struct {
	Fields        []Field
	Path          []string
	Fetch         Fetch
	operationType ast.OperationType
}

func (o *Object) OperationType() ast.OperationType {
	return o.operationType
}

type ArgsResolver interface {
	ResolveArgs(args []Argument, data []byte, prefix string) ResolvedArgs
}

type Fetch interface {
	Fetch(ctx Context, data []byte, argsResolver ArgsResolver, suffix string, buffers *LockableBufferMap)
}

type SingleFetch struct {
	Source     *DataSourceInvocation
	BufferName string
	mu         sync.Mutex
}

func (s *SingleFetch) Fetch(ctx Context, data []byte, argsResolver ArgsResolver, path string, buffers *LockableBufferMap) {
	bufferName := path + "." + s.BufferName
	hash := xxhash.Sum64String(bufferName)
	buffer, exists := buffers.Buffers[hash]
	if !exists {
		buffer = bytes.NewBuffer(make([]byte, 0, 1024))
		buffers.Lock()
		buffers.Buffers[hash] = buffer
		buffers.Unlock()
	}
	s.Source.DataSource.Resolve(ctx, argsResolver.ResolveArgs(s.Source.Args, data, s.BufferName), buffer)
}

type SerialFetch struct {
	Fetches []Fetch
}

func (s *SerialFetch) Fetch(ctx Context, data []byte, argsResolver ArgsResolver, suffix string, buffers *LockableBufferMap) {
	for i := 0; i < len(s.Fetches); i++ {
		s.Fetches[i].Fetch(ctx, data, argsResolver, suffix, buffers)
	}
}

type ParallelFetch struct {
	wg      sync.WaitGroup
	Fetches []Fetch
}

func (p *ParallelFetch) Fetch(ctx Context, data []byte, argsResolver ArgsResolver, suffix string, buffers *LockableBufferMap) {
	for i := 0; i < len(p.Fetches); i++ {
		p.wg.Add(1)
		go func(fetch Fetch, ctx Context, data []byte, argsResolver ArgsResolver) {
			fetch.Fetch(ctx, data, argsResolver, suffix, buffers)
			p.wg.Done()
		}(p.Fetches[i], ctx, data, argsResolver)
	}
	p.wg.Wait()
}

func (o *Object) HasResolvers() bool {
	for i := 0; i < len(o.Fields); i++ {
		if o.Fields[i].HasResolvers() {
			return true
		}
	}
	return false
}

func (*Object) Kind() NodeKind {
	return ObjectKind
}

type Stream struct {
	SourceInvocation *DataSourceInvocation
	Value            Node
	operationType    ast.OperationType
}

func (s *Stream) OperationType() ast.OperationType {
	return s.operationType
}

func (s *Stream) Kind() NodeKind {
	return StreamKind
}

func (s *Stream) HasResolvers() bool {
	if s.SourceInvocation != nil {
		return true
	}
	return s.Value.HasResolvers()
}

type BooleanCondition interface {
	Evaluate(ctx Context, data []byte) bool
}

type Field struct {
	Name        []byte
	Value       Node
	Skip        BooleanCondition
	HasResolver bool
}

func (f *Field) HasResolvers() bool {
	return f.HasResolver || f.Value.HasResolvers()
}

type IfEqual struct {
	Left, Right Argument
}

func (i *IfEqual) Evaluate(ctx Context, data []byte) bool {
	var left []byte
	var right []byte

	switch value := i.Left.(type) {
	case *ContextVariableArgument:
		left = ctx.Variables[xxhash.Sum64(value.VariableName)]
	case *ObjectVariableArgument:
		left, _, _, _ = jsonparser.Get(data, value.Path...)
	case *StaticVariableArgument:
		left = value.Value
	}

	switch value := i.Right.(type) {
	case *ContextVariableArgument:
		right = ctx.Variables[xxhash.Sum64(value.VariableName)]
	case *ObjectVariableArgument:
		right, _, _, _ = jsonparser.Get(data, value.Path...)
	case *StaticVariableArgument:
		right = value.Value
	}

	return bytes.Equal(left, right)
}

type IfNotEqual struct {
	Left, Right Argument
}

func (i *IfNotEqual) Evaluate(ctx Context, data []byte) bool {
	equal := IfEqual{
		Left:  i.Left,
		Right: i.Right,
	}
	return !equal.Evaluate(ctx, data)
}

func (*Field) Kind() NodeKind {
	return FieldKind
}

type Value struct {
	Path       []string
	QuoteValue bool
}

func (value *Value) HasResolvers() bool {
	return false
}

func (*Value) Kind() NodeKind {
	return ValueKind
}

type List struct {
	Path  []string
	Value Node
}

func (l *List) HasResolvers() bool {
	return l.Value.HasResolvers()
}

func (*List) Kind() NodeKind {
	return ListKind
}

type DataSourceInvocation struct {
	Args       []Argument
	DataSource DataSource
}
