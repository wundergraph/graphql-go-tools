package resolve

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

// TestParseErrorBehavior verifies case-insensitive parsing of error behavior
// strings, including whitespace trimming and unknown value rejection.
func TestParseErrorBehavior(t *testing.T) {
	tests := []struct {
		input    string
		expected ErrorBehavior
		ok       bool
	}{
		{"PROPAGATE", ErrorBehaviorPropagate, true},
		{"propagate", ErrorBehaviorPropagate, true},
		{"Propagate", ErrorBehaviorPropagate, true},
		{"  PROPAGATE  ", ErrorBehaviorPropagate, true},
		{"NULL", ErrorBehaviorNull, true},
		{"null", ErrorBehaviorNull, true},
		{"Null", ErrorBehaviorNull, true},
		{"HALT", ErrorBehaviorHalt, true},
		{"halt", ErrorBehaviorHalt, true},
		{"Halt", ErrorBehaviorHalt, true},
		{"", ErrorBehaviorPropagate, false},
		{"INVALID", ErrorBehaviorPropagate, false},
		{"nullify", ErrorBehaviorPropagate, false},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			result, ok := ParseErrorBehavior(tc.input)
			assert.Equal(t, tc.expected, result)
			assert.Equal(t, tc.ok, ok)
		})
	}
}

// TestErrorBehaviorString verifies String() output for all error behavior
// values, including the default for unknown values.
func TestErrorBehaviorString(t *testing.T) {
	assert.Equal(t, "PROPAGATE", ErrorBehaviorPropagate.String())
	assert.Equal(t, "NULL", ErrorBehaviorNull.String())
	assert.Equal(t, "HALT", ErrorBehaviorHalt.String())
	assert.Equal(t, "PROPAGATE", ErrorBehavior(99).String()) // unknown defaults to PROPAGATE
}

// TestErrorBehaviorPropagate verifies PROPAGATE mode (default): a null
// non-nullable field bubbles up to the nearest nullable parent.
func TestErrorBehaviorPropagate(t *testing.T) {
	data := `{"user":{"name":null}}`
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.ErrorBehavior = ErrorBehaviorPropagate

	err := res.Init(ctx, []byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)

	// user is nullable, name is non-nullable
	// When name is null, user should become null (bubbling)
	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("user"),
				Value: &Object{
					Path:     []string{"user"},
					Nullable: true,
					TypeName: "User",
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)

	// In PROPAGATE mode, the null bubbles up to user
	expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":null}}`
	assert.Equal(t, compactJSONForAssert(t, expected), compactJSONForAssert(t, out.String()))
}

// TestErrorBehaviorNull verifies NULL mode: non-nullable fields return null
// at the error site without bubbling up to the parent.
func TestErrorBehaviorNull(t *testing.T) {
	data := `{"user":{"name":null}}`
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.ErrorBehavior = ErrorBehaviorNull

	err := res.Init(ctx, []byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)

	// user is nullable, name is non-nullable
	// In NULL mode, name returns null but user should NOT become null
	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("user"),
				Value: &Object{
					Path:     []string{"user"},
					Nullable: true,
					TypeName: "User",
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)

	// In NULL mode, the null does NOT bubble up - user has a name field with null
	expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":{"user":{"name":null}}}`
	assert.Equal(t, compactJSONForAssert(t, expected), compactJSONForAssert(t, out.String()))
}

// TestErrorBehaviorHalt verifies HALT mode: the first null non-nullable
// field makes the entire data field null.
func TestErrorBehaviorHalt(t *testing.T) {
	data := `{"user":{"name":null}}`
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.ErrorBehavior = ErrorBehaviorHalt

	err := res.Init(ctx, []byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)

	// user is nullable, name is non-nullable
	// In HALT mode, data becomes null on the first error
	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("user"),
				Value: &Object{
					Path:     []string{"user"},
					Nullable: true,
					TypeName: "User",
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: false,
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)

	// In HALT mode, data becomes null
	expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]}],"data":null}`
	assert.Equal(t, compactJSONForAssert(t, expected), compactJSONForAssert(t, out.String()))
}

// TestErrorBehaviorNullWithMultipleFields verifies NULL mode collects
// multiple errors from different non-nullable fields without propagating
// any of them to the parent object.
func TestErrorBehaviorNullWithMultipleFields(t *testing.T) {
	data := `{"user":{"name":null,"email":"test@example.com","age":null}}`
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.ErrorBehavior = ErrorBehaviorNull

	err := res.Init(ctx, []byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)

	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("user"),
				Value: &Object{
					Path:     []string{"user"},
					Nullable: true,
					TypeName: "User",
					Fields: []*Field{
						{
							Name: []byte("name"),
							Value: &String{
								Path:     []string{"name"},
								Nullable: false, // non-nullable but null -> error, no bubbling in NULL mode
							},
						},
						{
							Name: []byte("email"),
							Value: &String{
								Path:     []string{"email"},
								Nullable: true,
							},
						},
						{
							Name: []byte("age"),
							Value: &Integer{
								Path:     []string{"age"},
								Nullable: false, // non-nullable but null -> error, no bubbling in NULL mode
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)

	// In NULL mode, the user object should still exist with both errors collected
	expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.name'.","path":["user","name"]},{"message":"Cannot return null for non-nullable field 'Query.user.age'.","path":["user","age"]}],"data":{"user":{"name":null,"email":"test@example.com","age":null}}}`
	assert.Equal(t, compactJSONForAssert(t, expected), compactJSONForAssert(t, out.String()))
}

// TestErrorBehaviorWithNestedObjects verifies NULL mode with deeply nested
// objects: the null stays at the leaf and does not bubble through
// intermediate nullable parents.
func TestErrorBehaviorWithNestedObjects(t *testing.T) {
	data := `{"user":{"profile":{"address":{"city":null}}}}`
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.ErrorBehavior = ErrorBehaviorNull

	err := res.Init(ctx, []byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)

	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("user"),
				Value: &Object{
					Path:     []string{"user"},
					Nullable: true,
					TypeName: "User",
					Fields: []*Field{
						{
							Name: []byte("profile"),
							Value: &Object{
								Path:     []string{"profile"},
								Nullable: true,
								TypeName: "Profile",
								Fields: []*Field{
									{
										Name: []byte("address"),
										Value: &Object{
											Path:     []string{"address"},
											Nullable: true,
											TypeName: "Address",
											Fields: []*Field{
												{
													Name: []byte("city"),
													Value: &String{
														Path:     []string{"city"},
														Nullable: false, // non-nullable at deep level
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)

	// In NULL mode, the null doesn't bubble up through address, profile, or user
	expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.user.profile.address.city'.","path":["user","profile","address","city"]}],"data":{"user":{"profile":{"address":{"city":null}}}}}`
	assert.Equal(t, compactJSONForAssert(t, expected), compactJSONForAssert(t, out.String()))
}

// TestErrorBehaviorWithArrays verifies NULL mode with arrays: a null
// non-nullable field in one array item does not affect other items.
func TestErrorBehaviorWithArrays(t *testing.T) {
	data := `{"users":[{"name":"Alice"},{"name":null},{"name":"Charlie"}]}`
	res := NewResolvable(nil, ResolvableOptions{})
	ctx := NewContext(context.Background())
	ctx.ExecutionOptions.ErrorBehavior = ErrorBehaviorNull

	err := res.Init(ctx, []byte(data), ast.OperationTypeQuery)
	assert.NoError(t, err)

	object := &Object{
		Fields: []*Field{
			{
				Name: []byte("users"),
				Value: &Array{
					Path:     []string{"users"},
					Nullable: true,
					Item: &Object{
						Nullable: true,
						TypeName: "User",
						Fields: []*Field{
							{
								Name: []byte("name"),
								Value: &String{
									Path:     []string{"name"},
									Nullable: false, // non-nullable
								},
							},
						},
					},
				},
			},
		},
	}

	out := &bytes.Buffer{}
	err = res.Resolve(context.Background(), object, nil, out)
	assert.NoError(t, err)

	// In NULL mode, the array should still contain all items
	// The second item's name will be null (error) but the item itself should remain
	expected := `{"errors":[{"message":"Cannot return null for non-nullable field 'Query.users.name'.","path":["users",1,"name"]}],"data":{"users":[{"name":"Alice"},{"name":null},{"name":"Charlie"}]}}`
	assert.Equal(t, compactJSONForAssert(t, expected), compactJSONForAssert(t, out.String()))
}

// TestHaltExecution verifies the HaltExecution flag on Resolvable: set by
// HALT mode on first error, cleared by Reset().
func TestHaltExecution(t *testing.T) {
	res := NewResolvable(nil, ResolvableOptions{})
	assert.False(t, res.HaltExecution())

	res.haltExecution = true
	assert.True(t, res.HaltExecution())

	// Reset should clear the flag
	res.Reset()
	assert.False(t, res.HaltExecution())
}
