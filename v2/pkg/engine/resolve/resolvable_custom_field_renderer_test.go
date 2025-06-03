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

// Test implementation of CustomResolve for testing CustomNode
type testCustomResolve struct {
	resolveFunc func(ctx *Context, value []byte) ([]byte, error)
}

func (t *testCustomResolve) Resolve(ctx *Context, value []byte) ([]byte, error) {
	return t.resolveFunc(ctx, value)
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
	t.Run("string non nullable", func(t *testing.T) {
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
	t.Run("string list", func(t *testing.T) {
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

	t.Run("static string", func(t *testing.T) {
		t.Parallel()
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, nil, ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &StaticString{
						Value: "Static Hello",
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
		assert.Equal(t, `{"data":{"value":"Static Hello"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "String", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable) // StaticString is never nullable
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "Static Hello", string(value.Data)) // Raw value without quotes
			_, err := out.Write([]byte("Custom Static"))        // Don't add quotes here, walkStaticString handles them
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"Custom Static"}}`, out.String())
	})

	t.Run("boolean nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":true}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Boolean{
						Path:     []string{"value"},
						Nullable: true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Boolean",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":true}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Boolean", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "true", string(value.Data))
			_, err := out.Write([]byte("false"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":false}}`, out.String())
	})

	t.Run("boolean non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":false}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Boolean{
						Path:     []string{"value"},
						Nullable: false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Boolean",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":false}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Boolean", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "false", string(value.Data))
			_, err := out.Write([]byte("true"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":true}}`, out.String())
	})

	t.Run("integer nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":42}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Integer{
						Path:     []string{"value"},
						Nullable: true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Int",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":42}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Int", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "42", string(value.Data))
			_, err := out.Write([]byte("999"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":999}}`, out.String())
	})

	t.Run("integer non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":123}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Integer{
						Path:     []string{"value"},
						Nullable: false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Int",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":123}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Int", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "123", string(value.Data))
			_, err := out.Write([]byte("456"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":456}}`, out.String())
	})

	t.Run("float nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":3.14}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Float{
						Path:     []string{"value"},
						Nullable: true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Float",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":3.14}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Float", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "3.14", string(value.Data))
			_, err := out.Write([]byte("2.71"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":2.71}}`, out.String())
	})

	t.Run("float non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":9.99}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Float{
						Path:     []string{"value"},
						Nullable: false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Float",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":9.99}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Float", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "9.99", string(value.Data))
			_, err := out.Write([]byte("1.23"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":1.23}}`, out.String())
	})

	t.Run("bigint nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"123456789012345"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &BigInt{
						Path:     []string{"value"},
						Nullable: true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "BigInt",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"123456789012345"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "BigInt", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"123456789012345"`, string(value.Data))
			_, err := out.Write([]byte(`"999999999999999"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"999999999999999"}}`, out.String())
	})

	t.Run("bigint non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"987654321098765"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &BigInt{
						Path:     []string{"value"},
						Nullable: false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "BigInt",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"987654321098765"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "BigInt", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"987654321098765"`, string(value.Data))
			_, err := out.Write([]byte(`"111111111111111"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"111111111111111"}}`, out.String())
	})

	t.Run("scalar nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"2023-01-01T00:00:00Z"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Scalar{
						Path:     []string{"value"},
						Nullable: true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "DateTime",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"2023-01-01T00:00:00Z"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "DateTime", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"2023-01-01T00:00:00Z"`, string(value.Data))
			_, err := out.Write([]byte(`"2024-01-01T00:00:00Z"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"2024-01-01T00:00:00Z"}}`, out.String())
	})

	t.Run("scalar non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"UUID-123-456"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Scalar{
						Path:     []string{"value"},
						Nullable: false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "UUID",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"UUID-123-456"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "UUID", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"UUID-123-456"`, string(value.Data))
			_, err := out.Write([]byte(`"UUID-789-012"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"UUID-789-012"}}`, out.String())
	})

	t.Run("custom node nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":{"name":"test"}}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)

		customResolve := &testCustomResolve{
			resolveFunc: func(ctx *Context, value []byte) ([]byte, error) {
				return []byte(`"resolved_custom"`), nil
			},
		}

		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &CustomNode{
						CustomResolve: customResolve,
						Path:          []string{"value"},
						Nullable:      true,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Custom",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"resolved_custom"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Custom", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"resolved_custom"`, string(value.Data))
			_, err := out.Write([]byte(`"renderer_custom"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"renderer_custom"}}`, out.String())
	})

	t.Run("custom node non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":123}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)

		customResolve := &testCustomResolve{
			resolveFunc: func(ctx *Context, value []byte) ([]byte, error) {
				return []byte("246"), nil // double the input number
			},
		}

		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &CustomNode{
						CustomResolve: customResolve,
						Path:          []string{"value"},
						Nullable:      false,
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Custom",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":246}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Custom", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, "246", string(value.Data))
			_, err := out.Write([]byte("999"))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":999}}`, out.String())
	})

	t.Run("enum nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"ACTIVE"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Enum{
						Path:     []string{"value"},
						Nullable: true,
						TypeName: "Status",
						Values:   []string{"ACTIVE", "INACTIVE", "PENDING"},
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Status",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"ACTIVE"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Status", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, true, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"ACTIVE"`, string(value.Data))
			_, err := out.Write([]byte(`"PENDING"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"PENDING"}}`, out.String())
	})

	t.Run("enum non nullable", func(t *testing.T) {
		t.Parallel()
		input := `{"value":"RED"}`
		res := NewResolvable(ResolvableOptions{})
		ctx := &Context{}
		err := res.Init(ctx, []byte(input), ast.OperationTypeQuery)
		assert.NoError(t, err)
		assert.NotNil(t, res)
		object := &Object{
			Fields: []*Field{
				{
					Name: []byte("value"),
					Value: &Enum{
						Path:     []string{"value"},
						Nullable: false,
						TypeName: "Color",
						Values:   []string{"RED", "GREEN", "BLUE"},
					},
					Info: &FieldInfo{
						Name:                "value",
						ExactParentTypeName: "Query",
						NamedType:           "Color",
					},
				},
			},
		}

		out := &bytes.Buffer{}
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"RED"}}`, out.String())

		renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
			assert.Equal(t, "value", value.Name)
			assert.Equal(t, "Color", value.Type)
			assert.Equal(t, "Query", value.ParentType)
			assert.Equal(t, false, value.IsList)
			assert.Equal(t, false, value.IsNullable)
			assert.Equal(t, "Query", value.Path)
			assert.Equal(t, `"RED"`, string(value.Data))
			_, err := out.Write([]byte(`"BLUE"`))
			return err
		})
		ctx.SetFieldValueRenderer(renderer)

		out.Reset()
		err = res.Resolve(context.Background(), object, nil, out)
		assert.NoError(t, err)
		assert.Equal(t, `{"data":{"value":"BLUE"}}`, out.String())
	})
}
