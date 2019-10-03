package execution

import (
	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/internal/pkg/unsafebytes"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"io"
)

type Executor struct {
	Context Context
	out     io.Writer
	err     error
}

func (e *Executor) Execute(ctx Context, node Node, w io.Writer) error {
	e.Context = ctx
	e.out = w
	e.err = nil
	e.resolveNode(node, nil)
	return e.err
}

func (e *Executor) write(data []byte) {
	if e.err != nil {
		return
	}
	_, e.err = e.out.Write(data)
}

func (e *Executor) resolveNode(node Node, data []byte) {
	switch node := node.(type) {
	case *Object:
		if data != nil && len(node.Path) != 0 {
			data, _, _, e.err = jsonparser.Get(data, unsafebytes.BytesToString(node.Path))
		}
		e.write(literal.LBRACE)
		for i := 0; i < len(node.Fields); i++ {
			if i != 0 {
				e.write(literal.COMMA)
			}
			e.resolveNode(&node.Fields[i], data)
		}
		e.write(literal.RBRACE)
	case *Field:
		if node.Resolve != nil {
			data = node.Resolve.Resolver.Resolve(e.Context, node.Resolve.Args)
		}
		e.write(literal.QUOTE)
		e.write(node.Name)
		e.write(literal.QUOTE)
		e.write(literal.COLON)
		e.resolveNode(node.Value, data)
	case *Value:
		var dataType jsonparser.ValueType
		data, dataType, _, e.err = jsonparser.Get(data, unsafebytes.BytesToString(node.Path))
		quote := dataType != jsonparser.Boolean && dataType != jsonparser.Number
		if quote {
			e.write(literal.QUOTE)
		}
		e.write(data)
		if quote {
			e.write(literal.QUOTE)
		}
	case *List:
		e.write(literal.LBRACK)
		first := true
		_, e.err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			if first {
				first = !first
			} else {
				e.write(literal.COMMA)
			}
			e.resolveNode(node.Value, value)
		}, unsafebytes.BytesToString(node.Path))
		e.write(literal.RBRACK)
	}
}
