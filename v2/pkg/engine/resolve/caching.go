package resolve

import (
	"bytes"

	"github.com/wundergraph/astjson"
)

type CacheKeyTemplate interface {
	RenderCacheKey(ctx *Context, data *astjson.Value, out *bytes.Buffer) error
}

type RootQueryCacheKeyTemplate struct {
	Fields []CacheKeyQueryRootField
}

type CacheKeyQueryRootField struct {
	Name string
	Args []CacheKeyQueryRootFieldArgument
}

type CacheKeyQueryRootFieldArgument struct {
	Name      string
	Variables InputTemplate
}

func (r *RootQueryCacheKeyTemplate) RenderCacheKey(ctx *Context, data *astjson.Value, out *bytes.Buffer) error {
	_, err := out.WriteString("Query")
	if err != nil {
		return err
	}

	// Process each field
	for _, field := range r.Fields {
		_, err = out.WriteString("::")
		if err != nil {
			return err
		}

		// Add field name
		_, err = out.WriteString(field.Name)
		if err != nil {
			return err
		}

		// Process each argument
		for _, arg := range field.Args {
			// Add argument separator ":"
			_, err = out.WriteString(":")
			if err != nil {
				return err
			}

			// Add argument name
			_, err = out.WriteString(arg.Name)
			if err != nil {
				return err
			}

			// Add argument separator ":"
			_, err = out.WriteString(":")
			if err != nil {
				return err
			}

			err = arg.Variables.Render(ctx, data, out)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

type EntityQueryCacheKeyTemplate struct {
	Keys *ResolvableObjectVariable
}

func (e *EntityQueryCacheKeyTemplate) RenderCacheKey(ctx *Context, data *astjson.Value, out *bytes.Buffer) error {
	return e.Keys.Renderer.RenderVariable(ctx.ctx, data, out)
}
