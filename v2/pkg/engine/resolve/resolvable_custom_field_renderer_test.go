package resolve

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/astjson"

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

type fieldValueTestCase struct {
	name                 string
	input                string
	fieldValue           Node
	fieldInfo            *FieldInfo
	expectedOutput       string
	expectedFieldValue   FieldValue
	rendererOutput       string
	expectedWithRenderer string
}

func TestResolvable_CustomFieldRenderer(t *testing.T) {
	t.Parallel()

	testCases := []fieldValueTestCase{
		{
			name:  "string nullable",
			input: `{"value":"Hello World!"}`,
			fieldValue: &String{
				Path:     []string{"value"},
				Nullable: true,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "String",
			},
			expectedOutput: `{"data":{"value":"Hello World!"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "String",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				Path:       "Query",
				Data:       []byte(`"Hello World!"`),
			},
			rendererOutput:       `"xxx"`,
			expectedWithRenderer: `{"data":{"value":"xxx"}}`,
		},
		{
			name:  "string non nullable",
			input: `{"value":"Hello World!"}`,
			fieldValue: &String{
				Path:     []string{"value"},
				Nullable: false,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "String",
			},
			expectedOutput: `{"data":{"value":"Hello World!"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "String",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				Path:       "Query",
				Data:       []byte(`"Hello World!"`),
			},
			rendererOutput:       `"xxx"`,
			expectedWithRenderer: `{"data":{"value":"xxx"}}`,
		},
		{
			name:  "string list",
			input: `{"value":["Hello World!"]}`,
			fieldValue: &Array{
				Path: []string{"value"},
				Item: &String{},
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "String",
			},
			expectedOutput: `{"data":{"value":["Hello World!"]}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "String",
				ParentType: "Query",
				IsListItem: true,
				IsNullable: false,
				Path:       "Query.value",
				Data:       []byte(`"Hello World!"`),
			},
			rendererOutput:       `"xxx"`,
			expectedWithRenderer: `{"data":{"value":["xxx"]}}`,
		},
		{
			name:  "boolean nullable",
			input: `{"value":true}`,
			fieldValue: &Boolean{
				Path:     []string{"value"},
				Nullable: true,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Boolean",
			},
			expectedOutput: `{"data":{"value":true}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Boolean",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				Path:       "Query",
				Data:       []byte("true"),
			},
			rendererOutput:       "false",
			expectedWithRenderer: `{"data":{"value":false}}`,
		},
		{
			name:  "boolean non nullable",
			input: `{"value":false}`,
			fieldValue: &Boolean{
				Path:     []string{"value"},
				Nullable: false,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Boolean",
			},
			expectedOutput: `{"data":{"value":false}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Boolean",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				Path:       "Query",
				Data:       []byte("false"),
			},
			rendererOutput:       "true",
			expectedWithRenderer: `{"data":{"value":true}}`,
		},
		{
			name:  "integer nullable",
			input: `{"value":42}`,
			fieldValue: &Integer{
				Path:     []string{"value"},
				Nullable: true,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Int",
			},
			expectedOutput: `{"data":{"value":42}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Int",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				Path:       "Query",
				Data:       []byte("42"),
			},
			rendererOutput:       "999",
			expectedWithRenderer: `{"data":{"value":999}}`,
		},
		{
			name:  "integer non nullable",
			input: `{"value":123}`,
			fieldValue: &Integer{
				Path:     []string{"value"},
				Nullable: false,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Int",
			},
			expectedOutput: `{"data":{"value":123}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Int",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				Path:       "Query",
				Data:       []byte("123"),
			},
			rendererOutput:       "456",
			expectedWithRenderer: `{"data":{"value":456}}`,
		},
		{
			name:  "float nullable",
			input: `{"value":3.14}`,
			fieldValue: &Float{
				Path:     []string{"value"},
				Nullable: true,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Float",
			},
			expectedOutput: `{"data":{"value":3.14}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Float",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				Path:       "Query",
				Data:       []byte("3.14"),
			},
			rendererOutput:       "2.71",
			expectedWithRenderer: `{"data":{"value":2.71}}`,
		},
		{
			name:  "float non nullable",
			input: `{"value":9.99}`,
			fieldValue: &Float{
				Path:     []string{"value"},
				Nullable: false,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Float",
			},
			expectedOutput: `{"data":{"value":9.99}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Float",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				Path:       "Query",
				Data:       []byte("9.99"),
			},
			rendererOutput:       "1.23",
			expectedWithRenderer: `{"data":{"value":1.23}}`,
		},
		{
			name:  "bigint nullable",
			input: `{"value":"123456789012345"}`,
			fieldValue: &BigInt{
				Path:     []string{"value"},
				Nullable: true,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "BigInt",
			},
			expectedOutput: `{"data":{"value":"123456789012345"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "BigInt",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				Path:       "Query",
				Data:       []byte(`"123456789012345"`),
			},
			rendererOutput:       `"999999999999999"`,
			expectedWithRenderer: `{"data":{"value":"999999999999999"}}`,
		},
		{
			name:  "bigint non nullable",
			input: `{"value":"987654321098765"}`,
			fieldValue: &BigInt{
				Path:     []string{"value"},
				Nullable: false,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "BigInt",
			},
			expectedOutput: `{"data":{"value":"987654321098765"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "BigInt",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				Path:       "Query",
				Data:       []byte(`"987654321098765"`),
			},
			rendererOutput:       `"111111111111111"`,
			expectedWithRenderer: `{"data":{"value":"111111111111111"}}`,
		},
		{
			name:  "scalar nullable",
			input: `{"value":"2023-01-01T00:00:00Z"}`,
			fieldValue: &Scalar{
				Path:     []string{"value"},
				Nullable: true,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "DateTime",
			},
			expectedOutput: `{"data":{"value":"2023-01-01T00:00:00Z"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "DateTime",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				Path:       "Query",
				Data:       []byte(`"2023-01-01T00:00:00Z"`),
			},
			rendererOutput:       `"2024-01-01T00:00:00Z"`,
			expectedWithRenderer: `{"data":{"value":"2024-01-01T00:00:00Z"}}`,
		},
		{
			name:  "scalar non nullable",
			input: `{"value":"UUID-123-456"}`,
			fieldValue: &Scalar{
				Path:     []string{"value"},
				Nullable: false,
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "UUID",
			},
			expectedOutput: `{"data":{"value":"UUID-123-456"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "UUID",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				Path:       "Query",
				Data:       []byte(`"UUID-123-456"`),
			},
			rendererOutput:       `"UUID-789-012"`,
			expectedWithRenderer: `{"data":{"value":"UUID-789-012"}}`,
		},
		{
			name:  "enum nullable",
			input: `{"value":"ACTIVE"}`,
			fieldValue: &Enum{
				Path:     []string{"value"},
				Nullable: true,
				TypeName: "Status",
				Values:   []string{"ACTIVE", "INACTIVE", "PENDING"},
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Status",
			},
			expectedOutput: `{"data":{"value":"ACTIVE"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Status",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: true,
				IsEnum:     true,
				Path:       "Query",
				Data:       []byte(`"ACTIVE"`),
			},
			rendererOutput:       `"PENDING"`,
			expectedWithRenderer: `{"data":{"value":"PENDING"}}`,
		},
		{
			name:  "enum non nullable",
			input: `{"value":"RED"}`,
			fieldValue: &Enum{
				Path:     []string{"value"},
				Nullable: false,
				TypeName: "Color",
				Values:   []string{"RED", "GREEN", "BLUE"},
			},
			fieldInfo: &FieldInfo{
				Name:                "value",
				ExactParentTypeName: "Query",
				NamedType:           "Color",
			},
			expectedOutput: `{"data":{"value":"RED"}}`,
			expectedFieldValue: FieldValue{
				Name:       "value",
				Type:       "Color",
				ParentType: "Query",
				IsListItem: false,
				IsNullable: false,
				IsEnum:     true,
				Path:       "Query",
				Data:       []byte(`"RED"`),
			},
			rendererOutput:       `"BLUE"`,
			expectedWithRenderer: `{"data":{"value":"BLUE"}}`,
		},
	}

	for _, tc := range testCases {
		tc.expectedFieldValue.ParsedData = astjson.MustParseBytes(tc.expectedFieldValue.Data)
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Setup
			res := NewResolvable(ResolvableOptions{})
			ctx := &Context{}

			var input []byte
			if tc.input != "" {
				input = []byte(tc.input)
			}

			err := res.Init(ctx, input, ast.OperationTypeQuery)
			assert.NoError(t, err)
			assert.NotNil(t, res)

			object := &Object{
				Fields: []*Field{
					{
						Name:  []byte("value"),
						Value: tc.fieldValue,
						Info:  tc.fieldInfo,
					},
				},
			}

			// Test without renderer
			out := &bytes.Buffer{}
			err = res.Resolve(context.Background(), object, nil, out)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedOutput, out.String())

			// Test with renderer
			renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
				assert.Equal(t, tc.expectedFieldValue, value)
				_, err := out.Write([]byte(tc.rendererOutput))
				return err
			})
			ctx.SetFieldValueRenderer(renderer)

			out.Reset()
			err = res.Resolve(context.Background(), object, nil, out)
			assert.NoError(t, err)
			assert.Equal(t, tc.expectedWithRenderer, out.String())
		})
	}

	// Custom node tests require special handling due to CustomResolve
	t.Run("custom node tests", func(t *testing.T) {
		customNodeTestCases := []struct {
			name                 string
			input                string
			nullable             bool
			customResolveFunc    func(ctx *Context, value []byte) ([]byte, error)
			expectedOutput       string
			expectedFieldValue   FieldValue
			rendererOutput       string
			expectedWithRenderer string
		}{
			{
				name:     "custom node nullable",
				input:    `{"value":{"name":"test"}}`,
				nullable: true,
				customResolveFunc: func(ctx *Context, value []byte) ([]byte, error) {
					return []byte(`"resolved_custom"`), nil
				},
				expectedOutput: `{"data":{"value":"resolved_custom"}}`,
				expectedFieldValue: FieldValue{
					Name:       "value",
					Type:       "Custom",
					ParentType: "Query",
					IsListItem: false,
					IsNullable: true,
					Path:       "Query",
					Data:       []byte(`"resolved_custom"`),
				},
				rendererOutput:       `"renderer_custom"`,
				expectedWithRenderer: `{"data":{"value":"renderer_custom"}}`,
			},
			{
				name:     "custom node non nullable",
				input:    `{"value":123}`,
				nullable: false,
				customResolveFunc: func(ctx *Context, value []byte) ([]byte, error) {
					return []byte("246"), nil // double the input number
				},
				expectedOutput: `{"data":{"value":246}}`,
				expectedFieldValue: FieldValue{
					Name:       "value",
					Type:       "Custom",
					ParentType: "Query",
					IsListItem: false,
					IsNullable: false,
					Path:       "Query",
					Data:       []byte("246"),
				},
				rendererOutput:       "999",
				expectedWithRenderer: `{"data":{"value":999}}`,
			},
		}

		for _, tc := range customNodeTestCases {
			tc.expectedFieldValue.ParsedData = astjson.MustParseBytes(tc.expectedFieldValue.Data)
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				input := []byte(tc.input)
				res := NewResolvable(ResolvableOptions{})
				ctx := &Context{}
				err := res.Init(ctx, input, ast.OperationTypeQuery)
				assert.NoError(t, err)
				assert.NotNil(t, res)

				customResolve := &testCustomResolve{
					resolveFunc: tc.customResolveFunc,
				}

				object := &Object{
					Fields: []*Field{
						{
							Name: []byte("value"),
							Value: &CustomNode{
								CustomResolve: customResolve,
								Path:          []string{"value"},
								Nullable:      tc.nullable,
							},
							Info: &FieldInfo{
								Name:                "value",
								ExactParentTypeName: "Query",
								NamedType:           "Custom",
							},
						},
					},
				}

				// Test without renderer
				out := &bytes.Buffer{}
				err = res.Resolve(context.Background(), object, nil, out)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedOutput, out.String())

				// Test with renderer
				renderer := createTestFieldValueRenderer(func(ctx *Context, value FieldValue, out io.Writer) error {
					assert.Equal(t, tc.expectedFieldValue, value)
					_, err := out.Write([]byte(tc.rendererOutput))
					return err
				})
				ctx.SetFieldValueRenderer(renderer)

				out.Reset()
				err = res.Resolve(context.Background(), object, nil, out)
				assert.NoError(t, err)
				assert.Equal(t, tc.expectedWithRenderer, out.String())
			})
		}
	})
}
