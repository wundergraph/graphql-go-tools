package plan

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/ast"
	"github.com/wundergraph/graphql-go-tools/v2/pkg/engine/resolve"
)

func TestGetSchemaUsageInfo(t *testing.T) {
	source := resolve.TypeFieldSource{
		IDs: []string{"https://swapi.dev/api"},
	}
	res := &resolve.GraphQLResponse{
		Info: &resolve.GraphQLResponseInfo{
			OperationType: ast.OperationTypeQuery,
		},
		Data: &resolve.Object{
			Nullable: false,
			Fields: []*resolve.Field{
				{
					Name: []byte("searchResults"),
					Info: &resolve.FieldInfo{
						Name:            "searchResults",
						ParentTypeNames: []string{"Query"},
						Source:          source,
					},
					Value: &resolve.Array{
						Path:                []string{"searchResults"},
						Nullable:            true,
						ResolveAsynchronous: false,
						Item: &resolve.Object{
							Nullable: true,
							Fields: []*resolve.Field{
								{
									Name: []byte("__typename"),
									Value: &resolve.String{
										Path:     []string{"__typename"},
										Nullable: false,
									},
									Info: &resolve.FieldInfo{
										Name:            "__typename",
										ParentTypeNames: []string{"Human", "Droid"},
										Source:          source,
									},
								},
								{
									Name: []byte("name"),
									Value: &resolve.String{
										Path:     []string{"name"},
										Nullable: false,
									},
									OnTypeNames: [][]byte{[]byte("Human"), []byte("Droid")},
									Info: &resolve.FieldInfo{
										Name:            "name",
										ParentTypeNames: []string{"Human", "Droid"},
										Source:          source,
									},
								},
								{
									Name: []byte("length"),
									Value: &resolve.Float{
										Path:     []string{"length"},
										Nullable: false,
									},
									OnTypeNames: [][]byte{[]byte("Starship")},
									Info: &resolve.FieldInfo{
										Name:            "length",
										ParentTypeNames: []string{"Starship"},
										Source:          source,
									},
								},
								{
									Name: []byte("user"),
									Info: &resolve.FieldInfo{
										Name:            "user",
										ParentTypeNames: []string{"searchResults"},
										Source:          source,
									},
									Value: &resolve.Object{
										Path:     []string{"user"},
										Nullable: true,
										Fields: []*resolve.Field{
											{
												Name: []byte("account"),
												Info: &resolve.FieldInfo{
													Name:            "account",
													ParentTypeNames: []string{"user"},
													Source:          source,
												},
												Value: &resolve.Object{
													Path:     []string{"account"},
													Nullable: true,
													Fields: []*resolve.Field{
														{
															Name: []byte("name"),
															Info: &resolve.FieldInfo{
																Name:            "name",
																ParentTypeNames: []string{"account"},
																Source:          source,
															},
															Value: &resolve.String{
																Path: []string{"name"},
															},
														},
														{
															Name: []byte("shippingInfo"),
															Info: &resolve.FieldInfo{
																Name:            "shippingInfo",
																ParentTypeNames: []string{"account"},
																Source:          source,
															},
															Value: &resolve.Object{
																Path:     []string{"shippingInfo"},
																Nullable: true,
																Fields: []*resolve.Field{
																	{
																		Name: []byte("zip"),
																		Info: &resolve.FieldInfo{
																			Name:            "zip",
																			ParentTypeNames: []string{"shippingInfo"},
																			Source:          source,
																		},
																		Value: &resolve.String{
																			Path: []string{"zip"},
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
				},
			},
		},
	}
	syncUsage := GetSchemaUsageInfo(&SynchronousResponsePlan{
		Response: res,
	})
	subscriptionUsage := GetSchemaUsageInfo(&SubscriptionResponsePlan{
		Response: &resolve.GraphQLSubscription{
			Response: res,
		},
	})
	expected := SchemaUsageInfo{
		OperationType: ast.OperationTypeQuery,
		TypeFields: []TypeFieldUsageInfo{
			{
				FieldName: "searchResults",
				TypeNames: []string{"Query"},
				Path:      []string{"searchResults"},
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "__typename"},
				TypeNames: []string{"Human", "Droid"},
				FieldName: "__typename",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "name"},
				TypeNames: []string{"Human", "Droid"},
				FieldName: "name",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "length"},
				TypeNames: []string{"Starship"},
				FieldName: "length",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user"},
				TypeNames: []string{"searchResults"},
				FieldName: "user",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account"},
				TypeNames: []string{"user"},
				FieldName: "account",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account", "name"},
				TypeNames: []string{"account"},
				FieldName: "name",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account", "shippingInfo"},
				TypeNames: []string{"account"},
				FieldName: "shippingInfo",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
			{
				Path:      []string{"searchResults", "user", "account", "shippingInfo", "zip"},
				TypeNames: []string{"shippingInfo"},
				FieldName: "zip",
				Source: TypeFieldSource{
					IDs: []string{"https://swapi.dev/api"},
				},
			},
		},
	}
	assert.Equal(t, expected, syncUsage)
	assert.Equal(t, expected, subscriptionUsage)
}
