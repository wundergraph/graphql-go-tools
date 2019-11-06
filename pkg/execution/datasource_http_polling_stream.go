package execution

import (
	"bytes"
	"github.com/jensneuse/graphql-go-tools/pkg/ast"
	"github.com/jensneuse/graphql-go-tools/pkg/astvisitor"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
	"go.uber.org/zap"
	"io"
	"sync"
	"time"
)

type HttpPollingStreamDataSourcePlanner struct {
	walker                *astvisitor.Walker
	operation, definition *ast.Document
	log                   *zap.Logger
	args                  []Argument
}

func NewHttpPollingStreamDataSourcePlanner(logger *zap.Logger) *HttpPollingStreamDataSourcePlanner {
	return &HttpPollingStreamDataSourcePlanner{
		log: logger,
	}
}

func (h *HttpPollingStreamDataSourcePlanner) DirectiveName() []byte {
	return []byte("HttpPollingStreamDataSource")
}

func (h *HttpPollingStreamDataSourcePlanner) Plan() (DataSource, []Argument) {
	return &HttpPollingStreamDataSource{
		log: h.log,
	}, h.args
}

func (h *HttpPollingStreamDataSourcePlanner) Initialize(walker *astvisitor.Walker, operation, definition *ast.Document, args []Argument, resolverParameters []ResolverParameter) {
	h.walker, h.operation, h.definition, h.args = walker, operation, definition, args
}

func (h *HttpPollingStreamDataSourcePlanner) EnterInlineFragment(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) LeaveInlineFragment(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) EnterSelectionSet(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) LeaveSelectionSet(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) EnterField(ref int) {

}

func (h *HttpPollingStreamDataSourcePlanner) LeaveField(ref int) {
	definition, exists := h.walker.FieldDefinition(ref)
	if !exists {
		return
	}
	directive, exists := h.definition.FieldDefinitionDirectiveByName(definition, h.DirectiveName())
	if !exists {
		return
	}
	value, exists := h.definition.DirectiveArgumentValueByName(directive, literal.URL)
	if !exists {
		return
	}
	variableValue := h.definition.StringValueContentBytes(value.Ref)
	arg := &StaticVariableArgument{
		Name:  literal.URL,
		Value: variableValue,
	}
	h.args = append([]Argument{arg}, h.args...)
	value, exists = h.definition.DirectiveArgumentValueByName(directive, literal.HOST)
	if !exists {
		return
	}
	variableValue = h.definition.StringValueContentBytes(value.Ref)
	arg = &StaticVariableArgument{
		Name:  literal.HOST,
		Value: variableValue,
	}
	h.args = append([]Argument{arg}, h.args...)

	var staticValue []byte
	value, ok := h.definition.DirectiveArgumentValueByName(directive, []byte("data"))
	if !ok || value.Kind != ast.ValueKindString {
		staticValue = literal.NULL
	} else {
		staticValue = h.definition.StringValueContentBytes(value.Ref)
	}
	staticValue = bytes.ReplaceAll(staticValue, literal.BACKSLASH, nil)
	h.args = append(h.args, &StaticVariableArgument{
		Value: staticValue,
	})
}

type HttpPollingStreamDataSource struct {
	log    *zap.Logger
	once   sync.Once
	ch     chan []byte
	closed bool
	delay  time.Duration
}

func (h *HttpPollingStreamDataSource) Resolve(ctx Context, args ResolvedArgs, out io.Writer) Instruction {
	data := args.ByKey([]byte("data"))
	h.once.Do(func() {
		h.ch = make(chan []byte, 1)
		go h.startStream(ctx, data)
	})
	if h.closed {
		return CloseConnection
	}
	select {
	case data := <-h.ch:
		_, err := out.Write(data)
		if err != nil {
			h.log.Error("HttpPollingStreamDataSource.Resolve",
				zap.Error(err),
			)
		}
	case <-ctx.Done():
		h.closed = true
		return CloseConnection
	}
	return KeepStreamAlive
}

func (h *HttpPollingStreamDataSource) startStream(ctx Context, data []byte) {
	for {
		time.Sleep(h.delay)
		select {
		case <-ctx.Done():
			return
		case h.ch <- data:
			continue
		}
	}
}
