package resolve

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/wundergraph/astjson"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/datasource/httpclient"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/lexer/literal"
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
	Segments           []TemplateSegment
}

type InputTemplate struct {
	Segments []TemplateSegment
	// SetTemplateOutputToNullOnVariableNull will safely return "null" if one of the template variables renders to null
	// This is the case, e.g. when using batching and one sibling is null, resulting in a null value for one batch item
	// Returning null in this case tells the batch implementation to skip this item
	SetTemplateOutputToNullOnVariableNull bool
}

func SetInputUndefinedVariables(preparedInput InputTemplateWriter, undefinedVariables []string) error {
	if len(undefinedVariables) > 0 {
		output, err := httpclient.SetUndefinedVariables(preparedInput.Bytes(), undefinedVariables)
		if err != nil {
			return err
		}

		preparedInput.Reset()
		_, _ = preparedInput.Write(output)
	}

	return nil
}

// errSetTemplateOutputNull is a private sentinel used for control flow to signal
// that the template output should be set to JSON null. It must not be surfaced
// to callers; renderSegments intercepts it and writes literal.NULL instead.
var errSetTemplateOutputNull = errors.New("set to null")

type InputTemplateWriter interface {
	io.Writer
	io.StringWriter
	Reset()
	Bytes() []byte
}

func (i *InputTemplate) Render(ctx *Context, data *astjson.Value, preparedInput InputTemplateWriter) error {
	var undefinedVariables []string

	if err := i.renderSegments(ctx, data, i.Segments, preparedInput, &undefinedVariables); err != nil {
		return err
	}

	return SetInputUndefinedVariables(preparedInput, undefinedVariables)
}

func (i *InputTemplate) RenderAndCollectUndefinedVariables(ctx *Context, data *astjson.Value, preparedInput InputTemplateWriter, undefinedVariables *[]string) (err error) {
	err = i.renderSegments(ctx, data, i.Segments, preparedInput, undefinedVariables)
	return
}

func (i *InputTemplate) renderSegments(ctx *Context, data *astjson.Value, segments []TemplateSegment, preparedInput InputTemplateWriter, undefinedVariables *[]string) (err error) {
	for _, segment := range segments {
		switch segment.SegmentType {
		case StaticSegmentType:
			_, _ = preparedInput.Write(segment.Data)
		case VariableSegmentType:
			switch segment.VariableKind {
			case ObjectVariableKind:
				err = i.renderObjectVariable(ctx.Context(), data, segment, preparedInput)
			case ContextVariableKind:
				var undefined bool
				undefined, err = i.renderContextVariable(ctx, segment, preparedInput)
				if undefined {
					*undefinedVariables = append(*undefinedVariables, segment.VariableSourcePath[0])
				}
			case ResolvableObjectVariableKind:
				err = i.renderResolvableObjectVariable(ctx.Context(), data, segment, preparedInput)
			case HeaderVariableKind:
				err = i.renderHeaderVariable(ctx, segment.VariableSourcePath, preparedInput)
			default:
				err = fmt.Errorf("InputTemplate.Render: cannot resolve variable of kind: %d", segment.VariableKind)
			}

			if err != nil {
				if errors.Is(err, errSetTemplateOutputNull) {
					preparedInput.Reset()
					_, _ = preparedInput.Write(literal.NULL)
					return nil
				}
				return err
			}
		}
	}

	return err
}

func (i *InputTemplate) renderObjectVariable(ctx context.Context, variables *astjson.Value, segment TemplateSegment, preparedInput InputTemplateWriter) error {
	value := variables.Get(segment.VariableSourcePath...)
	if value == nil || value.Type() == astjson.TypeNull {
		if i.SetTemplateOutputToNullOnVariableNull {
			return errSetTemplateOutputNull
		}
		_, _ = preparedInput.Write(literal.NULL)
		return nil
	}
	return segment.Renderer.RenderVariable(ctx, value, preparedInput)
}

func (i *InputTemplate) renderResolvableObjectVariable(ctx context.Context, objectData *astjson.Value, segment TemplateSegment, preparedInput InputTemplateWriter) error {
	return segment.Renderer.RenderVariable(ctx, objectData, preparedInput)
}

func (i *InputTemplate) renderContextVariable(ctx *Context, segment TemplateSegment, preparedInput InputTemplateWriter) (variableWasUndefined bool, err error) {
	variableSourcePath := segment.VariableSourcePath
	if len(variableSourcePath) == 1 && ctx.RemapVariables != nil {
		nameToUse, hasMapping := ctx.RemapVariables[variableSourcePath[0]]
		if hasMapping && nameToUse != variableSourcePath[0] {
			variableSourcePath = []string{nameToUse}
		}
	}

	value := ctx.Variables.Get(variableSourcePath...)
	if value == nil {
		_, _ = preparedInput.Write(literal.NULL)
		return true, nil
	} else if value.Type() == astjson.TypeNull {
		return false, segment.Renderer.RenderVariable(ctx.Context(), value, preparedInput)
	}
	return false, segment.Renderer.RenderVariable(ctx.Context(), value, preparedInput)
}

func (i *InputTemplate) renderHeaderVariable(ctx *Context, path []string, preparedInput InputTemplateWriter) error {
	if len(path) != 1 {
		return errHeaderPathInvalid
	}
	value := ctx.Request.Header.Values(path[0])
	if len(value) == 0 {
		return nil
	}
	if len(value) == 1 {
		if _, err := preparedInput.WriteString(value[0]); err != nil {
			return err
		}
		return nil
	}
	for j := range value {
		if j != 0 {
			if _, err := preparedInput.Write(literal.COMMA); err != nil {
				return err
			}
		}
		if _, err := preparedInput.WriteString(value[j]); err != nil {
			return err
		}
	}
	return nil
}
