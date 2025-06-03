package resolve

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

type testFieldValueRenderer struct {
	render func(ctx *Context, value FieldValue, out io.Writer) error
}

func (t *testFieldValueRenderer) RenderFieldValue(ctx *Context, value FieldValue, out io.Writer) error {
	return t.render(ctx, value, out)
}

func createTestFieldValueRenderer(renderFunc func(ctx *Context, value FieldValue, out io.Writer) error) *testFieldValueRenderer {
	return &testFieldValueRenderer{
		render: renderFunc,
	}
}

func TestResolvable_CustomFieldRenderer(t *testing.T) {
	t.Parallel()
	t.Run("string nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"Hello World!"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &String{
						Path:     []string{"value"},
						Nullable: true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "String",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"Hello World!"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "String", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"Hello World!"`, string(value.Data))
			_, err := out.Write([]byte(`"xxx"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"xxx"}}`, out.String())
	})
	t.Run("string nun nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"Hello World!"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &String{
						Path:     []string{"value"},
						Nullable: false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "String",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"Hello World!"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "String", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"Hello World!"`, string(value.Data))
			_, err := out.Write([]byte(`"xxx"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"xxx"}}`, out.String())
	})
	t.Run("string array", func(t *testing.T) {
		t.Parallel()
		input := `{"value":["Hello World!"]}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Array{
						Path: []string{"value"},
						Item: &String{},
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "String",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":["Hello World!"]}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "String", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, true, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query.value", value.Path)
			assert.Equal(t, `"Hello World!"`, string(value.Data))
			_, err := out.Write([]byte(`"xxx"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":["xxx"]}}`, out.String())
	})
}
