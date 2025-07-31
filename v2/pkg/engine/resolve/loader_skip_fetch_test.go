package resolve

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/astjson"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
)

func TestLoader_canSkipFetch(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		info          *FetchInfo
		items         []*astjson.Value
		wantResult    bool
		wantRemaining int                                            // -1 means check for empty, otherwise check exact count
		checkFn       func(t *testing.T, remaining []*astjson.Value) // optional custom validation
	}{
		{
			name: "single item with Query operation",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Scalar{
								Path:     []string{"id"},
								Nullable: false,
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"id": "123"}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "single item with Mutation operation",
			info: &FetchInfo{
				OperationType: ast.OperationTypeMutation,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Scalar{
								Path:     []string{"id"},
								Nullable: false,
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"id": "123"}`)),
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "single item with null type",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData:  &Object{Fields: []*Field{}},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`null`)),
			},
			wantResult:    true,
			wantRemaining: 1, // null item remains
		},
		{
			name: "single item with all required data",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("name"),
										Value: &Scalar{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123", "name": "John"}}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "single item missing required field",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("name"),
										Value: &Scalar{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123"}}`)), // missing "name"
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "single item missing nullable field",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("email"),
										Value: &Scalar{
											Path:     []string{"email"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123"}}`)), // missing nullable "email"
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "single item with null value on required path",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": null}}`)), // null value on required field
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "single item with null value on nullable path",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("email"),
										Value: &Scalar{
											Path:     []string{"email"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123", "email": null}}`)), // null value on nullable field
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "multiple items all can be skipped",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("id"),
							Value: &Scalar{
								Path:     []string{"id"},
								Nullable: false,
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"id": "123"}`)),
				astjson.MustParseBytes([]byte(`{"id": "456"}`)),
				astjson.MustParseBytes([]byte(`{"id": "789"}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "multiple items some can be skipped",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("name"),
										Value: &Scalar{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123", "name": "John"}}`)),  // complete
				astjson.MustParseBytes([]byte(`{"user": {"id": "456"}}`)),                  // missing name
				astjson.MustParseBytes([]byte(`{"user": {"id": "789", "name": "Alice"}}`)), // complete
			},
			wantResult:    false,
			wantRemaining: 1,
			checkFn: func(t *testing.T, remaining []*astjson.Value) {
				// Check that the remaining item is the incomplete one
				user := remaining[0].Get("user")
				assert.Equal(t, "456", string(user.Get("id").GetStringBytes()))
			},
		},
		{
			name: "multiple items none can be skipped",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("name"),
										Value: &Scalar{
											Path:     []string{"name"},
											Nullable: false,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123"}}`)), // missing name
				astjson.MustParseBytes([]byte(`{"user": {"id": "456"}}`)), // missing name
				astjson.MustParseBytes([]byte(`{"user": {"id": "789"}}`)), // missing name
			},
			wantResult:    false,
			wantRemaining: 3,
		},
		{
			name: "nullable array that is null",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("tags"),
										Value: &Array{
											Path:     []string{"tags"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123", "tags": null}}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "nullable array that is empty",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: false,
								Fields: []*Field{
									{
										Name: []byte("id"),
										Value: &Scalar{
											Path:     []string{"id"},
											Nullable: false,
										},
									},
									{
										Name: []byte("tags"),
										Value: &Array{
											Path:     []string{"tags"},
											Nullable: true,
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"user": {"id": "123", "tags": []}}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "deeply nested structure",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("user"),
							Value: &Object{
								Path:     []string{"user"},
								Nullable: true,
								Fields: []*Field{
									{
										Name: []byte("account"),
										Value: &Object{
											Path:     []string{"account"},
											Nullable: true,
											Fields: []*Field{
												{
													Name: []byte("__typename"),
													Value: &Scalar{
														Path:     []string{"__typename"},
														Nullable: false,
													},
												},
												{
													Name: []byte("id"),
													Value: &Scalar{
														Path:     []string{"id"},
														Nullable: false,
													},
												},
												{
													Name: []byte("info"),
													Value: &Object{
														Path:     []string{"info"},
														Nullable: true,
														Fields: []*Field{
															{
																Name: []byte("a"),
																Value: &Scalar{
																	Path:     []string{"a"},
																	Nullable: false,
																},
															},
															{
																Name: []byte("b"),
																Value: &Scalar{
																	Path:     []string{"b"},
																	Nullable: false,
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
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{
					"user": {
						"account": {
							"__typename": "Account",
							"id": "123",
							"info": {
								"a": "valueA",
								"b": "valueB"
							}
						}
					}
				}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "nil info",
			info: nil,
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"id": "123"}`)),
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "nil ProvidesData",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData:  nil,
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"id": "123"}`)),
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "array with scalar items - valid",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("tags"),
							Value: &Array{
								Path:     []string{"tags"},
								Nullable: false,
								Item: &Scalar{
									Path:     []string{},
									Nullable: false,
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"tags": ["tag1", "tag2", "tag3"]}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "array with scalar items - invalid (null item in non-nullable array)",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("tags"),
							Value: &Array{
								Path:     []string{"tags"},
								Nullable: false,
								Item: &Scalar{
									Path:     []string{},
									Nullable: false,
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"tags": ["tag1", null, "tag3"]}`)), // null item in non-nullable array
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "array with scalar items - valid (null item in nullable array)",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("tags"),
							Value: &Array{
								Path:     []string{"tags"},
								Nullable: false,
								Item: &Scalar{
									Path:     []string{},
									Nullable: true, // nullable scalar items
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"tags": ["tag1", null, "tag3"]}`)), // null item in nullable array
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "array with object items - valid",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Path:     []string{"users"},
								Nullable: false,
								Item: &Object{
									Path:     []string{},
									Nullable: false,
									Fields: []*Field{
										{
											Name: []byte("id"),
											Value: &Scalar{
												Path:     []string{"id"},
												Nullable: false,
											},
										},
										{
											Name: []byte("name"),
											Value: &Scalar{
												Path:     []string{"name"},
												Nullable: false,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"users": [{"id": "1", "name": "John"}, {"id": "2", "name": "Jane"}]}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "array with object items - invalid (missing required field)",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("users"),
							Value: &Array{
								Path:     []string{"users"},
								Nullable: false,
								Item: &Object{
									Path:     []string{},
									Nullable: false,
									Fields: []*Field{
										{
											Name: []byte("id"),
											Value: &Scalar{
												Path:     []string{"id"},
												Nullable: false,
											},
										},
										{
											Name: []byte("name"),
											Value: &Scalar{
												Path:     []string{"name"},
												Nullable: false,
											},
										},
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"users": [{"id": "1", "name": "John"}, {"id": "2"}]}`)), // missing "name" field
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "nested arrays - valid",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("matrix"),
							Value: &Array{
								Path:     []string{"matrix"},
								Nullable: false,
								Item: &Array{
									Path:     []string{},
									Nullable: false,
									Item: &Scalar{
										Path:     []string{},
										Nullable: false,
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"matrix": [["a", "b"], ["c", "d"], ["e", "f"]]}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "nested arrays - invalid (null in inner non-nullable array)",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("matrix"),
							Value: &Array{
								Path:     []string{"matrix"},
								Nullable: false,
								Item: &Array{
									Path:     []string{},
									Nullable: false,
									Item: &Scalar{
										Path:     []string{},
										Nullable: false,
									},
								},
							},
						},
					},
				},
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"matrix": [["a", "b"], ["c", null], ["e", "f"]]}`)), // null in inner array
			},
			wantResult:    false,
			wantRemaining: 1,
		},
		{
			name: "array of objects with nested arrays - complex valid case",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("groups"),
							Value: &Array{
								Path:     []string{"groups"},
								Nullable: false,
								Item: &Object{
									Path:     []string{},
									Nullable: false,
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &Scalar{
												Path:     []string{"name"},
												Nullable: false,
											},
										},
										{
											Name: []byte("members"),
											Value: &Array{
												Path:     []string{"members"},
												Nullable: false,
												Item: &Object{
													Path:     []string{},
													Nullable: false,
													Fields: []*Field{
														{
															Name: []byte("id"),
															Value: &Scalar{
																Path:     []string{"id"},
																Nullable: false,
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
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"groups": [{"name": "admins", "members": [{"id": "1"}, {"id": "2"}]}, {"name": "users", "members": [{"id": "3"}]}]}`)),
			},
			wantResult:    true,
			wantRemaining: -1, // empty
		},
		{
			name: "array of objects with nested arrays - complex invalid case",
			info: &FetchInfo{
				OperationType: ast.OperationTypeQuery,
				ProvidesData: &Object{
					Fields: []*Field{
						{
							Name: []byte("groups"),
							Value: &Array{
								Path:     []string{"groups"},
								Nullable: false,
								Item: &Object{
									Path:     []string{},
									Nullable: false,
									Fields: []*Field{
										{
											Name: []byte("name"),
											Value: &Scalar{
												Path:     []string{"name"},
												Nullable: false,
											},
										},
										{
											Name: []byte("members"),
											Value: &Array{
												Path:     []string{"members"},
												Nullable: false,
												Item: &Object{
													Path:     []string{},
													Nullable: false,
													Fields: []*Field{
														{
															Name: []byte("id"),
															Value: &Scalar{
																Path:     []string{"id"},
																Nullable: false,
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
			},
			items: []*astjson.Value{
				astjson.MustParseBytes([]byte(`{"groups": [{"name": "admins", "members": [{"id": "1"}, {}]}, {"name": "users", "members": [{"id": "3"}]}]}`)), // missing id in one member
			},
			wantResult:    false,
			wantRemaining: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			loader := &Loader{}

			// Make a copy of items to avoid mutation affecting the test data
			itemsCopy := make([]*astjson.Value, len(tt.items))
			copy(itemsCopy, tt.items)

			remaining, result := loader.canSkipFetch(tt.info, itemsCopy)

			assert.Equal(t, tt.wantResult, result, "result mismatch")

			if tt.wantRemaining == -1 {
				assert.Empty(t, remaining, "expected empty remaining items")
			} else {
				assert.Len(t, remaining, tt.wantRemaining, "remaining items count mismatch")
			}

			if tt.checkFn != nil {
				tt.checkFn(t, remaining)
			}
		})
	}
}
