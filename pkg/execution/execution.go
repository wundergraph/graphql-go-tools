package execution

import (
	"bytes"
	"fmt"
	"github.com/buger/jsonparser"
	"github.com/cespare/xxhash"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
	"strconv"
	"sync"
)

type Executor struct {
	context Context
	out     io.Writer
	err     error
	buffers map[uint64]*bytes.Buffer
}

func NewExecutor() *Executor {
	return &Executor{
		buffers: map[uint64]*bytes.Buffer{},
	}
}

type Instruction int

const (
	KeepStream Instruction = iota + 1
	CloseConnection
)

func (e *Executor) Execute(ctx Context, node Node, w io.Writer) (instruction Instruction, err error) {
	e.context = ctx
	e.out = w
	e.err = nil
	e.resolveNode(node, nil, "query")
	return instruction, e.err
}

func (e *Executor) write(data []byte) {
	if e.err != nil {
		return
	}
	_, e.err = e.out.Write(data)
}

func (e *Executor) resolveNode(node Node, data []byte, path string) {
	switch node := node.(type) {
	case *Stream:
		for {
			buf := bytes.Buffer{}
			node.SourceInvocation.DataSource.Resolve(e.context, e.ResolveArgs(node.SourceInvocation.Args, data), &buf)
			data = buf.Bytes()
			e.resolveNode(node.Value, data, path)
		}
	case *Object:
		if data != nil && node.Path != nil {
			data, _, _, e.err = jsonparser.Get(data, node.Path...)
			if e.err == jsonparser.KeyPathNotFoundError {
				e.err = nil
				e.write(literal.NULL)
				return
			}
		}
		if bytes.Equal(data, literal.NULL) {
			e.write(literal.NULL)
			return
		}
		e.write(literal.LBRACE)

		if node.Fetch != nil {
			node.Fetch.Fetch(e.context, data, e, path, &e.buffers)
		}

		for i := 0; i < len(node.Fields); i++ {
			if node.Fields[i].Skip != nil {
				if node.Fields[i].Skip.Evaluate(e.context, data) {
					continue
				}
			}
			if i != 0 {
				e.write(literal.COMMA)
			}
			e.resolveNode(&node.Fields[i], data, path)
		}
		e.write(literal.RBRACE)
	case *Field:
		path = path + "." + unsafebytes.BytesToString(node.Name)
		if node.BufferName != "" {
			//data = node.Resolve.DataSource.Resolve(e.context, e.ResolveArgs(node.Resolve.Args, data))
			//  node.ResolvedData.Bytes()
			fmt.Printf("accessing buffer: \"%s\"\n", path)
			buffer, ok := e.buffers[xxhash.Sum64String(path)]
			if !ok {
				fmt.Printf("Buffer not found for key: \"%s\"\n", path)
				e.write(literal.QUOTE)
				e.write(node.Name)
				e.write(literal.QUOTE)
				e.write(literal.COLON)
				e.write(literal.NULL)
				return
			}
			data = buffer.Bytes()
		}
		strData, nodeName := string(data), string(node.Name)
		_, _ = strData, nodeName
		e.write(literal.QUOTE)
		e.write(node.Name)
		e.write(literal.QUOTE)
		e.write(literal.COLON)
		if len(data) == 0 && !node.Value.HasResolvers() {
			e.write(literal.NULL)
			return
		}
		e.resolveNode(node.Value, data, path)
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
		path = path + "."
		i := 0
		_, e.err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			if i == 0 {
				e.write(literal.LBRACK)
			} else {
				e.write(literal.COMMA)
			}
			e.resolveNode(node.Value, value, path+strconv.Itoa(i))
			i++
		}, node.Path...)
		if i == 0 || e.err == jsonparser.KeyPathNotFoundError {
			e.err = nil
			e.write(literal.LBRACK)
		}
		e.write(literal.RBRACK)
	}
}

func (e *Executor) ResolveArgs(args []Argument, data []byte) ResolvedArgs {
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

type Context struct {
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

func (a ResolvedArgs) ByKey(key []byte) []byte {
	for i := 0; i < len(a); i++ {
		if bytes.Equal(a[i].Key, key) {
			return a[i].Value
		}
	}
	return nil
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
	Fields []Field
	Path   []string
	Fetch  Fetch
}

type ArgsResolver interface {
	ResolveArgs(args []Argument, data []byte) ResolvedArgs
}

type Fetch interface {
	Fetch(ctx Context, data []byte, argsResolver ArgsResolver, suffix string, buffers *map[uint64]*bytes.Buffer)
}

type SingleFetch struct {
	Source     *DataSourceInvocation
	BufferName string
	mu         sync.Mutex
}

func (s *SingleFetch) Fetch(ctx Context, data []byte, argsResolver ArgsResolver, path string, buffers *map[uint64]*bytes.Buffer) {
	bufferName := path + "." + s.BufferName
	hash := xxhash.Sum64String(bufferName)
	buffer, exists := (*buffers)[hash]
	if !exists {
		buffer = bytes.NewBuffer(make([]byte, 0, 1024))
		s.mu.Lock()
		(*buffers)[hash] = buffer
		s.mu.Unlock()
	}
	s.Source.DataSource.Resolve(ctx, argsResolver.ResolveArgs(s.Source.Args, data), buffer)
	fmt.Printf("setting buffer: \"%s\" len: %d\n", bufferName, buffer.Len())
}

type SerialFetch struct {
	Fetches []Fetch
}

func (s *SerialFetch) Fetch(ctx Context, data []byte, argsResolver ArgsResolver, suffix string, buffers *map[uint64]*bytes.Buffer) {
	for i := 0; i < len(s.Fetches); i++ {
		s.Fetches[i].Fetch(ctx, data, argsResolver, suffix, buffers)
	}
}

type ParallelFetch struct {
	wg      sync.WaitGroup
	Fetches []Fetch
}

func (p *ParallelFetch) Fetch(ctx Context, data []byte, argsResolver ArgsResolver, suffix string, buffers *map[uint64]*bytes.Buffer) {
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
	Name  []byte
	Value Node
	//Resolve      *DataSourceInvocation
	Skip       BooleanCondition
	BufferName string
}

func (f *Field) HasResolvers() bool {
	return f.BufferName != "" || f.Value.HasResolvers()
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
