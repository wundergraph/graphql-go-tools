package resolve

import (
	"context"
	"errors"
	"fmt"

	"github.com/buger/jsonparser"

	"github.com/wundergraph/graphql-go-tools/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/pkg/fastbuffer"
	"github.com/wundergraph/graphql-go-tools/pkg/lexer/literal"
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
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
}

var setTemplateOutputNull = errors.New("set to null")

func (i *InputTemplate) Render(ctx *Context, data []byte, preparedInput *fastbuffer.FastBuffer) error {
	var undefinedVariables []string

	for _, segment := range i.Segments {
		var err error
		switch segment.SegmentType {
		case StaticSegmentType:
			preparedInput.WriteBytes(segment.Data)
		case VariableSegmentType:
			switch segment.VariableKind {
			case ObjectVariableKind:
				err = i.renderObjectVariable(ctx.Context(), data, segment, preparedInput)
			case ContextVariableKind:
				var undefined bool
				undefined, err = i.renderContextVariable(ctx, segment, preparedInput)
				if undefined {
					undefinedVariables = append(undefinedVariables, segment.VariableSourcePath[0])
				}
			case HeaderVariableKind:
				err = i.renderHeaderVariable(ctx, segment.VariableSourcePath, preparedInput)
			default:
				err = fmt.Errorf("InputTemplate.Render: cannot resolve variable of kind: %d", segment.VariableKind)
			}
			if err != nil {
				if errors.Is(err, setTemplateOutputNull) {
					preparedInput.Reset()
					preparedInput.WriteBytes(literal.NULL)
					return nil
				}
				return err
			}
		}
	}

	if len(undefinedVariables) > 0 {
		output := httpclient.SetUndefinedVariables(preparedInput.Bytes(), undefinedVariables)
		// The returned slice might be different, we need to copy back the data
		preparedInput.Reset()
		preparedInput.WriteBytes(output)
	}
	return nil
}

func (i *InputTemplate) renderObjectVariable(ctx context.Context, variables []byte, segment TemplateSegment, preparedInput *fastbuffer.FastBuffer) error {
	value, valueType, offset, err := jsonparser.Get(variables, segment.VariableSourcePath...)
	if err != nil || valueType == jsonparser.Null {
		if i.SetTemplateOutputToNullOnVariableNull {
			return setTemplateOutputNull
		}
		preparedInput.WriteBytes(literal.NULL)
		return nil
	}
	if valueType == jsonparser.String {
		value = variables[offset-len(value)-2 : offset]
		switch segment.Renderer.GetKind() {
		case VariableRendererKindPlain, VariableRendererKindPlanWithValidation:
			if plainRenderer, ok := (segment.Renderer).(*PlainVariableRenderer); ok {
				plainRenderer.rootValueType.Value = valueType
			}
		}
	}
	return segment.Renderer.RenderVariable(ctx, value, preparedInput)
}

func (i *InputTemplate) renderContextVariable(ctx *Context, segment TemplateSegment, preparedInput *fastbuffer.FastBuffer) (variableWasUndefined bool, err error) {
	value, valueType, offset, err := jsonparser.Get(ctx.Variables, segment.VariableSourcePath...)
	if err != nil || valueType == jsonparser.Null {
		if err == jsonparser.KeyPathNotFoundError {
			preparedInput.WriteBytes(literal.NULL)
			return true, nil
		}
		return false, segment.Renderer.RenderVariable(ctx.Context(), value, preparedInput)
	}
	if valueType == jsonparser.String {
		value = ctx.Variables[offset-len(value)-2 : offset]
		switch segment.Renderer.GetKind() {
		case VariableRendererKindPlain, VariableRendererKindPlanWithValidation:
			if plainRenderer, ok := (segment.Renderer).(*PlainVariableRenderer); ok {
				plainRenderer.rootValueType.Value = valueType
			}
		}
	}
	return false, segment.Renderer.RenderVariable(ctx.Context(), value, preparedInput)
}

func (i *InputTemplate) renderHeaderVariable(ctx *Context, path []string, preparedInput *fastbuffer.FastBuffer) error {
	if len(path) != 1 {
		return errHeaderPathInvalid
	}
	value := ctx.Request.Header.Values(path[0])
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
