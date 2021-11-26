package resolve

import (
	"fmt"
	"net/textproto"

	"github.com/buger/jsonparser"
	"github.com/jensneuse/graphql-go-tools/pkg/fastbuffer"
	"github.com/jensneuse/graphql-go-tools/pkg/lexer/literal"
)

type SegmentType int

const (
	StaticSegmentType SegmentType = iota + 1
	VariableSegmentType
)

type TemplateSegment struct {
	SegmentType        SegmentType
	Data               []byte
	VariableKind       VariableKind
	VariableSourcePath []string
	Renderer           VariableRenderer
}

type InputTemplate struct {
	Segments []TemplateSegment
}

func (i *InputTemplate) Render(ctx *Context, data []byte, preparedInput *fastbuffer.FastBuffer) (err error) {
	for j := range i.Segments {
		switch i.Segments[j].SegmentType {
		case StaticSegmentType:
			preparedInput.WriteBytes(i.Segments[j].Data)
		case VariableSegmentType:
			switch i.Segments[j].VariableKind {
			case ObjectVariableKind:
				err = i.renderObjectVariable(data, i.Segments[j], preparedInput)
			case ContextVariableKind:
				err = i.renderContextVariable(ctx, i.Segments[j], preparedInput)
			case HeaderVariableKind:
				err = i.renderHeaderVariable(ctx, i.Segments[j].VariableSourcePath, preparedInput)
			default:
				err = fmt.Errorf("InputTemplate.Render: cannot resolve variable of kind: %d", i.Segments[j].VariableKind)
			}
			if err != nil {
				return err
			}
		}
	}
	return
}

func (i *InputTemplate) renderObjectVariable(variables []byte, segment TemplateSegment, preparedInput *fastbuffer.FastBuffer) error {
	value, valueType, _, err := jsonparser.Get(variables, segment.VariableSourcePath...)
	if err != nil || valueType == jsonparser.Null {
		preparedInput.WriteBytes(literal.NULL)
		return nil
	}
	return segment.Renderer.RenderVariable(value, preparedInput)
}

func (i *InputTemplate) renderContextVariable(ctx *Context, segment TemplateSegment, preparedInput *fastbuffer.FastBuffer) error {
	value, valueType, _, err := jsonparser.Get(ctx.Variables, segment.VariableSourcePath...)
	if err != nil || valueType == jsonparser.Null {
		preparedInput.WriteBytes(literal.NULL)
		return nil
	}
	return segment.Renderer.RenderVariable(value, preparedInput)
}

func renderArrayCSV(data []byte, valueType jsonparser.ValueType, buf *fastbuffer.FastBuffer) error {
	isFirst := true
	_, err := jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
		if dataType != valueType {
			return
		}
		if isFirst {
			isFirst = false
		} else {
			_, _ = buf.Write(literal.COMMA)
		}
		_, _ = buf.Write(value)
	})
	return err
}

func renderGraphQLValue(data []byte, valueType jsonparser.ValueType, omitObjectKeyQuotes, escapeQuotes bool, buf *fastbuffer.FastBuffer) (err error) {
	switch valueType {
	case jsonparser.String:
		if escapeQuotes {
			buf.WriteBytes(literal.BACKSLASH)
		}
		buf.WriteBytes(literal.QUOTE)
		buf.WriteBytes(data)
		if escapeQuotes {
			buf.WriteBytes(literal.BACKSLASH)
		}
		buf.WriteBytes(literal.QUOTE)
	case jsonparser.Object:
		buf.WriteBytes(literal.LBRACE)
		first := true
		err = jsonparser.ObjectEach(data, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
			if !first {
				buf.WriteBytes(literal.COMMA)
			} else {
				first = false
			}
			if !omitObjectKeyQuotes {
				if escapeQuotes {
					buf.WriteBytes(literal.BACKSLASH)
				}
				buf.WriteBytes(literal.QUOTE)
			}
			buf.WriteBytes(key)
			if !omitObjectKeyQuotes {
				if escapeQuotes {
					buf.WriteBytes(literal.BACKSLASH)
				}
				buf.WriteBytes(literal.QUOTE)
			}
			buf.WriteBytes(literal.COLON)
			return renderGraphQLValue(value, dataType, omitObjectKeyQuotes, escapeQuotes, buf)
		})
		if err != nil {
			return err
		}
		buf.WriteBytes(literal.RBRACE)
	case jsonparser.Null:
		buf.WriteBytes(literal.NULL)
	case jsonparser.Boolean:
		buf.WriteBytes(data)
	case jsonparser.Array:
		buf.WriteBytes(literal.LBRACK)
		first := true
		var arrayErr error
		_, err = jsonparser.ArrayEach(data, func(value []byte, dataType jsonparser.ValueType, offset int, err error) {
			if !first {
				buf.WriteBytes(literal.COMMA)
			} else {
				first = false
			}
			arrayErr = renderGraphQLValue(value, dataType, omitObjectKeyQuotes, escapeQuotes, buf)
		})
		if arrayErr != nil {
			return arrayErr
		}
		if err != nil {
			return err
		}
		buf.WriteBytes(literal.RBRACK)
	case jsonparser.Number:
		buf.WriteBytes(data)
	}
	return
}

func (i *InputTemplate) renderHeaderVariable(ctx *Context, path []string, preparedInput *fastbuffer.FastBuffer) error {
	if len(path) != 1 {
		return errHeaderPathInvalid
	}
	// Header.Values is available from go 1.14
	// value := ctx.Request.Header.Values(path[0])
	// could be simplified once go 1.12 support will be dropped
	canonicalName := textproto.CanonicalMIMEHeaderKey(path[0])
	value := ctx.Request.Header[canonicalName]
	if len(value) == 0 {
		return nil
	}
	if len(value) == 1 {
		preparedInput.WriteString(value[0])
		return nil
	}
	for j := range value {
		if j != 0 {
			preparedInput.WriteBytes(literal.COMMA)
		}
		preparedInput.WriteString(value[j])
	}
	return nil
}
